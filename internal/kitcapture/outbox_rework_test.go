package kitcapture_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hev/kit/internal/filearchive"
	"github.com/hev/kit/internal/kitcapture"
	"github.com/hev/kit/pkg/kitapi"
	"github.com/hev/kit/pkg/kitapi/kitapitest"
)

var bee = kitapitest.Identity{KeyID: "B", AuthorID: "B-user", AuthorName: "Bee", Host: "laptop", TeamID: "team-B", TeamSlug: "bees"}

func archiveAt(t *testing.T, root, team string) *kitcapture.Archive {
	t.Helper()
	files, err := filearchive.Open(t.TempDir(), "hev-traces")
	if err != nil {
		t.Fatal(err)
	}
	outbox, err := kitcapture.OpenOutbox(root, team)
	if err != nil {
		t.Fatal(err)
	}
	return kitcapture.New(files, outbox)
}

func filesIn(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Type().IsRegular() && strings.HasSuffix(e.Name(), ".json") {
			raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			out[e.Name()] = string(raw)
		}
	}
	return out
}

// Regression for the review probe TestReviewQueueReboundToNewTeam: a
// session queued under team A while offline is not delivered to team B
// after logout and login to B on the same machine. A's batches stay on disk
// untouched and go to A when A signs in again.
func TestOutboxQueueNotReboundToNewTeam(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "outbox")
	a := archiveAt(t, root, ada.TeamID)
	capture(t, a, "team-A-only", "private A content queued offline")
	before := filesIn(t, a.Outbox.Dir())
	if len(before) != 3 {
		t.Fatalf("A queued %d batches, want 3", len(before))
	}

	// Log out of A, log in to B: the daemon reopens the outbox for B.
	srvB := kitapitest.New(t, "key-B", bee)
	b, err := kitcapture.OpenOutbox(root, bee.TeamID)
	if err != nil {
		t.Fatal(err)
	}
	res, err := b.Drain(ctx, srvB.Client("key-B"), nil)
	if err != nil || res.Sent != 0 {
		t.Fatalf("B's drain = %+v, %v", res, err)
	}
	rows, err := srvB.Traces.ListSessionRows(-1, nil)
	if err != nil || len(rows) != 0 {
		t.Fatalf("team B received %d sessions (%v), want none", len(rows), err)
	}
	for _, p := range []string{"POST /v1/traces/sessions", "POST /v1/traces/blocks", "POST /v1/traces/chunks"} {
		if n := srvB.Requests(p); n != 0 {
			t.Fatalf("team B got %d %s", n, p)
		}
	}
	// Even handed B's key directly, A's outbox refuses to send.
	res, err = a.Outbox.Drain(ctx, srvB.Client("key-B"), nil)
	if !errors.Is(err, kitcapture.ErrTeamMismatch) || res.Sent != 0 {
		t.Fatalf("A's outbox drained with B's key = %+v, %v", res, err)
	}
	if rows, _ := srvB.Traces.ListSessionRows(-1, nil); len(rows) != 0 {
		t.Fatalf("team B received %d sessions from A's outbox", len(rows))
	}
	if after := filesIn(t, a.Outbox.Dir()); len(after) != len(before) {
		t.Fatalf("A's batches changed: %d -> %d", len(before), len(after))
	} else {
		for name, body := range before {
			if after[name] != body {
				t.Fatalf("A's batch %s changed", name)
			}
		}
	}

	// A signs in again: its batches go to A.
	srvA := kitapitest.New(t, key, ada)
	a2, err := kitcapture.OpenOutbox(root, ada.TeamID)
	if err != nil {
		t.Fatal(err)
	}
	if res, err := a2.Drain(ctx, srvA.Client(key), nil); err != nil || res.Sent != 3 {
		t.Fatalf("A's drain = %+v, %v", res, err)
	}
	if rows, _ := srvA.Traces.ListSessionRows(-1, nil); len(rows) != 1 || rows[0].SessionID != "team-A-only" {
		t.Fatalf("team A sessions = %+v", rows)
	}
}

// Batches left in the outbox root by a kit that did not bind them to a team
// are never sent to whichever team is signed in; they stay on disk.
func TestOutboxUnboundBatchesNeverSent(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "outbox")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(root, "00000000000000000001-000001-1-sessions.json")
	body := `{"rows":[{"id":"old","session_id":"old","harness":"claude","summary":"queued before binding"}]}`
	if err := os.WriteFile(legacy, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	srv := kitapitest.New(t, key, ada)
	a := archiveAt(t, root, ada.TeamID)
	capture(t, a, "new", "bound to A")
	if res, err := a.Outbox.Drain(ctx, srv.Client(key), nil); err != nil || res.Sent != 3 {
		t.Fatalf("drain = %+v, %v", res, err)
	}
	rows, _ := srv.Traces.ListSessionRows(-1, nil)
	if len(rows) != 1 || rows[0].SessionID != "new" {
		t.Fatalf("server sessions = %+v", rows)
	}
	unbound, err := a.Outbox.Unbound()
	if err != nil || len(unbound) != 1 {
		t.Fatalf("unbound = %v, %v", unbound, err)
	}
	if raw, err := os.ReadFile(legacy); err != nil || string(raw) != body {
		t.Fatalf("legacy batch touched: %q, %v", raw, err)
	}
}

// A team id that is not a plain name still gets its own directory; without
// a team id the outbox holds batches and never sends them.
func TestOutboxTeamDirectory(t *testing.T) {
	root := t.TempDir()
	// No team id recorded: capture still queues, nothing is sent.
	none, err := kitcapture.OpenOutbox(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := none.Enqueue(kitcapture.KindSessions, []byte(`{"rows":[]}`)); err != nil {
		t.Fatal(err)
	}
	srv := kitapitest.New(t, key, ada)
	if res, err := none.Drain(context.Background(), srv.Client(key), nil); !errors.Is(err, kitcapture.ErrNoTeam) || res.Sent != 0 || res.Pending != 1 {
		t.Fatalf("drain with no team = %+v, %v", res, err)
	}
	if srv.Requests("POST /v1/traces/sessions") != 0 {
		t.Fatal("no-team outbox posted")
	}
	o1, err := kitcapture.OpenOutbox(root, "../escape")
	if err != nil {
		t.Fatal(err)
	}
	o2, err := kitcapture.OpenOutbox(root, "org_1")
	if err != nil {
		t.Fatal(err)
	}
	teams := filepath.Join(root, "teams")
	for _, o := range []*kitcapture.Outbox{o1, o2} {
		if filepath.Dir(o.Dir()) != teams {
			t.Fatalf("outbox dir %s is not under %s", o.Dir(), teams)
		}
	}
	if o1.Dir() == o2.Dir() || none.Dir() == o1.Dir() || none.Dir() == o2.Dir() {
		t.Fatal("two teams share a directory")
	}
}

// 402 billing_read_only: the drain stops and keeps the batch queued (no
// dead-lettering, no logout); capture keeps writing locally; once the team
// pays, a later drain delivers everything and failed/ stays empty.
func TestOutboxReadOnlyThenPaid(t *testing.T) {
	ctx := context.Background()
	srv := kitapitest.New(t, key, ada)
	c := srv.Client(key)
	a := newArchive(t)

	srv.SetReadOnly(true)
	capture(t, a, "s1", "captured while the team was read-only")
	res, err := a.Outbox.Drain(ctx, c, nil)
	if !kitapi.ReadOnly(err) || res.Sent != 0 || res.Failed != 0 || res.Pending != 3 {
		t.Fatalf("drain while read-only = %+v, %v", res, err)
	}
	if !strings.Contains(err.Error(), "billing_read_only") {
		t.Fatalf("402 error does not say why: %v", err)
	}
	if n := srv.Requests("POST /v1/traces/chunks") + srv.Requests("POST /v1/traces/blocks") + srv.Requests("POST /v1/traces/sessions"); n != 1 {
		t.Fatalf("drain kept posting after a 402: %d posts", n)
	}
	// Capture continues locally and queues.
	capture(t, a, "s2", "captured later, still read-only")
	if rows, _ := a.ListSessionRows(-1, nil); len(rows) != 2 {
		t.Fatalf("local sessions = %d", len(rows))
	}
	if p, _ := a.Outbox.Pending(); len(p) != 6 {
		t.Fatalf("pending = %d, want 6", len(p))
	}
	if _, err := c.Whoami(ctx); err != nil {
		t.Fatalf("the key stopped working: %v", err)
	}

	srv.SetReadOnly(false) // the team paid
	res, err = a.Outbox.Drain(ctx, c, nil)
	if err != nil || res.Sent != 6 || res.Failed != 0 || res.Pending != 0 {
		t.Fatalf("drain after paying = %+v, %v", res, err)
	}
	rows, _ := srv.Traces.ListSessionRows(-1, nil)
	if len(rows) != 2 {
		t.Fatalf("server sessions = %+v", rows)
	}
	if failed, _ := os.ReadDir(filepath.Join(a.Outbox.Dir(), "failed")); len(failed) != 0 {
		t.Fatalf("failed/ holds %d files", len(failed))
	}
}

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// The background drainer logs a one-line billing diagnostic on 402, keeps
// the batches, and delivers them once the team pays.
func TestDrainerReadOnlyRecovers(t *testing.T) {
	srv := kitapitest.New(t, key, ada)
	c := srv.Client(key)
	a := newArchive(t)
	srv.SetReadOnly(true)
	capture(t, a, "s1", "drained after paying")
	var logs syncBuffer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d := &kitcapture.Drainer{Outbox: a.Outbox, Client: c, Logger: slog.New(slog.NewTextHandler(&logs, nil)), Interval: 10 * time.Millisecond, MaxBackoff: 20 * time.Millisecond}
	done := make(chan struct{})
	go func() { d.Run(ctx); close(done) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := d.Last(); kitapi.ReadOnly(err) && strings.Contains(logs.String(), "read-only for billing") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("drainer never reported the 402: %s", logs.String())
		}
		time.Sleep(5 * time.Millisecond)
	}
	srv.SetReadOnly(false)
	for {
		p, _ := a.Outbox.Pending()
		if len(p) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("outbox still holds %v", p)
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done
	if rows, _ := srv.Traces.ListSessionRows(-1, nil); len(rows) != 1 {
		t.Fatalf("server sessions = %d", len(rows))
	}
	if failed, _ := os.ReadDir(filepath.Join(a.Outbox.Dir(), "failed")); len(failed) != 0 {
		t.Fatalf("failed/ holds %d files", len(failed))
	}
}

// At its bound the outbox first drops the oldest refused batches, then
// refuses new ones with ErrFull and a log line; capture itself carries on
// with the rows in the local archive.
func TestOutboxBound(t *testing.T) {
	ctx := context.Background()
	srv := kitapitest.New(t, key, ada)
	var logs syncBuffer
	a := newArchive(t)
	a.Outbox.MaxBatches = 4
	a.Outbox.Logger = slog.New(slog.NewTextHandler(&logs, nil))

	// Two refused batches in failed/.
	for i := 0; i < 2; i++ {
		if err := a.Outbox.Enqueue(kitcapture.KindSessions, []byte(`not json`)); err != nil {
			t.Fatal(err)
		}
	}
	if res, err := a.Outbox.Drain(ctx, srv.Client(key), nil); err != nil || res.Failed != 2 {
		t.Fatalf("drain = %+v, %v", res, err)
	}
	srv.SetDown(true)
	// Three queued batches: one refused batch makes way.
	capture(t, a, "s1", "first")
	if n, _, _ := a.Outbox.Usage(); n != 4 {
		t.Fatalf("usage = %d batches, want 4", n)
	}
	if !strings.Contains(logs.String(), "removed the oldest refused batches") {
		t.Fatalf("no prune log:\n%s", logs.String())
	}
	// Full of queued work: the next capture is refused, loudly, and the
	// rows stay local.
	capture(t, a, "s2", "second")
	if err := a.Outbox.Enqueue(kitcapture.KindSessions, []byte(`{"rows":[]}`)); !errors.Is(err, kitcapture.ErrFull) {
		t.Fatalf("enqueue at the bound = %v", err)
	}
	if !strings.Contains(logs.String(), "kit outbox full") {
		t.Fatalf("no full log:\n%s", logs.String())
	}
	if n, _ := a.Outbox.Refused(); n == 0 {
		t.Fatal("refusals not counted")
	}
	if n, _, _ := a.Outbox.Usage(); n != 4 {
		t.Fatalf("usage = %d batches, want 4", n)
	}
	if rows, _ := a.ListSessionRows(-1, nil); len(rows) != 2 {
		t.Fatalf("local sessions = %d, want 2", len(rows))
	}
	// Byte bound too.
	b := newArchive(t)
	b.Outbox.MaxBytes = 10
	if err := b.Outbox.Enqueue(kitcapture.KindSessions, []byte(`{"rows":[1,2,3]}`)); !errors.Is(err, kitcapture.ErrFull) {
		t.Fatalf("enqueue over the byte bound = %v", err)
	}
}
