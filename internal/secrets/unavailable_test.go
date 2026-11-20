package secrets

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
)

// A caller with no 1Password credential, or another account's, was told "no
// local endpoint serves this account" while the daemon was up on the pipe
// (win-oven, 2026-11-17). The three cases are now told apart, naming
// schemes and settings only.
func TestUnavailableReasonsTellNoEndpointFromRefusal(t *testing.T) {
	c := fake(t)
	calls := fakeCalls(t)
	ep := isolatedEndpoint(t)
	t.Setenv(EndpointEnv, ep)
	ctx := context.Background()

	// (a) nothing listening.
	err := Ping(ctx)
	if r, _ := UnavailableReason(err); r != UnavailableNoEndpoint || !errors.Is(err, ErrUnavailable) {
		t.Fatalf("no endpoint: %v (%s)", err, r)
	}
	s, err := ReadLocalStatus(ctx)
	if s == nil || s.Daemon || s.DaemonUnavailable != UnavailableNoEndpoint {
		t.Fatalf("status with no endpoint: %+v, %v", s, err)
	}

	serveOn(t, ep, c)
	if err := Ping(ctx); err != nil {
		t.Fatalf("matching caller: %v", err)
	}

	// (b) up, but this caller has no 1Password credential.
	t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "")
	err = Ping(ctx)
	r, providers := UnavailableReason(err)
	if r != UnavailableNoCredential || !slices.Equal(providers, []string{"op"}) || !errors.Is(err, ErrUnavailable) || err.Error() != ErrUnavailable.Error() {
		t.Fatalf("no credential: %v (%s %v)", err, r, providers)
	}
	s, _ = ReadLocalStatus(ctx)
	if s == nil || s.Daemon || s.DaemonUnavailable != UnavailableNoCredential || !slices.Equal(s.DaemonUnavailableProviders, []string{"op"}) {
		t.Fatalf("status with no credential: %+v", s)
	}
	d := DescribeUnavailable(err, ep)
	if !strings.Contains(d, "daemon is up at "+ep) || !strings.Contains(d, "no 1Password credential") {
		t.Fatalf("describe no credential: %q", d)
	}

	// (c) up, but this caller's credential is another account's.
	t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "another-accounts-token")
	err = Ping(ctx)
	if r, p := UnavailableReason(err); r != UnavailableOtherAccount || !slices.Equal(p, []string{"op"}) {
		t.Fatalf("other account: %v (%s %v)", err, r, p)
	}
	s, _ = ReadLocalStatus(ctx)
	if s == nil || s.DaemonUnavailable != UnavailableOtherAccount {
		t.Fatalf("status with another account: %+v", s)
	}
	raw, _ := json.Marshal(s)
	for _, secret := range []string{"another-accounts-token", "fake-service-account-token", fingerprint("another-accounts-token"), fingerprint("fake-service-account-token")} {
		if strings.Contains(string(raw), secret) || strings.Contains(DescribeUnavailable(err, ep), secret) {
			t.Fatal("status carries credential or fingerprint material")
		}
	}
	var notice strings.Builder
	if _, err := ResolveResult(ctx, refs(), Options{CacheOnly: true, Notice: &notice}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("cache-only with another account: %v", err)
	} else if r, _ := UnavailableReason(err); r != UnavailableOtherAccount {
		t.Fatalf("cache-only reason %s", r)
	}
	if n := len(calls()); n != 0 {
		t.Fatalf("telling the cases apart made %d requests", n)
	}
}

// An older daemon refuses without saying what it serves, and an older client
// sends only the 1Password fingerprint: both still work.
func TestUnavailableReasonsCompatibleWithOlderPeers(t *testing.T) {
	// A refusal without Serves: every older daemon serves 1Password.
	if u := unavailableFor(map[string]string{}, nil, nil); u.Reason != UnavailableNoCredential || !slices.Equal(u.Providers, []string{"op"}) {
		t.Fatalf("old daemon, no credential: %+v", u)
	}
	if u := unavailableFor(map[string]string{"op": "x"}, nil, nil); u.Reason != UnavailableOtherAccount {
		t.Fatalf("old daemon, other account: %+v", u)
	}
	// A resolution of a scheme the caller has no credential for.
	if u := unavailableFor(map[string]string{"op": "x"}, []string{"op"}, []string{"op", azkvScheme}); u.Reason != UnavailableNoCredential || !slices.Equal(u.Providers, []string{azkvScheme}) {
		t.Fatalf("resolution missing a credential: %+v", u)
	}

	// An older client's request (Account only) against this daemon.
	c := fake(t)
	ep := isolatedEndpoint(t)
	serveOn(t, ep, c)
	ask := func(q any) response {
		t.Helper()
		conn, err := dial(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		if err := json.NewEncoder(conn).Encode(q); err != nil {
			t.Fatal(err)
		}
		var r response
		if err := json.NewDecoder(conn).Decode(&r); err != nil {
			t.Fatal(err)
		}
		return r
	}
	type oldRequest struct {
		Account string
		Refs    []Ref
		Status  bool
	}
	if r := ask(oldRequest{Account: fingerprint("fake-service-account-token"), Status: true}); r.Unavailable || r.Status == nil {
		t.Fatalf("old client, matching account: %+v", r)
	}
	r := ask(oldRequest{Account: fingerprint("someone-else"), Status: true})
	if !r.Unavailable || !slices.Equal(r.Serves, []string{"op"}) {
		t.Fatalf("old client, other account: %+v", r)
	}
	// What an older client decodes from that refusal: Unavailable, as before.
	var old struct{ Unavailable bool }
	raw, _ := json.Marshal(r)
	if json.Unmarshal(raw, &old) != nil || !old.Unavailable {
		t.Fatal("older client cannot read the refusal")
	}
}
