package cluster

import (
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/memberlist"
)

func ip(n int) *int { return &n }

// preActivityFacts is Facts as builds before activity declared it.
type preActivityFacts struct {
	Encoding int       `json:"encoding"`
	Name     string    `json:"name"`
	Version  string    `json:"version"`
	Channel  string    `json:"channel"`
	OS       string    `json:"os"`
	Arch     string    `json:"arch"`
	Started  time.Time `json:"started"`
	Dispatch bool      `json:"dispatch"`
	Cap      int       `json:"cap"`
}

func TestActivityEncodeDecodeAndBound(t *testing.T) {
	at := time.Date(2026, 11, 17, 17, 0, 0, 0, time.UTC)
	f := LocalFacts("a-very-long-member-name-for-bounds", true, 65536)
	f.Version = strings.Repeat("v", 64)
	f.Channel, f.OS, f.Arch = strings.Repeat("c", 16), strings.Repeat("o", 16), strings.Repeat("a", 16)
	f.Activity = &Activity{AgentsWorking: ip(maxActivityCount), AgentsIdle: ip(maxActivityCount), HeadlessRuns: ip(maxActivityCount), Expediters: ip(maxActivityCount), At: at}
	b, _ := json.Marshal(f)
	if len(b) > memberlist.MetaMaxSize {
		t.Fatalf("worst-case metadata %d bytes exceeds %d", len(b), memberlist.MetaMaxSize)
	}
	var back Facts
	if e := json.Unmarshal(b, &back); e != nil || back.Activity == nil || *back.Activity.Expediters != maxActivityCount || !back.Activity.At.Equal(at) {
		t.Fatalf("round trip: %v %+v", e, back.Activity)
	}
	// A Windows node knows runs and Expediters only: agent keys are absent, not 0.
	win, _ := json.Marshal(Activity{HeadlessRuns: ip(0), Expediters: ip(1), At: at})
	if string(win) != `{"headless_runs":0,"expediters":1,"at":"2026-11-17T17:00:00Z"}` {
		t.Fatalf("windows shape %s", win)
	}
	full, _ := json.Marshal(Activity{AgentsWorking: ip(2), AgentsIdle: ip(1), HeadlessRuns: ip(3), Expediters: ip(0), At: at})
	if string(full) != `{"agents_working":2,"agents_idle":1,"headless_runs":3,"expediters":0,"at":"2026-11-17T17:00:00Z"}` {
		t.Fatalf("full shape %s", full)
	}
}

func TestActivityOldPeerCompatibility(t *testing.T) {
	n := node(t, "alpha", nil, nil)
	at := time.Now().UTC().Truncate(time.Second)
	newer := LocalFacts("bravo", true, 2)
	newer.Name = "bravo"
	newer.Activity = &Activity{HeadlessRuns: ip(1), Expediters: ip(0), At: at}
	newMeta, _ := json.Marshal(newer)
	// An older build decodes and validates new metadata exactly as before.
	var old preActivityFacts
	if e := json.Unmarshal(newMeta, &old); e != nil || old.Encoding != 1 || old.Name != "bravo" || old.Cap != 2 {
		t.Fatalf("old decoder: %v %+v", e, old)
	}
	// This build accepts an older peer's metadata and shows no activity.
	oldMeta, _ := json.Marshal(preActivityFacts{Encoding: 1, Name: "charlie", Version: "v0.7.0", Channel: "release", OS: "linux", Arch: "amd64", Started: at})
	for _, p := range []*memberlist.Node{
		{Name: "bravo", Addr: net.ParseIP("127.0.0.2"), Port: 7946, Meta: newMeta},
		{Name: "charlie", Addr: net.ParseIP("127.0.0.3"), Port: 7946, Meta: oldMeta},
	} {
		if e := n.NotifyAlive(p); e != nil {
			t.Fatalf("%s rejected: %v", p.Name, e)
		}
		n.NotifyJoin(p)
	}
	// Out-of-range counts drop the summary, never the member.
	bad := newer
	bad.Name = "delta"
	bad.Activity = &Activity{HeadlessRuns: ip(-1), At: at}
	badMeta, _ := json.Marshal(bad)
	dp := &memberlist.Node{Name: "delta", Addr: net.ParseIP("127.0.0.4"), Port: 7946, Meta: badMeta}
	if e := n.NotifyAlive(dp); e != nil {
		t.Fatal(e)
	}
	n.NotifyJoin(dp)
	seen := map[string]Member{}
	for _, m := range n.Members() {
		seen[m.Name] = m
	}
	if a := seen["bravo"].Activity; a == nil || *a.HeadlessRuns != 1 || a.AgentsWorking != nil || a.Stale {
		t.Fatalf("bravo activity %+v", a)
	}
	if seen["charlie"].Activity != nil || seen["charlie"].State != "alive" {
		t.Fatalf("charlie %+v", seen["charlie"])
	}
	if seen["delta"].Activity != nil || seen["delta"].State != "alive" {
		t.Fatalf("delta %+v", seen["delta"])
	}
	out, _ := json.Marshal(seen["charlie"])
	if strings.Contains(string(out), "activity") || strings.Contains(string(out), "self") {
		t.Fatalf("old member JSON carries new keys: %s", out)
	}
}

func TestActivityStaleShownNotDropped(t *testing.T) {
	n := node(t, "alpha", nil, nil)
	old := time.Now().Add(-3 * time.Minute)
	if e := n.SetActivity(Activity{HeadlessRuns: ip(2), At: old}); e != nil {
		t.Fatal(e)
	}
	var self Member
	for _, m := range n.Members() {
		if m.Self {
			self = m
		}
	}
	if self.Name != "alpha" || self.Activity == nil || !self.Activity.Stale || *self.Activity.HeadlessRuns != 2 {
		t.Fatalf("%+v", self)
	}
	if got := self.Activity.Short(); got != "2r (stale)" {
		t.Fatalf("short %q", got)
	}
	// Stale is the reader's judgement; it is never published.
	if strings.Contains(string(n.NodeMeta(memberlist.MetaMaxSize)), "stale") {
		t.Fatal("stale gossiped")
	}
	if e := n.SetActivity(Activity{}); e == nil {
		t.Fatal("zero time accepted")
	}
}

func TestActivityShort(t *testing.T) {
	at := time.Now()
	for _, c := range []struct {
		a    *Activity
		want string
	}{
		{nil, "-"},
		{&Activity{At: at}, "-"},
		{&Activity{AgentsWorking: ip(2), AgentsIdle: ip(1), HeadlessRuns: ip(3), Expediters: ip(1), At: at}, "2w/1i 3r 1x"},
		{&Activity{HeadlessRuns: ip(0), Expediters: ip(0), At: at}, "0r 0x"},
	} {
		if got := c.a.Short(); got != c.want {
			t.Fatalf("%+v: %q want %q", c.a, got, c.want)
		}
	}
}

// Gossip carries each node's activity to every member; each node marks
// exactly one member, itself, as self.
func TestActivityGossipAndSelf(t *testing.T) {
	a := node(t, "alpha", nil, nil)
	b := node(t, "bravo", []string{a.Address()}, nil)
	c := node(t, "charlie", []string{a.Address()}, nil)
	ns := []*Node{a, b, c}
	eventually(t, 5*time.Second, "three alive", func() bool { return allAlive(ns, 3) })
	at := time.Now().UTC().Truncate(time.Second)
	if e := a.SetActivity(Activity{AgentsWorking: ip(2), AgentsIdle: ip(1), HeadlessRuns: ip(3), Expediters: ip(1), At: at}); e != nil {
		t.Fatal(e)
	}
	if e := b.SetActivity(Activity{HeadlessRuns: ip(0), Expediters: ip(2), At: at}); e != nil {
		t.Fatal(e)
	}
	eventually(t, 5*time.Second, "activity on every member", func() bool {
		for _, n := range ns {
			for _, m := range n.Members() {
				switch m.Name {
				case "alpha":
					if m.Activity == nil || m.Activity.Short() != "2w/1i 3r 1x" {
						return false
					}
				case "bravo":
					if m.Activity == nil || m.Activity.Short() != "0r 2x" {
						return false
					}
				case "charlie":
					if m.Activity != nil {
						return false
					}
				}
			}
		}
		return true
	})
	for _, n := range ns {
		selves := []string{}
		for _, m := range n.Members() {
			if m.Self {
				selves = append(selves, m.Name)
			}
		}
		if len(selves) != 1 || selves[0] != n.cfg.Name {
			t.Fatalf("%s sees self %v", n.cfg.Name, selves)
		}
	}
	// A node that has left does not re-advertise itself alive.
	if e := c.Leave(); e != nil {
		t.Fatal(e)
	}
	if e := c.SetActivity(Activity{HeadlessRuns: ip(1), At: at}); e == nil {
		t.Fatal("left node published")
	}
}
