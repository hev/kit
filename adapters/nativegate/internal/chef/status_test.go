package chef

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestReadStatusDoesNotInitializeAuthority(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "missing", "authority")
	if _, err := (Store{Dir: dir}).ReadStatus(); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatal("status created files", entries, err)
	}
	s := Store{Dir: t.TempDir()}
	if _, err := s.Acquire("owner", time.Minute); err != nil {
		t.Fatal(err)
	}
	expected, err := s.Status()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(filepath.Join(s.Dir, "lock")); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(s.Dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	actual, err := s.ReadStatus()
	if err != nil || !reflect.DeepEqual(expected, actual) {
		t.Fatal(actual, err)
	}
	after, err := os.ReadFile(filepath.Join(s.Dir, "state.json"))
	if err != nil || string(before) != string(after) {
		t.Fatal("status changed journal", err)
	}
	if _, err = os.Stat(filepath.Join(s.Dir, "lock")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("status recreated lock", err)
	}
}
