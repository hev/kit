//go:build linux || darwin || freebsd || openbsd || netbsd

package maintenance

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestSnapshotHandleRejectsFIFOAndSymlinkWithoutBlockingOrCreation(t *testing.T) {
	dir := canonicalTempDir(t)
	fifo := filepath.Join(dir, "state.json")
	must(t, syscall.Mkfifo(fifo, 0600))
	done := make(chan error, 1)
	go func() {
		f, e := openRegularSnapshot(fifo)
		if f != nil {
			f.Close()
		}
		done <- e
	}()
	select {
	case e := <-done:
		if e == nil {
			t.Fatal("accepted FIFO")
		}
	case <-time.After(time.Second):
		t.Fatal("blocked on FIFO")
	}
	_, e := (Store{Dir: dir}).ReadSnapshot()
	if e == nil {
		t.Fatal("accepted FIFO snapshot")
	}
	if _, e = os.Stat(filepath.Join(dir, "lock")); !os.IsNotExist(e) {
		t.Fatal("read created writer lock")
	}
	target := filepath.Join(dir, "regular")
	must(t, os.WriteFile(target, []byte("{}"), 0600))
	link := filepath.Join(dir, "link")
	must(t, os.Symlink(target, link))
	f, e := openRegularSnapshot(link)
	if f != nil {
		f.Close()
	}
	if e == nil {
		t.Fatal("followed swapped snapshot symlink")
	}
}
