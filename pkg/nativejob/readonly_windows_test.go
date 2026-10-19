//go:build windows

package nativejob

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	fs "github.com/hev/kit/pkg/nativefs/windowsfs"
)

func readonlyPrivateFile(t *testing.T, path string) {
	t.Helper()
	f, e := fs.CreatePrivateFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if e = errors.Join(f.Sync(), f.Close(), fs.SyncDirectory(filepath.Dir(path))); e != nil {
		t.Fatal(e)
	}
}
func TestWindowsJournalReadOnlyLeaseNeverCreatesAndDeniesReplacement(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
	if e := fs.CreatePrivateDir(root, false); e != nil {
		t.Fatal(e)
	}
	db := filepath.Join(root, "journal.sqlite")
	readonlyPrivateFile(t, db)
	if release, e := journalOpenReadOnly(root); e == nil || release != nil {
		t.Fatal("missing sidecars accepted")
	}
	for _, name := range []string{"journal.sqlite-wal", "journal.sqlite-shm"} {
		if _, e := os.Stat(filepath.Join(root, name)); !os.IsNotExist(e) {
			t.Fatal("readonly created sidecar", name, e)
		}
		readonlyPrivateFile(t, filepath.Join(root, name))
	}
	release, e := journalOpenReadOnly(root)
	if e != nil {
		t.Fatal(e)
	}
	if f, e := fs.OpenExisting(db, true); e == nil {
		f.Close()
		t.Fatal("writer opened held readonly snapshot")
	}
	if e = os.Rename(db, db+".moved"); e == nil {
		t.Fatal("held readonly original replaced")
	}
	if e = release(); e != nil {
		t.Fatal(e)
	}
	if e = release(); e != nil {
		t.Fatal("close not idempotent", e)
	}
	f, e := fs.OpenExisting(db, true)
	if e != nil {
		t.Fatal("own snapshot lease not released", e)
	}
	if e = f.Close(); e != nil {
		t.Fatal(e)
	}
}
func TestWindowsJournalReadOnlyIdleClosedStore(t *testing.T) {
	root := filepath.Join(t.TempDir(), "journal")
	if journalPlatformPreflight() != nil {
		if _, e := ReadOnlyIdle(context.Background(), root, windowsProofEnrollment); e == nil {
			t.Fatal("unsupported read-only platform accepted")
		}
		if _, e := os.Stat(root); !os.IsNotExist(e) {
			t.Fatal("unsupported observer created root")
		}
		t.Skip("positive idle-journal proof requires actual supported Windows26300")
	}
	s, e := Open(root, windowsProofEnrollment)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"journal.sqlite", "journal.sqlite-wal", "journal.sqlite-shm"} {
		info, e := os.Stat(filepath.Join(root, name))
		if e != nil {
			t.Log("closed-store artifact", name, e)
		} else {
			t.Log("closed-store artifact", name, info.Size())
		}
	}
	proof, e := ReadOnlyIdle(context.Background(), root, windowsProofEnrollment)
	if e != nil || proof.Version != IdleProofVersion || proof.JobCount != 0 || proof.CompletedACKCount != 0 {
		t.Fatal("actual closed idle journal proof", proof, e)
	}
	if _, e = ReadOnlyIdle(context.Background(), root, Enrollment{HostID: "foreign", ExecutorID: "other"}); e == nil {
		t.Fatal("foreign enrollment accepted")
	}
}
