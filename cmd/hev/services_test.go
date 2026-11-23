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

	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/secrets"
	"github.com/hev/kit/internal/secrets/fakeop"
	"github.com/hev/kit/internal/services"
)

func TestServiceCLIAndDaemonShareStateSecretsAndRedaction(t *testing.T) {
	h := newShellHost(t, false)
	t.Setenv("HEV_SECRETS_ENDPOINT", filepath.Join(h.root, "secrets.sock"))
	t.Setenv("HEV_SECRETS_CACHE", "")
	t.Setenv("HEVLAYER_API_KEY", "")
	t.Setenv("HEVLAYER_API_KEY_REF", "")
	t.Setenv("HEVLAYER_API_KEY_OP_REF", "")
	t.Setenv("HEV_ARCHIVE_DIR", filepath.Join(h.root, "archive"))
	t.Setenv("HEVD_GH_USER", "services-fixture")
	t.Setenv("HEV_ORDERS_DIR", filepath.Join(h.root, "dispatch"))
	t.Setenv("HEV_CONFIG", filepath.Join(h.root, "config.toml"))
	t.Setenv("HEV_FEED_DIR", filepath.Join(h.root, "feed"))

	pat := "ghp_1234567890abcdefghijklmnopqrstuvwxyz"
	script := filepath.Join(h.root, "svc.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"+
		"trap 'exit 0' TERM INT\n"+
		"printf 'secret=%s\\n' \"$FIXTURE_TOKEN\"\n"+
		"printf 'pat=%s\\n' '"+pat+"'\n"+
		"printf 'dir=%s\\n' \"$(pwd)\"\n"+
		"printf 'ready-line\\n'\n"+
		"while :; do sleep 0.05; done\n"), 0700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(os.Getenv("HEV_CONFIG"), []byte(`
[capture]
scan_roots = ["fixture-root"]

[[service]]
name = "preview"
cmd = ["`+script+`"]
stop_timeout = "5s"
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
		cmd := newServiceCommand()
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		err := cmd.ExecuteContext(ctx)
		return out.String(), err
	}

	tick := time.NewTicker(40 * time.Millisecond)
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

	inspectPreview := func() (services.Details, error) {
		var d services.Details
		out, err := call("show", "preview", "--json")
		if err != nil {
			return d, err
		}
		err = json.Unmarshal([]byte(out), &d)
		return d, err
	}

	var detail services.Details
	for {
		var err error
		detail, err = inspectPreview()
		if err == nil && detail.Status == services.StatusRunning && detail.PID > 0 &&
			strings.Contains(detail.Logs, "ready-line") {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("hevd did not start preview service; last detail=%+v", detail)
		case <-tick.C:
		}
	}
	firstPID := detail.PID

	for _, forbidden := range []string{fakeop.Value, pat, h.home, h.root} {
		if strings.Contains(detail.Logs, forbidden) {
			t.Fatalf("show --json leaked %q in logs: %s", forbidden, detail.Logs)
		}
	}
	if !strings.Contains(detail.Logs, "secret=[REDACTED]") || !strings.Contains(detail.Logs, "pat=[REDACTED:github-pat#") {
		t.Fatalf("expected scrubbed markers in logs: %s", detail.Logs)
	}

	// ls (table and --json)
	lsOut, err := call("ls")
	if err != nil || !strings.Contains(lsOut, "preview") || !strings.Contains(lsOut, "running") {
		t.Fatalf("ls failed: %v\n%s", err, lsOut)
	}
	lsJSON, err := call("ls", "--json")
	if err != nil || !strings.Contains(lsJSON, `"status":"running"`) {
		t.Fatalf("ls --json failed: %v\n%s", err, lsJSON)
	}

	// show (human readable)
	showText, err := call("show", "preview")
	if err != nil || !strings.Contains(showText, "secret=[REDACTED]") || strings.Contains(showText, fakeop.Value) || strings.Contains(showText, pat) {
		t.Fatalf("show failed or leaked secret: %v\n%s", err, showText)
	}

	// logs -n 2
	logsOut, err := call("logs", "preview", "-n", "2")
	if err != nil {
		t.Fatalf("logs -n 2 failed: %v", err)
	}
	logLines := strings.Split(strings.TrimSuffix(logsOut, "\n"), "\n")
	if len(logLines) != 2 || logLines[1] != "ready-line" || strings.Contains(logsOut, fakeop.Value) || strings.Contains(logsOut, h.home) {
		t.Fatalf("unexpected logs -n 2 output: %q", logLines)
	}

	// restart -> stops old PID and starts a new PID, reusing the daemon secrets cache!
	restartOut, err := call("restart", "preview")
	if err != nil || !strings.Contains(restartOut, "preview restarted") {
		t.Fatalf("restart failed: %v %q", err, restartOut)
	}
	detail, err = inspectPreview()
	if err != nil {
		t.Fatalf("show after restart failed: %v", err)
	}
	if detail.Status != services.StatusRunning || detail.PID <= 0 || detail.PID == firstPID {
		t.Fatalf("expected new running PID after restart (was %d, got %+v)", firstPID, detail)
	}

	// stop -> stops cleanly and stays stopped
	stopOut, err := call("stop", "preview")
	if err != nil || !strings.Contains(stopOut, "preview stopped") {
		t.Fatalf("stop failed: %v %q", err, stopOut)
	}
	detail, err = inspectPreview()
	if err != nil {
		t.Fatalf("show after stop failed: %v", err)
	}
	if detail.Status != services.StatusStopped || detail.PID != 0 {
		t.Fatalf("expected stopped status and pid 0, got %+v", detail)
	}

	// start -> starts again
	startOut, err := call("start", "preview")
	if err != nil || !strings.Contains(startOut, "preview started") {
		t.Fatalf("start failed: %v %q", err, startOut)
	}
	detail, err = inspectPreview()
	if err != nil || detail.Status != services.StatusRunning || detail.PID <= 0 {
		t.Fatalf("expected running after start, got %+v %v", detail, err)
	}

	// Refusal cases on CLI
	if _, err := call("show", "unknown"); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("expected unknown service refusal, got %v", err)
	}
	if _, err := call("logs", "preview", "-n", "-1"); err == nil || !strings.Contains(err.Error(), "nonnegative") {
		t.Fatalf("expected negative lines refusal, got %v", err)
	}

	h.noValueAnywhere(t)
	reads := 0
	for _, line := range h.op.CallLines(t) {
		if line == "inject" {
			reads++
		}
	}
	if reads != 1 {
		t.Fatalf("initial start, restart, and start must share one provider read via cache, saw %d", reads)
	}
}
