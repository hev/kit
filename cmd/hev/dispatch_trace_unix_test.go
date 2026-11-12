//go:build !windows

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

// hev dispatch ORDER.md stamps the run's meta.json with a trace: a new one
// with a root intake ask, or with --trace / trace: a continuation of an
// existing one. The target repository is missing, so admission records the
// run and fails before any worker launch.
func TestDispatchStampsTrace(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, k := range []string{"TRACEPARENT", "HEV_TRACE_ID", "HEV_ORCHESTRATOR", "CLAUDE_SESSION_ID", "CODEX_THREAD_ID"} {
		t.Setenv(k, "")
	}
	t.Setenv("CLAUDE_CODE_SESSION_ID", "foh-harness-session")
	t.Setenv("HEV_CALLER_SESSION", "foh-test")
	bin := t.TempDir()
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	if err := os.WriteFile(filepath.Join(bin, "tmux"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfg, []byte("[hosts.here]\nlocal = true\ncap = 4\n"), 0600); err != nil {
		t.Fatal(err)
	}
	dispatchOrder := func(name, extra string, args ...string) dispatch.Meta {
		t.Helper()
		order := filepath.Join(t.TempDir(), "order.md")
		if err := os.WriteFile(order, []byte("---\nname: "+name+"\nrepo: missing\nhost: here\n"+extra+"---\nShip the trace view.\n## Acceptance\n- Done\n"), 0600); err != nil {
			t.Fatal(err)
		}
		cmd := newDispatchCommand()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(append([]string{"--config", cfg, order}, args...))
		if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "missing") {
			t.Fatalf("%v %s", err, out.String())
		}
		b, err := os.ReadFile(filepath.Join(dispatch.Root(), name, "meta.json"))
		if err != nil {
			t.Fatal(err)
		}
		var m dispatch.Meta
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		if saved, _ := os.ReadFile(filepath.Join(dispatch.Root(), name, "order.md")); strings.Contains(string(saved), "trace:") {
			t.Fatalf("trace key reached the target: %s", saved)
		}
		return m
	}

	first := dispatchOrder("first", "")
	root := dispatch.RootAsk(first)
	if !dispatch.ValidTraceID(first.TraceID) || root == nil || root.Text != "Ship the trace view." || root.Caller.Harness != "claude" || root.Caller.HarnessSession != "foh-harness-session" || root.Caller.Session != "foh-test" || root.Caller.Host == "" || root.Time.IsZero() {
		t.Fatalf("new trace: %+v", first)
	}

	flag := dispatchOrder("second", "", "--trace", first.TraceID)
	if flag.TraceID != first.TraceID || dispatch.RootAsk(flag) != nil || len(flag.Asks) != 1 || flag.Asks[0].Action != "continue" {
		t.Fatalf("--trace: %+v", flag)
	}

	key := dispatchOrder("third", "trace: "+first.TraceID+"\n")
	if key.TraceID != first.TraceID || dispatch.RootAsk(key) != nil {
		t.Fatalf("trace: key: %+v", key)
	}

	order := filepath.Join(t.TempDir(), "order.md")
	os.WriteFile(order, []byte("---\nname: bad\nrepo: missing\nhost: here\n---\nx\n## Acceptance\n- Done\n"), 0600)
	cmd := newDispatchCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--config", cfg, order, "--trace", "not-a-trace"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "invalid trace") {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dispatch.Root(), "bad")); !os.IsNotExist(err) {
		t.Fatal("an invalid --trace created a run")
	}
}
