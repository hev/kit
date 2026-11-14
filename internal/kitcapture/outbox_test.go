package kitcapture_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/filearchive"
	"github.com/hev/kit/internal/kitcapture"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
	"github.com/hev/kit/pkg/kitapi"
	"github.com/hev/kit/pkg/kitapi/kitapitest"
)

const key = "hk_outbox_test"

var ada = kitapitest.Identity{KeyID: "k1", AuthorID: "u1", AuthorName: "Ada", Host: "laptop", TeamID: "t1", TeamSlug: "acme"}

func newArchive(t *testing.T) *kitcapture.Archive {
	t.Helper()
	files, err := filearchive.Open(t.TempDir(), "hev-traces")
	if err != nil {
		t.Fatal(err)
	}
	outbox, err := kitcapture.OpenOutbox(filepath.Join(t.TempDir(), "outbox"))
	if err != nil {
		t.Fatal(err)
	}
	return kitcapture.New(files, outbox)
}

func capture(t *testing.T, a *kitcapture.Archive, id, text string) {
	t.Helper()
	now := time.Now().UnixMilli()
	if _, err := a.WriteSessions([]trace.SessionRow{{ID: id, SessionID: id, Harness: "claude", Start: now, End: now, Summary: "session " + id}}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.WriteBlocks([]trace.BlockRow{{ID: id + ":1", SessionID: id, Seq: 1, Role: "user", BlockType: "text", Text: text}}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Write([]layer.Row{{ID: id + ":c1", SessionID: id, Text: text, Harness: "claude"}}); err != nil {
		t.Fatal(err)
	}
}

// Capture during an outage queues rows without touching the network;
// the outbox drains in order once the API answers, and nothing is lost.
func TestOutboxDrainsAfterOutage(t *testing.T) {
	ctx := context.Background()
	srv := kitapitest.New(t, key, ada)
	c := srv.Client(key)
	c.Retries = 1
	a := newArchive(t)

	srv.SetDown(true)
	capture(t, a, "s1", "first captured while the API was down")
	capture(t, a, "s2", "second captured while the API was down")
	if got := srv.Requests("POST /v1/traces/sessions"); got != 0 {
		t.Fatalf("capture called the API %d times", got)
	}
	pending, _ := a.Outbox.Pending()
	if len(pending) != 6 {
		t.Fatalf("pending = %d, want 6", len(pending))
	}
	res, err := a.Outbox.Drain(ctx, c, nil)
	if err == nil || !kitapi.Retryable(err) || res.Sent != 0 || res.Pending != 6 {
		t.Fatalf("drain while down = %+v, %v", res, err)
	}
	// Local reads still work offline.
	if rows, _ := a.ListSessionRows(-1, nil); len(rows) != 2 {
		t.Fatalf("local sessions = %d", len(rows))
	}

	srv.SetDown(false)
	srv.RateLimit(1) // and one 429 on the way back
	var seen []kitcapture.DrainResult
	res, err = a.Outbox.Drain(ctx, c, func(r kitcapture.DrainResult) { seen = append(seen, r) })
	if err != nil || res.Sent != 6 || res.Pending != 0 || len(seen) != 6 {
		t.Fatalf("drain after outage = %+v, %v (progress %d)", res, err, len(seen))
	}
	rows, _ := srv.Traces.ListSessionRows(-1, nil)
	if len(rows) != 2 || rows[0].AuthorID != "u1" {
		t.Fatalf("server sessions = %+v", rows)
	}
	hits, _, err := srv.Traces.PhraseHits("captured while the API was down", 10, nil)
	if err != nil || len(hits) != 2 {
		t.Fatalf("server chunks = %d, %v", len(hits), err)
	}
	// Draining again is a no-op; resending would be harmless (upserts).
	if res, err := a.Outbox.Drain(ctx, c, nil); err != nil || res.Sent != 0 {
		t.Fatalf("second drain = %+v, %v", res, err)
	}
}

// A batch the API refuses for good moves aside instead of blocking the
// queue; a rejected key stops the drain and keeps everything queued.
func TestOutboxRefusalsAndRevokedKey(t *testing.T) {
	ctx := context.Background()
	srv := kitapitest.New(t, key, ada)
	c := srv.Client(key)
	a := newArchive(t)
	if err := a.Outbox.Enqueue(kitcapture.KindSessions, []byte(`{"rows": not json`)); err != nil {
		t.Fatal(err)
	}
	capture(t, a, "s1", "after a bad batch")
	res, err := a.Outbox.Drain(ctx, c, nil)
	if err != nil || res.Failed != 1 || res.Sent != 3 {
		t.Fatalf("drain = %+v, %v", res, err)
	}
	failed, _ := os.ReadDir(filepath.Join(a.Outbox.Dir(), "failed"))
	if len(failed) != 2 { // the batch and its .error note
		t.Fatalf("failed/ holds %d files", len(failed))
	}

	if err := a.DeleteSessionArchive("s1"); err != nil {
		t.Fatal(err)
	}
	srv.Revoke(key)
	capture(t, a, "s2", "after revocation")
	res, err = a.Outbox.Drain(ctx, c, nil)
	if !errors.Is(err, kitapi.ErrUnauthorized) || res.Pending != 4 {
		t.Fatalf("drain with revoked key = %+v, %v", res, err)
	}
	srv.AddKey(key, ada)
	if _, err := a.Outbox.Drain(ctx, c, nil); err != nil {
		t.Fatal(err)
	}
	rows, _ := srv.Traces.ListSessionRows(-1, nil)
	if len(rows) != 1 || rows[0].SessionID != "s2" {
		t.Fatalf("server sessions after delete = %+v", rows)
	}
}

// The background drainer empties the outbox on its own once the API is back.
func TestDrainerRecovers(t *testing.T) {
	srv := kitapitest.New(t, key, ada)
	c := srv.Client(key)
	c.Retries = 0
	a := newArchive(t)
	srv.SetDown(true)
	capture(t, a, "s1", "drained in the background")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d := &kitcapture.Drainer{Outbox: a.Outbox, Client: c, Interval: 10 * time.Millisecond, MaxBackoff: 20 * time.Millisecond}
	done := make(chan struct{})
	go func() { d.Run(ctx); close(done) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := d.Last(); err != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("drainer never tried")
		}
		time.Sleep(5 * time.Millisecond)
	}
	srv.SetDown(false)
	d.Kick()
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
	if _, err := d.Last(); err != nil && !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("last drain: %v", err)
	}
}
