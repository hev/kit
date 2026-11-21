// Package v1 serves the versioned Board API over kit's shared board store.
package v1

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hev/kit/internal/blob"
	"github.com/hev/kit/internal/board/store"
	"github.com/hev/kit/pkg/search"
)

// Caller is the verified principal. Board selection belongs to the host.
type Caller struct {
	Author, Name, Host string
	Admin              bool
}

type Authenticate func(*http.Request) (Caller, bool)

// Handler mounts the full /v1/board/ paths. Open one store and use one
// dedicated data directory per account. The directory holds images and
// replay records and must be retained across dashboard restarts.
func Handler(st *store.Store, dir string, auth Authenticate) http.Handler {
	return HandlerWithLegacy(st, dir, auth, nil)
}

// HandlerWithLegacy serializes both versions against the same account store.
// The host must authenticate requests before dispatching to this handler.
func HandlerWithLegacy(st *store.Store, dir string, auth Authenticate, legacy http.Handler) http.Handler {
	h := &handler{st: st, blobs: blob.New(filepath.Join(dir, "blobs"), 0), dir: dir, auth: auth}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/board/boards", h.boards)
	mux.HandleFunc("GET /v1/board/posts", h.list)
	mux.HandleFunc("POST /v1/board/posts", h.create)
	mux.HandleFunc("GET /v1/board/posts/{id}", h.read)
	mux.HandleFunc("PATCH /v1/board/posts/{id}", h.edit)
	mux.HandleFunc("DELETE /v1/board/posts/{id}", h.remove)
	mux.HandleFunc("POST /v1/board/posts/{id}/replies", h.reply)
	mux.HandleFunc("POST /v1/board/posts/{id}/useful", h.useful)
	mux.HandleFunc("GET /v1/board/search", h.search)
	mux.HandleFunc("GET /v1/board/blobs", h.limits)
	mux.HandleFunc("POST /v1/board/blobs", h.upload)
	mux.HandleFunc("GET /v1/board/blobs/{sha256}", h.download)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isLegacy := legacy != nil && strings.HasPrefix(r.URL.Path, "/api/")
		if !isLegacy {
			w.Header().Set("Cache-Control", "no-store")
		}
		caller, ok := h.auth(r)
		if !ok || caller.Author == "" {
			w.Header().Set("WWW-Authenticate", "Bearer")
			Error(w, 401, "unauthorized", "A valid bearer key is required.")
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), callerKey{}, caller))
		isVersionedRead := strings.HasPrefix(r.URL.Path, "/v1/board/posts/") && r.URL.Query().Get("peek") != "true"
		isLegacyRead := strings.HasPrefix(r.URL.Path, "/api/threads/") && r.URL.Query().Get("peek") == ""
		if r.Method != http.MethodGet && r.Method != http.MethodHead || isVersionedRead || isLegacyRead {
			release, err := lockAccount(r.Context(), dir)
			if err != nil {
				fail(w, err)
				return
			}
			defer release()
		}
		if isLegacy {
			legacy.ServeHTTP(w, r)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

type handler struct {
	st    *store.Store
	blobs *blob.Store
	dir   string
	auth  Authenticate
}

type callerKey struct{}

func callerOf(r *http.Request) Caller { return r.Context().Value(callerKey{}).(Caller) }

func respond(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// Error writes the v1 error envelope without reflecting upstream details.
func Error(w http.ResponseWriter, status int, code, message string) {
	respond(w, status, map[string]string{"error": message, "code": code})
}

func fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound), errors.Is(err, blob.ErrNotFound):
		Error(w, 404, "not_found", "The requested resource was not found.")
	case errors.Is(err, store.ErrForbidden):
		Error(w, 403, "forbidden", "Only the author or an account admin may change the post content.")
	case errors.Is(err, store.ErrLocked):
		Error(w, 409, "locked", "The post is locked.")
	case errors.Is(err, store.ErrInvalid):
		Error(w, 400, "bad_request", "Check the board, fields and image references.")
	case errors.Is(err, blob.ErrTooLarge):
		Error(w, 413, "payload_too_large", "The image exceeds the upload limit.")
	case errors.Is(err, blob.ErrUnsupported):
		Error(w, 415, "unsupported_media_type", "The bytes are not an accepted image.")
	default:
		Error(w, 502, "upstream_error", "The board store could not complete the request.")
	}
}

func postID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		fail(w, store.ErrNotFound)
		return 0, false
	}
	return id, true
}

func decode(w http.ResponseWriter, r *http.Request, value any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20))
	if err := d.Decode(value); err != nil || d.Decode(new(any)) != io.EOF {
		fail(w, store.ErrInvalid)
		return false
	}
	return true
}

type pageRequest struct {
	offset, limit int
	since         time.Time
}

var boardSlug = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

func pageParams(w http.ResponseWriter, r *http.Request) (pageRequest, bool) {
	p := pageRequest{limit: 50}
	q := r.URL.Query()
	if value := q.Get("limit"); value != "" {
		limit, err := strconv.Atoi(value)
		if err != nil || limit < 1 || limit > 200 {
			fail(w, store.ErrInvalid)
			return p, false
		}
		p.limit = limit
	}
	if value := q.Get("cursor"); value != "" {
		raw, err := base64.RawURLEncoding.DecodeString(value)
		offset, parseErr := strconv.Atoi(string(raw))
		if err != nil || parseErr != nil || offset < 1 || offset > math.MaxInt-201 {
			fail(w, store.ErrInvalid)
			return p, false
		}
		p.offset = offset
	}
	if value := q.Get("since"); value != "" {
		instant, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			duration, parseErr := search.ParseSince(value)
			if parseErr != nil {
				fail(w, store.ErrInvalid)
				return p, false
			}
			instant = time.Now().Add(-duration)
		}
		p.since = instant
	}
	if board := q.Get("board"); board != "" && !boardSlug.MatchString(board) {
		fail(w, store.ErrInvalid)
		return p, false
	}
	return p, true
}

func (p pageRequest) cursor(n int) *string {
	if n <= p.limit {
		return nil
	}
	cursor := base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(p.offset + p.limit)))
	return &cursor
}

func (h *handler) boards(w http.ResponseWriter, r *http.Request) {
	boards, err := h.st.Boards(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	sort.Slice(boards, func(i, j int) bool { return boards[i].Sort < boards[j].Sort })
	data := make([]boardView, 0, len(boards))
	for _, b := range boards {
		data = append(data, boardView{Slug: b.Slug, Name: b.Name, Blurb: b.Blurb, Sort: b.Sort, Threads: b.Threads, Posts: b.Posts, Last: ref(b.Last)})
	}
	respond(w, 200, map[string]any{"data": data})
}

func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	p, ok := pageParams(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	posts, err := h.st.Threads(r.Context(), store.ListOpts{
		Board: q.Get("board"), Author: q.Get("author"), Tags: q["tag"], Since: p.since,
		Offset: p.offset, Limit: p.limit + 1, PinnedFirst: true,
	})
	if err != nil {
		fail(w, err)
		return
	}
	data := make([]postView, 0, min(p.limit, len(posts)))
	for _, t := range posts[:min(p.limit, len(posts))] {
		data = append(data, view(t, nil))
	}
	respond(w, 200, map[string]any{"data": data, "next_cursor": p.cursor(len(posts))})
}

type messageInput struct {
	store.Poster
	Body   string     `json:"body"`
	Images []imageRef `json:"images"`
}

func (in messageInput) poster(r *http.Request) store.Poster {
	p := in.Poster
	c := callerOf(r)
	p.Author, p.AuthorName, p.Host = c.Author, c.Name, c.Host
	if p.Role == "" {
		p.Role = "agent"
	}
	return p
}

func (in messageInput) valid() bool {
	return strings.TrimSpace(in.Body) != "" && utf8.RuneCountInString(in.Body) <= 1000000
}

func (h *handler) create(w http.ResponseWriter, r *http.Request) {
	var in struct {
		messageInput
		Board string   `json:"board"`
		Title string   `json:"title"`
		Tags  []string `json:"tags"`
	}
	h.replay(w, r, func(w http.ResponseWriter) {
		if !decode(w, r, &in) {
			return
		}
		if !in.valid() || strings.TrimSpace(in.Title) == "" {
			fail(w, store.ErrInvalid)
			return
		}
		images, err := h.images(in.Images)
		if err != nil {
			fail(w, err)
			return
		}
		t, err := h.st.CreateThread(r.Context(), store.NewThread{Board: in.Board, Title: in.Title, Tags: in.Tags, Body: in.Body, Images: images, Poster: in.poster(r)})
		if err != nil {
			fail(w, err)
			return
		}
		w.Header().Set("Location", "/v1/board/posts/"+strconv.FormatInt(t.ID, 10))
		respond(w, 201, view(*t, &store.Post{Body: t.Body(), Images: images}))
	})
}

func (h *handler) read(w http.ResponseWriter, r *http.Request) {
	id, ok := postID(w, r)
	if !ok {
		return
	}
	if r.URL.Query().Get("peek") != "true" {
		if err := h.st.View(r.Context(), id); err != nil {
			fail(w, err)
			return
		}
	}
	t, op, replies, err := h.detail(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	respond(w, 200, map[string]any{"post": view(*t, op), "replies": replies})
}

func (h *handler) detail(ctx context.Context, id int64) (*store.Thread, *store.Post, []replyView, error) {
	t, err := h.st.Thread(ctx, id)
	if err != nil {
		return nil, nil, nil, err
	}
	posts, err := h.st.Posts(ctx, id)
	if err != nil {
		return nil, nil, nil, err
	}
	op := &store.Post{Body: t.Body()}
	replies := make([]replyView, 0, len(posts))
	for _, p := range posts {
		if p.N == 1 {
			op = &p
		} else {
			replies = append(replies, replyOf(p))
		}
	}
	return t, op, replies, nil
}

func (h *handler) edit(w http.ResponseWriter, r *http.Request) {
	id, ok := postID(w, r)
	if !ok {
		return
	}
	var input map[string]json.RawMessage
	if !decode(w, r, &input) {
		return
	}
	var edit store.Edit
	fields := map[string]any{"title": &edit.Title, "body": &edit.Body, "tags": &edit.Tags, "pinned": &edit.Pinned, "locked": &edit.Locked}
	for key, raw := range input {
		if target, ok := fields[key]; !ok || string(raw) == "null" || json.Unmarshal(raw, target) != nil {
			fail(w, store.ErrInvalid)
			return
		}
	}
	if len(input) == 0 {
		fail(w, store.ErrInvalid)
		return
	}
	c := callerOf(r)
	editor := h.st.Backend.(store.Editor)
	if err := editor.EditThread(r.Context(), id, c.Author, c.Admin, edit); err != nil {
		fail(w, err)
		return
	}
	t, op, _, err := h.detail(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	respond(w, 200, view(*t, op))
}

func (h *handler) remove(w http.ResponseWriter, r *http.Request) {
	id, ok := postID(w, r)
	if !ok {
		return
	}
	c := callerOf(r)
	if err := h.st.Backend.(store.Editor).DeleteThread(r.Context(), id, c.Author, c.Admin); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(204)
}

func (h *handler) reply(w http.ResponseWriter, r *http.Request) {
	id, ok := postID(w, r)
	if !ok {
		return
	}
	h.replay(w, r, func(w http.ResponseWriter) {
		var in messageInput
		if !decode(w, r, &in) {
			return
		}
		if !in.valid() {
			fail(w, store.ErrInvalid)
			return
		}
		images, err := h.images(in.Images)
		if err != nil {
			fail(w, err)
			return
		}
		p, err := h.st.Reply(r.Context(), store.NewPost{ThreadID: id, Body: in.Body, Images: images, Poster: in.poster(r)})
		if err != nil {
			fail(w, err)
			return
		}
		respond(w, 201, replyOf(*p))
	})
}

func (h *handler) useful(w http.ResponseWriter, r *http.Request) {
	id, ok := postID(w, r)
	if !ok {
		return
	}
	n, err := h.st.Useful(r.Context(), id, callerOf(r).Author)
	if err != nil {
		fail(w, err)
		return
	}
	respond(w, 200, map[string]any{"id": id, "useful": n})
}

func (h *handler) search(w http.ResponseWriter, r *http.Request) {
	p, ok := pageParams(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	phrase, also := q.Get("q"), q["also"]
	if strings.TrimSpace(phrase) == "" || len(phrase) > 8192 || len(also) >= search.MaxPhrasings {
		fail(w, store.ErrInvalid)
		return
	}
	st, err := h.st.RoutedSearch(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	hits, err := st.Search(r.Context(), store.SearchOpts{
		Q: phrase, Also: also, Board: q.Get("board"), Tags: q["tag"], Author: q.Get("author"), Role: q.Get("role"),
		After: p.since, Limit: p.offset + p.limit + 1,
	})
	if err != nil {
		fail(w, err)
		return
	}
	hits = hits[min(p.offset, len(hits)):]
	data := make([]hitView, 0, min(p.limit, len(hits)))
	for _, hit := range hits[:min(p.limit, len(hits))] {
		out := hitView{Post: view(hit.Thread, nil), N: hit.PostN, Snippet: hit.Snippet, Score: hit.Score}
		if hit.PostN > 1 {
			reply := replyOf(hit.Post)
			out.Reply = &reply
		}
		data = append(data, out)
	}
	route, engine, semantic := st.SearchRoute()
	respond(w, 200, map[string]any{"data": data, "next_cursor": p.cursor(len(hits)), "q": phrase, "also": alsoOrEmpty(also), "route": route, "engine": engine, "semantic": semantic})
}

func alsoOrEmpty(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}
