//go:build !windows

package platformops

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLockExclusionReleaseAndNoFollow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	release, e := LockPath(path, false)
	if e != nil {
		t.Fatal(e)
	}
	_, e = LockPath(path, true)
	if !IsLockBusy(e) {
		t.Fatalf("lock failed to exclude writer: %v", e)
	}
	release()
	again, e := LockPath(path, true)
	if e != nil {
		t.Fatal(e)
	}
	again()
	target := filepath.Join(t.TempDir(), "target")
	if e = os.WriteFile(target, []byte("sentinel"), 0600); e != nil {
		t.Fatal(e)
	}
	link := filepath.Join(t.TempDir(), "symlink")
	if e = os.Symlink(target, link); e != nil {
		t.Fatal(e)
	}
	if release, e := LockPath(link, false); e == nil {
		release()
		t.Fatal("followed lock symlink")
	}
	b, e := os.ReadFile(target)
	if e != nil || string(b) != "sentinel" {
		t.Fatalf("target modified: %s %v", b, e)
	}
}
