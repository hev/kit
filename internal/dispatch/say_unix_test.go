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
	b, err = os.ReadFile(filepath.Join(dir, "order.md"))
	if err != nil || !strings.Contains(string(b), "## Rework\n\nended note") {
		t.Fatalf("%s %v", b, err)
	}
}
