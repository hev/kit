package dispatch

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// installHangSource puts the synthetic testdata plugin first on PATH.
func installHangSource(t *testing.T) (OrchestratorSpec, string) {
	t.Helper()
	s := sourceFixture(t)
	sourceCommand = execSourcePlugin
	s.Source.Plugin = "hang-source"
	bin, work := t.TempDir(), t.TempDir()
	name := "hev-hang-source"
	if runtime.GOOS == "windows" {
		name += ".cmd"
	}
	b, e := os.ReadFile(filepath.Join("testdata", "source", name))
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(bin, name), b, 0700); e != nil {
		t.Fatal(e)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HEV_TEST_DIR", work)
	return s, work
}

// testSourceTimeoutKillsTree cancels a plugin whose grandchild holds stdout
// and later writes a marker. Only a tree kill prevents the marker.
func testSourceTimeoutKillsTree(t *testing.T, survive time.Duration) {
	s, work := installHangSource(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, e := execSourcePlugin(ctx, s, sourceInvocation{Args: []string{"events"}})
		done <- e
	}()
	deadline := time.Now().Add(60 * time.Second)
	for !exists(filepath.Join(work, "started")) {
		if time.Now().After(deadline) {
			t.Fatal("grandchild never started")
		}
		time.Sleep(20 * time.Millisecond)
	}
	start := time.Now()
	cancel()
	select {
	case e := <-done:
		if e == nil {
			t.Fatal("cancelled plugin succeeded")
		}
	case <-time.After(sourceWaitDelay + 10*time.Second):
		t.Fatal("plugin exec hung after cancellation")
	}
	if waited := time.Since(start); waited > sourceWaitDelay+5*time.Second {
		t.Fatal("cancellation took", waited)
	}
	time.Sleep(survive)
	if exists(filepath.Join(work, "survived")) {
		t.Fatal("grandchild survived the plugin timeout")
	}
	log, _ := os.ReadFile(filepath.Join(OrchestratorDir(s.Name), "supervisor.log"))
	if !containsLine(string(log), "source hang-source events: diagnostic line") {
		t.Fatalf("stderr not logged: %q", log)
	}
}

func containsLine(text, line string) bool {
	for _, l := range splitLines(text) {
		if l == line {
			return true
		}
	}
	return false
}

func splitLines(text string) []string {
	var out []string
	start := 0
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			out = append(out, trimCR(text[start:i]))
			start = i + 1
		}
	}
	if start < len(text) {
		out = append(out, trimCR(text[start:]))
	}
	return out
}

func trimCR(s string) string {
	if len(s) > 0 && s[len(s)-1] == '\r' {
		return s[:len(s)-1]
	}
	return s
}
