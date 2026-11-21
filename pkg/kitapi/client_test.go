package kitapi_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/pkg/archive"
	"github.com/hev/kit/pkg/kitapi"
	"github.com/hev/kit/pkg/kitapi/kitapitest"
)

const key = "hk_test_key"

var ada = kitapitest.Identity{KeyID: "k1", AuthorID: "u1", AuthorName: "Ada", Host: "laptop", TeamID: "t1", TeamSlug: "acme"}

func TestContractRoundTrip(t *testing.T) {
	ctx := context.Background()
	srv := kitapitest.New(t, key, ada)
	c := srv.Client(key)

	who, err := c.Whoami(ctx)
	if err != nil || who.Team.Slug != "acme" || who.Author.Name != "Ada" {
		t.Fatalf("whoami = %+v, %v", who, err)
	}
	now := time.Now().UnixMilli()
	n, err := c.PostSessions(ctx, []archive.SessionRow{{ID: "s1", SessionID: "s1", Harness: "claude", Start: now, End: now, Summary: "fixture", AuthorID: "mallory"}})
	if err != nil || n != 1 {
		t.Fatalf("sessions = %d, %v", n, err)
	}
	if _, err := c.PostBlocks(ctx, []archive.BlockRow{{ID: "s1:1", SessionID: "s1", Seq: 1, Role: "user", BlockType: "text", Text: "find the purple walrus"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.PostChunks(ctx, []archive.Row{{ID: "s1:c1", SessionID: "s1", Text: "find the purple walrus", Harness: "claude"}}); err != nil {
		t.Fatal(err)
	}
	rows, _ := srv.Traces.ListSessionRows(-1, nil)
	if len(rows) != 1 || rows[0].AuthorID != "u1" || rows[0].Host != "laptop" {
		t.Fatalf("server did not stamp the author: %+v", rows)
	}
	res, err := c.Search(ctx, "purple walrus", kitapi.ModePhrase, nil)
	if err != nil || len(res.Sessions) != 1 || res.Sessions[0].ID != "s1" {
		t.Fatalf("search = %+v, %v", res, err)
	}
	if raw, err := c.Session(ctx, "s1", nil); err != nil || !strings.Contains(string(raw), "purple walrus") {
		t.Fatalf("session = %s, %v", raw, err)
	}
	for _, f := range []func() ([]byte, error){
		func() ([]byte, error) { return c.Sessions(ctx, nil) },
		func() ([]byte, error) { return c.Stats(ctx, nil) },
		func() ([]byte, error) { return c.Values(ctx, map[string][]string{"facet": {"harness"}}) },
	} {
		if _, err := f(); err != nil {
			t.Fatal(err)
		}
	}

	th, err := c.CreateThread(ctx, kitapi.ThreadIn{Board: "floor", Title: "hello", Body: "first"})
	if err != nil || th.Author != "Ada" {
		t.Fatalf("thread = %+v, %v", th, err)
	}
	if _, err := c.Reply(ctx, th.ID, kitapi.PostIn{Body: "second"}); err != nil {
		t.Fatal(err)
	}
	if n, err := c.Useful(ctx, th.ID); err != nil || n != 1 {
		t.Fatalf("useful = %d, %v", n, err)
	}
	if err := c.Pin(ctx, th.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := c.Lock(ctx, th.ID, true); err != nil {
		t.Fatal(err)
	}
	view, err := c.Thread(ctx, th.ID)
	if err != nil || len(view.Posts) != 2 || !view.Thread.Pinned || !view.Thread.Locked {
		t.Fatalf("thread view = %+v, %v", view, err)
	}
	if ts, err := c.Threads(ctx, nil); err != nil || len(ts) != 1 {
		t.Fatalf("threads = %d, %v", len(ts), err)
	}
	if hits, err := c.BoardSearch(ctx, map[string][]string{"q": {"second"}}); err != nil || len(hits) == 0 {
		t.Fatalf("board search = %d, %v", len(hits), err)
	}
	if u, err := c.OpenSession(ctx); err != nil || !strings.HasPrefix(u, srv.URL) {
		t.Fatalf("open session = %q, %v", u, err)
	}

	srv.AddKey("hk_other", bob)
	if err := srv.Client("hk_other").DeleteSession(ctx, "s1"); err == nil {
		t.Fatal("deleted another author's session")
	}
	if err := c.DeleteSession(ctx, "s1"); err != nil {
		t.Fatal(err)
	}
	if rows, _ := srv.Traces.ListSessionRows(-1, nil); len(rows) != 0 {
		t.Fatalf("session survived delete: %d", len(rows))
	}
}

var bob = kitapitest.Identity{KeyID: "k2", AuthorID: "u2", AuthorName: "Bob", Host: "desk", TeamID: "t1", TeamSlug: "acme"}

func TestRetriesOn429And5xx(t *testing.T) {
	ctx := context.Background()
	srv := kitapitest.New(t, key, ada)
	c := srv.Client(key)
	srv.RateLimit(2)
	if _, err := c.Whoami(ctx); err != nil {
		t.Fatalf("after 429s: %v", err)
	}
	if got := srv.Requests("GET /v1/whoami"); got != 3 {
		t.Fatalf("requests = %d, want 3", got)
	}
	srv.SetDown(true)
	c.Retries = 2
	_, err := c.Whoami(ctx)
	var e *kitapi.Error
	if !errors.As(err, &e) || e.Status != http.StatusServiceUnavailable || !kitapi.Retryable(err) {
		t.Fatalf("down: %v", err)
	}
	srv.SetDown(false)
	srv.Revoke(key)
	if _, err := c.Whoami(ctx); !errors.Is(err, kitapi.ErrUnauthorized) || kitapi.Retryable(err) {
		t.Fatalf("revoked: %v", err)
	}
}

func TestBatchesRespectLimits(t *testing.T) {
	rows := make([]archive.Row, 1201)
	for i := range rows {
		rows[i] = archive.Row{ID: "r", Text: "x"}
	}
	bodies, err := kitapi.Batches(rows)
	if err != nil || len(bodies) != 3 {
		t.Fatalf("batches = %d, %v", len(bodies), err)
	}
	big := strings.Repeat("y", 1<<20)
	rows = make([]archive.Row, 9)
	for i := range rows {
		rows[i] = archive.Row{ID: "r", Text: big}
	}
	bodies, err = kitapi.Batches(rows)
	if err != nil || len(bodies) != 3 {
		t.Fatalf("byte-limited batches = %d, %v", len(bodies), err)
	}
	for _, b := range bodies {
		if len(b) > kitapi.MaxBatchBytes {
			t.Fatalf("batch of %d bytes", len(b))
		}
	}
	if _, err := kitapi.Batches([]archive.Row{{Text: strings.Repeat("z", kitapi.MaxBatchBytes)}}); err == nil {
		t.Fatal("oversized row accepted")
	}
}
