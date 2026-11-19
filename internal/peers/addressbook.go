package peers

import (
	"context"
	"net"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/hev/kit/internal/cluster"
	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/hostlabel"
)

// A peers address names its machine by host label: Adams-MacBook-Air/foh-bcc,
// routed as macbook/foh-bcc. Neither name resolves from the other hosts, so
// an SSH send to it failed with "Could not resolve hostname". Before SSH, a
// label that is neither an ssh config alias nor resolvable is looked up in
// the address books this host already has: the dispatch config's hosts and
// the cluster's members, whose addresses are their Tailscale IPs.

// Tests replace the lookups.
var (
	sshConfigHostname = func(ctx context.Context, host string) string {
		ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		b, err := exec.CommandContext(ctx, "ssh", "-G", "--", host).Output()
		if err != nil {
			return ""
		}
		if m := sshConfigHostnameLine.FindSubmatch(b); m != nil {
			return string(m[1])
		}
		return ""
	}
	resolveName = func(ctx context.Context, host string) bool {
		ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		_, err := net.DefaultResolver.LookupHost(ctx, host)
		return err == nil
	}
	addressBookHosts = func() map[string]dispatch.Host {
		cfg, err := dispatch.Load(dispatch.ConfigPath(""))
		if err != nil {
			return nil
		}
		return cfg.Hosts
	}
	addressBookMembers = func(ctx context.Context) []cluster.Member {
		ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		r, err := cluster.Query(ctx, cluster.Request{Action: "ls"})
		if err != nil {
			return nil
		}
		return r.Members
	}
)

var sshConfigHostnameLine = regexp.MustCompile(`(?m)^hostname (\S+)`)

// SSHDestination resolves a peers host label to the destination SSH should
// use. An explicit destination (user@, an IP or a dotted name), an ssh config
// alias and a resolvable name are kept, so ssh config still decides users
// and keys; otherwise the dispatch config's host of that label, then the
// cluster member of that name or peers label. With no entry the label is
// returned and SSH reports it as before.
func SSHDestination(ctx context.Context, label string) string {
	if label == "" || strings.ContainsAny(label, "@.:") || net.ParseIP(label) != nil {
		return label
	}
	if h := sshConfigHostname(ctx, label); h != "" && !strings.EqualFold(h, label) {
		return label
	}
	if resolveName(ctx, label) {
		return label
	}
	hosts := addressBookHosts()
	for _, same := range []func(a, b string) bool{strings.EqualFold, hostlabel.Same} {
		for name, h := range hosts {
			if h.SSH != "" && !h.Local && same(name, label) {
				return h.SSH
			}
		}
	}
	members := addressBookMembers(ctx)
	for _, same := range []func(a, b string) bool{strings.EqualFold, hostlabel.Same} {
		for _, m := range members {
			// A dead member's Tailscale address is still its address.
			if m.Self {
				continue
			}
			if same(m.Name, label) || (m.Peers != "" && same(m.Peers, label)) {
				if ip, _, err := net.SplitHostPort(m.Address); err == nil && net.ParseIP(ip) != nil {
					return ip
				}
			}
		}
	}
	return label
}
