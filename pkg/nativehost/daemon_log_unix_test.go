//go:build linux || darwin

package nativehost

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDaemonLogExclusivePrivateNoRepair(t *testing.T) {
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	_ = os.Chmod(root, 0700)
	first, e := OpenDaemonLog(root)
	if e != nil {
		t.Fatal(e)
	}
	defer first.Close()
	if _, e = first.WriteString("original\n"); e != nil {
		t.Fatal(e)
	}
	second, e := OpenDaemonLog(root)
	if e != nil {
		t.Fatal(e)
	}
	defer second.Close()
	if first.Name() == second.Name() {
		t.Fatal("reused log")
	}
	b, e := os.ReadFile(first.Name())
	if e != nil || string(b) != "original\n" {
		t.Fatal("old log changed")
	}
	s, _ := first.Stat()
	if s.Mode().Perm() != 0600 {
		t.Fatal(s.Mode())
	}
	unsafe := filepath.Join(root, "unsafe")
	_ = os.Mkdir(unsafe, 0755)
	if f, e := OpenDaemonLog(unsafe); e == nil {
		f.Close()
		t.Fatal("unsafe root accepted")
	}
	entries, _ := os.ReadDir(unsafe)
	if len(entries) != 0 {
		t.Fatal("unsafe root mutated")
	}
}
