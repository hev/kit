package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hev/kit/internal/dispatch"
	"github.com/spf13/cobra"
)

const swarmExample = `---
kind: swarm
name: logfix
goal: Replace every deprecated logging call in app with the structured logger, tests green, one draft PR.
reply: owner-session
host: build-1
every: 30m
max_lifetime: 2d
harness: claude
model: claude-opus-5-5
budget: {concurrent_runs: 4, runs_per_day: 20}
reconcile_every: 2m
restart: {limit: 3, backoff: 2m, max_backoff: 30m}
roles:
  - name: migrate
    shape: partition
    replicas: 3
    pieces: [api, worker, cli, storage, web]
    stall: 45m
    order: {repo: app, base: main, harness: codex, model: model-a, timeout: 90, class: code}
  - name: integrate
    shape: single
    after: [migrate]
    effects: pr
    order: {repo: app, base: main, harness: claude, model: claude-opus-5-5, timeout: 60}
    placement: {hosts: [build-1]}
---
## Problem

app still calls log.Printf and the old logx helpers in five packages.
Move each call to slog with structured fields. Keep messages and levels.

## Acceptance
- No call to the deprecated API remains (grep proof in the report).
- go test ./... passes on the integration branch.
- One draft PR against main.

## Role: migrate

Convert only your piece's package and its tests. Push after every commit.

### Acceptance
- Your package has no deprecated calls; its tests pass.

## Role: integrate

Merge the listed piece branches onto your branch in order, fix conflicts,
run the whole test suite, open one draft PR.

### Acceptance
- All piece branches are merged; the whole suite passes; the PR lists each piece.
`

func TestSwarmUpDryRunIsAcceptedDownstream(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HEV_DISPATCH_CONFIG", filepath.Join(home, "none.toml"))
	t.Setenv("PEERS_SELF", "test-host")
	t.Setenv("HEV_ORCHESTRATOR", "")
	dir := t.TempDir()
	file := filepath.Join(dir, "logfix.md")
	if e := os.WriteFile(file, []byte(swarmExample), 0600); e != nil {
		t.Fatal(e)
	}
	cmd := newSwarmCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"up", "-f", file, "--dry-run"})
	if e := cmd.Execute(); e != nil {
		t.Fatal(e)
	}
	// Split the printed spec and orders on their separators.
	parts := regexp.MustCompile(`(?m)^--- (orchestrator|order) ([a-z0-9-]+) ---\n`).Split(out.String(), -1)
	heads := regexp.MustCompile(`(?m)^--- (orchestrator|order) ([a-z0-9-]+) ---\n`).FindAllStringSubmatch(out.String(), -1)
	if len(heads) != 7 || heads[0][1] != "orchestrator" || !strings.Contains(parts[0], "swarm logfix: owner build-1, 2 roles, 6 units") {
		t.Fatalf("dry run output:\n%s", out.String())
	}
	spec := filepath.Join(dir, "spec.md")
	if e := os.WriteFile(spec, []byte(parts[1]), 0600); e != nil {
		t.Fatal(e)
	}
	apply := newOrchestrateCommand()
	var applied bytes.Buffer
	apply.SetOut(&applied)
	apply.SetArgs([]string{"apply", "--dry-run", "-f", spec})
	if e := apply.Execute(); e != nil || !strings.Contains(applied.String(), "valid logfix") {
		t.Fatalf("orchestrate apply --dry-run: %v %s", e, applied.String())
	}
	for i, h := range heads[1:] {
		o, e := dispatch.Parse(parts[i+2])
		if e != nil || o.Name != h[2] {
			t.Fatalf("order %s: %+v %v\n%s", h[2], o, e, parts[i+2])
		}
	}
	for _, p := range []string{dispatch.Root(), dispatch.OrchestratorConfigDir()} {
		if _, e := os.Stat(p); !os.IsNotExist(e) {
			t.Fatalf("dry run wrote %s", p)
		}
	}
	// A later-phase field is refused by name, without a usage dump.
	full := strings.Replace(swarmExample, "every: 30m", "every: 30m\nstandby: [build-2]", 1)
	if e := os.WriteFile(file, []byte(full), 0600); e != nil {
		t.Fatal(e)
	}
	cmd = newSwarmCommand()
	out.Reset()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"up", "-f", file, "--dry-run"})
	if e := cmd.Execute(); e == nil || !strings.Contains(e.Error(), "standby needs swarm phase 4") || strings.Contains(out.String(), "Usage:") {
		t.Fatalf("refusal: %v\n%s", e, out.String())
	}
}

func TestSwarmSurfaceAndHelp(t *testing.T) {
	c := newSwarmCommand()
	for _, n := range []string{"up", "ps", "logs", "say", "pause", "resume", "down"} {
		if sub, _, e := c.Find([]string{n}); e != nil || sub.Name() != n {
			t.Fatalf("missing swarm %s", n)
		}
	}
	ps, _, _ := c.Find([]string{"ps"})
	if ps.Flags().Lookup("json") == nil {
		t.Fatal("ps lacks --json")
	}
	root := &cobra.Command{Use: "hev", Long: rootCmd.Long}
	orch := newOrchestrateCommand()
	root.AddCommand(orch)
	groupClusterCommands(root)
	var out bytes.Buffer
	orch.SetOut(&out)
	if err := orch.Help(); err != nil || !strings.Contains(out.String(), "swarm") {
		t.Fatalf("orchestrate help missing swarm: %v\n%s", err, out.String())
	}
	out.Reset()
	root.SetOut(&out)
	root.SetArgs([]string{"orchestrate", "swarm", "up", "--help"})
	if err := root.Execute(); err != nil || !strings.Contains(out.String(), "hev orchestrate swarm up") {
		t.Fatalf("direct orchestrate swarm help failed: %v\n%s", err, out.String())
	}
}
