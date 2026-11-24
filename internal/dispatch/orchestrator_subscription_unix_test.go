//go:build !windows

package dispatch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/attention"
)

func TestOrchestratorSubscriptionRemoteTimeoutFailsOpen(t *testing.T) {
	emitter := subscriptionEmitter(t, `[notify]
client = true
[[notify.orchestrators]]
name = "remote-orch"
host = "remote"
events = ["done"]
sources = ["run"]
`)
	if len(emitter.Config.Orchestrators) != 1 || len(emitter.Config.Orchestrators[0].Events) != 1 {
		t.Fatalf("forwarding route did not load: %+v", emitter.Config.Orchestrators)
	}
	if err := os.WriteFile(ConfigPath(""), []byte("[hosts.remote]\nssh = 'remote'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	captured := filepath.Join(bin, "request")
	t.Setenv("CAPTURED_REQUEST", captured)
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte("#!/bin/sh\ncat > \"$CAPTURED_REQUEST\"\nexec sleep 30\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	n := attention.New("done", "run", "test-host", "job", "key", "complete",
		"private-secret /private/customer/repo", attention.Links{PR: "https://private.example/pull/7", Run: "/private/logs/run"})
	start := time.Now()
	if err := emitter.Emit(context.Background(), n, ""); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	if elapsed < 1500*time.Millisecond || elapsed > 4*time.Second {
		t.Fatalf("remote attempt took %s, want the two-second deadline", elapsed)
	}
	b, err := os.ReadFile(captured)
	if err != nil || !strings.Contains(string(b), `"event":{"event":"done","source":"run","host":"test-host","name":"job","state":"complete"}`) {
		t.Fatalf("expected metadata-only RPC, got %s, %v", b, err)
	}
	for _, forbidden := range []string{"private-secret", "/private/", "private.example", `"text"`, `"links"`} {
		if strings.Contains(string(b), forbidden) {
			t.Fatalf("remote request exposed %q", forbidden)
		}
	}
}
