package server

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/board/store"
)

func TestCleanRelKeepsPagesInsideTheirCanvas(t *testing.T) {
	for p, ok := range map[string]bool{
		"index.html": true, "img/a.png": true, "data/x.json": true,
		"": false, "/etc/passwd": false, "../1/index.html": false, "a/../../b": false,
		"./index.html": false, "a//b": false, `a\b`: false, "..": false,
	} {
		if got := cleanRel(p) == p && p != ""; got != ok {
			t.Errorf("cleanRel(%q) accepted=%v, want %v", p, got, ok)
		}
	}
}

func TestCanvasFilesAreServedSandboxed(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "7", "img"), 0o755)
	os.WriteFile(filepath.Join(dir, "7", "index.html"), []byte("<h1>hi</h1>"), 0o644)
	os.WriteFile(filepath.Join(dir, "7", "img", "a.svg"), []byte("<svg/>"), 0o644)
	os.WriteFile(filepath.Join(dir, "7", ".version"), []byte("1\n"), 0o644)
	t.Setenv("FACTORY_CANVAS_DIR", dir)
	s, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	h := s.Handler()
	get := func(p string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", p, nil))
		return w
	}
	if w := get("/c/7/"); w.Code != 200 || !strings.Contains(w.Body.String(), "hi") ||
		!strings.HasPrefix(w.Header().Get("Content-Security-Policy"), "sandbox ") ||
		strings.Contains(w.Header().Get("Content-Security-Policy"), "allow-same-origin") {
		t.Fatalf("index: %d %q csp=%q", w.Code, w.Body.String(), w.Header().Get("Content-Security-Policy"))
	}
	if w := get("/c/7/img/a.svg"); w.Code != 200 || !strings.Contains(w.Header().Get("Content-Type"), "svg") {
		t.Errorf("asset: %d %q", w.Code, w.Header().Get("Content-Type"))
	}
	for _, p := range []string{"/c/7/.version", "/c/7/nope.html", "/c/x/", "/c/7/img/"} {
		if w := get(p); w.Code != 404 {
			t.Errorf("%s: %d, want 404", p, w.Code)
		}
	}
	if w := get("/c/7"); w.Code != 301 {
		t.Errorf("/c/7: %d, want a redirect to /c/7/", w.Code)
	}
	if w := get("/c/_/canvas.css"); w.Code != 200 || !strings.Contains(w.Body.String(), "--signal") {
		t.Errorf("canvas.css: %d", w.Code)
	}
	if w := get("/c/_/template.html"); w.Code != 200 || !strings.Contains(w.Body.String(), "/c/_/canvas.js") {
		t.Errorf("template: %d", w.Code)
	}
}

func TestCanvasBoardIsAGallery(t *testing.T) {
	s, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	render := func(slug string) string {
		var b strings.Builder
		err := s.tmpl.ExecuteTemplate(&b, "board.html", map[string]any{
			"F": frame{Title: "x"}, "Board": store.Board{Slug: slug, Name: slug},
			"Threads": []store.Thread{{ID: 171, Board: slug, Title: "A job through the board", Replies: 1}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return b.String()
	}
	if got := render("canvas"); !strings.Contains(got, `src="/c/171/"`) || !strings.Contains(got, "1 reply") {
		t.Errorf("canvas board draws no preview of canvas 171:\n%s", got)
	}
	if got := render("tooling"); strings.Contains(got, "/c/171/") {
		t.Error("another board drew canvas previews")
	}
}
