//go:build !windows

package hooks

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/testenv"
)

func TestRunLifecycleDeliversToUserHandlers(t *testing.T) {
	bin, out, logDir := t.TempDir(), t.TempDir(), t.TempDir()
	// Each handler is primed once after it is written: macOS checks a new
	// executable the first time it runs, serialised across processes, and
	// three of those checks alone can outlast the 500ms budget below.
	write := func(name, body string) {
		exe := filepath.Join(bin, name)
		if err := os.WriteFile(exe, []byte("#!/bin/sh\n[ \"$1\" = "+testenv.PrimeArg+" ] && exit 0\n"+body), 0755); err != nil {
			t.Fatal(err)
		}
		testenv.PrimeExec(t, nil, exe)
	}
	write("hev-hook-record", `cat > "`+out+`/stdin"; echo "$1 $HEV_HOOK_EVENT $HEV_HOOK_HARNESS" > "`+out+`/args"`+"\n")
	write("hev-hook-broken", "exit 3\n")
	write("hev-hook-slow", "sleep 30\n")
	t.Setenv("HEV_HOOKS", "")
	payload := []byte(`{"schema":"hev.attention.event/v1","event":"needs-you","name":"kit-x"}`)
	start := time.Now()
	RunLifecycle(context.Background(), "needs-you", payload, bin, Config{Enabled: true, Timeout: 500 * time.Millisecond}, Log{Dir: logDir})
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("a slow handler held the emitter for %s", elapsed)
	}
	got, _ := os.ReadFile(filepath.Join(out, "stdin"))
	args, _ := os.ReadFile(filepath.Join(out, "args"))
	if string(got) != string(payload) || strings.TrimSpace(string(args)) != "needs-you needs-you kit" {
		t.Fatalf("handler saw %q with %q", got, args)
	}
	warnings, _ := os.ReadFile(filepath.Join(logDir, "warnings.log"))
	if !strings.Contains(string(warnings), "hev-hook-broken failed on needs-you") || !strings.Contains(string(warnings), "hev-hook-slow failed on needs-you: timed out") {
		t.Fatalf("warnings %s", warnings)
	}

	// The kill switch skips handlers.
	os.Remove(filepath.Join(out, "stdin"))
	t.Setenv("HEV_HOOKS", "off")
	RunLifecycle(context.Background(), "needs-you", payload, bin, Config{Enabled: true, Timeout: time.Second}, Log{Dir: logDir})
	if _, err := os.Stat(filepath.Join(out, "stdin")); err == nil {
		t.Fatal("HEV_HOOKS=off still ran handlers")
	}
	if !ValidLifecycleEvent("ready") || ValidLifecycleEvent("pre-tool") {
		t.Fatal("lifecycle events")
	}
}
