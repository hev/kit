package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hev/kit/internal/blob"
	boardserver "github.com/hev/kit/internal/board/server"
	"github.com/hev/kit/internal/board/settings"
	"github.com/hev/kit/internal/board/store"
)

func boardPNG(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 6, 4))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// fileBoard is a filesystem board in a scratch directory: a memo with a
// reply that carries an image, and a tooling thread.
func fileBoard(t *testing.T, seed bool) (*store.Store, string) {
	t.Helper()
	dir := t.TempDir()
	old := settings.Current
	settings.Current = settings.Config{Dir: dir, CanvasDir: filepath.Join(t.TempDir(), "canvas")}
	t.Cleanup(func() { settings.Current = old })
	st, err := store.NewFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !seed {
		return st, ""
	}
	ctx := context.Background()
	who := store.Poster{Author: "worker-kit", Role: "worker", Host: "mini"}
	memo, err := st.CreateThread(ctx, store.NewThread{Board: "memos", Title: "Keep Asks out of OSS", Tags: []string{"kit", "ui"}, Body: "The **board** replaces asks in hev serve.", Poster: who})
	if err != nil {
		t.Fatal(err)
	}
	m, _, err := blob.New(boardserver.BlobDir(), 0).Put(boardPNG(t))
	if err != nil {
		t.Fatal(err)
	}
	img := store.Image{SHA256: m.SHA256, Name: "shot.png", Type: m.Type, Size: m.Size, Width: m.Width, Height: m.Height}
	if _, err := st.Reply(ctx, store.NewPost{ThreadID: memo.ID, Body: "Screenshot attached.", Images: []store.Image{img}, Poster: who}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateThread(ctx, store.NewThread{Board: "tooling", Title: "Port 7700 collides", Tags: []string{"ports"}, Body: "Use another port.", Poster: who}); err != nil {
		t.Fatal(err)
	}
	return st, m.SHA256
}

func localBoardServer(t *testing.T, st *store.Store) http.Handler {
	t.Helper()
	api, err := boardserver.New(st)
	if err != nil {
		t.Fatal(err)
	}
	return New(&MemoryReader{}).WithBoard(LocalBoard(api.Handler(), st.Info().Mode)).Handler()
}

func get(t *testing.T, h http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	return w
}

func decodeAs[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %q: %v", w.Body.String(), err)
	}
	return v
}

func threadTitles(ts []store.Thread) []string {
	out := []string{}
	for _, t := range ts {
		out = append(out, t.Title)
	}
	return out
}

// Rule: the page offers two sections, Traces and Board, and nothing of Asks.
func TestNavigationIsTracesAndBoard(t *testing.T) {
	st, _ := fileBoard(t, false)
	h := localBoardServer(t, st)
	body := get(t, h, "GET", "/").Body.String()
	nav := body[strings.Index(body, `<nav class="tabs" id="tabs"`):]
	nav = nav[:strings.Index(nav, "</nav>")]
	for _, want := range []string{`data-v="traces">Traces<`, `data-v="board" hidden>Board<`} {
		if !strings.Contains(nav, want) {
			t.Errorf("navigation lacks %s:\n%s", want, nav)
		}
	}
	if n := strings.Count(nav, "<button"); n != 2 {
		t.Errorf("navigation has %d sections, want Traces and Board only:\n%s", n, nav)
	}
	if !strings.Contains(body, `"board":{"enabled":true,"mode":"filesystem"}`) {
		t.Error("page data does not enable the Board tab")
	}
	lower := strings.ToLower(body)
	for _, gone := range []string{"/api/asks", "v-asks", "v-ask\"", "askview", "asksview", "askhref", ">asks<", "trace_context"} {
		if strings.Contains(lower, gone) {
			t.Errorf("page still carries Asks code %q", gone)
		}
	}
	for _, path := range []string{"/api/asks", "/api/asks/abc", "/asks", "/ask/abc"} {
		if w := get(t, h, "GET", path); w.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, w.Code)
		}
	}
	// Without a board (the hosted dashboard) the tab stays hidden and its API is absent.
	bare := New(&MemoryReader{}).Handler()
	if b := get(t, bare, "GET", "/").Body.String(); !strings.Contains(b, `"board":{"enabled":false}`) {
		t.Error("a server without a board still enables the Board tab")
	}
	if w := get(t, bare, "GET", "/api/board/boards"); w.Code != http.StatusNotFound {
		t.Errorf("board API without a board = %d, want 404", w.Code)
	}
}

// Rule: a wrapper's tabs appear after Traces and Board; unsafe links are dropped.
func TestWrapperTabs(t *testing.T) {
	h := New(&MemoryReader{}).WithTabs(
		Tab{Label: "Orders", Href: "/pro/orders"},
		Tab{Label: "<Keys>", Href: "https://pro.example/keys?a=1&b=2"},
		Tab{Label: "Evil", Href: "javascript:alert(1)"},
		Tab{Label: "Proto", Href: "//evil.example/x"},
		Tab{Label: " ", Href: "/blank"},
	).Handler()
	body := get(t, h, "GET", "/").Body.String()
	for _, want := range []string{`<a class="tab" href="/pro/orders">Orders</a>`, `<a class="tab" href="https://pro.example/keys?a=1&amp;b=2">&lt;Keys&gt;</a>`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing wrapper tab %s", want)
		}
	}
	for _, refused := range []string{"javascript:", "evil.example", "/blank"} {
		if strings.Contains(body, refused) {
			t.Errorf("unsafe tab %q rendered", refused)
		}
	}
}

// Rule: against a filesystem board, the tab lists, filters by board and by
// tag, searches, and reads a whole thread with replies and images.
func TestBoardOverLocalFiles(t *testing.T) {
	st, sha := fileBoard(t, true)
	h := localBoardServer(t, st)

	boards := decodeAs[[]store.Board](t, get(t, h, "GET", "/api/board/boards"))
	counts := map[string]int{}
	for _, b := range boards {
		counts[b.Slug] = b.Threads
	}
	if counts["memos"] != 1 || counts["tooling"] != 1 || counts["floor"] != 0 {
		t.Errorf("board counts = %v", counts)
	}

	all := decodeAs[[]store.Thread](t, get(t, h, "GET", "/api/board/threads"))
	if len(all) != 2 {
		t.Fatalf("threads = %v", threadTitles(all))
	}
	memos := decodeAs[[]store.Thread](t, get(t, h, "GET", "/api/board/threads?board=memos"))
	if got := threadTitles(memos); len(got) != 1 || got[0] != "Keep Asks out of OSS" {
		t.Errorf("board=memos kept %v", got)
	}
	tagged := decodeAs[[]store.Thread](t, get(t, h, "GET", "/api/board/threads?tag=ports"))
	if got := threadTitles(tagged); len(got) != 1 || got[0] != "Port 7700 collides" {
		t.Errorf("tag=ports kept %v", got)
	}
	if none := decodeAs[[]store.Thread](t, get(t, h, "GET", "/api/board/threads?board=floor")); len(none) != 0 {
		t.Errorf("empty board listed %v", threadTitles(none))
	}
	if none := decodeAs[[]store.Thread](t, get(t, h, "GET", "/api/board/threads?board=memos&tag=ports")); len(none) != 0 {
		t.Errorf("board and tag together kept %v", threadTitles(none))
	}

	hits := decodeAs[[]store.Hit](t, get(t, h, "GET", "/api/board/search?q=screenshot"))
	if len(hits) != 1 || hits[0].Thread.Title != "Keep Asks out of OSS" || hits[0].PostN != 2 {
		t.Errorf("search hits = %+v", hits)
	}
	if hits := decodeAs[[]store.Hit](t, get(t, h, "GET", "/api/board/search?q=screenshot&board=tooling")); len(hits) != 0 {
		t.Errorf("search with another board kept %+v", hits)
	}
	if hits := decodeAs[[]store.Hit](t, get(t, h, "GET", "/api/board/search?q=port&tag=ports")); len(hits) != 1 {
		t.Errorf("search with tag = %+v", hits)
	}
	if hits := decodeAs[[]store.Hit](t, get(t, h, "GET", "/api/board/search?q=nothingmatchesthis")); len(hits) != 0 {
		t.Errorf("unmatched search = %+v", hits)
	}
	if w := get(t, h, "GET", "/api/board/search?q=+"); w.Code != http.StatusBadRequest {
		t.Errorf("blank search = %d, want 400", w.Code)
	}

	read := decodeAs[struct {
		Thread store.Thread `json:"thread"`
		Posts  []store.Post `json:"posts"`
	}](t, get(t, h, "GET", "/api/board/threads/1"))
	if read.Thread.Title != "Keep Asks out of OSS" || len(read.Posts) != 2 || read.Posts[1].Body != "Screenshot attached." {
		t.Fatalf("thread = %+v", read)
	}
	if ims := read.Posts[1].Images; len(ims) != 1 || ims[0].SHA256 != sha || ims[0].Name != "shot.png" {
		t.Errorf("reply images = %+v", ims)
	}
	// Reading in hev serve never counts as a view.
	if th, _ := st.Thread(context.Background(), 1); th.Views != 0 {
		t.Errorf("reading counted %d views", th.Views)
	}

	img := get(t, h, "GET", "/api/board/blob/"+sha)
	if img.Code != 200 || img.Header().Get("Content-Type") != "image/png" || img.Header().Get("X-Content-Type-Options") != "nosniff" || !bytes.Equal(img.Body.Bytes(), boardPNG(t)) {
		t.Errorf("image = %d %v", img.Code, img.Header())
	}
}

// Rule: missing, unknown and refused board requests answer 404/405 with a
// fixed message; only the read routes are reachable and nothing writes.
func TestBoardRefusals(t *testing.T) {
	st, _ := fileBoard(t, false)
	h := localBoardServer(t, st)

	// An empty board is a valid, empty answer.
	if ts := decodeAs[[]store.Thread](t, get(t, h, "GET", "/api/board/threads")); len(ts) != 0 {
		t.Errorf("empty board listed %v", ts)
	}
	if bs := decodeAs[[]store.Board](t, get(t, h, "GET", "/api/board/boards")); len(bs) == 0 {
		t.Error("an empty board lists no boards")
	}
	for path, want := range map[string]int{
		"/api/board/threads/1":                       404, // no such thread
		"/api/board/threads/abc":                     404,
		"/api/board/threads/1/posts":                 404,
		"/api/board/blob/" + strings.Repeat("a", 64): 404, // not stored
		"/api/board/blob/ABC":                        404,
		"/api/board/canvas":                          404,
		"/api/board/stats":                           404,
		"/api/board/online":                          404,
		"/api/board/users/x":                         404,
		"/api/board/":                                404,
	} {
		w := get(t, h, "GET", path)
		if w.Code != want {
			t.Errorf("GET %s = %d, want %d", path, w.Code, want)
		}
		if strings.Contains(w.Body.String(), st.Info().Location) {
			t.Errorf("GET %s leaks the board directory: %s", path, w.Body.String())
		}
	}
	// A path that climbs out is cleaned by the mux and never reaches the board.
	if w := get(t, h, "GET", "/api/board/blob/../../etc/passwd"); w.Code == 200 || strings.Contains(w.Body.String(), "root:") {
		t.Errorf("climbing path = %d", w.Code)
	}
	for _, path := range []string{"/api/board/threads", "/api/board/threads/1/posts", "/api/board/blobs"} {
		if w := get(t, h, "POST", path); w.Code != http.StatusMethodNotAllowed && w.Code != http.StatusNotFound {
			t.Errorf("POST %s = %d, want refused", path, w.Code)
		}
	}
	if ts, _ := st.Threads(context.Background(), store.ListOpts{}); len(ts) != 0 {
		t.Errorf("a refused write created %v", ts)
	}
}

// Rule: board errors reach the page as fixed messages, never with the
// store's paths, endpoints or keys.
func TestBoardErrorsAreRedacted(t *testing.T) {
	secret := "/Users/someone/.hev/board/threads/1.json key=hl_secret_123 https://layer.internal/v2/namespaces/x"
	for _, code := range []int{400, 404, 500, 502, 418} {
		api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(code)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": secret})
		})
		h := New(&MemoryReader{}).WithBoard(LocalBoard(api, "filesystem")).Handler()
		for _, path := range []string{"/api/board/threads", "/api/board/threads/9", "/api/board/search?q=x", "/api/board/boards"} {
			w := get(t, h, "GET", path)
			if w.Code < 400 {
				t.Errorf("%d upstream: GET %s = %d", code, path, w.Code)
			}
			for _, leak := range []string{"/Users", "hl_secret", "layer.internal", ".hev"} {
				if strings.Contains(w.Body.String(), leak) {
					t.Errorf("%d upstream: GET %s leaks %q: %s", code, path, leak, w.Body.String())
				}
			}
		}
	}
	h := New(&MemoryReader{}).WithBoard(UnavailableBoard()).Handler()
	if w := get(t, h, "GET", "/api/board/boards"); w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "board unavailable") {
		t.Errorf("unavailable board = %d %s", w.Code, w.Body.String())
	}
	if b := get(t, h, "GET", "/").Body.String(); !strings.Contains(b, `"board":{"enabled":true,"mode":"unavailable"}`) {
		t.Error("an unavailable board hides the tab instead of saying so")
	}
}

// hostedBoard is a mock board server, as a hosted (Layer) board answers.
func hostedBoard(t *testing.T, sha string, img []byte) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var seen []string
	thread := store.Thread{ID: 7, Board: "learnings", Title: "Hosted thread", Tags: []string{"layer"}, Replies: 1, ImageCount: 1, Poster: store.Poster{Author: "a"}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Method+" "+r.URL.RequestURI())
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query()
		switch {
		case r.URL.Path == "/api/storage":
			_, _ = io.WriteString(w, `{"mode":"layer"}`)
		case r.URL.Path == "/api/boards":
			_ = json.NewEncoder(w).Encode([]store.Board{{Slug: "learnings", Name: "Learnings", Threads: 1}, {Slug: "memos", Name: "Memos"}})
		case r.URL.Path == "/api/threads":
			out := []store.Thread{}
			if (q.Get("board") == "" || q.Get("board") == "learnings") && (q.Get("tag") == "" || q.Get("tag") == "layer") {
				out = append(out, thread)
			}
			_ = json.NewEncoder(w).Encode(out)
		case r.URL.Path == "/api/search":
			out := []store.Hit{}
			if q.Get("q") == "hosted" {
				out = append(out, store.Hit{Thread: thread, PostN: 1, Snippet: "a [hosted] thread"})
			}
			_ = json.NewEncoder(w).Encode(out)
		case r.URL.Path == "/api/threads/7":
			_ = json.NewEncoder(w).Encode(map[string]any{"thread": thread, "posts": []store.Post{
				{N: 1, ThreadID: 7, Body: "opening"},
				{N: 2, ThreadID: 7, Body: "reply", Images: []store.Image{{SHA256: sha, Name: "p.png", Type: "image/png", URL: "/blob/" + sha}}},
			}})
		case r.URL.Path == "/blob/"+sha:
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(img)
		default:
			w.WriteHeader(404)
			_, _ = io.WriteString(w, `{"error":"no such thread at /srv/board on layer.internal"}`)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, func() []string { mu.Lock(); defer mu.Unlock(); return append([]string(nil), seen...) }
}

// Rule: against a hosted board server the tab reads the same shapes, forwards
// only the read parameters, and never counts a view.
func TestBoardOverHostedServer(t *testing.T) {
	img := boardPNG(t)
	sha := strings.Repeat("b", 64)
	srv, seen := hostedBoard(t, sha, img)
	b, err := RemoteBoard(srv.URL+"/", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	h := New(&MemoryReader{}).WithBoard(b).Handler()

	if bs := decodeAs[[]store.Board](t, get(t, h, "GET", "/api/board/boards")); len(bs) != 2 || bs[0].Threads != 1 {
		t.Errorf("boards = %+v", bs)
	}
	if ts := decodeAs[[]store.Thread](t, get(t, h, "GET", "/api/board/threads?board=learnings&tag=layer&author=x&peek=0")); len(ts) != 1 {
		t.Errorf("filtered threads = %v", ts)
	}
	if ts := decodeAs[[]store.Thread](t, get(t, h, "GET", "/api/board/threads?tag=other")); len(ts) != 0 {
		t.Errorf("other tag kept %v", ts)
	}
	if hs := decodeAs[[]store.Hit](t, get(t, h, "GET", "/api/board/search?q=hosted&board=learnings")); len(hs) != 1 {
		t.Errorf("search = %+v", hs)
	}
	read := decodeAs[struct {
		Posts []store.Post `json:"posts"`
	}](t, get(t, h, "GET", "/api/board/threads/7"))
	if len(read.Posts) != 2 || len(read.Posts[1].Images) != 1 {
		t.Errorf("thread = %+v", read)
	}
	if w := get(t, h, "GET", "/api/board/blob/"+sha); w.Code != 200 || !bytes.Equal(w.Body.Bytes(), img) {
		t.Errorf("image = %d", w.Code)
	}
	if st := decodeAs[map[string]any](t, get(t, h, "GET", "/api/board/storage")); st["mode"] != "remote" || st["store"] != "layer" {
		t.Errorf("storage = %v", st)
	}
	w := get(t, h, "GET", "/api/board/threads/8")
	if w.Code != 404 || strings.Contains(w.Body.String(), "/srv") || strings.Contains(w.Body.String(), "layer.internal") {
		t.Errorf("missing hosted thread = %d %s", w.Code, w.Body.String())
	}
	for _, req := range seen() {
		if !strings.HasPrefix(req, "GET ") {
			t.Errorf("non-GET reached the board: %s", req)
		}
		if strings.Contains(req, "author=") || strings.Contains(req, "peek=0") {
			t.Errorf("forwarded a parameter the tab does not use: %s", req)
		}
		if strings.HasPrefix(req, "GET /api/threads/7") && !strings.Contains(req, "peek=1") {
			t.Errorf("thread read counts a view: %s", req)
		}
	}

	// A board server that is down or slow is a fixed 502, without its address.
	sleeper := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer sleeper.Close()
	slow, err := RemoteBoard(sleeper.URL, 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if w := get(t, New(&MemoryReader{}).WithBoard(slow).Handler(), "GET", "/api/board/boards"); w.Code != http.StatusBadGateway {
		t.Errorf("slow board server = %d", w.Code)
	}
	if time.Since(start) > time.Second {
		t.Errorf("slow board server held the request %v", time.Since(start))
	}
	down := httptest.NewServer(http.NotFoundHandler())
	addr := down.URL
	down.Close()
	gone, err := RemoteBoard(addr, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	w = get(t, New(&MemoryReader{}).WithBoard(gone).Handler(), "GET", "/api/board/boards")
	if w.Code != http.StatusBadGateway || strings.Contains(w.Body.String(), strings.TrimPrefix(addr, "http://")) {
		t.Errorf("board server down = %d %s", w.Code, w.Body.String())
	}
	for _, bad := range []string{"", "ftp://x", "http://", "http://h/#f", "http://h/?a=1", "not a url"} {
		if _, err := RemoteBoard(bad, time.Second); err == nil {
			t.Errorf("RemoteBoard(%q) accepted", bad)
		}
	}
}
