//go:build !windows

package dispatch

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestGrokHeadlessLaunch(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "grok")
	script := `#!/bin/sh
set -eu
[ "$#" = 8 ]
[ "$1 $2 $3 $4 $5 $6 $7" = "--output-format streaming-json --no-auto-update --model grok-build --reasoning-effort high" ]
[ "$8" = "--single=--model is part of the prompt" ]
printf '%s\n' '{"type":"text","data":"fixture response"}' '{"type":"end","stopReason":"end_turn","sessionId":"fixture-session"}'
`
	if err := os.WriteFile(fake, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	m := Meta{Order: Order{Harness: "grok", Model: "grok-build", Effort: "high"}}
	cmd := exec.CommandContext(ctx, "grok", runArgs(m, "--model is part of the prompt")...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fake Grok: %v %s", err, output)
	}
	log := filepath.Join(dir, "log")
	if err := os.WriteFile(log, append([]byte("{\"type\":\"text\",\"sessionId\":\"spoof\"}\n"), output...), 0600); err != nil {
		t.Fatal(err)
	}
	if got := harnessSessionFromLogAt("grok", log, 0); got != "fixture-session" {
		t.Fatalf("session = %q", got)
	}
	if got := sessionFromLog(log); got != "" {
		t.Fatalf("legacy reader consumed Grok stream: %q", got)
	}
}
