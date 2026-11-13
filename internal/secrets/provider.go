package secrets

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Provider is one secret store the cache resolves through. The reference's
// scheme (the text before "://") picks the provider. A provider makes real
// requests only from Read, and only when the cache asks: the cache owns the
// memory store, TTLs, miss coalescing, cooldowns, counters and the read log,
// so every provider gets the same rules (RFC 0009).
//
// A third provider is one new file: a type implementing Provider and an init
// that calls register with its scheme, its reference validator and its
// constructor.
type Provider interface {
	// Scheme is the reference scheme without "://", e.g. "op" or "azkv".
	Scheme() string
	// Name is the human name used in errors and status ("1Password").
	Name() string
	// Kind names a batch read in the read log ("inject", "get").
	Kind() string
	// Credential reports, without any request, which credential type a read
	// would use (a type label, never a value), or an error naming what to set.
	// Interactive sign-in is never an option.
	Credential() (string, error)
	// Fingerprint identifies the configured credential for endpoint account
	// matching: a SHA-256 hex digest, or "" when the provider is not
	// configured. It never contains a value.
	Fingerprint() string
	// Read makes the real requests for unique, validated references, one
	// attempt each and no retry, and returns their values in order with the
	// number of real requests made. A rate limit is a *RateLimit; any other
	// error is sanitized: no value, token, reference or raw upstream output.
	Read(ctx context.Context, uris []string) ([]string, int, error)
}

// RateLimit is a provider's rate-limit answer. The cache turns it into a
// cooldown for that provider only.
type RateLimit struct {
	// Class is "daily rate limit", "hourly rate limit" or "rate limit".
	Class string
	// Reset is an explicit reset time found in the answer, if any.
	Reset time.Time
	// RetryIn is a "try again in N" hint found in the answer, if any.
	RetryIn time.Duration
	// After is an authoritative protocol delay (an HTTP Retry-After). When
	// set it is honoured as given, bounded to 1s..24h.
	After time.Duration
}

func (r *RateLimit) Error() string { return r.Class }

type providerSpec struct {
	// order sorts providers in status and in a mixed request's reads (lower
	// first; 1Password is 0).
	order    int
	scheme   string
	name     string // the store's name in notices, as its Provider.Name
	validate func(uri string) bool
	build    func(c *Cache) Provider
}

var registry []providerSpec

// register adds a provider.
func register(s providerSpec) {
	for _, r := range registry {
		if r.scheme == s.scheme {
			panic("secrets: duplicate provider scheme " + s.scheme)
		}
	}
	registry = append(registry, s)
	sort.SliceStable(registry, func(i, j int) bool { return registry[i].order < registry[j].order })
}

// Schemes lists the registered reference schemes in order.
func Schemes() []string {
	out := make([]string, len(registry))
	for i, r := range registry {
		out[i] = r.scheme
	}
	return out
}

func schemeOf(uri string) string {
	s, _, ok := strings.Cut(uri, "://")
	if !ok {
		return ""
	}
	return s
}

func specFor(uri string) (providerSpec, bool) {
	s := schemeOf(uri)
	for _, r := range registry {
		if r.scheme == s {
			return r, true
		}
	}
	return providerSpec{}, false
}

// ValidRef reports whether uri is a well-formed reference for a registered
// provider. Callers outside the package use it to accept either scheme.
func ValidRef(uri string) bool {
	sp, ok := specFor(uri)
	return ok && !strings.ContainsAny(uri, "\r\n{}\x00") && sp.validate(uri)
}

func fingerprint(parts ...string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(parts, "\x00"))))
}

// Credential references: a provider's own credential (an Azure service
// principal's client secret, for example) may itself be a reference to
// another provider. The chain is carried in the context so a cycle (azkv
// needing azkv, or a longer loop) is refused instead of deadlocking.
type credChainKey struct{}

func credChain(ctx context.Context) []string {
	v, _ := ctx.Value(credChainKey{}).([]string)
	return v
}

// errCredentialCycle is returned when a credential reference loops back to a
// provider already resolving its own credential.
var errCredentialCycle = errors.New("credential reference cycle: a provider's credential cannot be read through itself")

// credentialValue resolves a provider's own credential reference through this
// cache (memory only, TTL bounded) on behalf of provider from.
func (c *Cache) credentialValue(ctx context.Context, from, name, ref string, ttl time.Duration) (string, error) {
	chain := append(append([]string(nil), credChain(ctx)...), from)
	to := schemeOf(ref)
	for _, s := range chain {
		if s == to {
			return "", errCredentialCycle
		}
	}
	ctx = context.WithValue(ctx, credChainKey{}, chain)
	vals, _, err := c.ResolveSources(ctx, from+" credential", []Ref{{Name: name, URI: ref, TTL: ttl}})
	if err != nil {
		return "", fmt.Errorf("read %s for %s: %w", name, from, err)
	}
	return vals[name], nil
}
