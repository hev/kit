package peers

import (
	"context"
	"testing"

	"github.com/hev/kit/internal/cluster"
	"github.com/hev/kit/internal/dispatch"
)

// fakeAddressBook is oven's view on 2026-11-18: ssh config has a mini alias,
// the dispatch config names win-oven, and the cluster has the laptop at its
// Tailscale address. Nothing resolves macbook or adams-macbook-air.
func fakeAddressBook(t *testing.T) {
	t.Helper()
	oldSSH, oldResolve, oldHosts, oldMembers := sshConfigAlias, resolveName, addressBookHosts, addressBookMembers
	t.Cleanup(func() {
		sshConfigAlias, resolveName, addressBookHosts, addressBookMembers = oldSSH, oldResolve, oldHosts, oldMembers
	})
	sshConfigAlias = func(_ context.Context, host string) bool { return host == "mini" }
	resolveName = func(_ context.Context, host string) bool { return host == "oven-dns" }
	addressBookHosts = func() map[string]dispatch.Host {
		return map[string]dispatch.Host{"oven": {Local: true}, "win-oven": {SSH: "hev@192.0.2.40"}}
	}
	addressBookMembers = func(context.Context) []cluster.Member {
		laptop := cluster.Member{Facts: cluster.Facts{Name: "laptop", Peers: "macbook"}, Address: "192.0.2.10:7946", State: "alive"}
		mini := cluster.Member{Facts: cluster.Facts{Name: "mini", Peers: "mini"}, Address: "192.0.2.20:7946", State: "alive"}
		self := cluster.Member{Facts: cluster.Facts{Name: "oven", Peers: "oven"}, Address: "192.0.2.30:7946", State: "alive", Self: true}
		return []cluster.Member{self, mini, laptop}
	}
}

func TestOwnerResolvesThroughAddressBook(t *testing.T) {
	fakeAddressBook(t)
	ctx := context.Background()
	for label, want := range map[string]string{
		// The owner's laptop: its peers label in the cluster, not the bare name.
		"macbook":           "192.0.2.10",
		"Adams-MacBook-Air": "192.0.2.10",
		"adams-macbook-air": "192.0.2.10",
		// The dispatch config's host.
		"win-oven": "hev@192.0.2.40",
		// An ssh config alias and a resolvable name stay as they are, so ssh
		// config still chooses user and key.
		"mini":     "mini",
		"oven-dns": "oven-dns",
		// Explicit destinations are never rewritten.
		"hev@10.0.0.1": "hev@10.0.0.1",
		"100.1.2.3":    "100.1.2.3",
		"box.ts.net":   "box.ts.net",
		// No entry: the label, and ssh reports it as before.
		"nowhere": "nowhere",
	} {
		if got := SSHDestination(ctx, label); got != want {
			t.Errorf("SSHDestination(%q) = %q, want %q", label, got, want)
		}
	}
}

// Labels that match more than one address book entry with different
// addresses keep the bare label instead of picking one: dispatch hosts are a
// map, so a pick would change from call to call, and list order would favour
// a dead member (review finding 4).
func TestAddressBookAmbiguityKeepsLabel(t *testing.T) {
	fakeAddressBook(t)
	ctx := context.Background()
	hosts := map[string]dispatch.Host{"work-macbook": {SSH: "a@192.0.2.1"}, "studio-macbook-pro": {SSH: "b@192.0.2.2"}, "box": {SSH: "c@192.0.2.3"}}
	members := []cluster.Member{
		{Facts: cluster.Facts{Name: "old-laptop", Peers: "macbook"}, Address: "192.0.2.11:7946", State: "dead"},
		{Facts: cluster.Facts{Name: "laptop", Peers: "macbook"}, Address: "192.0.2.10:7946", State: "alive"},
		{Facts: cluster.Facts{Name: "mini-a", Peers: "mini"}, Address: "192.0.2.20:7946", State: "alive"},
		{Facts: cluster.Facts{Name: "mini-b", Peers: "mini"}, Address: "192.0.2.20:7946", State: "alive"},
	}
	for _, c := range []struct {
		hosts       bool
		label, want string
	}{
		// An exact host name is not ambiguous.
		{true, "work-macbook", "a@192.0.2.1"},
		{true, "box", "c@192.0.2.3"},
		// Folded, it matches both Macs in the dispatch config.
		{true, "Adams-MacBook-Air", "Adams-MacBook-Air"},
		// Two members, one dead, both peers=macbook: refused.
		{false, "Adams-MacBook-Air", "Adams-MacBook-Air"},
		{false, "laptop", "192.0.2.10"},
		// Two entries with the same address are one machine.
		{false, "mac-mini", "192.0.2.20"},
	} {
		addressBookHosts = func() map[string]dispatch.Host {
			if c.hosts {
				return hosts
			}
			return nil
		}
		addressBookMembers = func(context.Context) []cluster.Member { return members }
		for i := 0; i < 20; i++ {
			if got := SSHDestination(ctx, c.label); got != c.want {
				t.Fatalf("hosts=%v SSHDestination(%q) = %q, want %q", c.hosts, c.label, got, c.want)
			}
		}
	}
}
