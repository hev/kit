//go:build !windows

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/dispatch"
)

func TestDispatchDecisionSurface(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bin := t.TempDir()
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := os.WriteFile(filepath.Join(bin, "tmux"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(bin, "config.toml")
	if err := os.WriteFile(config, []byte("[hosts.worker]\nlocal=true\ncap=2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	end := time.Now().UTC()
	for name, result := range map[string]dispatch.Result{
		"pick-db": {End: end, Outcome: dispatch.OutcomeNeedsDecision, Key: "db", Question: "Postgres or SQLite?"},
		"stuck":   {End: end, Outcome: dispatch.OutcomeBlocked, Reason: "no staging credentials"},
		// An older hev writes no outcome.
		"legacy": {End: end},
	} {
		dir := filepath.Join(home, "dispatch", name)
		if err := os.MkdirAll(filepath.Join(dir, "wt"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := dispatch.WriteJSON(filepath.Join(dir, "meta.json"), dispatch.Meta{Order: dispatch.Order{Name: name, Host: "worker", Repo: "project"}, Branch: "d/" + name}); err != nil {
			t.Fatal(err)
		}
		if err := dispatch.WriteJSON(filepath.Join(dir, "result.json"), result); err != nil {
			t.Fatal(err)
		}
	}
	run := func(args ...string) (string, error) {
		cmd := newDispatchCommand()
		var out bytes.Buffer
		cmd.SetArgs(append([]string{"--config", config}, args...))
		cmd.SetOut(&out)
		cmd.SetErr(&bytes.Buffer{})
		err := cmd.Execute()
		return out.String(), err
	}

	out, err := run("ls")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "DECISION") || !strings.Contains(out, "needs-review  db: Postgres or SQLite?") || !strings.Contains(out, "blocked       blocked: no staging credentials") {
		t.Fatalf("ls:\n%s", out)
	}

	out, err = run("show", "pick-db", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var show dispatch.Response
	if err = json.Unmarshal([]byte(out), &show); err != nil {
		t.Fatal(err)
	}
	var result dispatch.Result
	if err = json.Unmarshal([]byte(show.Files["result.json"]), &result); err != nil || result.Key != "db" || result.Question != "Postgres or SQLite?" || result.Outcome != dispatch.OutcomeNeedsDecision {
		t.Fatalf("show --json result %+v %v", result, err)
	}

	for _, tc := range []struct{ name, key, want string }{
		{"pick-db", "cache", "unknown decision key cache; open decision is db"},
		{"pick-db", "bad key", "invalid decision key"},
		{"stuck", "db", "unknown decision key db; stuck has no open decision"},
		{"legacy", "db", "unknown decision key db; legacy has no open decision"},
	} {
		if _, err = run("say", tc.name, "--resolve", tc.key, "SQLite"); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s --resolve %s: %v", tc.name, tc.key, err)
		}
	}
	b, _ := os.ReadFile(filepath.Join(home, "dispatch", "legacy", "result.json"))
	if !strings.Contains(string(b), `"end"`) {
		t.Fatal("refused resolve reworked the run")
	}
}
