package peers

import (
	"os"
	"strings"
	"testing"

	"github.com/hev/kit/internal/hostlabel"
)

// Orders written on the laptop before the canonical label carry
// Adams-MacBook-Air/NAME. Other hosts deliver them to macbook, the name they
// resolve, never to the raw hostname ssh cannot.
func TestLegacyLaptopReplyRoutesToMacbook(t *testing.T) {
	e := &Engine{Host: "mini"}
	for _, address := range []string{"Adams-MacBook-Air/foh-hev-6", "adams-macbook-air/foh-hev-6", "ADAMS-MACBOOK-AIR.local/foh-hev-6", "macbook/foh-hev-6"} {
		if host, target := e.split(address); host != "macbook" || target != "foh-hev-6" {
			t.Fatalf("split(%q) = %q, %q", address, host, target)
		}
	}
	if host, _ := e.split("Oven/foh-1"); host != "Oven" {
		t.Fatalf("non-Mac label rewritten: %q", host)
	}
	laptop := &Engine{Host: "macbook"}
	if host, target := laptop.split("Adams-MacBook-Air/foh-hev-6"); host != "" || target != "foh-hev-6" {
		t.Fatalf("laptop did not keep its own legacy address local: %q %q", host, target)
	}
}

// New names this host with the shared label, so OwnAddress and peer
// addresses agree with dispatch notifications. Recipient incarnations keep
// the label they were always hashed with, so a live pane's inbox survives
// the upgrade.
func TestNewUsesSharedHostLabel(t *testing.T) {
	t.Setenv("PEERS_SELF", "")
	e := New()
	if want := hostlabel.Self(os.Hostname); e.Host != want {
		t.Fatalf("Host = %q, want %q", e.Host, want)
	}
	raw, _ := os.Hostname()
	if want := strings.SplitN(raw, ".", 2)[0]; e.incarnation() != want {
		t.Fatalf("incarnation host = %q, want %q", e.incarnation(), want)
	}
	t.Setenv("PEERS_SELF", "desk")
	if e := New(); e.Host != "desk" || e.incarnation() != "desk" {
		t.Fatalf("PEERS_SELF ignored: %q %q", e.Host, e.incarnation())
	}
	if (&Engine{Host: "x"}).incarnation() != "x" {
		t.Fatal("bare engine incarnation")
	}
}
