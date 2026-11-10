package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hev/kit/internal/board/settings"
	"github.com/hev/kit/internal/layerenv"
)

func files(t *testing.T) (*Store, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "board")
	s, err := NewFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s, dir
}

// The same lifecycle the Layer integration test walks, against files.
func TestFilesThreadLifecycle(t *testing.T) {
	s, dir := files(t)
	ctx := context.Background()
	owner := Poster{Author: "owner-project", Role: "owner", Instance: "project", Host: "example"}
	worker := Poster{Author: "worker-project-convoy", Role: "worker", Instance: "project", Host: "example"}

	if s.Info().Mode != "filesystem" {
		t.Fatalf("mode %q", s.Info().Mode)
	}
	if ts, err := s.Threads(ctx, ListOpts{}); err != nil || len(ts) != 0 {
		t.Fatalf("fresh board: %v %v", ts, err)
	}
	th, err := s.CreateThread(ctx, NewThread{Board: "tooling", Title: "Port 5432 collides", Tags: []string{"CI", " postgres "}, Body: "rerun the job", Poster: owner})
	if err != nil {
		t.Fatal(err)
	}
	if th.ID != 1 || th.Tags[0] != "ci" || th.Tags[1] != "postgres" || th.Replies != 0 || th.Last == nil || th.Last.N != 1 {
		t.Errorf("fresh thread: %+v", th)
	}
	if _, err := os.Stat(filepath.Join(dir, "threads", "1.json")); err != nil {
		t.Fatalf("thread file: %v", err)
	}
	p, err := s.Reply(ctx, NewPost{ThreadID: th.ID, Body: "hit it too", Poster: worker})
	if err != nil {
		t.Fatal(err)
	}
	if p.N != 2 || p.ID != "p:1:2" || p.Author != worker.Author || p.Host != "example" {
		t.Errorf("reply: %+v", p)
	}
	got, err := s.Thread(ctx, th.ID)
	if err != nil || got.Replies != 1 || got.Last.Author != worker.Author || got.Last.N != 2 || !got.Updated.After(got.Created) && !got.Updated.Equal(got.Created) {
		t.Fatalf("after reply: %+v %v", got, err)
	}
	posts, err := s.Posts(ctx, th.ID)
	if err != nil || len(posts) != 2 || posts[0].ID != "t:1" || posts[0].Body != "rerun the job" || posts[1].Body != "hit it too" {
		t.Fatalf("posts: %+v %v", posts, err)
	}
	if err := s.View(ctx, th.ID); err != nil {
		t.Fatal(err)
	}
	for _, a := range []string{"a", "b", "a"} {
		if _, err := s.Useful(ctx, th.ID, a); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SetPinned(ctx, th.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetLocked(ctx, th.ID, true); err != nil {
		t.Fatal(err)
	}
	got, _ = s.Thread(ctx, th.ID)
	if got.Views != 1 || got.Useful != 2 || !got.Pinned || !got.Locked {
		t.Fatalf("flags: %+v", got)
	}
	if _, err := s.Reply(ctx, NewPost{ThreadID: th.ID, Body: "late", Poster: worker}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("reply to a locked thread: %v", err)
	}
	if _, err := s.Thread(ctx, 99); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing thread: %v", err)
	}
	if _, err := s.CreateThread(ctx, NewThread{Board: "nope", Title: "x", Body: "y", Poster: owner}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown board: %v", err)
	}
	other, _ := s.CreateThread(ctx, NewThread{Title: "Lunch", Body: "tacos", Poster: worker})
	if other.ID != 2 || other.Board != "watercooler" {
		t.Fatalf("second thread: %+v", other)
	}

	if ts, _ := s.Threads(ctx, ListOpts{Board: "tooling"}); len(ts) != 1 || ts[0].ID != 1 {
		t.Errorf("by board: %+v", ts)
	}
	if ts, _ := s.Threads(ctx, ListOpts{Tag: "CI"}); len(ts) != 1 {
		t.Errorf("by tag: %+v", ts)
	}
	if ts, _ := s.Threads(ctx, ListOpts{Author: worker.Author}); len(ts) != 2 {
		t.Errorf("by author (opened or replied): %+v", ts)
	}
	if ts, _ := s.Threads(ctx, ListOpts{}); len(ts) != 2 || ts[0].ID != 2 {
		t.Errorf("newest activity first: %+v", ts)
	}
	bs, _ := s.Boards(ctx)
	for _, b := range bs {
		if b.Slug == "tooling" && (b.Threads != 1 || b.Posts != 2 || b.Last == nil) {
			t.Errorf("tooling board: %+v", b)
		}
	}
	st, err := s.Stats(ctx)
	if err != nil || st.Threads != 2 || st.Posts != 3 || st.Posters != 2 || st.Newest != worker.Author || len(st.Online) != 2 {
		t.Errorf("stats: %+v %v", st, err)
	}
	on, _ := s.Online(ctx, time.Minute)
	if len(on) != 2 || on[0].Author != worker.Author || on[0].Doing != "Lunch" {
		t.Errorf("online: %+v", on)
	}
	prof, _ := s.Profiles(ctx, []string{owner.Author, worker.Author})
	if prof[owner.Author].Posts != 1 || prof[owner.Author].Useful != 2 || prof[worker.Author].Posts != 2 {
		t.Errorf("profiles: %+v", prof)
	}

	// The format is plain JSON an operator can read.
	var doc map[string]any
	b, _ := os.ReadFile(filepath.Join(dir, "threads", "1.json"))
	if err := json.Unmarshal(b, &doc); err != nil || doc["title"] != "Port 5432 collides" || len(doc["replies"].([]any)) != 1 {
		t.Fatalf("thread file: %s %v", b, err)
	}
}

func TestFilesSearchIsLexicalWithFilters(t *testing.T) {
	s, _ := files(t)
	ctx := context.Background()
	alice := Poster{Author: "alice", Role: "worker"}
	bob := Poster{Author: "bob", Role: "human"}
	pg, _ := s.CreateThread(ctx, NewThread{Board: "tooling", Title: "Postgres port collides in CI", Tags: []string{"ci", "postgres"}, Body: "Port 5432 is already allocated when two jobs run.", Poster: alice})
	s.Reply(ctx, NewPost{ThreadID: pg.ID, Body: "Use a random host port; the collision goes away.", Poster: bob})
	cache, _ := s.CreateThread(ctx, NewThread{Board: "learnings", Title: "Go build cache", Tags: []string{"go"}, Body: "Clearing the module cache fixed the flaky build.", Poster: bob})
	s.CreateThread(ctx, NewThread{Board: "watercooler", Title: "Lunch", Body: "Tacos on Friday.", Poster: alice})

	hits, err := s.Search(ctx, SearchOpts{Q: "postgres ports"})
	if err != nil || len(hits) != 1 || hits[0].Thread.ID != pg.ID || hits[0].PostN != 1 {
		t.Fatalf("plural folding and title: %+v %v", hits, err)
	}
	if hits, _ := s.Search(ctx, SearchOpts{Q: "port allocated"}); len(hits) == 0 || !strings.Contains(hits[0].Snippet, "[Port]") {
		t.Fatalf("snippet marks the words: %+v", hits)
	}
	if hits[0].Post.ID != "t:1" || hits[0].Post.Author != "alice" || hits[0].Thread.Title == "" {
		t.Fatalf("hit shape: %+v", hits[0])
	}
	if hits, _ := s.Search(ctx, SearchOpts{Q: "random host"}); len(hits) != 1 || hits[0].PostN != 2 || hits[0].Post.ID != "p:1:2" {
		t.Fatalf("a reply is a hit: %+v", hits)
	}
	if hits, _ := s.Search(ctx, SearchOpts{Q: "nothing matches zebra"}); len(hits) != 0 {
		t.Fatalf("no match: %+v", hits)
	}
	if hits, _ := s.Search(ctx, SearchOpts{Q: "zebra", Also: []string{"module cache"}}); len(hits) != 1 || hits[0].Thread.ID != cache.ID {
		t.Fatalf("--also: %+v", hits)
	}
	for name, c := range map[string]struct {
		o    SearchOpts
		want int
	}{
		"board":          {SearchOpts{Q: "port cache", Board: "learnings"}, 1},
		"tag":            {SearchOpts{Q: "port cache", Tags: []string{"CI"}}, 1},
		"every tag":      {SearchOpts{Q: "port cache", Tags: []string{"ci", "go"}}, 0},
		"by":             {SearchOpts{Q: "port", Author: "bob"}, 1},
		"role":           {SearchOpts{Q: "port", Role: "worker"}, 1},
		"top":            {SearchOpts{Q: "port cache tacos", Limit: 2}, 2},
		"since":          {SearchOpts{Q: "port", Since: "1h"}, 1},
		"also widens":    {SearchOpts{Q: "tacos", Also: []string{"cache"}}, 2},
		"empty question": {SearchOpts{Q: "  "}, 0},
	} {
		hits, err := s.Search(ctx, c.o)
		if err != nil || len(hits) != c.want {
			t.Errorf("%s: %d hits, want %d (%v)", name, len(hits), c.want, err)
		}
	}
	if _, err := s.Search(ctx, SearchOpts{Q: "x", Board: "nope"}); !errors.Is(err, ErrInvalid) {
		t.Errorf("unknown board: %v", err)
	}
	if _, err := s.Search(ctx, SearchOpts{Q: "x", Since: "soon"}); !errors.Is(err, ErrInvalid) {
		t.Errorf("bad since: %v", err)
	}
	if _, err := s.Search(ctx, SearchOpts{Q: "a", Also: strings.Fields("b c d e f g h i")}); !errors.Is(err, ErrInvalid) {
		t.Errorf("too many phrasings: %v", err)
	}
}

// Several stores on one directory stand in for several `hev board` processes:
// each has its own lock handle, exactly as separate processes would.
func TestFilesConcurrentWritersLoseNothing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "board")
	const writers, each = 8, 12
	stores := make([]*Store, writers)
	for i := range stores {
		s, err := NewFiles(dir)
		if err != nil {
			t.Fatal(err)
		}
		stores[i] = s
	}
	ctx := context.Background()
	seed, err := stores[0].CreateThread(ctx, NewThread{Board: "floor", Title: "shared", Body: "reply here", Poster: Poster{Author: "seed"}})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, writers*each*3)
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			s := stores[w]
			for i := 0; i < each; i++ {
				who := Poster{Author: fmt.Sprintf("w%d", w)}
				if _, err := s.CreateThread(ctx, NewThread{Board: "floor", Title: fmt.Sprintf("w%d-%d", w, i), Body: "body", Poster: who}); err != nil {
					errs <- err
				}
				if _, err := s.Reply(ctx, NewPost{ThreadID: seed.ID, Body: fmt.Sprintf("w%d-%d", w, i), Poster: who}); err != nil {
					errs <- err
				}
				if _, err := s.Useful(ctx, seed.ID, who.Author); err != nil {
					errs <- err
				}
				if err := s.View(ctx, seed.ID); err != nil {
					errs <- err
				}
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	s := stores[0]
	ts, err := s.Threads(ctx, ListOpts{Limit: 1000})
	if err != nil || len(ts) != writers*each+1 {
		t.Fatalf("threads: %d, %v", len(ts), err)
	}
	ids := map[int64]bool{}
	titles := map[string]bool{}
	for _, th := range ts {
		ids[th.ID], titles[th.Title] = true, true
	}
	if len(ids) != writers*each+1 || len(titles) != writers*each+1 {
		t.Fatalf("ids %d titles %d: a thread was overwritten", len(ids), len(titles))
	}
	posts, err := s.Posts(ctx, seed.ID)
	if err != nil || len(posts) != writers*each+1 {
		t.Fatalf("replies: %d, %v", len(posts), err)
	}
	bodies := map[string]bool{}
	for i, p := range posts {
		if p.N != i+1 {
			t.Fatalf("post %d has n=%d", i, p.N)
		}
		bodies[p.Body] = true
	}
	if len(bodies) != writers*each+1 {
		t.Fatalf("distinct replies %d", len(bodies))
	}
	th, _ := s.Thread(ctx, seed.ID)
	if th.Replies != writers*each || th.Useful != writers || th.Views != writers*each {
		t.Fatalf("counters: replies %d useful %d views %d", th.Replies, th.Useful, th.Views)
	}
	left, _ := filepath.Glob(filepath.Join(dir, "threads", ".hev-state-*"))
	if len(left) != 0 {
		t.Fatalf("temporary files left: %v", left)
	}
}

func noKey(t *testing.T) {
	t.Helper()
	for _, name := range append(layerenv.Names(), "FACTORY_BOARD_LAYER_API_KEY") {
		t.Setenv(name, "")
		os.Unsetenv(name)
	}
	t.Setenv("HEV_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
}

func TestOpenPicksFilesWithoutAKey(t *testing.T) {
	noKey(t)
	dir := filepath.Join(t.TempDir(), "board")
	settings.Current = settings.Config{Dir: dir}
	t.Cleanup(func() { settings.Current = settings.Config{} })
	s, err := Open()
	if err != nil || s.Info().Mode != "filesystem" || s.Info().Location != dir {
		t.Fatalf("%+v %v", s, err)
	}
	// The board's documented contract: no key is read from the TOML file.
	os.WriteFile(os.Getenv("HEV_CONFIG"), []byte("[layer]\napi_key = \"in-config\"\n"), 0o600)
	if s, err := Open(); err != nil || s.Info().Mode != "filesystem" {
		t.Fatalf("config key: %+v %v", s, err)
	}
}

func TestOpenUsesLayerWithAKeyAndWritesNoFiles(t *testing.T) {
	noKey(t)
	var calls int
	var mu sync.Mutex
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer layer-test-key" || !strings.HasPrefix(r.URL.Path, "/v2/namespaces/board-ns") {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/query") {
			w.Write([]byte(`{"rows":[]}`))
			return
		}
		w.Write([]byte(`{"status":"OK"}`))
	}))
	defer backend.Close()
	dir := filepath.Join(t.TempDir(), "board")
	settings.Current = settings.Config{Dir: dir, Endpoint: backend.URL, Namespace: "board-ns"}
	t.Cleanup(func() { settings.Current = settings.Config{} })
	t.Setenv("FACTORY_BOARD_LAYER_API_KEY", "layer-test-key")
	s, err := Open()
	if err != nil || s.Info().Mode != "layer" || s.Info().KeySource != "FACTORY_BOARD_LAYER_API_KEY" {
		t.Fatalf("%+v %v", s, err)
	}
	if _, err := s.Threads(context.Background(), ListOpts{}); err != nil {
		t.Fatal(err)
	}
	s.CreateThread(context.Background(), NewThread{Title: "t", Body: "b", Poster: Poster{Author: "a"}})
	if calls == 0 {
		t.Fatal("Layer mode did not reach Layer")
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Layer mode touched the board directory: %v", err)
	}
}

func TestOpenRefusesAnUnreadableKey(t *testing.T) {
	noKey(t)
	dir := filepath.Join(t.TempDir(), "board")
	settings.Current = settings.Config{Dir: dir}
	t.Cleanup(func() { settings.Current = settings.Config{} })
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HEVLAYER_API_KEY_OP_REF", "op://vault/item/field")
	if s, err := Open(); err == nil {
		t.Fatalf("fell back to %s", s.Info().Mode)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("an unreadable key created the board directory: %v", err)
	}
}
