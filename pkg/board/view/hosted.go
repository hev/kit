package view

import (
	"encoding/json"
	"errors"
	"github.com/hev/kit/internal/blob"
	"github.com/hev/kit/internal/board/store"
	"io"
	"io/fs"
	"net/http"
	"strings"
)

// BlobMeta is metadata derived from the image bytes.
type BlobMeta = blob.Meta

// BlobStore supplies image storage. Hosted adapters must scope it to the team.
// Implementations validate hashes, byte limits and image types before storage.
type BlobStore interface {
	MaxBytes() int64
	Check([]byte) (BlobMeta, error)
	Put([]byte) (BlobMeta, bool, error)
	PutReader(io.Reader) (BlobMeta, bool, error)
	Get(string) ([]byte, BlobMeta, error)
	Stat(string) (BlobMeta, error)
}

// CanvasStore supplies published files, scoped to this handler's team.
// Save atomically replaces a canvas and returns its new version number.
// Open must only accept validated relative paths and return a seekable file.
type CanvasStore interface {
	Save(id int64, files map[string][]byte) (int, error)
	Open(id int64, name string) (fs.File, error)
}

type HostedConfig struct {
	BasePath string
	ReadOnly bool
	Blobs    BlobStore
	Canvases CanvasStore
}

// NewHosted constructs the shared UI without reading configuration, the
// environment, or local paths. All stores are supplied by the host.
func NewHosted(st *store.Store, cfg HostedConfig) (*Server, error) {
	if st == nil || cfg.Blobs == nil || cfg.Canvases == nil {
		return nil, errors.New("board: explicit board, blob and canvas stores are required")
	}
	s := &Server{st: st, hosted: true, basePath: cfg.BasePath, readOnly: cfg.ReadOnly, blobStore: cfg.Blobs, canvasStore: cfg.Canvases}
	if err := s.initialize(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Server) hostedCanvas(w http.ResponseWriter, r *http.Request, id int64, rel string) {
	f, err := s.canvasStore.Open(id, rel)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	seek, ok := f.(io.ReadSeeker)
	if !ok {
		http.Error(w, "canvas unavailable", 500)
		return
	}
	st, err := f.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Security-Policy", canvasCSP)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, rel, st.ModTime(), seek)
}

// Hosted templates never consult machine-specific role configuration.
func (s *Server) initial(author, role string) string {
	if s.hosted {
		if author == "" {
			return "?"
		}
		return strings.ToUpper(string([]rune(author)[0]))
	}
	return initial(author, role)
}

// Prefix only URL metadata, never strings in a post body or title.
func (s *Server) writeJSON(w http.ResponseWriter, code int, v any) {
	if !s.hosted {
		writeJSON(w, code, v)
		return
	}
	b, e := json.Marshal(v)
	if e != nil {
		http.Error(w, "response unavailable", 500)
		return
	}
	var value any
	if json.Unmarshal(b, &value) != nil {
		http.Error(w, "response unavailable", 500)
		return
	}
	var prefix func(any)
	prefix = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, v := range x {
				if k == "url" {
					if p, ok := v.(string); ok && strings.HasPrefix(p, "/blob/") {
						x[k] = s.basePath + p
					}
				} else {
					prefix(v)
				}
			}
		case []any:
			for _, v := range x {
				prefix(v)
			}
		}
	}
	prefix(value)
	writeJSON(w, code, value)
}
