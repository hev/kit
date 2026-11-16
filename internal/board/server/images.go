package server

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"time"

	"github.com/hev/kit/internal/blob"
	"github.com/hev/kit/internal/board/settings"
	"github.com/hev/kit/internal/board/store"
)

// BlobDir is where the board keeps image bytes in either storage mode: under
// the board directory on the host that serves it. Layer has no object store,
// so in Layer mode the rows carry the metadata and the bytes stay here.
func BlobDir() string { return filepath.Join(settings.Dir(), "blobs") }

// blobFail answers an image the store refused with a status that says why.
func blobFail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, blob.ErrTooLarge):
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": err.Error()})
	case errors.Is(err, blob.ErrUnsupported):
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": err.Error()})
	case errors.Is(err, blob.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	default:
		fail(w, err)
	}
}

// BlobLimits is what an upload may be, so a client can refuse a file before
// it sends any of a post's images.
type BlobLimits struct {
	MaxBytes   int64    `json:"max_bytes"`
	MaxPerPost int      `json:"max_per_post"`
	Types      []string `json:"types"`
}

func (s *Server) apiBlobLimits(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, BlobLimits{MaxBytes: s.blobs.MaxBytes(), MaxPerPost: store.MaxImages, Types: blob.Accepted})
}

type blobOut struct {
	blob.Meta
	URL     string `json:"url"`
	Created bool   `json:"created"`
}

// apiBlob stores the request body as one image. The body is the raw bytes;
// the media type is sniffed from them and any Content-Type is ignored.
func (s *Server) apiBlob(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	m, created, err := s.blobs.PutReader(r.Body)
	if err != nil {
		blobFail(w, err)
		return
	}
	code := http.StatusOK
	if created {
		code = http.StatusCreated
	}
	writeJSON(w, code, blobOut{Meta: m, URL: store.ImagePath(m.SHA256), Created: created})
}

// blobFile serves an image by hash. The bytes never change, so they are
// cached for good; the type is the sniffed one and the browser may not guess.
func (s *Server) blobFile(w http.ResponseWriter, r *http.Request) {
	b, m, err := s.blobs.Get(r.PathValue("sha256"))
	if err != nil {
		if errors.Is(err, blob.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", m.Type)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "public, max-age=31536000, immutable")
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	h.Set("ETag", `"`+m.SHA256+`"`)
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(b))
}

// attach turns the images a client names (hash and file name) into the
// metadata a post stores, read from the blob store. An image that was not
// uploaded first is refused.
func (s *Server) attach(refs []store.Image) ([]store.Image, error) {
	if len(refs) > store.MaxImages {
		return nil, invalidf("at most %d images on one post, got %d", store.MaxImages, len(refs))
	}
	var out []store.Image
	for _, ref := range refs {
		m, err := s.blobs.Stat(ref.SHA256)
		if errors.Is(err, blob.ErrNotFound) {
			return nil, invalidf("image %q is not on this board; upload it to /api/blobs first", ref.SHA256)
		}
		if err != nil {
			return nil, err
		}
		out = append(out, store.Image{SHA256: m.SHA256, Name: store.CleanName(ref.Name), Type: m.Type, Size: m.Size, Width: m.Width, Height: m.Height})
	}
	return out, nil
}

// formImages reads the "images" files of a multipart form, checks every one
// before storing any, then stores them. A refused file stores nothing.
func (s *Server) formImages(r *http.Request) ([]store.Image, error) {
	if r.MultipartForm == nil {
		return nil, nil
	}
	var files []*multipart.FileHeader
	for _, fh := range r.MultipartForm.File["images"] {
		if fh.Filename == "" && fh.Size == 0 {
			continue // the browser sends an empty part when no file is chosen
		}
		files = append(files, fh)
	}
	if len(files) > store.MaxImages {
		return nil, fmt.Errorf("at most %d images on one post, got %d", store.MaxImages, len(files))
	}
	datas := make([][]byte, len(files))
	for i, fh := range files {
		f, err := fh.Open()
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(io.LimitReader(f, s.blobs.MaxBytes()+1))
		f.Close()
		if err != nil {
			return nil, err
		}
		if _, err := s.blobs.Check(b); err != nil {
			return nil, fmt.Errorf("%s: %w", fh.Filename, err)
		}
		datas[i] = b
	}
	var out []store.Image
	for i, b := range datas {
		m, _, err := s.blobs.Put(b)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", files[i].Filename, err)
		}
		out = append(out, store.Image{SHA256: m.SHA256, Name: store.CleanName(files[i].Filename), Type: m.Type, Size: m.Size, Width: m.Width, Height: m.Height})
	}
	return out, nil
}

// parseForm reads a plain or multipart form, bounded by what eight images
// and a body can need.
func (s *Server) parseForm(w http.ResponseWriter, r *http.Request) error {
	r.Body = http.MaxBytesReader(w, r.Body, store.MaxImages*s.blobs.MaxBytes()+4<<20)
	if err := r.ParseMultipartForm(32 << 20); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		return err
	}
	return nil
}
