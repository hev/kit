//go:build linux || darwin

package nativejob

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestReadOnlyOriginalNeverEnrollsMissingOrDifferentJournal(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	enrollment := Enrollment{HostID: "host", ExecutorID: "executor"}
	if _, e := ReadOnlyOriginal(context.Background(), root, enrollment, ""); e == nil {
		t.Fatal("missing journal accepted")
	}
	if _, e := os.Stat(root); !os.IsNotExist(e) {
		t.Fatal("read created journal")
	}
	s, e := Open(root, enrollment)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if _, e := ReadOnlyOriginal(context.Background(), root, enrollment, ""); e != nil {
		t.Fatal(e)
	}
	if _, e := ReadOnlyOriginal(context.Background(), root, Enrollment{HostID: "foreign", ExecutorID: "executor"}, ""); e == nil {
		t.Fatal("different enrollment accepted")
	}
	var count int
	if e = s.db.QueryRow(`SELECT count(*) FROM jobs`).Scan(&count); e != nil || count != 0 {
		t.Fatal("read created job", count, e)
	}
}
