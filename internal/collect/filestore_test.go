package collect

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/filearchive"
	"github.com/hev/kit/internal/redact"
)

// newFileHarness is newHarness on the no-key stores: a FileTurns per allowed
// namespace in a file archive under root, and app boards in files there too.
// Unlike leakyIndex, these stores scope reads themselves, so the tests below
// show the store's isolation as well as the server's.
func newFileHarness(t *testing.T) (*harness, string) {
	t.Helper()
	root := t.TempDir()
	scrubber, err := redact.New(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	indexes := map[string]TurnIndex{}
	for _, ns := range []string{nsA, nsB} {
		st, err := filearchive.Open(root, LayerNamespace("", ns))
		if err != nil {
			t.Fatal(err)
		}
		indexes[ns] = &FileTurns{Store: st, Namespace: ns}
	}
	boards, err := NewFileBoards(filepath.Join(root, "hev-app-boards"))
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{t: t, sink: &memSink{}, logs: &bytes.Buffer{}}
	logger := slog.New(slog.NewTextHandler(h.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	h.srv, err = New(Config{Key: testKey, Indexes: indexes, Boards: boards, Sink: h.sink, Scrubber: scrubber, Host: "test-host", Logger: logger})
	if err != nil {
		t.Fatal(err)
	}
	h.http = httptest.NewServer(h.srv.Handler())
	t.Cleanup(h.http.Close)
	return h, root
}

func (h *harness) search(ns, owner, tenant, query string) searchResp {
	h.t.Helper()
	code, body := h.do("POST", "/v1/app/traces/search", testKey, map[string]any{"namespace": ns, "owner": owner, "tenant": tenant, "query": query})
	if code != 200 {
		h.t.Fatalf("search %s/%s %q = %d %s", ns, owner, query, code, body)
	}
	var sr searchResp
	if err := json.Unmarshal([]byte(body), &sr); err != nil {
		h.t.Fatal(err)
	}
	return sr
}

func (h *harness) conversation(ns, owner, tenant, chat string) (int, convResp) {
	h.t.Helper()
	code, body := h.do("GET", "/v1/app/traces/conversations/"+chat+"?namespace="+ns+"&owner="+owner+"&tenant="+tenant, testKey, nil)
	var cr convResp
	if code == 200 {
		if err := json.Unmarshal([]byte(body), &cr); err != nil {
			h.t.Fatal(err)
		}
	}
	return code, cr
}

func turnIDs(sr searchResp) []string {
	out := []string{}
	for _, h := range sr.Hits {
		out = append(out, h.TurnID)
	}
	return out
}

// Two owners share a chat id; each sees only their own turn, by search and
// by conversation fetch, and another tenant sees neither.
func TestFileTurnsOwnerSplit(t *testing.T) {
	h, root := newFileHarness(t)
	h.export(userA)
	h.export(userB)
	for _, c := range []struct{ owner, want, not string }{{"101", "ki-run-1", "ki-run-2"}, {"202", "ki-run-2", "ki-run-1"}} {
		// Both turns' answers have "the window"/"the key": a word in both.
		got := turnIDs(h.search(nsA, c.owner, "story", "the"))
		if len(got) != 1 || got[0] != c.want {
			t.Fatalf("owner %s search = %v, want only %s", c.owner, got, c.want)
		}
		code, cr := h.conversation(nsA, c.owner, "story", "chat-shared")
		if code != 200 || len(cr.Turns) != 1 || cr.Turns[0].TurnID != c.want {
			t.Fatalf("owner %s conversation = %d %+v, want only %s", c.owner, code, cr, c.want)
		}
	}
	if got := turnIDs(h.search(nsA, "101", "story", "noir Chicago")); len(got) != 0 {
		t.Fatalf("owner 101 found owner 202's words: %v", got)
	}
	if got := turnIDs(h.search(nsA, "101", "other", "the")); len(got) != 0 {
		t.Fatalf("another tenant found %v", got)
	}
	if code, _ := h.conversation(nsA, "303", "story", "chat-shared"); code != 404 {
		t.Fatalf("an owner with no turns in the chat = %d, want 404", code)
	}
	// Each owner's turns are their own file: two owners, two files.
	files := chunkFiles(t, filepath.Join(root, "hev-app-traces-"+nsA))
	if len(files) != 2 {
		t.Fatalf("chunk files = %v, want one per owner", files)
	}
}

// The same user and chat in two namespaces: each namespace's search and
// fetch return only that namespace's turn, and each namespace's turns are
// written only in its own archive namespace.
func TestFileTurnsNamespaceIsolation(t *testing.T) {
	h, root := newFileHarness(t)
	h.export(userA)
	inB := userA
	inB.ns, inB.turn, inB.trace, inB.prompt = nsB, "ki-run-b", "3132333435363738393a3b3c3d3e3f40", "Open a heist story in Lisbon, staging copy"
	h.export(inB)
	for ns, want := range map[string]string{nsA: "ki-run-1", nsB: "ki-run-b"} {
		got := turnIDs(h.search(ns, "101", "story", "heist Lisbon"))
		if len(got) != 1 || got[0] != want {
			t.Fatalf("%s search = %v, want only %s", ns, got, want)
		}
		code, cr := h.conversation(ns, "101", "story", "chat-shared")
		if code != 200 || len(cr.Turns) != 1 || cr.Turns[0].TurnID != want {
			t.Fatalf("%s conversation = %d %+v, want only %s", ns, code, cr, want)
		}
		for _, f := range chunkFiles(t, filepath.Join(root, "hev-app-traces-"+ns)) {
			b, _ := os.ReadFile(f)
			if !strings.Contains(string(b), `"namespace":"`+ns+`"`) || strings.Count(string(b), "\n") != 1 {
				t.Fatalf("%s holds %s", f, b)
			}
		}
	}
}

func TestFileTurnsSearchLimitSinceAndRanking(t *testing.T) {
	h, _ := newFileHarness(t)
	h.export(userA)
	second := userA
	second.turn, second.trace, second.prompt, second.answer = "ki-run-3", "2122232425262728292a2b2c2d2e2f30", "Now the Lisbon chase", "The tram tilted downhill."
	second.start = userA.start.Add(time.Hour)
	h.export(second)
	// Both match "Lisbon" equally: newest first. A search needs every word,
	// so "heist Lisbon" finds only the first.
	got := turnIDs(h.search(nsA, "101", "story", "Lisbon"))
	if len(got) != 2 || got[0] != "ki-run-3" {
		t.Fatalf("Lisbon = %v, want both, newest first", got)
	}
	if got := turnIDs(h.search(nsA, "101", "story", "heist Lisbon")); len(got) != 1 || got[0] != "ki-run-1" {
		t.Fatalf("heist Lisbon = %v", got)
	}
	_, body := h.do("POST", "/v1/app/traces/search", testKey, map[string]any{"namespace": nsA, "owner": "101", "tenant": "story", "query": "Lisbon", "limit": 1})
	if strings.Count(body, `"turnId"`) != 1 {
		t.Fatalf("limit 1 = %s", body)
	}
	_, body = h.do("POST", "/v1/app/traces/search", testKey, map[string]any{"namespace": nsA, "owner": "101", "tenant": "story", "query": "Lisbon", "since": "2026-11-17T10:30:00Z"})
	if strings.Contains(body, "ki-run-1") || !strings.Contains(body, "ki-run-3") {
		t.Fatalf("since = %s", body)
	}
	code, cr := h.conversation(nsA, "101", "story", "chat-shared")
	if code != 200 || len(cr.Turns) != 2 || cr.Turns[0].TurnID != "ki-run-1" || cr.Turns[1].TurnID != "ki-run-3" {
		t.Fatalf("conversation = %+v", cr)
	}
	if len(cr.Turns[0].Tools) != 1 || cr.Turns[0].Tools[0] != (Tool{Name: "searchStories", OK: true}) {
		t.Fatalf("tools = %+v", cr.Turns[0].Tools)
	}
}

func TestFileTurnsAPINeedsAnAllowedNamespace(t *testing.T) {
	h, _ := newFileHarness(t)
	h.export(userA)
	for _, c := range []struct {
		body map[string]any
		want int
	}{
		{map[string]any{"owner": "101", "tenant": "story", "query": "heist"}, 400},
		{map[string]any{"namespace": "story-prod", "owner": "101", "tenant": "story", "query": "heist"}, 403},
		{map[string]any{"namespace": "hev-app-traces-story-a", "owner": "101", "tenant": "story", "query": "heist"}, 403},
		{map[string]any{"namespace": nsA, "tenant": "story", "query": "heist"}, 400},
	} {
		if code, body := h.do("POST", "/v1/app/traces/search", testKey, c.body); code != c.want || strings.Contains(body, "ki-run-1") {
			t.Errorf("search %v = %d %s, want %d", c.body, code, body, c.want)
		}
	}
	for q, want := range map[string]int{"": 400, "&namespace=story-prod": 403, "&namespace=hev-traces": 403} {
		if code, body := h.do("GET", "/v1/app/traces/conversations/chat-shared?owner=101&tenant=story"+q, testKey, nil); code != want || strings.Contains(body, "ki-run-1") {
			t.Errorf("conversation %q = %d %s, want %d", q, code, body, want)
		}
	}
}

// Redaction runs before anything reaches the archive: no file under the
// root holds the secret, in turns or in board posts.
func TestFileTurnsRedactedBeforeWrite(t *testing.T) {
	h, root := newFileHarness(t)
	s := userA
	s.prompt = "use my token " + githubToken + " to fetch the draft"
	s.answer = "I used " + githubToken + " and fetched it."
	h.export(s)
	if code, body := h.do("POST", "/v1/app/boards/story-memory/posts", testKey, map[string]any{"author": "101", "title": "key " + githubToken, "body": "remember " + githubToken, "tags": []string{"job:123"}}); code != 200 {
		t.Fatalf("post = %d %s", code, body)
	}
	found := 0
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, _ := os.ReadFile(p)
		if strings.Contains(string(b), "ghp_") {
			t.Errorf("%s holds the secret", p)
		}
		if strings.Contains(string(b), "[REDACTED:") {
			found++
		}
		return nil
	})
	if found < 2 {
		t.Fatalf("redaction markers in %d files, want the turn and the post", found)
	}
}

func TestFileBoards(t *testing.T) {
	h, root := newFileHarness(t)
	code, body := h.do("POST", "/v1/app/boards/story-memory/posts", testKey, map[string]any{"author": "101", "authorName": "Ana", "title": "Second person", "body": "Ana wants second person narration.", "tags": []string{"job:123", "style"}})
	if code != 200 {
		t.Fatalf("post = %d %s", code, body)
	}
	h.do("POST", "/v1/app/boards/story-memory/posts", testKey, map[string]any{"author": "202", "title": "Noir", "body": "Ben wants noir narration.", "tags": []string{"job:456"}})
	if code, body := h.do("POST", "/v1/app/boards/Bad Board/posts", testKey, map[string]any{"author": "a", "title": "t", "body": "b"}); code != 400 {
		t.Fatalf("post to a bad board = %d %s", code, body)
	}
	_, body = h.do("POST", "/v1/app/boards/story-memory/search", testKey, map[string]any{"tags": []string{"job:123"}})
	if !strings.Contains(body, `"authorName":"Ana"`) || strings.Contains(body, "Noir") {
		t.Fatalf("by tag = %s", body)
	}
	_, body = h.do("POST", "/v1/app/boards/story-memory/search", testKey, map[string]any{"query": "narration", "tags": []string{"job:456"}})
	if !strings.Contains(body, "Noir") || strings.Contains(body, "Second person") {
		t.Fatalf("by query and tag = %s", body)
	}
	if _, err := os.Stat(filepath.Join(root, "hev-app-boards", "threads", "1.json")); err != nil {
		t.Fatalf("board thread not in the archive root: %v", err)
	}
}

func chunkFiles(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	_ = filepath.WalkDir(filepath.Join(dir, "chunks"), func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".jsonl") {
			out = append(out, p)
		}
		return nil
	})
	return out
}

// A cold start: the archive namespace is empty (nothing exported yet), or
// its directory is gone altogether (deleted under a running collector, or a
// lazily opened store that has never been written). Search is zero hits and
// a conversation fetch is 404, never an error: the file-archive twin of the
// Layer path's cold-start case.
func TestFileTurnsEmptyOrMissingArchiveIsZeroHits(t *testing.T) {
	h, root := newFileHarness(t)
	check := func(stage string) {
		t.Helper()
		for _, ns := range []string{nsA, nsB} {
			if got := h.search(ns, "101", "story", "heist Lisbon"); len(got.Hits) != 0 {
				t.Fatalf("%s: %s search = %+v, want no hits", stage, ns, got)
			}
			code, body := h.do("POST", "/v1/app/traces/search", testKey, map[string]any{
				"namespace": ns, "owner": "101", "tenant": "story", "query": "heist", "since": "2026-01-01T00:00:00Z"})
			if code != 200 || !strings.Contains(body, `"hits":[]`) {
				t.Fatalf("%s: %s search with since = %d %s, want 200 and an empty hits array", stage, ns, code, body)
			}
			if code, _ := h.conversation(ns, "101", "story", "chat-shared"); code != 404 {
				t.Fatalf("%s: %s conversation = %d, want 404", stage, ns, code)
			}
		}
	}
	check("empty archive")

	// Another namespace's turns don't make this one less empty.
	h.export(userA)
	if got := h.search(nsB, "101", "story", "heist Lisbon"); len(got.Hits) != 0 {
		t.Fatalf("%s found %s's turn: %+v", nsB, nsA, got)
	}

	// The archive directory removed under the running server.
	if err := os.RemoveAll(filepath.Join(root, "hev-app-traces-"+nsA)); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, "hev-app-traces-"+nsB)); err != nil {
		t.Fatal(err)
	}
	check("missing archive")

	// The next export recreates the namespace's archive, and search finds it.
	h.export(userA)
	if got := turnIDs(h.search(nsA, "101", "story", "heist Lisbon")); len(got) != 1 || got[0] != "ki-run-1" {
		t.Fatalf("search after the archive was recreated = %v, want ki-run-1", got)
	}

	// A store opened lazily over a root with no archive at all.
	lazy, err := filearchive.OpenLazy(filepath.Join(t.TempDir(), "never"), LayerNamespace("", nsA))
	if err != nil {
		t.Fatal(err)
	}
	ft := &FileTurns{Store: lazy, Namespace: nsA}
	hits, err := ft.Search(t.Context(), "101", "story", "heist", time.Time{}, 10)
	if err != nil || hits == nil || len(hits) != 0 {
		t.Fatalf("lazy store search = %#v, %v; want an empty, non-nil slice and no error", hits, err)
	}
	turns, err := ft.Conversation(t.Context(), "101", "story", "chat-shared")
	if err != nil || len(turns) != 0 {
		t.Fatalf("lazy store conversation = %v, %v; want none and no error", turns, err)
	}
}
