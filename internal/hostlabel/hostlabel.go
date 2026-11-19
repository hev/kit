// Package hostlabel names this machine the way every peers address, dispatch
// notification and follow-through match does, so a reply address written on one
// host resolves on the others.
package hostlabel

import (
	"os"
	"strings"
)

// Canonical is the peers label for a hostname: its first DNS label, lower
// case, with Adam's Macs folded to the names other hosts know them by
// (anything containing "mac-mini" is "mini", "macbook" is "macbook").
func Canonical(host string) string {
	host = strings.ToLower(strings.SplitN(strings.TrimSpace(host), ".", 2)[0])
	switch {
	case strings.Contains(host, "mac-mini"):
		return "mini"
	case strings.Contains(host, "macbook"):
		return "macbook"
	}
	return host
}

// Self is this machine's peers label: PEERS_SELF verbatim when set, otherwise
// the canonical form of hostname().
func Self(hostname func() (string, error)) string {
	if s := os.Getenv("PEERS_SELF"); s != "" {
		return s
	}
	h, _ := hostname()
	return Canonical(h)
}

// Same reports whether two host labels name the same machine.
func Same(a, b string) bool {
	return strings.EqualFold(a, b) || Canonical(a) == Canonical(b)
}

// Route is the host to deliver to for a label taken from an address. Only
// the Mac aliases are rewritten (Adams-MacBook-Air becomes macbook); any
// other label is kept as written so configured transports still match it.
func Route(host string) string {
	if c := Canonical(host); c == "macbook" || c == "mini" {
		return c
	}
	return host
}

// Address rewrites the host part of a HOST/NAME peers address with Route,
// so a stored reply of Adams-MacBook-Air/foh-1 (any case) is macbook/foh-1.
// An address without a host is returned unchanged.
func Address(addr string) string {
	h, n, ok := strings.Cut(addr, "/")
	if !ok || h == "" {
		return addr
	}
	return Route(h) + "/" + n
}

// SameAddress reports whether two peers addresses name the same participant,
// treating Mac host aliases as one host.
func SameAddress(a, b string) bool {
	return a == b || Address(a) == Address(b)
}
