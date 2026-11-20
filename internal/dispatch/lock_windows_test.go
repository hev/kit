package dispatch

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsAdmissionIgnoresStaleLegacyLock(t *testing.T) {
	t.Setenv("USERPROFILE", t.TempDir())
	if err := os.MkdirAll(filepath.Join(Root(), ".lockdir"), 0700); err != nil {
		t.Fatal(err)
	}
	unlock, err := lockRoot()
	if err != nil {
		t.Fatal(err)
	}
	unlock()
	// A kernel lock uses one persistent inode and releases on handle/process exit.
	unlock, err = lockRoot()
	if err != nil {
		t.Fatal(err)
	}
	unlock()
}
