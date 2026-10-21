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

const overviewTimeout = 3 * time.Second
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

// Only selected metadata is exposed, never briefs, pane titles or raw errors.
func overviewProject(row map[string]any, keys ...string) map[string]any {
	out := map[string]any{}
	for _, k := range keys {
		switch v := row[k].(type) {
		case string:
			out[k] = strings.Map(func(r rune) rune {
				if unicode.IsControl(r) {
					return ' '
				}
				return r
			}, v)
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
	scopes := []string{"selected node only; not a fleet registry or readiness claim", "configured coordination backend (PlanetScale); active orders within backend list window (up to 100 by ID), not a total", "local peers and backend-provided snapshots; no fleet-wide liveness claim"}
	var wg sync.WaitGroup
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
				s.Items = append(s.Items, overviewProject(row, "id", "status", "owner", "phase", "next_action", "incarnation", "host", "updated_at", "lease_until"))
			}
		}(i, s)
	}
	wg.Wait()
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
			item := overviewProject(row, "address", "session", "agent", "state", "role", "activity")
			item["host"] = host
			item["source"] = source
			item["source_at"] = at
			item["stale"] = stale
			s.Items = append(s.Items, item)
		}
		s.Sources = append(s.Sources, map[string]any{"host": host, "source": source, "source_at": at, "stale": stale, "observed_peer_count": len(rows)})
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
			s.Sources = append(s.Sources, map[string]any{"host": h.Host, "source": h.Source, "state": "unknown"})
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
			fmt.Fprintf(cmd.OutOrStdout(), "%s — %s — %s\n  %s; read at %s\n", []string{"Nodes", "Orders", "Peers"}[i], s.State, s.DetailCommand, s.Scope, s.ObservedAt)
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
				fmt.Fprint(cmd.OutOrStdout(), "  ")
				for _, k := range []string{"id", "address", "status", "state", "owner", "phase", "next_action", "role", "agent", "host", "source", "source_at", "stale"} {
					if v, ok := item[k]; ok {
						fmt.Fprintf(cmd.OutOrStdout(), "%s=%s  ", k, overviewText(v))
					}
				}
				fmt.Fprintln(cmd.OutOrStdout())
			}
			sourceLimit := len(s.Sources)
			if sourceLimit > 8 {
				sourceLimit = 8
			}
			for _, source := range s.Sources[:sourceLimit] {
				b, _ := json.Marshal(source)
				fmt.Fprintf(cmd.OutOrStdout(), "  source %s\n", overviewText(string(b)))
			}
			if len(s.Sources) > sourceLimit {
				fmt.Fprintf(cmd.OutOrStdout(), "  %d more sources; use %s\n", len(s.Sources)-sourceLimit, s.DetailCommand)
			}
			if len(s.Items) > limit {
				fmt.Fprintf(cmd.OutOrStdout(), "  %d more; use %s\n", len(s.Items)-limit, s.DetailCommand)
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
	r := []rune(fmt.Sprint(v))
	if len(r) > 256 {
		return string(r[:256]) + "…"
	}
	return string(r)
}
