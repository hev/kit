//go:build windows

package update

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// On Windows a running executable cannot be overwritten or deleted. The
// update renames the running hev.exe aside and moves the new build into its
// name; the running process is untouched, and rollback works the same way.
func TestWindowsReplacesRunningExecutable(t *testing.T) {
	if !renameSwap {
		t.Fatal("Windows must switch by rename")
	}
	h := newHarness(t, edgeA)
	h.repo.publish(edgeA, publishOpt{})
	h.repo.publish(edgeB, publishOpt{})

	// A real running executable at the installed name: a copy of this test
	// binary, held open by the process (TestMain sleeps).
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := copyExecutable(self, h.link); err != nil {
		t.Fatal(err)
	}
	run := exec.Command(h.link)
	run.Env = append(os.Environ(), "HEV_UPDATE_TEST_SLEEP=1")
	if err := run.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { run.Process.Kill(); run.Wait() })
	// Overwriting it in place is what Windows refuses.
	if err := os.WriteFile(h.link, []byte("x"), 0o755); err == nil {
		t.Fatal("expected Windows to refuse overwriting a running executable")
	}

	u := h.updater(edgeA)
	if _, err := u.Run(context.Background(), Request{Channel: ChannelEdge}); err != nil {
		t.Fatal(err)
	}
	if h.running(t) != edgeB {
		t.Fatalf("installed name holds %q, want %s", h.running(t), edgeB)
	}
	// The process is still running from the file it started as.
	if run.ProcessState != nil {
		t.Fatal("running process ended")
	}
	if !sameContent(filepath.Join(h.home, ".hev", "versions", edgeA, "hev.exe"), self) {
		t.Fatal("the running build was not kept for rollback")
	}
	if left, _ := filepath.Glob(h.link + ".old-*"); len(left) != 1 {
		t.Fatalf("the running build should stay aside until it exits: %v", left)
	}

	u.Current = edgeB
	if _, err := u.Run(context.Background(), Request{Rollback: true}); err != nil {
		t.Fatal(err)
	}
	if !sameContent(h.link, self) {
		t.Fatal("rollback did not restore the previous build")
	}
}
