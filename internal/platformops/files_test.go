package platformops

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicStateReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state")
	for _, text := range []string{"old", "new"} {
		if err := WriteFileAtomic(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != text {
			t.Fatalf("read %q: %v", got, err)
		}
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary files leaked: %v %v", entries, err)
	}
}
