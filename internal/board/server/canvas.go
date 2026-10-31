package server

import (
	"encoding/json"
	"fmt"
	"github.com/hev/kit/internal/board/settings"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hev/kit/internal/board/store"
)

const MaxCanvas = 32 << 20

const canvasCSP = "sandbox allow-scripts allow-popups allow-popups-to-escape-sandbox allow-forms allow-modals allow-downloads allow-top-navigation-by-user-activation"

func CanvasDir() string {
	if d := settings.Current.CanvasDir; d != "" {
		return d
	}
	if d := os.Getenv("FACTORY_CANVAS_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hev", "canvas")
}

type CanvasIn struct {
	Title   string            `json:"title"`
	Summary string            `json:"summary"`
	Tags    []string          `json:"tags"`
	To      int64             `json:"to,omitempty"`
	Files   map[string][]byte `json:"files"`
	Poster  store.Poster      `json:"poster"`
}

type CanvasOut struct {
	ID      int64  `json:"id"`
	URL     string `json:"url"`     // the page, as the operator's browser reaches it
	Path    string `json:"path"`    // /c/<id>/, relative to the board
	Thread  string `json:"thread"`  // /t/<id>
	Version int    `json:"version"` // 1 for a new canvas
}

func (s *Server) apiCanvas(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var in CanvasIn
	if err := json.NewDecoder(io.LimitReader(r.Body, MaxCanvas*4/3+1<<20)).Decode(&in); err != nil {
		fail(w, fmt.Errorf("%w: bad canvas: %v", store.ErrInvalid, err))
		return
	}
	out, err := s.publish(r, in)
	if err != nil {
		fail(w, err)
		return
	}
	out.URL = publicBase(r) + out.Path
	writeJSON(w, 201, out)
}

func (s *Server) publish(r *http.Request, in CanvasIn) (*CanvasOut, error) {
	ctx := r.Context()
	in.Title = strings.TrimSpace(in.Title)
	in.Summary = strings.TrimSpace(in.Summary)
	if in.To == 0 && in.Title == "" {
		return nil, invalidf("a canvas needs a title")
	}
	if _, ok := in.Files["index.html"]; !ok {
		return nil, invalidf("a canvas needs an index.html")
	}
	total := 0
	for p, b := range in.Files {
		if cleanRel(p) != p {
			return nil, invalidf("%q is not a plain relative path", p)
		}
		total += len(b)
	}
	if total > MaxCanvas {
		return nil, invalidf("%d MB is over the %d MB a canvas may carry", total>>20, MaxCanvas>>20)
	}
	if err := os.MkdirAll(s.canvas, 0o755); err != nil {
		return nil, err
	}
	stage, err := os.MkdirTemp(s.canvas, ".stage-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(stage)
	for p, b := range in.Files {
		dst := filepath.Join(stage, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			return nil, err
		}
	}

	if in.To != 0 {
		t, err := s.st.Thread(ctx, in.To)
		if err != nil {
			return nil, err
		}
		if t.Board != "canvas" {
			return nil, invalidf("#%d is a %s thread, not a canvas", t.ID, t.Board)
		}
		dir := filepath.Join(s.canvas, strconv.FormatInt(t.ID, 10))
		v := versions(dir) + 1
		if _, err := os.Stat(dir); err == nil {
			if err := os.Rename(dir, filepath.Join(s.canvas, fmt.Sprintf(".%d.v%d", t.ID, v-1))); err != nil {
				return nil, err
			}
		}
		if err := os.Rename(stage, dir); err != nil {
			return nil, err
		}
		writeVersion(dir, v)
		body := fmt.Sprintf("**Updated to v%d.** [Open it](/c/%d/)", v, t.ID)
		if in.Summary != "" {
			body += "\n\n" + in.Summary
		}
		if _, err := s.st.Reply(ctx, store.NewPost{ThreadID: t.ID, Body: body, Poster: in.Poster}); err != nil {
			return nil, err
		}
		return &CanvasOut{ID: t.ID, Path: fmt.Sprintf("/c/%d/", t.ID), Thread: fmt.Sprintf("/t/%d", t.ID), Version: v}, nil
	}

	body := in.Summary
	if body == "" {
		body = in.Title
	}
	t, err := s.st.CreateThread(ctx, store.NewThread{
		Board: "canvas", Title: in.Title, Tags: in.Tags, Poster: in.Poster, Body: body,
		Suffix: func(id int64) string { return fmt.Sprintf("\n\n[Open the canvas](/c/%d/)", id) },
	})
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(s.canvas, strconv.FormatInt(t.ID, 10))
	os.RemoveAll(dir) // a canvas left by a thread id that was since reused
	if err := os.Rename(stage, dir); err != nil {
		return nil, err
	}
	writeVersion(dir, 1)
	return &CanvasOut{ID: t.ID, Path: fmt.Sprintf("/c/%d/", t.ID), Thread: fmt.Sprintf("/t/%d", t.ID), Version: 1}, nil
}

func (s *Server) canvasFile(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	rel := r.PathValue("path")
	if rel == "" || strings.HasSuffix(rel, "/") {
		rel += "index.html"
	}
	if err != nil || cleanRel(rel) != rel || strings.HasPrefix(path.Base(rel), ".") {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(filepath.Join(s.canvas, strconv.FormatInt(id, 10), filepath.FromSlash(rel)))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		http.NotFound(w, r)
		return
	}
	h := w.Header()
	h.Set("Content-Security-Policy", canvasCSP)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "no-cache")
	if ct := mime.TypeByExtension(path.Ext(rel)); ct != "" {
		h.Set("Content-Type", ct)
	}
	http.ServeContent(w, r, rel, st.ModTime(), f)
}
func (s *Server) canvasKit(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	b, err := ui.ReadFile("ui/canvas/" + name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if ct := mime.TypeByExtension(path.Ext(name)); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
	http.ServeContent(w, r, name, time.Time{}, strings.NewReader(string(b)))
}
func publicBase(r *http.Request) string {
	if u := settings.Current.PublicURL; u != "" {
		return strings.TrimRight(u, "/")
	}
	if u := os.Getenv("FACTORY_BOARD_PUBLIC_URL"); u != "" {
		return strings.TrimRight(u, "/")
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	host := r.Host
	if settings.Current.TrustedProxy {
		if v := r.Header.Get("X-Forwarded-Proto"); v == "http" || v == "https" {
			scheme = v
		}
		if v := r.Header.Get("X-Forwarded-Host"); v != "" {
			host = v
		}
	}
	return scheme + "://" + host
}

func cleanRel(p string) string {
	if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, `\`) {
		return ""
	}
	c := path.Clean(p)
	if c != p || c == "." || c == ".." || strings.HasPrefix(c, "../") {
		return ""
	}
	return c
}

func versions(dir string) int {
	b, err := os.ReadFile(filepath.Join(dir, ".version"))
	if err != nil {
		return 1
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || n < 1 {
		return 1
	}
	return n
}

func writeVersion(dir string, v int) {
	_ = os.WriteFile(filepath.Join(dir, ".version"), []byte(strconv.Itoa(v)+"\n"), 0o644)
}

func invalidf(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{store.ErrInvalid}, args...)...)
}
