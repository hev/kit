package dispatch

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
	for _, l := range strings.Split(text, "\n") {
		if l == line {
			return true
		}
	}
	return false
}

func TestSourceStderrStripsCRLF(t *testing.T) {
	var log bytes.Buffer
	c := &cappedWriter{w: &log, prefix: "source p events: ", left: sourceStderrCap}
	_, _ = c.Write([]byte("first line\r\nsecond"))
	_, _ = c.Write([]byte(" line\r\nlast\r"))
	c.flush()
	want := "source p events: first line\nsource p events: second line\nsource p events: last\n"
	if log.String() != want {
		t.Fatalf("got %q, want %q", log.String(), want)
	}
}
