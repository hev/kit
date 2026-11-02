//go:build !windows

package dispatch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPeerSendLiveAndEndedNeverReworks(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	bin := t.TempDir()
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	tmux := filepath.Join(bin, "tmux")
	os.WriteFile(tmux, []byte("#!/bin/sh\necho d-example\n"), 0700)
	dir := filepath.Join(Root(), "example")
	os.MkdirAll(filepath.Join(dir, "wt"), 0700)
	WriteJSON(filepath.Join(dir, "meta.json"), Meta{Order: Order{Name: "example"}, Branch: "d/example"})
	original := "original order"
	os.WriteFile(filepath.Join(dir, "order.md"), []byte(original), 0600)
	r, err := Handle(Request{Action: "peer-send", Name: "example", Note: "live message"})
	if err != nil || r.Message != "delivered to run inbox" {
		t.Fatal(r, err)
	}
	os.WriteFile(tmux, []byte("#!/bin/sh\nexit 0\n"), 0700)
	r, err = Handle(Request{Action: "peer-send", Name: "example", Note: "ended message"})
	if err != nil || !strings.Contains(r.Message, "held") {
		t.Fatal(r, err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "order.md"))
	if string(b) != original {
		t.Fatal("peer message started rework")
	}
	b, _ = os.ReadFile(filepath.Join(dir, "inbox.md"))
	if !strings.Contains(string(b), "live message") || !strings.Contains(string(b), "ended message") {
		t.Fatal(string(b))
	}
}

func TestPeerNotificationHasStableRunSender(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sender")
	script := filepath.Join(t.TempDir(), "notify")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s' \"$PEERS_FROM\" > \"$1\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := Notify(context.Background(), []string{script, path}, Meta{Order: Order{Name: "example", Host: "build-host"}}, Result{}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "build-host/example" {
		t.Fatal(string(b), err)
	}
}
