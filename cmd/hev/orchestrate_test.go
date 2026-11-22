package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/dispatch"
)

func TestOrchestrateDryRunAndDefaultOff(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	const example = `---
kind: orchestrator
name: release-train
goal: Every PR labelled release-0.9 is merged with green CI by Friday.
host: build-1              # where rounds run; default = host of the managed runs
mode: loop                 # once | loop
every: 20m
until: goal                # goal | 2026-11-08 | never (needs max_lifetime)
max_lifetime: 7d
harness: claude
model: claude-opus-5-5
reply: my-session          # peers address that hears about decisions and blockers

manages:                   # scope; nothing outside it can be touched
  runs: {selector: "issue=REL-*"}   # or names: [...] or "mine"
  repos: [app]

authority:                 # default is say + notify only
  - say
  - rework
  - retry
  - validate
  - dispatch               # start new runs within scope

budget:
  concurrent_runs: 3
  runs_per_day: 20
  usd_per_day: 40

handoff:
  rotate_after: 30         # rounds; then a summarised checkpoint starts a new lineage

idle: skip                 # skip | run — whether a round with no new evidence calls the model
source: none               # or a plugin, see "Sources"
---
Standing brief.
`
	file := filepath.Join(t.TempDir(), "example.md")
	if e := os.WriteFile(file, []byte(example), 0600); e != nil {
		t.Fatal(e)
	}
	cmd := newOrchestrateCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"apply", "--dry-run", "-f", file})
	if e := cmd.Execute(); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(out.String(), "valid release-train") {
		t.Fatal(out.String())
	}
	if _, e := os.Stat(dispatch.OrchestratorConfigDir()); !os.IsNotExist(e) {
		t.Fatal("dry run created config")
	}
	if _, e := os.Stat(dispatch.Root()); !os.IsNotExist(e) {
		t.Fatal("dry run created runtime state")
	}
	cmd = newOrchestrateCommand()
	out.Reset()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"ls", "--json"})
	if e := cmd.Execute(); e != nil {
		t.Fatal(e)
	}
	if strings.TrimSpace(out.String()) != "[]" {
		t.Fatal(out.String())
	}
}
func TestOrchestrateSurface(t *testing.T) {
	c := newOrchestrateCommand()
	for _, n := range []string{"apply", "start", "ls", "show", "logs", "say", "wake", "pause", "resume", "stop", "handoff", "reconcile", "render", "follow"} {
		sub, _, e := c.Find([]string{n})
		if e != nil || sub == c {
			t.Fatalf("missing %s", n)
		}
	}
}

func TestOrchestrateJSONContract(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if _, e := dispatch.ApplyOrchestrator("---\nkind: orchestrator\nname: inventory\ngoal: Observe test work\nmanages: {runs: {names: [task]}}\n---\nBrief", false); e != nil {
		t.Fatal(e)
	}
	cmd := newOrchestrateCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"ls", "--json"})
	if e := cmd.Execute(); e != nil {
		t.Fatal(e)
	}
	var rows []map[string]any
	if e := json.Unmarshal(out.Bytes(), &rows); e != nil {
		t.Fatal(e)
	}
	if len(rows) != 1 {
		t.Fatal(rows)
	}
	for _, key := range []string{"name", "goal", "mode", "state", "start", "next_wake", "rounds", "lineage", "managed_runs", "runs_today", "spend_usd", "last_notice", "consecutive_failures"} {
		if _, ok := rows[0][key]; !ok {
			t.Fatal("missing stable field", key)
		}
	}
	if rows[0]["spend_usd"] != nil || rows[0]["state"] != "ready" {
		t.Fatal(rows)
	}
}

func TestOrchestrateNoticeVisibility(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if _, err := dispatch.ApplyOrchestrator("---\nkind: orchestrator\nname: visible\ngoal: Observe\nmanages: {runs: {names: [task]}}\n---\nBrief", false); err != nil {
		t.Fatal(err)
	}
	notice := dispatch.OrchestratorNotice{ID: "event-one", To: "owner", Status: "queued", Error: "unreachable", Message: "Decision needed"}
	if err := dispatch.WriteJSON(filepath.Join(dispatch.OrchestratorDir("visible"), "last-notice.json"), notice); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"show", "visible"}, {"ls", "--json"}} {
		cmd := newOrchestrateCommand()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"event-one", "queued", "unreachable", "Decision needed"} {
			if !strings.Contains(out.String(), want) {
				t.Fatal(args, out.String())
			}
		}
	}
}
