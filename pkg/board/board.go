// Package board is kit's message board for programs outside this module:
// the store interface, its Layer and file implementations, and the HTTP
// handler behind `hev board serve` (the /api/... routes).
//
// A hosted server mounts Handler with an Author func, so every post, thread
// and useful mark is attributed to the authenticated caller and never to
// what the request body claims.
package board

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/hev/kit/internal/board/layer"
	"github.com/hev/kit/internal/board/server"
	"github.com/hev/kit/internal/board/store"
)

type (
	Store      = store.Store
	Backend    = store.Backend
	Info       = store.Info
	Board      = store.Board
	Poster     = store.Poster
	Thread     = store.Thread
	Post       = store.Post
	Hit        = store.Hit
	Stats      = store.Stats
	Online     = store.Online
	Profile    = store.Profile
	Image      = store.Image
	ListOpts   = store.ListOpts
	SearchOpts = store.SearchOpts
	NewThread  = store.NewThread
	NewPost    = store.NewPost
)

var (
	ErrNotFound = store.ErrNotFound
	ErrInvalid  = store.ErrInvalid
)

// LayerConfig is how a server reaches its own board namespace (one
// namespace per board; nothing is suffixed).
type LayerConfig struct {
	Endpoint  string
	Key       string
	Namespace string
	Model     string
	HTTP      *http.Client
}

// OpenLayer is a Layer-backed board with the caller's credentials. It reads
// no config file and no environment.
func OpenLayer(cfg LayerConfig) (*Store, error) {
	if strings.TrimSpace(cfg.Endpoint) == "" || strings.TrimSpace(cfg.Key) == "" || strings.TrimSpace(cfg.Namespace) == "" {
		return nil, errors.New("board: endpoint, key and namespace are required")
	}
	cl := layer.New(cfg.Endpoint, cfg.Key, cfg.Namespace, cfg.Model)
	if cfg.HTTP != nil {
		cl.WithHTTP(cfg.HTTP)
	}
	return store.New(cl), nil
}

// OpenFiles is a board in files under dir, the same store a keyless
// `hev board serve` uses; a good fake for tests.
func OpenFiles(dir string) (*Store, error) { return store.NewFiles(dir) }

// AuthorFunc names the authenticated caller of a request. An error answers
// 401 and the request goes no further.
type AuthorFunc func(*http.Request) (Poster, error)

// HandlerOptions configures Handler.
type HandlerOptions struct {
	// Author, when set, attributes every write to the caller it returns.
	// Without it the poster comes from the request, as on a local board.
	Author AuthorFunc
	// ReadOnly refuses every write with 403.
	ReadOnly bool
}

type posterKey struct{}

// stamped overrides the poster of every write with the request's caller.
type stamped struct{ Backend }

func callerOf(ctx context.Context) (Poster, bool) {
	p, ok := ctx.Value(posterKey{}).(Poster)
	return p, ok
}

func (s stamped) CreateThread(ctx context.Context, in NewThread) (*Thread, error) {
	if p, ok := callerOf(ctx); ok {
		in.Poster = p
	}
	return s.Backend.CreateThread(ctx, in)
}

func (s stamped) Reply(ctx context.Context, in NewPost) (*Post, error) {
	if p, ok := callerOf(ctx); ok {
		in.Poster = p
	}
	return s.Backend.Reply(ctx, in)
}

func (s stamped) Useful(ctx context.Context, threadID int64, author string) (int, error) {
	if p, ok := callerOf(ctx); ok {
		author = p.Author
	}
	return s.Backend.Useful(ctx, threadID, author)
}

// Handler serves the board's routes over st: the JSON API under /api/...
// (threads, posts, useful, pin, lock, search, stats) and the HTML pages.
// Mount it under a prefix with http.StripPrefix.
func Handler(st *Store, opts HandlerOptions) (http.Handler, error) {
	if st == nil {
		return nil, errors.New("board: a store is required")
	}
	if opts.Author != nil {
		st = store.Wrap(stamped{st.Backend}, st.Info())
	}
	srv, err := server.New(st)
	if err != nil {
		return nil, err
	}
	h := srv.Handler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if opts.ReadOnly && r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			http.Error(w, `{"error":"board is read-only; writes are disabled"}`, http.StatusForbidden)
			return
		}
		if opts.Author != nil {
			p, err := opts.Author(r)
			if err != nil || strings.TrimSpace(p.Author) == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"authentication required"}`))
				return
			}
			r = r.WithContext(context.WithValue(r.Context(), posterKey{}, p))
		}
		h.ServeHTTP(w, r)
	}), nil
}
