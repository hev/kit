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
	oldSSH, oldResolve, oldHosts, oldMembers := sshConfigHostname, resolveName, addressBookHosts, addressBookMembers
	t.Cleanup(func() {
		sshConfigHostname, resolveName, addressBookHosts, addressBookMembers = oldSSH, oldResolve, oldHosts, oldMembers
	})
	sshConfigHostname = func(_ context.Context, host string) string {
		if host == "mini" {
			return "adams-mac-mini"
		}
		return host // no ssh config entry: ssh -G echoes the name
	}
	resolveName = func(_ context.Context, host string) bool { return host == "oven-dns" }
	addressBookHosts = func() map[string]dispatch.Host {
		return map[string]dispatch.Host{"oven": {Local: true}, "win-oven": {SSH: "hev@100.107.229.73"}}
	}
	addressBookMembers = func(context.Context) []cluster.Member {
		laptop := cluster.Member{Facts: cluster.Facts{Name: "laptop", Peers: "macbook"}, Address: "100.66.207.41:7946", State: "alive"}
		mini := cluster.Member{Facts: cluster.Facts{Name: "mini", Peers: "mini"}, Address: "100.126.12.95:7946", State: "alive"}
		self := cluster.Member{Facts: cluster.Facts{Name: "oven", Peers: "oven"}, Address: "100.110.73.23:7946", State: "alive", Self: true}
		return []cluster.Member{self, mini, laptop}
	}
}

func TestOwnerResolvesThroughAddressBook(t *testing.T) {
	fakeAddressBook(t)
	ctx := context.Background()
	for label, want := range map[string]string{
		// The owner's laptop: its peers label in the cluster, not the bare name.
		"macbook":           "100.66.207.41",
		"Adams-MacBook-Air": "100.66.207.41",
		"adams-macbook-air": "100.66.207.41",
		// The dispatch config's host.
		"win-oven": "hev@100.107.229.73",
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
