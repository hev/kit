package board_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/hev/kit/internal/board/settings"
	"github.com/hev/kit/pkg/board"
)

type memoryBlobs struct {
	mu   sync.Mutex
	data map[string][]byte
}

func (b *memoryBlobs) MaxBytes() int64 { return 10 << 20 }
func (b *memoryBlobs) Check(v []byte) (board.BlobMeta, error) {
	if int64(len(v)) > b.MaxBytes() {
		return board.BlobMeta{}, board.ErrBlobTooLarge
	}
	return board.InspectImage(v)
}
func (b *memoryBlobs) Put(v []byte) (board.BlobMeta, bool, error) {
	m, e := b.Check(v)
	if e != nil {
		return m, false, e
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.data == nil {
		b.data = map[string][]byte{}
	}
	_, exists := b.data[m.SHA256]
	b.data[m.SHA256] = bytes.Clone(v)
	return m, !exists, nil
}
func (b *memoryBlobs) PutReader(r io.Reader) (board.BlobMeta, bool, error) {
	v, e := io.ReadAll(io.LimitReader(r, b.MaxBytes()+1))
	if e != nil {
		return board.BlobMeta{}, false, e
	}
	return b.Put(v)
}
func (b *memoryBlobs) Get(h string) ([]byte, board.BlobMeta, error) {
	b.mu.Lock()
	v, ok := b.data[h]
	b.mu.Unlock()
	if !ok {
		return nil, board.BlobMeta{}, board.ErrBlobNotFound
	}
	m, e := b.Check(v)
	return bytes.Clone(v), m, e
}
func (b *memoryBlobs) Stat(h string) (board.BlobMeta, error) { _, m, e := b.Get(h); return m, e }

type memoryCanvases struct {
	mu       sync.Mutex
	files    map[int64]fstest.MapFS
	versions map[int64]int
}

func (c *memoryCanvases) Save(id int64, files map[string][]byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.files == nil {
		c.files = map[int64]fstest.MapFS{}
		c.versions = map[int64]int{}
	}
	f := fstest.MapFS{}
	for k, v := range files {
		f[k] = &fstest.MapFile{Data: bytes.Clone(v)}
	}
	c.files[id] = f
	c.versions[id]++
	return c.versions[id], nil
}
func (c *memoryCanvases) Open(id int64, name string) (fs.File, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.files[id].Open(name)
}
func request(h http.Handler, method, path, ct, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if ct != "" {
		r.Header.Set("Content-Type", ct)
	}
	r.Header.Set("X-Author", "forged")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func hosted(t *testing.T, st board.Backend, ro bool, csrf func(*http.Request) error) (http.Handler, *memoryCanvases) {
	t.Helper()
	c := &memoryCanvases{}
	h, e := board.NewHosted(st, board.HostedOptions{BasePath: "/team/board", Principal: func(*http.Request) (board.Poster, error) {
		return board.Poster{Author: "Ada", Role: "human", Host: "team"}, nil
	}, ReadOnly: ro, CSRF: csrf, Blobs: &memoryBlobs{}, Canvases: c})
	if e != nil {
		t.Fatal(e)
	}
	return h, c
}
func store(t *testing.T) *board.Store {
	t.Helper()
	st, e := board.OpenFiles(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	return st
}
func allow(*http.Request) error { return nil }
func seed(t *testing.T, st *board.Store, title string) {
	t.Helper()
	_, e := st.CreateThread(context.Background(), board.NewThread{Board: "tooling", Title: title, Body: "fixture [thread](/t/1) [canvas](/c/1/)", Poster: board.Poster{Author: "Ada", Role: "human"}})
	if e != nil {
		t.Fatal(e)
	}
}

func TestHostedBoardTeamIsolationAndPrincipal(t *testing.T) {
	a, b := store(t), store(t)
	ha, _ := hosted(t, a, false, allow)
	hb, _ := hosted(t, b, false, allow)
	for _, in := range []struct{ p, ct, b string }{{"/api/threads", "application/json", `{"board":"tooling","title":"private A","body":"text","author":"forged","host":"forged","role":"agent"}`}, {"/new", "application/x-www-form-urlencoded", "board=tooling&title=form+A&body=text&author=forged"}} {
		w := request(ha, "POST", "/team/board"+in.p, in.ct, in.b)
		if w.Code != 201 && w.Code != 303 {
			t.Fatal(w.Code, w.Body.String())
		}
		if w.Code == 303 && !strings.HasPrefix(w.Header().Get("Location"), "/team/board/t/") {
			t.Fatal(w.Header())
		}
	}
	for _, in := range []struct{ p, ct, b string }{{"/api/threads/1/posts", "application/json", `{"body":"api reply","author":"forged"}`}, {"/t/1/reply", "application/x-www-form-urlencoded", "body=form+reply&author=forged"}, {"/t/1/useful", "application/x-www-form-urlencoded", "author=forged"}, {"/api/threads/1/useful", "application/json", `{"author":"forged"}`}} {
		w := request(ha, "POST", "/team/board"+in.p, in.ct, in.b)
		if w.Code >= 400 {
			t.Fatal(in.p, w.Code, w.Body.String())
		}
	}
	posts, e := a.Posts(context.Background(), 1)
	if e != nil {
		t.Fatal(e)
	}
	for _, p := range posts {
		if p.Author != "Ada" || p.Host != "team" || p.Role != "human" {
			t.Fatalf("forged poster: %+v", p)
		}
	}
	w := request(hb, "GET", "/team/board/api/threads", "", "")
	if w.Code != 200 || strings.Contains(w.Body.String(), "private A") {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(hb, "GET", "/team/board/t/1", "", "")
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
}
func TestHostedBoardReadOnlyEveryWriteAndReads(t *testing.T) {
	st := store(t)
	seed(t, st, "fixture")
	h, c := hosted(t, st, true, nil)
	_, _ = c.Save(1, map[string][]byte{"index.html": []byte("<h1>canvas</h1>")})
	for _, p := range []string{"/new", "/t/1/reply", "/t/1/useful", "/api/threads", "/api/threads/1/posts", "/api/threads/1/useful", "/api/threads/1/pin", "/api/threads/1/lock", "/api/canvas", "/api/blobs"} {
		for _, method := range []string{"POST", "PUT", "PATCH", "DELETE"} {
			w := request(h, method, "/team/board"+p, "application/json", `{}`)
			var e map[string]string
			if w.Code != 402 || json.Unmarshal(w.Body.Bytes(), &e) != nil || e["error"] == "" || w.Header().Get("Content-Type") != "application/json" {
				t.Fatal(method, p, w.Code, w.Body.String())
			}
		}
	}
	for _, p := range []string{"/", "/recent", "/b/tooling", "/t/1", "/u/Ada", "/search", "/new", "/api/threads", "/api/threads/1", "/api/boards", "/api/search", "/api/stats", "/api/health", "/api/storage", "/api/users/Ada", "/api/online", "/api/blobs", "/api", "/c/1/", "/c/_/canvas.css", "/static/board.css", "/static/avatars/agent.svg"} {
		w := request(h, "GET", "/team/board"+p, "", "")
		if w.Code != 200 {
			t.Fatal(p, w.Code, w.Body.String())
		}
	}
	th, e := st.Thread(context.Background(), 1)
	if e != nil || th.Views != 0 {
		t.Fatalf("read mutated store: %+v %v", th, e)
	}
}
func TestHostedBoardBasePathAndSafeOutput(t *testing.T) {
	st := store(t)
	seed(t, st, "fixture")
	h, _ := hosted(t, st, false, allow)
	urls := regexp.MustCompile(`(?:href|src|action)="([^"]*)"`)
	for _, p := range []string{"/", "/recent", "/b/tooling", "/t/1", "/u/Ada", "/search", "/new", "/api/threads", "/api/storage"} {
		w := request(h, "GET", "/team/board"+p, "", "")
		if w.Code != 200 {
			t.Fatal(p, w.Code, w.Body.String())
		}
		s := w.Body.String()
		for _, bad := range []string{"localhost", "127.0.0.1", "/home/", "/Users/", "wire-secret-team", "hk_secret", "tpuf_"} {
			if strings.Contains(s, bad) {
				t.Fatal(p, "leaked", bad)
			}
		}
		for _, m := range urls.FindAllStringSubmatch(s, -1) {
			if strings.HasPrefix(m[1], "/") && !strings.HasPrefix(m[1], "/team/board/") {
				t.Fatal(p, "unprefixed", m[1])
			}
		}
		if strings.Contains(s, `name="author" placeholder`) {
			t.Fatal("hosted author input")
		}
	}
}
func TestHostedBoardCSRFAndCanvasIdentity(t *testing.T) {
	st := store(t)
	h, c := hosted(t, st, false, func(*http.Request) error { return errors.New("bad token") })
	if w := request(h, "POST", "/team/board/new", "application/x-www-form-urlencoded", "board=tooling&title=no&body=no"); w.Code != 403 {
		t.Fatal(w.Code)
	}
	h, c = hosted(t, st, false, allow)
	w := request(h, "POST", "/team/board/api/canvas", "application/json", `{"title":"Canvas","files":{"index.html":"PGgxPmhpPC9oMT4="},"poster":{"author":"forged"}}`)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	th, e := st.Thread(context.Background(), 1)
	if e != nil || th.Author != "Ada" {
		t.Fatal(th, e)
	}
	if !strings.Contains(w.Body.String(), `"path": "/team/board/c/1/"`) {
		t.Fatal(w.Body.String())
	}
	f, e := c.Open(1, "index.html")
	if e != nil {
		t.Fatal(e)
	}
	f.Close()
}

type failingBackend struct{ board.Backend }

func (f failingBackend) Threads(context.Context, board.ListOpts) ([]board.Thread, error) {
	return nil, errors.New("/home/private hk_secret wire-secret-team http://localhost:1234")
}
func TestHostedBoardHidesBackendErrors(t *testing.T) {
	h, _ := hosted(t, failingBackend{store(t)}, false, allow)
	for _, p := range []string{"/recent", "/api/threads"} {
		w := request(h, "GET", "/team/board"+p, "", "")
		if w.Code != 500 || w.Body.String() != `{"error":"Internal Server Error"}` {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}

func TestHostedBoardImageMetadataPrefix(t *testing.T) {
	st := store(t)
	h, _ := hosted(t, st, false, allow)
	// A one-pixel GIF is accepted by the real inspector used by our memory store.
	data := []byte{'G', 'I', 'F', '8', '9', 'a', 1, 0, 1, 0, 0, 0, 0}
	r := httptest.NewRequest("POST", "/team/board/api/blobs", bytes.NewReader(data))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var uploaded struct {
		SHA256 string `json:"sha256"`
		URL    string `json:"url"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &uploaded); e != nil {
		t.Fatal(e)
	}
	if !strings.HasPrefix(uploaded.URL, "/team/board/blob/") {
		t.Fatal(uploaded.URL)
	}
	w = request(h, "POST", "/team/board/api/threads", "application/json", `{"board":"tooling","title":"image","body":"image","images":[{"sha256":"`+uploaded.SHA256+`"}]}`)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, p := range []string{"/api/threads/1", "/t/1"} {
		w = request(h, "GET", "/team/board"+p, "", "")
		if w.Code != 200 || strings.Contains(w.Body.String(), `"/blob/`) {
			t.Fatal(p, w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "/team/board/blob/") {
			t.Fatal("missing image URL", p, w.Body.String())
		}
	}
}

func TestHostedBoardIgnoresLocalConfiguration(t *testing.T) {
	old := settings.Current
	t.Cleanup(func() { settings.Current = old })
	settings.Current = settings.Config{ReadOnly: true, MaxImageBytes: 1, CanvasDir: "/home/private", PublicURL: "http://localhost:9999", Roles: []settings.Role{{Pattern: ".*", Role: "agent"}}}
	t.Setenv("FACTORY_BOARD_CORS", "https://untrusted.example")
	t.Setenv("FACTORY_CANVAS_DIR", "/home/private")
	t.Setenv("FACTORY_BOARD_PUBLIC_URL", "http://localhost:9999")
	st := store(t)
	h, _ := hosted(t, st, false, allow)
	w := request(h, "POST", "/team/board/new", "application/x-www-form-urlencoded", "board=tooling&title=fixture&body=hello&author=forged")
	if w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(h, "GET", "/team/board/t/1", "", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `role-human`) {
		t.Fatal(w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "localhost") {
		t.Fatal("local settings leaked")
	}
	r := httptest.NewRequest("GET", "/team/board/api/threads", nil)
	r.Header.Set("Origin", "https://untrusted.example")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal(w.Header())
	}
}
func TestHostedBoardAuthenticatesEveryRoute(t *testing.T) {
	opts := board.HostedOptions{BasePath: "/board", ReadOnly: true, Principal: func(*http.Request) (board.Poster, error) { return board.Poster{}, errors.New("not signed in") }, Blobs: &memoryBlobs{}, Canvases: &memoryCanvases{}}
	h, e := board.NewHosted(store(t), opts)
	if e != nil {
		t.Fatal(e)
	}
	for _, p := range []string{"/", "/api/threads", "/static/board.css", "/c/_/canvas.js"} {
		for _, method := range []string{"GET", "POST"} {
			w := request(h, method, "/board"+p+"?author=Ada&principal=Ada", "application/json", `{"author":"Ada"}`)
			if w.Code != 401 {
				t.Fatal(method, p, w.Code)
			}
		}
	}
}

func TestHostedBoardMarkdownLinksRespectBasePath(t *testing.T) {
	st := store(t)
	_, e := st.CreateThread(context.Background(), board.NewThread{Board: "tooling", Title: "links", Body: "[thread](/t/1) [canvas](/c/1/)", Poster: board.Poster{Author: "Ada"}})
	if e != nil {
		t.Fatal(e)
	}
	h, _ := hosted(t, st, false, allow)
	w := request(h, "GET", "/team/board/t/1", "", "")
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	for _, p := range []string{`href="/team/board/t/1"`, `href="/team/board/c/1/"`} {
		if !strings.Contains(w.Body.String(), p) {
			t.Fatal("unprefixed markdown", p, w.Body.String())
		}
	}
}
