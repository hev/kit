package peers

import (
	"context"
	"net"
	"os/exec"
	"slices"
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
	sshConfigAlias = func(ctx context.Context, host string) bool {
		return sshConfigMatches(ctx, "", host)
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

// sshConfigKeys are the ssh -G settings a Host block uses to say how to
// reach a machine. A block that sets any of them for the label makes it an
// alias, with or without HostName (ProxyCommand tailscale nc %h %p is one).
var sshConfigKeys = map[string]bool{
	"hostname": true, "user": true, "port": true, "identityfile": true, "certificatefile": true,
	"proxycommand": true, "proxyjump": true, "hostkeyalias": true, "identityagent": true, "identitiesonly": true,
}

// sshConfigNameKeys are the settings whose ssh -G value carries the host
// name itself when no block sets them.
var sshConfigNameKeys = map[string]bool{"hostname": true, "hostkeyalias": true, "proxycommand": true}

// sshConfigProbe matches no Host block but the catch-all ones.
const sshConfigProbe = "hev-addressbook-probe"

// sshConfigMatches reports whether ssh config (configFile, or the user's
// when empty) has a block for host beyond what every host gets: ssh -G of
// host and of a name no block matches differ in a reach setting once each
// name is replaced by a placeholder.
func sshConfigMatches(ctx context.Context, configFile, host string) bool {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	settings := func(name string) map[string]string {
		args := []string{"-G"}
		if configFile != "" {
			args = append(args, "-F", configFile)
		}
		b, err := exec.CommandContext(ctx, "ssh", append(args, "--", name)...).Output()
		if err != nil {
			return nil
		}
		out := map[string]string{}
		for _, line := range strings.Split(string(b), "\n") {
			key, value, _ := strings.Cut(strings.TrimSpace(line), " ")
			if !sshConfigKeys[key] {
				continue
			}
			value = strings.ToLower(value)
			// ssh lowercases the host name it prints. Only the settings
			// that default to the name are normalised: a label that is a
			// substring of another default (the user hev, ~/.ssh/id_*) must
			// not make those differ from the probe's.
			if sshConfigNameKeys[key] {
				value = strings.ReplaceAll(value, strings.ToLower(name), "%h")
			}
			out[key] += value + "\n"
		}
		return out
	}
	got, base := settings(host), settings(sshConfigProbe)
	if got == nil || base == nil {
		return false
	}
	for key := range sshConfigKeys {
		if got[key] != base[key] {
			return true
		}
	}
	return false
}

// SSHDestination resolves a peers host label to the destination SSH should
// use. An explicit destination (user@, an IP or a dotted name), an ssh config
// alias and a resolvable name are kept, so ssh config still decides users,
// keys and proxies; otherwise the dispatch config's host of that label or the
// cluster member of that name or peers label. Exact names in either book are
// tried before folded labels (hostlabel.Same) in either. When a step matches
// entries with different addresses (two Macs both fold to macbook), the label is kept
// rather than guessing, and SSH reports it as before; so it does with no
// entry.
func SSHDestination(ctx context.Context, label string) string {
	if label == "" || strings.ContainsAny(label, "@.:") || net.ParseIP(label) != nil {
		return label
	}
	if sshConfigAlias(ctx, label) || resolveName(ctx, label) {
		return label
	}
	hosts := addressBookHosts()
	members := addressBookMembers(ctx)
	// Each step looks at both books together, so a folded host name never
	// outranks an exact member name, and two books holding two addresses for
	// one label is as ambiguous as one book holding two.
	for _, same := range []func(a, b string) bool{strings.EqualFold, hostlabel.Same} {
		var found []string
		for name, h := range hosts {
			if h.SSH != "" && !h.Local && same(name, label) {
				found = appendNew(found, h.SSH)
			}
		}
		for _, m := range members {
			// A dead member's Tailscale address is still its address.
			if m.Self || !(same(m.Name, label) || (m.Peers != "" && same(m.Peers, label))) {
				continue
			}
			if ip, _, err := net.SplitHostPort(m.Address); err == nil && net.ParseIP(ip) != nil {
				found = appendNew(found, ip)
			}
		}
		if dest, done := pick(found, label); done {
			return dest
		}
	}
	return label
}

func appendNew(list []string, s string) []string {
	if slices.Contains(list, s) {
		return list
	}
	return append(list, s)
}

// pick returns the one destination found, or the label when the match is
// ambiguous; done is false when nothing matched.
func pick(found []string, label string) (string, bool) {
	switch len(found) {
	case 0:
		return "", false
	case 1:
		return found[0], true
	default:
		return label, true
	}
}
