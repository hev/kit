//go:build !windows

package dispatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An order that would queue (the host is at capacity) is still refused at
// once for two mode packs or an unknown skill; a valid one queues.
func TestQueuedOrderSkillsCheckedOnArrival(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	shelfHost(t)
	bin := t.TempDir()
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := os.WriteFile(filepath.Join(bin, "tmux"), []byte("#!/bin/sh\necho d-existing\n"), 0700); err != nil {
		t.Fatal(err)
	}
	order := func(name, skills string) string {
		return "---\nname: " + name + "\nhost: buildbox\nrepo: kit\nissue: TASK-1\nskills: [" + skills + "]\n---\n## Acceptance\n- done\n"
	}
	start := func(name, skills string) (Response, error) {
		return Handle(Request{Action: "start", Name: name, Order: order(name, skills), Meta: Meta{Order: Order{Name: name}}, Cap: 1})
	}
	if _, err := start("two-modes", "steering/drive, rival/flow"); err == nil || !strings.Contains(err.Error(), "at most one") {
		t.Fatalf("two modes on a full host: %v", err)
	}
	if _, err := start("unknown", "kit-tools/missing"); err == nil {
		t.Fatal("unknown skill queued")
	}
	for _, n := range []string{"two-modes", "unknown"} {
		if _, err := os.Stat(queuePath(n)); !os.IsNotExist(err) {
			t.Fatalf("%s left a queue entry: %v", n, err)
		}
	}
	r, err := start("one-mode", "steering/drive, kit-tools/review")
	if err != nil || !strings.Contains(r.Message, "queued one-mode") {
		t.Fatalf("valid order: %+v %v", r, err)
	}
}
