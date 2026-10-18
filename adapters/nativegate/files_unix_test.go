//go:build linux || darwin

package nativegate

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestReceiverCheckedReadRetainsObjectAndRefusesAlias(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "private")
	if e := os.WriteFile(path, []byte("original"), 0600); e != nil {
		t.Fatal(e)
	}
	f, e := openPrivate(path)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	other := filepath.Join(dir, "other")
	if e = os.WriteFile(other, []byte("replacement"), 0600); e != nil {
		t.Fatal(e)
	}
	if e = os.Rename(other, path); e != nil {
		t.Fatal(e)
	}
	b, e := io.ReadAll(f)
	if e != nil || string(b) != "original" {
		t.Fatalf("opened object changed %q %v", b, e)
	}
	alias := filepath.Join(dir, "alias")
	if e = os.Symlink(path, alias); e != nil {
		t.Fatal(e)
	}
	if f, e := openPrivate(alias); e == nil {
		f.Close()
		t.Fatal("alias accepted")
	}
}
func TestReceiverJournalOwnerCheckDistinctFromPrivateConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.sqlite")
	if e := os.WriteFile(path, []byte("fixture"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.Chmod(path, 0644); e != nil {
		t.Fatal(e)
	}
	if e := privateFile(path); e != nil {
		t.Fatal("existing owner journal rejected", e)
	}
	if f, e := openPrivate(path); e == nil {
		f.Close()
		t.Fatal("public-readable config accepted")
	}
}
func TestReceiverPublicationHasSingleImmutableWinner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ready.json")
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); results <- exclusiveJSON(path, map[string]int{"writer": i}) }()
	}
	wg.Wait()
	close(results)
	wins := 0
	for e := range results {
		if e == nil {
			wins++
		} else if !os.IsExist(e) {
			t.Fatal(e)
		}
	}
	if wins != 1 {
		t.Fatalf("winners=%d", wins)
	}
	before, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	var winner map[string]int
	if json.Unmarshal(before, &winner) != nil || len(winner) != 1 {
		t.Fatal("partial publication")
	}
	if e = exclusiveJSON(path, map[string]int{"writer": 99}); !os.IsExist(e) {
		t.Fatalf("replacement result %v", e)
	}
	after, e := os.ReadFile(path)
	if e != nil || string(before) != string(after) {
		t.Fatal("historical ready replaced")
	}
	entries, e := os.ReadDir(filepath.Dir(path))
	if e != nil || len(entries) != 1 {
		t.Fatal("temporary artifacts leaked", e)
	}
}
