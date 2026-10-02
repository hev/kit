package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestBackfillCheckpointReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "checkpoint.json")
	cp := backfillCheckpoint{Namespace: "archive", Since: 123, After: "acknowledged", Processed: 1}
	if err := saveBackfill(path, cp); err != nil {
		t.Fatal(err)
	}
	cp.After = "next"
	cp.Processed++
	if err := saveBackfill(path, cp); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got backfillCheckpoint
	if err = json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got != cp {
		t.Fatalf("checkpoint %+v != %+v", got, cp)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatal("temporary checkpoint leaked")
	}
	st, _ := os.Stat(path)
	if st.Mode().Perm() != 0600 {
		t.Fatal("private checkpoint permissions", st.Mode())
	}
}
