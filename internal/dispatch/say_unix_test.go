//go:build !windows

package dispatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSayLiveAndEnded(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	bin := t.TempDir()
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	tmux := filepath.Join(bin, "tmux")
	if err := os.WriteFile(tmux, []byte("#!/bin/sh\nif [ \"$1\" = list-sessions ]; then echo d-example; fi\n"), 0700); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(Root(), "example")
	if err := os.MkdirAll(filepath.Join(dir, "wt"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := WriteJSON(filepath.Join(dir, "meta.json"), Meta{Order: Order{Name: "example"}}); err != nil {
		t.Fatal(err)
	}
	for _, message := range []string{"first", "second"} {
		if _, err := Handle(Request{Action: "say", Name: "example", Note: message, Cap: 1}); err != nil {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile(filepath.Join(dir, "inbox.md"))
	if err != nil || !strings.Contains(string(b), "first") || !strings.Contains(string(b), "second") || strings.Count(string(b), "## ") != 2 {
		t.Fatalf("%s %v", b, err)
	}
	r, err := Handle(Request{Action: "show", Name: "example"})
	if err != nil || r.Files["inbox.md"] != string(b) {
		t.Fatalf("%+v %v", r, err)
	}
	if err := os.WriteFile(tmux, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "order.md"), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Handle(Request{Action: "say", Name: "example", Note: "ended note", Cap: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "inbox.md")); !os.IsNotExist(err) {
		t.Fatalf("stale inbox remains: %v", err)
	}
	archives, _ := filepath.Glob(filepath.Join(dir, "inbox.md.*"))
	if len(archives) != 1 {
		t.Fatalf("inbox archives: %v", archives)
	}
	archived, _ := os.ReadFile(archives[0])
	if string(archived) != string(b) {
		t.Fatalf("archive changed: %s", archived)
	}
	b, err = os.ReadFile(filepath.Join(dir, "order.md"))
	if err != nil || !strings.Contains(string(b), "## Rework\n\nended note") {
		t.Fatalf("%s %v", b, err)
	}
}

func TestInventoryIncludesUninitializedDirectories(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	bin := t.TempDir()
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	os.WriteFile(filepath.Join(bin, "tmux"), []byte("#!/bin/sh\nexit 0\n"), 0700)
	os.MkdirAll(filepath.Join(Root(), "reserved"), 0700)
	r, err := Handle(Request{Action: "list"})
	if err != nil || len(r.Names) != 1 || r.Names[0] != "reserved" {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestSayRechecksLivenessUnderLock(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	bin := t.TempDir()
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	dir := filepath.Join(Root(), "example")
	os.MkdirAll(filepath.Join(dir, "wt"), 0700)
	WriteJSON(filepath.Join(dir, "meta.json"), Meta{Order: Order{Name: "example"}})
	os.WriteFile(filepath.Join(dir, "order.md"), []byte("original"), 0600)
	// The initial snapshot says live, but the locked snapshot says ended.
	script := "#!/bin/sh\nif [ \"$1\" = list-sessions ] && [ ! -e '" + bin + "/seen' ]; then touch '" + bin + "/seen'; echo d-example; fi\n"
	os.WriteFile(filepath.Join(bin, "tmux"), []byte(script), 0700)
	r, err := Handle(Request{Action: "say", Name: "example", Note: "after exit", Cap: 1})
	if err != nil || r.Message != "started example" {
		t.Fatalf("%+v %v", r, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "inbox.md")); !os.IsNotExist(err) {
		t.Fatal("appended to ended run")
	}
	b, _ := os.ReadFile(filepath.Join(dir, "order.md"))
	if !strings.Contains(string(b), "## Rework\n\nafter exit") {
		t.Fatal(string(b))
	}
}
