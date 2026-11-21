//go:build !windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/cron"
	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/secrets"
	"github.com/hev/kit/internal/secrets/fakeop"
)

func TestCronCLIAndDaemonShareStateAndSecretsCache(t *testing.T) {
	h := newShellHost(t, false)
	t.Setenv("HEV_SECRETS_ENDPOINT", filepath.Join(h.root, "secrets.sock"))
	t.Setenv("HEV_SECRETS_CACHE", "")
	t.Setenv("HEVLAYER_API_KEY", "")
	t.Setenv("HEVLAYER_API_KEY_REF", "")
	t.Setenv("HEVLAYER_API_KEY_OP_REF", "")
	t.Setenv("HEV_ARCHIVE_DIR", filepath.Join(h.root, "archive"))
	t.Setenv("HEVD_GH_USER", "jobs-fixture")
	t.Setenv("HEV_ORDERS_DIR", filepath.Join(h.root, "dispatch"))
	t.Setenv("HEV_CONFIG", filepath.Join(h.root, "config.toml"))
	t.Setenv("HEV_FEED_DIR", filepath.Join(h.root, "feed"))
	if err := os.WriteFile(os.Getenv("HEV_CONFIG"), []byte(`
[capture]
scan_roots = ["fixture-root"]

[[cron]]
name = "scheduled"
every = "10s"
cmd = ["/bin/sh", "-c", 'printf "%s\n" "$FIXTURE_TOKEN"; printf "%s\n" "manual; literal"']
env_refs = { FIXTURE_TOKEN = "op://test/item/token" }
`), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Second)
	done := make(chan error, 1)
	go func() { done <- daemon.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("hevd returned: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("hevd did not stop promptly")
		}
	})
	call := func(args ...string) (string, error) {
		var out bytes.Buffer
		cmd := newCronCommand()
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		err := cmd.ExecuteContext(ctx)
		return out.String(), err
	}
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		if secrets.Ping(ctx) == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("hevd secrets cache did not start")
		case <-tick.C:
		}
	}
	var detail cron.Details
	for {
		out, err := call("show", "scheduled", "--json")
		if err != nil || json.Unmarshal([]byte(out), &detail) != nil {
			t.Fatal("could not inspect scheduled job")
		}
		if len(detail.Runs) != 0 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("hevd did not execute the scheduled tick")
		case <-tick.C:
		}
	}
	if detail.Runs[0].Status != "ok" || detail.Runs[0].Output != "[REDACTED]\nmanual; literal\n" {
		t.Fatal("expected scheduled tick to use the cache and redact its output")
	}
	if _, err := os.Stat(filepath.Join(h.home, ".hev", "cron", "scheduled", "state.json")); err != nil {
		t.Fatal("expected hevd to persist state under the cron directory")
	}
	assertCall := func(expected string, args ...string) {
		t.Helper()
		out, err := call(args...)
		if err != nil || !strings.Contains(out, expected) || strings.Contains(out, fakeop.Value) {
			t.Fatalf("unexpected result from %v (error %v)", args, err)
		}
	}
	assertCall("scheduled paused", "pause", "scheduled")
	assertCall(`"status":"paused"`, "ls", "--json")
	if out, err := call("run", "scheduled"); err == nil || !strings.Contains(out, "skipped_paused") {
		t.Fatal("manual run must honor a pause")
	}
	assertCall("scheduled resumed", "resume", "scheduled")
	assertCall("[REDACTED]\nmanual; literal", "run", "scheduled")
	assertCall("[REDACTED]", "show", "scheduled")
	assertCall("scheduled", "ls")
	assertCall(`"exit_code":0`, "show", "scheduled", "--json")
	h.noValueAnywhere(t)
	reads := 0
	for _, line := range h.op.CallLines(t) {
		if line == "inject" {
			reads++
		}
	}
	if reads != 1 {
		t.Fatalf("scheduled and manual jobs must share one provider read, saw %d", reads)
	}
}
