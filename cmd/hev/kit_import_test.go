package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	boardstore "github.com/hev/kit/internal/board/store"
	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/pkg/kitapi"
	"github.com/hev/kit/pkg/kitapi/kitapitest"
)

// loseAnswers lets a board write reach the server, then fails the call as
// if the answer was lost. fail decides, per request, whether and how.
type loseAnswers struct {
	next http.RoundTripper
	mu   sync.Mutex
	fail func(r *http.Request) error
}

func (l *loseAnswers) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := l.next.RoundTrip(r)
	if err != nil {
		return resp, err
	}
	l.mu.Lock()
	fail := l.fail
	l.mu.Unlock()
	if fail != nil {
		if ferr := fail(r); ferr != nil {
			resp.Body.Close()
			return nil, ferr
		}
	}
	return resp, nil
}

// hev login --import sends each local board thread and post once even when
// the API accepted a write and the answer was lost (retried in the same
// run) or the process stopped before saving progress (resumed in the next
// run): it records the send before the POST and looks for the post on the
// API before sending it again.
func TestKitImportBoardNoDuplicates(t *testing.T) {
	home := kitE2EHome(t)
	api := kitapitest.New(t, fakeKitKey, e2eIdentity)
	svc := newKitService(t, api)
	ctx := context.Background()
	oldBackoff := kitImportBackoff
	t.Cleanup(func() { kitImportBackoff = oldBackoff })
	kitImportBackoff = func(int) time.Duration { return 0 }

	local, err := boardstore.NewFiles(filepath.Join(home, ".hev", "board"))
	if err != nil {
		t.Fatal(err)
	}
	lt, err := local.CreateThread(ctx, boardstore.NewThread{Board: "learnings", Title: "same title", Body: "first thread", Poster: boardstore.Poster{Author: "bob", Host: "box"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{"reply one", "reply two"} {
		if _, err := local.Reply(ctx, boardstore.NewPost{ThreadID: lt.ID, Body: body, Poster: boardstore.Poster{Author: "carol"}}); err != nil {
			t.Fatal(err)
		}
	}
	// A second local thread with the same title, so finding by title alone
	// would be wrong.
	if _, err := local.CreateThread(ctx, boardstore.NewThread{Board: "learnings", Title: "same title", Body: "second thread", Poster: boardstore.Poster{Author: "bob"}}); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runLogin(ctx, &out, svc.srv.URL, ""); err != nil {
		t.Fatal(err)
	}

	lose := &loseAnswers{next: http.DefaultTransport}
	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	oldClient := kitClient
	t.Cleanup(func() { kitClient = oldClient })
	kitClient = func(k *daemon.KitConfig) *kitapi.Client {
		c := oldClient(k)
		c.HTTP = &http.Client{Transport: lose}
		return c
	}
	var lostCreate, lostReply, died bool
	lose.fail = func(r *http.Request) error {
		if r.Method != "POST" {
			return nil
		}
		switch {
		case r.URL.Path == "/v1/board/threads" && !lostCreate:
			lostCreate = true // accepted, answer lost: retried in this run
			return errors.New("connection reset by peer")
		case strings.HasSuffix(r.URL.Path, "/posts") && !lostReply:
			lostReply = true
			return errors.New("connection reset by peer")
		case strings.HasSuffix(r.URL.Path, "/posts") && !died:
			died = true // accepted, then the process stops before saving
			stop()
			return context.Canceled
		}
		return nil
	}
	if err := runKitImport(runCtx, &out); err == nil {
		t.Fatal("first import did not stop")
	}
	if !lostCreate || !lostReply || !died {
		t.Fatalf("faults not all hit: create=%v reply=%v died=%v", lostCreate, lostReply, died)
	}

	// Next run, nothing lost.
	lose.mu.Lock()
	lose.fail = nil
	lose.mu.Unlock()
	out.Reset()
	if err := runKitImport(ctx, &out); err != nil {
		t.Fatal(err)
	}
	threads, err := api.Board.Threads(ctx, boardstore.ListOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(threads) != 2 {
		t.Fatalf("API threads = %d, want 2", len(threads))
	}
	want := map[string][]string{"first thread": {"first thread", "reply one", "reply two"}, "second thread": {"second thread"}}
	for _, th := range threads {
		posts, _ := api.Board.Posts(ctx, th.ID)
		if len(posts) == 0 {
			t.Fatalf("thread %d has no posts", th.ID)
		}
		var key string
		for k := range want {
			if strings.HasSuffix(posts[0].Body, k) {
				key = k
			}
		}
		if key == "" || len(posts) != len(want[key]) {
			t.Fatalf("thread %d: %d posts, first %q", th.ID, len(posts), posts[0].Body)
		}
		for i, p := range posts {
			if !strings.HasSuffix(p.Body, want[key][i]) {
				t.Fatalf("thread %d post %d = %q, want …%q", th.ID, i+1, p.Body, want[key][i])
			}
		}
		delete(want, key)
	}
	// And a third run sends nothing.
	before := api.Requests("POST /v1/board/threads")
	if err := runKitImport(ctx, &out); err != nil {
		t.Fatal(err)
	}
	if api.Requests("POST /v1/board/threads") != before {
		t.Fatal("third import posted again")
	}
}
