//go:build linux || darwin

package nativejob

import (
	"os"
	"path/filepath"
	"testing"
)

func TestJournalRejectsExistingDatabaseLinksBeforeSQLiteMutation(t *testing.T) {
	for _, name := range []string{"journal.sqlite", "journal.sqlite-wal", "journal.sqlite-shm"} {
		t.Run(name, func(t *testing.T) {
			parent := t.TempDir()
			root := filepath.Join(parent, "journal")
			if err := journalPrepareRoot(root); err != nil {
				t.Fatal(err)
			}
			victim := filepath.Join(parent, "unrelated")
			if err := os.WriteFile(victim, []byte("preserve evidence"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(victim, filepath.Join(root, name)); err != nil {
				t.Fatal(err)
			}
			if s, err := Open(root, testEnrollment); err == nil {
				s.Close()
				t.Fatal("journal accepted linked file", name)
			}
			got, err := os.ReadFile(victim)
			if err != nil || string(got) != "preserve evidence" {
				t.Fatal("SQLite mutated external target", string(got), err)
			}
			if name != "journal.sqlite" {
				if _, err := os.Stat(filepath.Join(root, "journal.sqlite")); !os.IsNotExist(err) {
					t.Fatal("SQLite ran before sidecar validation", err)
				}
			}
		})
	}
}

func TestJournalFilesystemUnixPrivacyAndBarrierErrors(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "journal")
	if err := journalPrepareRoot(root); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0755); err != nil {
		t.Fatal(err)
	}
	if err := journalPrepareRoot(root); err == nil {
		t.Fatal("permissive root accepted")
	}
	st, err := os.Stat(root)
	if err != nil || st.Mode().Perm() != 0755 {
		t.Fatal("existing permissions silently changed", err)
	}
	if err := journalSyncDir(filepath.Join(parent, "absent")); err == nil {
		t.Fatal("missing metadata barrier accepted")
	}
	path := filepath.Join(parent, "output")
	if err := journalCreateOutputDir(path); err != nil {
		t.Fatal(err)
	}
	f, err := journalCreateOutputFile(filepath.Join(path, "stdout"))
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	if f, err = journalCreateOutputFile(filepath.Join(path, "stdout")); err == nil {
		f.Close()
		t.Fatal("output truncation accepted")
	}
	st, err = os.Stat(filepath.Join(path, "stdout"))
	if err != nil || st.Mode().Perm() != 0600 {
		t.Fatal("artifact mode changed", err)
	}
}
