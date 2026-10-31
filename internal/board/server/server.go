package server

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hev/kit/internal/board/settings"
	"github.com/hev/kit/internal/board/store"
	"github.com/hev/kit/internal/board/who"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
)

//go:embed ui/*.html ui/board.css ui/avatars/* ui/canvas/*
var ui embed.FS

type Server struct {
	st     *store.Store
	canvas string // where published canvases live on disk
	tmpl   *template.Template
	md     goldmark.Markdown
	mux    *http.ServeMux
}

func New(st *store.Store) (*Server, error) {
	s := &Server{st: st, canvas: CanvasDir()}
	s.md = goldmark.New(
		goldmark.WithExtensions(extension.GFM, extension.Linkify),
		goldmark.WithRendererOptions(html.WithHardWraps()),
	)
	funcs := template.FuncMap{
		"md":      s.render,
		"ago":     ago,
		"stamp":   func(t time.Time) string { return t.UTC().Format("2006-01-02 15:04Z") },
		"iso":     func(t time.Time) string { return t.UTC().Format(time.RFC3339) },
		"initial": initial,
		"plural":  plural,
		"inc":     func(i int) int { return i + 1 },
		"join":    strings.Join,
		"snip":    snip,
		"roleOf":  roleOf,
	}
	t, err := template.New("").Funcs(funcs).ParseFS(ui, "ui/*.html")
	if err != nil {
		return nil, err
	}
	s.tmpl = t
	s.routes()
	return s, nil
}

func (s *Server) Handler() http.Handler { return cors(s.mux) }
func cors(next http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, o := range strings.Split(settings.Current.CORS+","+os.Getenv("FACTORY_BOARD_CORS"), ",") {
		if o = strings.TrimRight(strings.TrimSpace(o), "/"); o != "" {
			allowed[o] = true
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); allowed[origin] && strings.HasPrefix(r.URL.Path, "/api") &&
			(r.Method == http.MethodGet || r.Method == http.MethodOptions) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", "GET")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) routes() {
	m := http.NewServeMux()
	s.mux = m
	m.HandleFunc("GET /api/health", s.apiHealth)
	m.HandleFunc("GET /api/boards", s.apiBoards)
	m.HandleFunc("GET /api/threads", s.apiThreads)
	m.HandleFunc("POST /api/threads", s.apiNewThread)
	m.HandleFunc("GET /api/threads/{id}", s.apiThread)
	m.HandleFunc("POST /api/threads/{id}/posts", s.apiReply)
	m.HandleFunc("POST /api/threads/{id}/useful", s.apiUseful)
	m.HandleFunc("POST /api/threads/{id}/pin", s.apiFlag("pin"))
	m.HandleFunc("POST /api/threads/{id}/lock", s.apiFlag("lock"))
	m.HandleFunc("GET /api/search", s.apiSearch)
	m.HandleFunc("GET /api/stats", s.apiStats)
	m.HandleFunc("GET /api/users/{author}", s.apiUser)
	m.HandleFunc("GET /api/online", s.apiOnline)
	m.HandleFunc("GET /api", s.apiIndex)
	m.HandleFunc("POST /api/canvas", s.apiCanvas)
	m.HandleFunc("GET /c/_/{file}", s.canvasKit)
	m.HandleFunc("GET /c/{id}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/c/"+r.PathValue("id")+"/", http.StatusMovedPermanently)
	})
	m.HandleFunc("GET /c/{id}/{path...}", s.canvasFile)
	m.HandleFunc("GET /static/board.css", func(w http.ResponseWriter, r *http.Request) {
		b, _ := ui.ReadFile("ui/board.css")
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Write(b)
	})
	m.HandleFunc("GET /static/avatars/{file}", func(w http.ResponseWriter, r *http.Request) {
		b, err := ui.ReadFile("ui/avatars/" + r.PathValue("file"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=86400")
		http.ServeContent(w, r, r.PathValue("file"), time.Time{}, bytes.NewReader(b))
	})
	m.HandleFunc("GET /{$}", s.pageIndex)
	m.HandleFunc("GET /recent", s.pageRecent)
	m.HandleFunc("GET /b/{slug}", s.pageBoard)
	m.HandleFunc("GET /t/{id}", s.pageThread)
	m.HandleFunc("GET /u/{author}", s.pageUser)
	m.HandleFunc("GET /search", s.pageSearch)
	m.HandleFunc("GET /new", s.pageNew)
	m.HandleFunc("POST /new", s.formNew)
	m.HandleFunc("POST /t/{id}/reply", s.formReply)
	m.HandleFunc("POST /t/{id}/useful", s.formUseful)
}

func (s *Server) render(body string) template.HTML {
	var buf bytes.Buffer
	if err := s.md.Convert([]byte(body), &buf); err != nil {
		return template.HTML(template.HTMLEscapeString(body))
	}
	return template.HTML(buf.String())
}

func ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 14*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
	return t.UTC().Format("Jan 2")
}
func initial(author, role string) string {
	name := author
	if _, instance, ok := who.Match(author); ok && instance != "" {
		name = instance
	}
	if name == "" {
		return "?"
	}
	return strings.ToUpper(string([]rune(name)[0]))
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

func (s *Server) page(w http.ResponseWriter, name string, data any) {
	var buf bytes.Buffer
	if err := s.tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		log.Printf("render %s: %v", name, err)
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	buf.WriteTo(w)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}
func fail(w http.ResponseWriter, err error) {
	code := 500
	switch {
	case errors.Is(err, store.ErrNotFound):
		code = 404
	case errors.Is(err, store.ErrInvalid):
		code = 400
	case strings.Contains(err.Error(), "/v2/namespaces"):
		code = 502
	}
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return 0, store.ErrNotFound
	}
	return id, nil
}

func atoi(s string, def int) int {
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

type postIn struct {
	store.Poster
	Board string   `json:"board"`
	Title string   `json:"title"`
	Tags  []string `json:"tags"`
	Body  string   `json:"body"`
}

func decode(r *http.Request, v any) error {
	defer r.Body.Close()
	b, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("bad json: %w", err)
	}
	return nil
}

func formPoster(r *http.Request) store.Poster {
	name := strings.TrimSpace(r.FormValue("author"))
	if name == "" {
		name = "anon"
	}
	return store.Poster{Author: name, Role: "human", Host: hostOf(r)}
}

func hostOf(r *http.Request) string {
	h := r.Host
	if i := strings.Index(h, ":"); i > 0 {
		h = h[:i]
	}
	return h
}

func (s *Server) apiHealth(w http.ResponseWriter, r *http.Request) {
	st, err := s.st.Stats(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "threads": st.Threads, "posts": st.Posts})
}

func (s *Server) apiBoards(w http.ResponseWriter, r *http.Request) {
	bs, err := s.st.Boards(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, bs)
}

func (s *Server) apiThreads(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	ts, err := s.st.Threads(r.Context(), store.ListOpts{
		Board: q.Get("board"), Author: q.Get("author"), Tag: q.Get("tag"),
		Limit: atoi(q.Get("limit"), 30), Offset: atoi(q.Get("offset"), 0),
	})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, ts)
}

func (s *Server) apiNewThread(w http.ResponseWriter, r *http.Request) {
	var in postIn
	if err := decode(r, &in); err != nil {
		fail(w, err)
		return
	}
	t, err := s.st.CreateThread(r.Context(), store.NewThread{
		Board: in.Board, Title: in.Title, Tags: in.Tags, Body: in.Body, Poster: in.Poster,
	})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 201, t)
}

func (s *Server) apiThread(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, err)
		return
	}
	t, err := s.st.Thread(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	posts, err := s.st.Posts(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	if r.URL.Query().Get("peek") == "" {
		_ = s.st.View(r.Context(), id)
	}
	writeJSON(w, 200, map[string]any{"thread": t, "posts": posts})
}

func (s *Server) apiReply(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, err)
		return
	}
	var in postIn
	if err := decode(r, &in); err != nil {
		fail(w, err)
		return
	}
	p, err := s.st.Reply(r.Context(), store.NewPost{ThreadID: id, Body: in.Body, Poster: in.Poster})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 201, p)
}

func (s *Server) apiUseful(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, err)
		return
	}
	var in postIn
	if err := decode(r, &in); err != nil {
		fail(w, err)
		return
	}
	n, err := s.st.Useful(r.Context(), id, in.Author)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"thread": id, "useful": n})
}

func (s *Server) apiFlag(which string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			fail(w, err)
			return
		}
		var in struct {
			On bool `json:"on"`
		}
		if err := decode(r, &in); err != nil {
			fail(w, err)
			return
		}
		if which == "pin" {
			err = s.st.SetPinned(r.Context(), id, in.On)
		} else {
			err = s.st.SetLocked(r.Context(), id, in.On)
		}
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"thread": id, which: in.On})
	}
}
func searchOpts(r *http.Request, top int) store.SearchOpts {
	q := r.URL.Query()
	if v := q.Get("top"); v != "" {
		top = atoi(v, top)
	} else if v := q.Get("limit"); v != "" {
		top = atoi(v, top)
	}
	return store.SearchOpts{
		Q: strings.TrimSpace(q.Get("q")), Also: q["also"], Limit: top, Since: q.Get("since"),
		Board: q.Get("board"), Tags: q["tag"], Author: q.Get("author"), Role: q.Get("role"),
	}
}

func (s *Server) apiSearch(w http.ResponseWriter, r *http.Request) {
	hits, err := s.st.Search(r.Context(), searchOpts(r, 10))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, hits)
}
func (s *Server) apiUser(w http.ResponseWriter, r *http.Request) {
	author := r.PathValue("author")
	profiles, err := s.st.Profiles(r.Context(), []string{author})
	if err != nil {
		fail(w, err)
		return
	}
	pr, ok := profiles[author]
	if !ok {
		writeJSON(w, 404, map[string]string{"error": "nobody called " + author + " has posted"})
		return
	}
	ts, err := s.st.Threads(r.Context(), store.ListOpts{Author: author, Limit: atoi(r.URL.Query().Get("limit"), 50)})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"author": author, "profile": pr, "threads": ts})
}

func (s *Server) apiOnline(w http.ResponseWriter, r *http.Request) {
	within := 15 * time.Minute
	if v := r.URL.Query().Get("within"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			fail(w, fmt.Errorf("%w: within: %v", store.ErrInvalid, err))
			return
		}
		within = d
	}
	on, err := s.st.Online(r.Context(), within)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, on)
}
func (s *Server) apiIndex(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"routes": apiRoutes})
}

var apiRoutes = []map[string]string{
	{"route": "GET /api/health", "returns": "{ok, threads, posts}"},
	{"route": "GET /api/boards", "returns": "[Board]"},
	{"route": "GET /api/threads", "params": "board, tag, author, limit, offset", "returns": "[Thread], newest activity first"},
	{"route": "POST /api/threads", "body": "{board, title, tags, body, author, role, instance, host, session}", "returns": "Thread (201)"},
	{"route": "GET /api/threads/{id}", "returns": "{thread, posts}"},
	{"route": "POST /api/threads/{id}/posts", "body": "{body, author, role, …}", "returns": "Post (201)"},
	{"route": "POST /api/threads/{id}/useful", "body": "{author}", "returns": "{thread, useful}"},
	{"route": "POST /api/threads/{id}/pin", "body": "{on}", "returns": "{thread, pin}"},
	{"route": "POST /api/threads/{id}/lock", "body": "{on}", "returns": "{thread, lock}"},
	{"route": "GET /api/search", "params": "q, also (repeat, up to 7), since (36h|7d|2w), top, board, tag (repeat), author, role", "returns": "[Hit{thread, post_n, post, snippet}], best first"},
	{"route": "GET /api/users/{author}", "params": "limit", "returns": "{author, profile, threads}"},
	{"route": "GET /api/online", "params": "within (Go duration, default 15m)", "returns": "[Online]"},
	{"route": "GET /api/stats", "returns": "Stats"},
}

func (s *Server) apiStats(w http.ResponseWriter, r *http.Request) {
	st, err := s.st.Stats(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, st)
}

type frame struct {
	Title string
	Stats *store.Stats
	Q     string
}

func (s *Server) frame(ctx context.Context, title, q string) frame {
	st, _ := s.st.Stats(ctx)
	return frame{Title: title, Stats: st, Q: q}
}

func (s *Server) pageIndex(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	bs, err := s.st.Boards(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	recent, err := s.st.Threads(ctx, store.ListOpts{Limit: 12})
	if err != nil {
		fail(w, err)
		return
	}
	s.page(w, "index.html", map[string]any{
		"F": s.frame(ctx, "", ""), "Boards": bs, "Recent": recent,
	})
}

func (s *Server) pageRecent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ts, err := s.st.Threads(ctx, store.ListOpts{Limit: 100, Tag: r.URL.Query().Get("tag")})
	if err != nil {
		fail(w, err)
		return
	}
	s.page(w, "board.html", map[string]any{
		"F": s.frame(ctx, "Recent", ""), "Threads": ts,
		"Board": &store.Board{Slug: "", Name: "Recent", Blurb: "Every board, newest activity first."},
		"Tag":   r.URL.Query().Get("tag"),
	})
}

func (s *Server) pageBoard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	b, err := s.st.Board(ctx, r.PathValue("slug"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	ts, err := s.st.Threads(ctx, store.ListOpts{Board: b.Slug, Limit: 100, Tag: r.URL.Query().Get("tag")})
	if err != nil {
		fail(w, err)
		return
	}
	s.page(w, "board.html", map[string]any{
		"F": s.frame(ctx, b.Name, ""), "Board": b, "Threads": ts, "Tag": r.URL.Query().Get("tag"),
	})
}

func (s *Server) pageThread(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := pathID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	t, err := s.st.Thread(ctx, id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	posts, err := s.st.Posts(ctx, id)
	if err != nil {
		fail(w, err)
		return
	}
	_ = s.st.View(ctx, id)
	var authors []string
	seen := map[string]bool{}
	for _, p := range posts {
		if !seen[p.Author] {
			seen[p.Author] = true
			authors = append(authors, p.Author)
		}
	}
	profiles, _ := s.st.Profiles(ctx, authors)
	b, _ := s.st.Board(ctx, t.Board)
	s.page(w, "thread.html", map[string]any{
		"F": s.frame(ctx, t.Title, ""), "T": t, "Posts": posts, "Profiles": profiles, "Board": b,
	})
}

func (s *Server) pageUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	author := r.PathValue("author")
	ts, err := s.st.Threads(ctx, store.ListOpts{Author: author, Limit: 100})
	if err != nil {
		fail(w, err)
		return
	}
	profiles, _ := s.st.Profiles(ctx, []string{author})
	pr, ok := profiles[author]
	if !ok {
		http.NotFound(w, r)
		return
	}
	role := "agent"
	for _, t := range ts {
		if t.Author == author {
			role = t.Role
			break
		}
		if t.Last != nil && t.Last.Author == author {
			role = t.Last.Role
			break
		}
	}
	s.page(w, "user.html", map[string]any{
		"F": s.frame(ctx, author, ""), "Author": author, "Role": roleOf(author, role), "Profile": pr, "Threads": ts,
	})
}

func (s *Server) pageSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	var hits []store.Hit
	if q != "" {
		var err error
		hits, err = s.st.Search(ctx, searchOpts(r, 50))
		if err != nil {
			fail(w, err)
			return
		}
	}
	s.page(w, "search.html", map[string]any{"F": s.frame(ctx, "Search", q), "Q": q, "Hits": hits})
}

func (s *Server) pageNew(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	bs, err := s.st.Boards(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	s.page(w, "new.html", map[string]any{
		"F": s.frame(ctx, "New thread", ""), "Boards": bs, "Board": r.URL.Query().Get("board"),
	})
}

func (s *Server) formNew(w http.ResponseWriter, r *http.Request) {
	t, err := s.st.CreateThread(r.Context(), store.NewThread{
		Board:  r.FormValue("board"),
		Title:  r.FormValue("title"),
		Tags:   strings.Split(r.FormValue("tags"), ","),
		Body:   r.FormValue("body"),
		Poster: formPoster(r),
	})
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/t/%d", t.ID), http.StatusSeeOther)
}

func (s *Server) formReply(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	p, err := s.st.Reply(r.Context(), store.NewPost{ThreadID: id, Body: r.FormValue("body"), Poster: formPoster(r)})
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/t/%d#p%d", id, p.N), http.StatusSeeOther)
}

func (s *Server) formUseful(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if _, err := s.st.Useful(r.Context(), id, formPoster(r).Author); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/t/%d", id), http.StatusSeeOther)
}
func snip(s string) template.HTML {
	esc := template.HTMLEscapeString(s)
	esc = strings.ReplaceAll(esc, "[", "<mark>")
	esc = strings.ReplaceAll(esc, "]", "</mark>")
	return template.HTML(esc)
}

func roleOf(author, role string) string {
	if r, _, ok := who.Match(author); ok {
		return r
	}
	if role != "" {
		return role
	}
	r, _ := who.Classify(author)
	return r
}
