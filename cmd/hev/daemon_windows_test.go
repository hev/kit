//go:build windows

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hev/kit/internal/platformops"
)

func TestCopyFileWindowsDestinationLock(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "source"), filepath.Join(dir, "hev.exe")
	for path, content := range map[string]string{src: "new binary", dst: "old binary"} {
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// os.Open does not share deletion on Windows, so replacement must fail
	// without truncating the installed file while this handle is open.
	locked, err := os.Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { locked.Close() })
	if err := copyFile(src, dst); err == nil {
		t.Fatal("replaced a locked destination")
	}
	assertContent := func(want string) {
		t.Helper()
		got, err := os.ReadFile(dst)
		if err != nil || string(got) != want {
			t.Fatalf("destination = %q, err = %v; want %q", got, err, want)
		}
		leftovers, err := filepath.Glob(filepath.Join(dir, ".hev-install-*"))
		if err != nil || len(leftovers) != 0 {
			t.Fatalf("temporary files = %v, err = %v", leftovers, err)
		}
	}
	assertContent("old binary")
	if err := locked.Close(); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("replace closed destination: %v", err)
	}
	assertContent("new binary")
}

func TestWindowsDaemonControlMissingPublication(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon.pid")
	testMissingDaemonPublication(t, func() (daemonControl, error) {
		return daemonControlResult(platformops.OpenDaemon(path))
	})
}
