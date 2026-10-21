package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/spf13/cobra"
)

const overviewTimeout = 5 * time.Second
const overviewMaxBytes = 1 << 20

type overviewError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type overviewSection struct {
	Sources       []map[string]any `json:"sources"`
	State         string           `json:"state"`
	Source        string           `json:"source"`
	Scope         string           `json:"scope"`
	DetailCommand string           `json:"detail_command"`
	ObservedAt    string           `json:"observed_at"`
	Items         []map[string]any `json:"items"`
	ClosedCount   int              `json:"closed_count"`
	Error         *overviewError   `json:"error"`
}
type overviewDocument struct {
	Version int             `json:"version"`
	Nodes   overviewSection `json:"nodes"`
	Orders  overviewSection `json:"orders"`
	Peers   overviewSection `json:"peers"`
}
type overviewBuffer struct{ buffer bytes.Buffer }

func (b *overviewBuffer) Len() int      { return b.buffer.Len() }
func (b *overviewBuffer) Bytes() []byte { return b.buffer.Bytes() }

func (b *overviewBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > overviewMaxBytes {
		return 0, errors.New("observation too large")
	}
	return b.buffer.Write(p)
}
func overviewRead(ctx context.Context, backend string, args []string) ([]byte, string) {
	child := exec.CommandContext(ctx, backend, args...)
	child.WaitDelay = 100 * time.Millisecond
	var out overviewBuffer
	child.Stdout = &out
	// Transport errors can contain credentials.
	child.Stderr = io.Discard
	err := child.Run()
	if ctx.Err() != nil {
		return nil, "timeout"
	}
	if err != nil {
		var start *exec.Error
		if errors.As(err, &start) || errors.Is(err, os.ErrNotExist) {
			return nil, "backend_unavailable"
		}
		return nil, "read_failed"
	}
	return out.Bytes(), ""
}
func overviewBackend(env, fallback string) string {
	if v := os.Getenv(env); v != "" {
		return v
	}
	return fallback
}
func overviewFail(s *overviewSection, code string) {
	s.State = "unknown"
	s.Error = &overviewError{code, map[string]string{
		"timeout":             "Observation exceeded its time bound; inspect the detail command.",
		"backend_unavailable": "Backend unavailable; inspect the detail command and existing backend configuration.",
		"read_failed":         "Backend read failed; inspect the detail command and existing configuration.",
		"malformed_data":      "Backend returned unsupported or malformed JSON.",
	}[code]}
}

// Only bounded selected metadata is exposed, never briefs, messages or raw errors.
func overviewProject(row map[string]any, keys ...string) map[string]any {
	out := map[string]any{}
	for _, k := range keys {
		switch v := row[k].(type) {
		case string:
			out[k] = strings.Map(func(r rune) rune {
				if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
					return ' '
				}
				return r
			}, v)
			out[k] = overviewBound(out[k].(string), 256)
		case float64, bool:
			out[k] = v
		}
	}
	return out
}
func overviewRows(raw json.RawMessage, identity string) ([]map[string]any, error) {
	var rows []map[string]any
	if len(raw) == 0 || string(raw) == "null" || json.Unmarshal(raw, &rows) != nil || rows == nil {
		return nil, errors.New("expected array")
	}
	for _, r := range rows {
		id, ok := r[identity].(string)
		if !ok || strings.TrimSpace(id) == "" {
			return nil, errors.New("missing identity")
		}
	}
	return rows, nil
}
func observeOverview(ctx context.Context, timeout time.Duration) overviewDocument {
	doc := overviewDocument{Version: 1}
	sections := []*overviewSection{&doc.Nodes, &doc.Orders, &doc.Peers}
	sources := []string{overviewBackend("HEV_COORDINATION_BACKEND", "hev-control"), overviewBackend("HEV_COORDINATION_BACKEND", "hev-control"), overviewBackend("HEV_PEERS_BACKEND", "peers")}
	names := []string{"nodes", "orders", "peers"}
	scopes := []string{"selected node", "orders and runs: each up to 100 by ID; window only", "local peers and snapshots"}
	var runs overviewSection
	runs.Items = []map[string]any{}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		raw, code := overviewRead(ctx, sources[1], []string{"runs", "list", "--json"})
		runs.ObservedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if code != "" {
			overviewFail(&runs, code)
			return
		}
		rows, err := overviewRows(raw, "id")
		if err != nil {
			overviewFail(&runs, "malformed_data")
			return
		}
		runs.Items = rows
	}()
	for i, s := range sections {
		*s = overviewSection{State: "ok", Source: names[i] + " backend", Scope: scopes[i], DetailCommand: "hev " + names[i], Items: []map[string]any{}, Sources: []map[string]any{}}
		wg.Add(1)
		go func(i int, s *overviewSection) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			args := []string{names[i], "list", "--json"}
			if i == 2 {
				args = []string{"ls", "--json"}
			}
			raw, code := overviewRead(ctx, sources[i], args)
			s.ObservedAt = time.Now().UTC().Format(time.RFC3339Nano)
			if code != "" {
				overviewFail(s, code)
				return
			}
			if i == 2 {
				if err := overviewPeers(raw, s); err != nil {
					overviewFail(s, "malformed_data")
				}
				return
			}
			rows, err := overviewRows(raw, "id")
			if err != nil {
				overviewFail(s, "malformed_data")
				return
			}
			for _, row := range rows {
				status, _ := row["status"].(string)
				if i == 1 && (status == "done" || status == "cancelled" || status == "closed") {
					s.ClosedCount++
					continue
				}
				item := overviewProject(row, "id", "status", "owner", "next_action", "incarnation", "host", "updated_at", "lease_until")
				if i == 0 {
					if obs, ok := row["observation"].(map[string]any); ok {
						for k, v := range overviewProject(obs, "cores", "load", "mem_free_pct", "live") {
							item[k] = v
						}
					}
				}
				s.Items = append(s.Items, item)
			}
		}(i, s)
	}
	wg.Wait()
	for _, order := range doc.Orders.Items {
		active := []map[string]any{}
		for _, run := range runs.Items {
			if run["order_id"] != order["id"] {
				continue
			}
			if run["status"] != "running" && run["status"] != "requested" && run["status"] != "unknown" {
				continue
			}
			item := overviewProject(run, "id", "order_id", "status", "node_id")
			if assignment, ok := run["assignment"].(map[string]any); ok {
				if role := assignment["role"]; role == "implementation" || role == "validation" {
					item["role"] = role
				}
			}
			active = append(active, item)
		}
		order["active_runs"] = active
		if runs.Error != nil {
			order["phase"] = "unknown"
		}
	}
	doc.Orders.Sources = append(doc.Orders.Sources, map[string]any{"source": "runs", "scope": "up to 100 by ID; absence is window-only", "observed_at": runs.ObservedAt, "observed_count": len(runs.Items)})
	if runs.Error != nil {
		delete(doc.Orders.Sources[0], "observed_count")
		doc.Orders.Sources[0]["error"] = runs.Error
		if doc.Orders.Error == nil {
			doc.Orders.State = "unknown"
			doc.Orders.Error = &overviewError{runs.Error.Code, "Runs unavailable; phase unknown."}
		}
	}
	return doc
}
func overviewPeers(raw []byte, s *overviewSection) error {
	type host struct {
		Host   string          `json:"host"`
		Source string          `json:"source"`
		At     float64         `json:"at"`
		Stale  *bool           `json:"stale"`
		Error  string          `json:"error"`
		Panes  json.RawMessage `json:"panes"`
	}
	var doc struct {
		Version int             `json:"version"`
		Host    string          `json:"host"`
		At      float64         `json:"at"`
		Panes   json.RawMessage `json:"panes"`
		Hosts   []host          `json:"hosts"`
	}
	if json.Unmarshal(raw, &doc) != nil || doc.Version != 1 || doc.Host == "" || doc.At <= 0 {
		return errors.New("unsupported peers")
	}
	add := func(raw json.RawMessage, host, source string, at float64, stale bool) error {
		rows, err := overviewRows(raw, "address")
		if err != nil {
			return err
		}
		for _, row := range rows {
			item := overviewProject(row, "address", "session", "agent", "state", "role", "activity", "title", "cwd", "branch")
			if title, ok := item["title"].(string); ok {
				item["title"] = overviewBound(strings.Join(strings.Fields(title), " "), 100)
			}
			item["host"] = overviewProject(map[string]any{"host": host}, "host")["host"]
			item["source"] = overviewProject(map[string]any{"source": source}, "source")["source"]
			item["source_at"] = at
			item["stale"] = stale
			s.Items = append(s.Items, item)
		}
		summary := overviewProject(map[string]any{"host": host, "source": source}, "host", "source")
		summary["source_at"] = at
		summary["stale"] = stale
		summary["observed_peer_count"] = len(rows)
		s.Sources = append(s.Sources, summary)
		return nil
	}
	if err := add(doc.Panes, doc.Host, "local", doc.At, false); err != nil {
		return err
	}
	for _, h := range doc.Hosts {
		if h.Host == "" {
			return errors.New("missing host")
		}
		if h.Error != "" {
			summary := overviewProject(map[string]any{"host": h.Host, "source": h.Source}, "host", "source")
			summary["state"] = "unknown"
			s.Sources = append(s.Sources, summary)
			overviewFail(s, "read_failed")
			continue
		}
		if h.At <= 0 || h.Stale == nil || h.Source == "" {
			return errors.New("missing timestamp")
		}
		if err := add(h.Panes, h.Host, h.Source, h.At, *h.Stale); err != nil {
			return err
		}
	}
	return nil
}
func runOverview(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	doc := observeOverview(cmd.Context(), overviewTimeout)
	asJSON, _ := cmd.Flags().GetBool("json")
	if asJSON {
		if err := json.NewEncoder(cmd.OutOrStdout()).Encode(doc); err != nil {
			return err
		}
	} else {
		for i, s := range []overviewSection{doc.Nodes, doc.Orders, doc.Peers} {
			fmt.Fprintf(cmd.OutOrStdout(), "%s — %s — %s\n  %s; read at %s\n", []string{"Nodes", "Orders", "Peers"}[i], s.State, s.DetailCommand, s.Source+"; "+s.Scope, s.ObservedAt)
			if s.Error != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "  unknown: %s\n", s.Error.Message)
			}
			if len(s.Items) == 0 && s.State == "ok" {
				fmt.Fprintln(cmd.OutOrStdout(), "  none observed in this scope")
			}
			limit := len(s.Items)
			if limit > 8 {
				limit = 8
			}
			for _, item := range s.Items[:limit] {
				keys := [][]string{{"id", "host", "status", "owner", "cores", "load", "mem_free_pct", "live"}, {"id", "status", "owner", "phase"}, {"address", "host", "role", "state", "title", "cwd", "branch", "stale"}}[i]
				fmt.Fprint(cmd.OutOrStdout(), "  ")
				for _, k := range keys {
					v, ok := item[k]
					if n, numeric := v.(float64); i == 0 && numeric && n < 0 {
						v = "?"
					}
					if !ok {
						if i == 0 && (k == "cores" || k == "load" || k == "mem_free_pct" || k == "live") {
							v = "?"
						} else {
							continue
						}
					}
					label := k
					if k == "mem_free_pct" {
						label = "mem-free%"
					}
					if k == "live" {
						label = "reported-workers"
					}
					fmt.Fprintf(cmd.OutOrStdout(), "%s=%s  ", label, overviewText(v))
				}
				if active, ok := item["active_runs"].([]map[string]any); ok {
					for n, run := range active {
						if n == 2 {
							fmt.Fprintf(cmd.OutOrStdout(), "+%d runs; hev runs  ", len(active)-n)
							break
						}
						fmt.Fprintf(cmd.OutOrStdout(), "work=%s/%s@%s  ", overviewValue(run, "role"), overviewValue(run, "status"), overviewValue(run, "node_id"))
					}
				}
				fmt.Fprintln(cmd.OutOrStdout())
			}
			for n, source := range s.Sources {
				if n == 8 {
					break
				}
				if i == 2 {
					fmt.Fprintf(cmd.OutOrStdout(), "  source %s %s at=%v stale=%v state=%s\n", overviewValue(source, "host"), overviewValue(source, "source"), overviewValue(source, "source_at"), overviewValue(source, "stale"), overviewSourceState(source))
				}
			}

			if len(s.Items) > limit {
				fmt.Fprintf(cmd.OutOrStdout(), "  %d more; use %s\n", len(s.Items)-limit, s.DetailCommand)
			}
			if len(s.Sources) > 8 {
				fmt.Fprintf(cmd.OutOrStdout(), "  %d more sources; use %s\n", len(s.Sources)-8, s.DetailCommand)
			}
			if s.ClosedCount > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "  %d closed orders omitted\n", s.ClosedCount)
			}
		}
	}
	if doc.Nodes.Error != nil || doc.Orders.Error != nil || doc.Peers.Error != nil {
		return &resourceBackendExit{code: 1}
	}
	return nil
}

// Terminal rows remain compact; JSON preserves the complete bounded projection.
func overviewText(v any) string {
	r := []rune(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return ' '
		}
		return r
	}, fmt.Sprint(v)))
	if len(r) > 256 {
		return string(r[:256]) + "…"
	}
	return string(r)
}

func overviewBound(s string, limit int) string {
	r := []rune(s)
	if len(r) > limit {
		return string(r[:limit]) + "…"
	}
	return s
}
func overviewValue(row map[string]any, key string) string {
	if v, ok := row[key]; ok {
		return overviewText(v)
	}
	return "?"
}

func overviewSourceState(source map[string]any) string {
	if v, ok := source["state"]; ok {
		return overviewText(v)
	}
	return "ok"
}
