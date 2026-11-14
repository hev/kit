package kitapi_test

import (
	"context"
	"errors"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/pkg/kitapi"
	"github.com/hev/kit/pkg/kitapi/kitapitest"
)

func peerFixture(t *testing.T) (*kitapitest.Server, *kitapi.Client, *kitapi.Client) {
	t.Helper()
	s := kitapitest.New(t, "a", kitapitest.Identity{TeamID: "A", AuthorID: "alice", Host: "alpha"})
	s.AddKey("b", kitapitest.Identity{TeamID: "B", AuthorID: "bob", Host: "beta"})
	return s, s.Client("a"), s.Client("b")
}
func message(id string) kitapi.PeerMessageIn {
	return kitapi.PeerMessageIn{ID: id, Sender: "alpha/sender", Recipients: []string{"beta/reader", "alpha/other"}, Time: time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC), Kind: "chat", ChatID: "thread1", Title: "Review", Body: "ready"}
}
func TestPeersTTLExpiryAndBeaconRevision(t *testing.T) {
	s, a, _ := peerFixture(t)
	ctx := context.Background()
	clock := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	s.SetPeersClock(func() time.Time { return clock })
	in := kitapi.PresenceIn{Host: "alpha", Version: "dev", OS: "linux", ObservedAt: clock, Sessions: []kitapi.PeerSession{{Name: "work", Harness: "codex", Project: "repo", Branch: "main", State: "working"}}}
	ack, e := a.BeaconPeers(ctx, in)
	if e != nil {
		t.Fatal(e)
	}
	clock = clock.Add(time.Minute)
	again, e := a.BeaconPeers(ctx, in)
	if e != nil || !reflect.DeepEqual(ack, again) {
		t.Fatalf("duplicate refreshed expiry: %+v %v", again, e)
	}
	page, e := a.PeerPresence(ctx)
	if e != nil || len(page.Machines) != 1 || page.Machines[0].AuthorID != "alice" {
		t.Fatalf("%+v %v", page, e)
	}
	clock = ack.ExpiresAt
	page, e = a.PeerPresence(ctx)
	if e != nil || len(page.Machines) != 0 {
		t.Fatalf("expired: %+v %v", page, e)
	}
	in.ObservedAt = clock
	ack, e = a.BeaconPeers(ctx, in)
	if e != nil || !ack.ExpiresAt.After(clock) {
		t.Fatal(ack, e)
	}
}
func TestPeersTeamIsolation(t *testing.T) {
	_, a, b := peerFixture(t)
	ctx := context.Background()
	for _, tc := range []struct {
		c    *kitapi.Client
		host string
	}{{a, "alpha"}, {b, "beta"}} {
		_, e := tc.c.BeaconPeers(ctx, kitapi.PresenceIn{Host: tc.host, ObservedAt: time.Now(), Sessions: []kitapi.PeerSession{}})
		if e != nil {
			t.Fatal(e)
		}
		m := message("123456781234123412341234567890ab")
		m.Sender = tc.host + "/sender"
		if _, e = tc.c.AppendPeerMessage(ctx, m); e != nil {
			t.Fatal(e)
		}
	}
	for _, tc := range []struct {
		c            *kitapi.Client
		host, author string
	}{{a, "alpha", "alice"}, {b, "beta", "bob"}} {
		p, e := tc.c.PeerPresence(ctx)
		if e != nil || len(p.Machines) != 1 || p.Machines[0].Host != tc.host {
			t.Fatalf("presence leaked %+v %v", p, e)
		}
		l, e := tc.c.PeerMessages(ctx, nil)
		if e != nil || len(l.Messages) != 1 || l.Messages[0].AuthorID != tc.author {
			t.Fatalf("log leaked %+v %v", l, e)
		}
	}
}
func TestPeersAppendIdempotentConflictAndReadOnly(t *testing.T) {
	s, a, _ := peerFixture(t)
	ctx := context.Background()
	m := message("123456781234123412341234567890ab")
	first, e := a.AppendPeerMessage(ctx, m)
	if e != nil {
		t.Fatal(e)
	}
	s.SetReadOnly(true)
	again, e := a.AppendPeerMessage(ctx, m)
	if e != nil || !reflect.DeepEqual(first, again) {
		t.Fatal(again, e)
	}
	m.Body = "changed"
	_, e = a.AppendPeerMessage(ctx, m)
	var apiErr *kitapi.Error
	if !errors.As(e, &apiErr) || apiErr.Status != 409 {
		t.Fatalf("conflict: %v", e)
	}
	m.ID = "123456781234123412341234567890ac"
	_, e = a.AppendPeerMessage(ctx, m)
	if !kitapi.ReadOnly(e) {
		t.Fatal(e)
	}
	if _, e = a.BeaconPeers(ctx, kitapi.PresenceIn{Host: "alpha", ObservedAt: time.Now()}); e != nil {
		t.Fatalf("read-only presence: %v", e)
	}
	l, e := a.PeerMessages(ctx, nil)
	if e != nil || len(l.Messages) != 1 || l.Messages[0].Body != "ready" || first.StoredBytes != kitapi.PeerStoredBytes(first.PeerMessageIn) {
		t.Fatalf("%+v %v", l, e)
	}
}
func TestPeersPagingFiltersAndValidation(t *testing.T) {
	_, a, b := peerFixture(t)
	ctx := context.Background()
	for _, id := range []string{"123456781234123412341234567890aa", "123456781234123412341234567890ab", "123456781234123412341234567890ac"} {
		if _, e := a.AppendPeerMessage(ctx, message(id)); e != nil {
			t.Fatal(e)
		}
	}
	q := url.Values{"limit": {"1"}, "machine": {"beta"}, "session": {"alpha/sender"}, "chat": {"thread1"}}
	first, e := a.PeerMessages(ctx, q)
	if e != nil || len(first.Messages) != 1 || !strings.HasSuffix(first.Messages[0].ID, "ac") || first.NextCursor == "" {
		t.Fatal(first, e)
	}
	q.Set("cursor", first.NextCursor)
	second, e := a.PeerMessages(ctx, q)
	if e != nil || len(second.Messages) != 1 || !strings.HasSuffix(second.Messages[0].ID, "ab") {
		t.Fatal(second, e)
	}
	// Cursor from another team must not select or reveal its rows.
	_, e = b.PeerMessages(ctx, q)
	var ae *kitapi.Error
	if !errors.As(e, &ae) || ae.Status != 400 {
		t.Fatal(e)
	}
	q.Set("machine", "different")
	_, e = a.PeerMessages(ctx, q)
	if !errors.As(e, &ae) || ae.Status != 400 {
		t.Fatal(e)
	}
	for _, q := range []url.Values{{"cursor": {"bad"}}, {"limit": {"201"}}} {
		if _, e = a.PeerMessages(ctx, q); !errors.As(e, &ae) || ae.Status != 400 {
			t.Fatal(e)
		}
	}
	bad := message("short")
	if _, e = a.AppendPeerMessage(ctx, bad); !errors.As(e, &ae) || ae.Status != 400 {
		t.Fatal(e)
	}
	bad = message("123456781234123412341234567890ad")
	bad.Sender = "beta/forged"
	if _, e = a.AppendPeerMessage(ctx, bad); !errors.As(e, &ae) || ae.Status != 403 {
		t.Fatal(e)
	}
	bad.Sender = "alpha/sender"
	bad.Body = strings.Repeat("x", kitapi.PeersMaxBytes+1)
	if _, e = a.AppendPeerMessage(ctx, bad); !errors.As(e, &ae) || ae.Status != 413 {
		t.Fatal(e)
	}
}
