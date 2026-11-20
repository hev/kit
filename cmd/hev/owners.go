package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/hostlabel"
	"github.com/hev/kit/internal/peers"
)

// Owner liveness, as orchestrate ls and the --orphaned views report it.
// Only gone counts as orphaned: an asleep host (the laptop with its lid shut)
// may come back with the session intact.
const (
	ownerLive   = "live"
	ownerGone   = "gone"
	ownerAsleep = "asleep"
	ownerNone   = "none"
)

// ownerPeers lists the sessions and run addresses peers sees on one host. An
// error means the host did not answer. Tests replace it with a fake peers.
var ownerPeers = func(ctx context.Context, host string) ([]string, error) {
	e := peers.New()
	var out bytes.Buffer
	args := []string{"ls", "--json", "-a"}
	var err error
	if hostlabel.Same(host, e.Host) {
		err = e.Run(ctx, args, nil, &out, io.Discard)
	} else {
		err = e.Transport.Execute(ctx, hostlabel.Route(host), args, nil, &out, io.Discard)
	}
	if err != nil {
		return nil, err
	}
	var inv struct {
		Panes []struct {
			Address string `json:"address"`
			Session string `json:"session"`
		} `json:"panes"`
	}
	if err = json.Unmarshal(out.Bytes(), &inv); err != nil {
		return nil, fmt.Errorf("peers inventory from %s: %w", host, err)
	}
	var names []string
	for _, p := range inv.Panes {
		names = append(names, p.Session)
		if _, n, ok := strings.Cut(p.Address, "/"); ok {
			names = append(names, n)
		}
	}
	return names, nil
}

// ownerOwnAddress is the caller's own peers identity, the default adopt --to.
var ownerOwnAddress = func(ctx context.Context) (string, error) {
	addr, err := peers.New().OwnAddress(ctx)
	if err != nil {
		return "", fmt.Errorf("cannot tell which session you are (%v); pass --to HOST/SESSION", err)
	}
	return addr, nil
}

// ownerProber asks peers on each host at most once per command.
type ownerProber struct {
	mu    sync.Mutex
	hosts map[string]*ownerHost
}

type ownerHost struct {
	once  sync.Once
	names map[string]bool
	err   error
}

func newOwnerProber() *ownerProber { return &ownerProber{hosts: map[string]*ownerHost{}} }

// splitOwner names the host and session of a reply. A bare session is on
// the host that sends the notices, defaultHost.
func splitOwner(reply, defaultHost string) (string, string) {
	if h, n, ok := strings.Cut(reply, "/"); ok && h != "" {
		return h, n
	}
	return defaultHost, reply
}

// status is live, gone, asleep or none (no reply).
func (p *ownerProber) status(ctx context.Context, reply, defaultHost string) string {
	if strings.TrimSpace(reply) == "" {
		return ownerNone
	}
	host, session := splitOwner(reply, defaultHost)
	key := hostlabel.Canonical(host)
	p.mu.Lock()
	h, ok := p.hosts[key]
	if !ok {
		h = &ownerHost{}
		p.hosts[key] = h
	}
	p.mu.Unlock()
	h.once.Do(func() {
		c, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		var names []string
		names, h.err = ownerPeers(c, host)
		h.names = map[string]bool{}
		for _, n := range names {
			h.names[n] = true
		}
	})
	switch {
	case h.err != nil:
		return ownerAsleep
	case h.names[session]:
		return ownerLive
	}
	return ownerGone
}

func ownerColumn(reply, status string) string {
	if status == ownerNone {
		return "-"
	}
	return reply + " (" + status + ")"
}

// orphanRow is one line of an --orphaned view.
type orphanRow struct {
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Host        string `json:"host"`
	State       string `json:"state"`
	Owner       string `json:"owner"`
	OwnerStatus string `json:"owner_status"`
	Adopt       string `json:"adopt"`
}

func adoptCommand(kind, name, host string) string {
	if kind == "orch" {
		return "hev orchestrate adopt " + strings.TrimPrefix(name, "o-") + " --host " + host
	}
	return "hev dispatch adopt " + name + " --host " + host
}

// openRow is a non-terminal orchestrator or an open run: not done, merged or
// stopped.
func openRow(row dispatch.Row) bool {
	switch row.State {
	case "done", "merged", "stopped", "complete", "handed-off":
		return false
	}
	return row.State != "" && row.State != "broken"
}

// orphanRows are open rows across an inventory whose owner is gone. kinds
// selects orch, run or both.
func orphanRows(ctx context.Context, inv map[string]dispatch.Response, kinds map[string]bool) []orphanRow {
	probe := newOwnerProber()
	var mu sync.Mutex
	var wg sync.WaitGroup
	out := []orphanRow{}
	for _, host := range dispatch.SortedHosts(inv) {
		r := inv[host]
		if r.Error != "" {
			continue
		}
		for _, row := range r.Rows {
			if !kinds[row.Kind] || !openRow(row) || row.Reply == "" {
				continue
			}
			wg.Add(1)
			go func(host string, row dispatch.Row) {
				defer wg.Done()
				label := row.Host
				if label == "" {
					label = host
				}
				st := probe.status(ctx, row.Reply, label)
				if st != ownerGone {
					return
				}
				mu.Lock()
				out = append(out, orphanRow{Kind: row.Kind, Name: row.Name, Host: host, State: row.State, Owner: row.Reply, OwnerStatus: st, Adopt: adoptCommand(row.Kind, row.Name, host)})
				mu.Unlock()
			}(host, row)
		}
	}
	wg.Wait()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Host != out[j].Host {
			return out[i].Host < out[j].Host
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func printOrphans(w io.Writer, rows []orphanRow, asJSON bool) error {
	if asJSON {
		return json.NewEncoder(w).Encode(rows)
	}
	if len(rows) == 0 {
		fmt.Fprintln(w, "no orphaned orchestrators or runs (owners that are asleep are not counted)")
		return nil
	}
	for _, r := range rows {
		fmt.Fprintf(w, "%s %s on %s (%s): owner %s is gone\n  %s\n", r.Kind, r.Name, r.Host, r.State, r.Owner, r.Adopt)
	}
	return nil
}

// stdinIsTTY reports whether a confirmation can be asked interactively.
func stdinIsTTY(in io.Reader) bool {
	f, ok := in.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

// orphanInventory is every reachable host's inventory, plus this machine's
// own when no configured host is local. Tests replace it.
var orphanInventory = func() map[string]dispatch.Response {
	inv := map[string]dispatch.Response{}
	hasLocal := false
	if cfg, err := dispatch.Load(dispatch.ConfigPath("")); err == nil {
		inv = dispatch.Inventory(cfg, "")
		for _, h := range cfg.Hosts {
			hasLocal = hasLocal || h.Local
		}
	}
	if !hasLocal {
		r, err := dispatch.Handle(dispatch.Request{Action: "list"})
		if err != nil {
			r.Error = err.Error()
		}
		inv[selfHostLabel()] = r
	}
	return inv
}

func selfHostLabel() string { return hostlabel.Self(os.Hostname) }
