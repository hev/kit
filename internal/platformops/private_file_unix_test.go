//go:build !windows

package platformops

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestReadPrivateFileRefusesUnsafeUnixObjects(t *testing.T) {
	temp, err := os.MkdirTemp("/tmp", "hev-private-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(temp) })
	dir, err := filepath.EvalSymlinks(temp)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "credential")
	if err := WriteFileAtomic(path, []byte("fixture-private"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadPrivateFile(path, 64); err != nil || string(got) != "fixture-private" {
		t.Fatal("private file refused")
	}
	if _, err := ReadPrivateFile(path, 4); err == nil {
		t.Fatal("oversized input accepted")
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPrivateFile(link, 64); err == nil {
		t.Fatal("symlink accepted")
	}
	fifo := filepath.Join(dir, "fifo")
	if err := unix.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPrivateFile(fifo, 64); err == nil {
		t.Fatal("FIFO accepted")
	}
	if err := os.Chmod(path, 0640); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPrivateFile(path, 64); err == nil {
		t.Fatal("group-readable file accepted")
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0777); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPrivateFile(path, 64); err == nil {
		t.Fatal("replaceable ancestor accepted")
	}
}
