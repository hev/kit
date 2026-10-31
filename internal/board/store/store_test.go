package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/hev/kit/internal/board/layer"
	"github.com/hev/kit/internal/board/settings"
)

// The store is a Layer namespace, so the test is an integration test: it
// writes real rows, embeds them for real, and drops the namespace after.
// Gated on FACTORY_BOARD_TEST=1 so `go test ./...` on a laptop without the
// credential — or without the appetite to spend embedding tokens — passes
// by skipping.
func open(t *testing.T) *Store {
	t.Helper()
	if os.Getenv("FACTORY_BOARD_TEST") == "" {
		t.Skip("set FACTORY_BOARD_TEST=1 to run against Layer")
	}
	if err := settings.Load(); err != nil {
		t.Fatal(err)
	}
	cl, err := layer.Load()
	if err != nil {
		t.Fatal(err)
	}
	cl.Namespace = fmt.Sprintf("kit-board-compat-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		if err := cl.DeleteNamespace(); err != nil {
			t.Errorf("delete scratch namespace %s: %v", cl.Namespace, err)
		}
	})
	return New(cl)
}

func TestThreadLifecycle(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	owner := Poster{Author: "owner-project", Role: "owner", Instance: "project", Host: "example"}
	worker := Poster{Author: "worker-project-convoy", Role: "worker", Instance: "project", Host: "example"}

	if ts, err := s.Threads(ctx, ListOpts{}); err != nil || len(ts) != 0 {
		t.Fatalf("fresh namespace: %v %v", ts, err)
	}
	th, err := s.CreateThread(ctx, NewThread{Board: "tooling", Title: "Port 5432 collides", Tags: []string{"CI", " postgres "}, Body: "rerun the job", Poster: owner})
	if err != nil {
		t.Fatal(err)
	}
	if th.ID != 1 || th.Tags[0] != "ci" || th.Tags[1] != "postgres" || th.Replies != 0 || th.Last == nil || th.Last.N != 1 {
		t.Errorf("fresh thread: %+v", th)
	}
	p, err := s.Reply(ctx, NewPost{ThreadID: th.ID, Body: "hit it too", Poster: worker})
	if err != nil {
		t.Fatal(err)
	}
	if p.N != 2 {
		t.Errorf("reply n = %d, want 2", p.N)
	}
	for i := 0; i < 2; i++ {
		if _, err := s.Useful(ctx, th.ID, worker.Author); err != nil {
			t.Fatal(err)
		}
	}
	if n, _ := s.Useful(ctx, th.ID, owner.Author); n != 2 {
		t.Errorf("useful = %d, want 2", n)
	}
	got, err := s.Thread(ctx, th.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Replies != 1 || got.Last.Author != worker.Author || got.Useful != 2 {
		t.Errorf("thread after reply: %+v", got)
	}
	posts, _ := s.Posts(ctx, th.ID)
	if len(posts) != 2 || posts[0].N != 1 || posts[0].Body != "rerun the job" || posts[1].Author != worker.Author {
		t.Errorf("posts: %+v", posts)
	}
	if err := s.SetLocked(ctx, th.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Reply(ctx, NewPost{ThreadID: th.ID, Body: "late", Poster: worker}); err == nil {
		t.Error("reply to a locked thread succeeded")
	}
	if _, err := s.Thread(ctx, 999); err != ErrNotFound {
		t.Errorf("missing thread: %v", err)
	}
	second, _ := s.CreateThread(ctx, NewThread{Board: "research", Title: "Which SQLite driver works without cgo", Body: "modernc is pure go", Poster: worker})
	if second.ID != 2 {
		t.Errorf("second id = %d", second.ID)
	}
	byWorker, _ := s.Threads(ctx, ListOpts{Author: worker.Author})
	if len(byWorker) != 2 {
		t.Errorf("threads by worker (opened one, replied in one): %d", len(byWorker))
	}
	hits, err := s.Search(ctx, SearchOpts{Q: "postgres port collision on CI", Also: []string{"5432"}, Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 || hits[0].Thread.ID != th.ID || hits[0].Post.Body == "" {
		t.Errorf("hybrid search: %+v", hits)
	}
	scoped, err := s.Search(ctx, SearchOpts{Q: "postgres port collision on CI", Board: "research", Limit: 5})
	if err != nil || len(scoped) != 1 || scoped[0].Thread.ID != second.ID {
		t.Errorf("board-scoped search: %+v %v", scoped, err)
	}
	tagged, err := s.Search(ctx, SearchOpts{Q: "driver", Tags: []string{"postgres"}, Limit: 5})
	if err != nil || len(tagged) != 1 || tagged[0].Thread.ID != th.ID {
		t.Errorf("tag-scoped search: %+v %v", tagged, err)
	}
	byAuthor, err := s.Search(ctx, SearchOpts{Q: "hit it", Author: worker.Author, Since: "1h", Limit: 5})
	if err != nil || len(byAuthor) == 0 || byAuthor[0].Post.Author != worker.Author {
		t.Errorf("author-scoped search: %+v %v", byAuthor, err)
	}
	if _, err := s.Search(ctx, SearchOpts{Q: "x", Since: "soon"}); !errors.Is(err, ErrInvalid) {
		t.Errorf("bad since: %v", err)
	}
	st, _ := s.Stats(ctx)
	if st.Threads != 2 || st.Posts != 3 || st.Posters != 2 || len(st.Online) != 2 {
		t.Errorf("stats: %+v", st)
	}
	pr, _ := s.Profiles(ctx, []string{owner.Author})
	if pr[owner.Author].Posts != 1 || pr[owner.Author].Useful != 2 {
		t.Errorf("profile: %+v", pr)
	}
}
