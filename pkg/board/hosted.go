package board

import (
	"context"
	"errors"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/hev/kit/internal/blob"
	"github.com/hev/kit/internal/board/store"
	"github.com/hev/kit/pkg/board/view"
)

type BlobMeta = view.BlobMeta
type BlobStore = view.BlobStore
type CanvasStore = view.CanvasStore

var ErrBlobNotFound = blob.ErrNotFound
var ErrBlobTooLarge = blob.ErrTooLarge
var ErrBlobUnsupported = blob.ErrUnsupported

// InspectImage derives the image metadata without reading local configuration.
func InspectImage(b []byte) (BlobMeta, error) { return blob.Inspect(b) }

// HostedOptions configures the authenticated board UI.
type HostedOptions struct {
	// BasePath is consumed by the handler, e.g. /board. Do not StripPrefix.
	BasePath string
	// Principal is resolved by the application on every request, including reads.
	Principal AuthorFunc
	ReadOnly  bool
	// CSRF verifies every write before the body is parsed. Supply the host's
	// CSRF middleware check, including token and/or origin validation for forms.
	// It is mandatory unless ReadOnly is set; an error returns JSON with 403.
	CSRF     func(*http.Request) error
	Blobs    BlobStore
	Canvases CanvasStore
}

// NewHosted serves the shared board pages and API with explicit team-scoped
// stores. Construct a handler for each tenant after authentication; no store
// or identity is selected from a browser-supplied field. This constructor
// consults no environment settings or local files.
func NewHosted(backend Backend, opts HostedOptions) (http.Handler, error) {
	if backend == nil || opts.Principal == nil || (!opts.ReadOnly && opts.CSRF == nil) {
		return nil, errors.New("board: store, principal and write CSRF verifier are required")
	}
	base := strings.TrimRight(opts.BasePath, "/")
	if base != "" && (!strings.HasPrefix(base, "/") || strings.ContainsAny(base, "?#\\\"'<> \t\r\n%") || strings.Contains(base, "//") || strings.Contains(base, "..")) {
		return nil, errors.New("board: invalid base path")
	}
	st := store.Wrap(stamped{backend}, store.Info{Mode: "hosted"})
	srv, err := view.NewHosted(st, view.HostedConfig{BasePath: base, ReadOnly: opts.ReadOnly, Blobs: opts.Blobs, Canvases: opts.Canvases})
	if err != nil {
		return nil, err
	}
	inner := srv.Handler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		p, e := opts.Principal(r)
		if e != nil || strings.TrimSpace(p.Author) == "" {
			hostedError(w, 401)
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" {
			if opts.ReadOnly {
				hostedError(w, 402)
				return
			}
			if opts.CSRF(r) != nil {
				hostedError(w, 403)
				return
			}
		}
		if base != "" && r.URL.Path != base && !strings.HasPrefix(r.URL.Path, base+"/") {
			http.NotFound(w, r)
			return
		}
		if path.Clean(r.URL.Path) != strings.TrimSuffix(r.URL.Path, "/") && r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		rr := r.Clone(context.WithValue(r.Context(), posterKey{}, p))
		rr.URL.Path = strings.TrimPrefix(r.URL.Path, base)
		rr.URL.RawPath = ""
		if rr.URL.Path == "" {
			rr.URL.Path = "/"
		}
		inner.ServeHTTP(&safeErrors{ResponseWriter: w}, rr)
	}), nil
}

func hostedError(w http.ResponseWriter, code int) {
	message := http.StatusText(code)
	if code == 402 {
		message = "board is read-only; writes are disabled"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = io.WriteString(w, `{"error":"`+message+`"}`)
}

// Store errors may contain operator paths or credentials. Hosted responses
// expose a stable error shape and status while keeping those details private.
type safeErrors struct {
	http.ResponseWriter
	failed bool
}

func (w *safeErrors) WriteHeader(code int) {
	if code >= 400 {
		w.failed = true
		w.Header().Del("Content-Length")
		hostedError(w.ResponseWriter, code)
		return
	}
	w.ResponseWriter.WriteHeader(code)
}
func (w *safeErrors) Write(b []byte) (int, error) {
	if w.failed {
		return len(b), nil
	}
	return w.ResponseWriter.Write(b)
}
