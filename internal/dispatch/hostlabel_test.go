package dispatch

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// notificationHost and peers addresses share one label.
func TestNotificationHostCanonical(t *testing.T) {
	old := hostName
	t.Cleanup(func() { hostName = old })
	t.Setenv("PEERS_SELF", "")
	for raw, want := range map[string]string{"Adams-MacBook-Air.local": "macbook", "Adams-Mac-mini": "mini", "Oven": "oven"} {
		hostName = func() (string, error) { return raw, nil }
		if got := notificationHost(); got != want {
			t.Fatalf("notificationHost(%q) = %q, want %q", raw, got, want)
		}
	}
	t.Setenv("PEERS_SELF", "desk")
	if got := notificationHost(); got != "desk" {
		t.Fatalf("PEERS_SELF ignored: %q", got)
	}
}

// A stored Adams-MacBook-Air/X reply is delivered to macbook/X.
func TestNotifyDeliversLegacyReplyToMacbook(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell recorder")
	}
	bin := t.TempDir()
	log := filepath.Join(bin, "to")
	if err := os.WriteFile(filepath.Join(bin, "recorder"), []byte("#!/bin/sh\nprintf '%s' \"$1\" > '"+log+"'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	m := Meta{Order: Order{Name: "example", Reply: "Adams-MacBook-Air/foh-hev-6"}}
	if err := Notify(context.Background(), []string{filepath.Join(bin, "recorder"), "{reply}"}, m, Result{End: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(log); err != nil || string(b) != "macbook/foh-hev-6" {
		t.Fatalf("delivered to %q: %v", b, err)
	}
}

// A follow-through or "mine" orchestrator started under either laptop label
// follows runs ordered under the other.
func TestMineSelectorMatchesLegacyLaptopReply(t *testing.T) {
	var s OrchestratorSpec
	s.Manages.Runs.Selector = "mine"
	for _, pair := range [][2]string{{"macbook/foh-hev-6", "Adams-MacBook-Air/foh-hev-6"}, {"Adams-MacBook-Air/foh-hev-6", "macbook/foh-hev-6"}} {
		s.Reply = pair[0]
		if !s.Includes(Order{Name: "run", Reply: pair[1]}) {
			t.Fatalf("%s did not include %s", pair[0], pair[1])
		}
	}
	s.Reply = "macbook/foh-hev-6"
	if s.Includes(Order{Name: "run", Reply: "mini/foh-hev-6"}) || s.Includes(Order{Name: "run", Reply: "macbook/foh-hev-7"}) {
		t.Fatal("matched another participant")
	}
}
