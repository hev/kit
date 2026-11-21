package v1

import (
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/hev/kit/internal/blob"
	"github.com/hev/kit/internal/board/store"
)

type boardView struct {
	Slug    string    `json:"slug"`
	Name    string    `json:"name"`
	Blurb   string    `json:"blurb,omitempty"`
	Sort    int       `json:"sort"`
	Threads int       `json:"threads"`
	Posts   int       `json:"posts"`
	Last    *replyRef `json:"last,omitempty"`
}

type replyRef struct {
	PostID  int64     `json:"post_id"`
	N       int       `json:"n"`
	Author  string    `json:"author"`
	Created time.Time `json:"created"`
	Title   string    `json:"title,omitempty"`
	Board   string    `json:"board,omitempty"`
}

func ref(p *store.Post) *replyRef {
	if p == nil {
		return nil
	}
	return &replyRef{PostID: p.ThreadID, N: p.N, Author: p.Author, Created: p.Created, Title: p.Title, Board: p.Board}
}

type postView struct {
	store.Poster
	ID         int64         `json:"id"`
	Board      string        `json:"board"`
	Title      string        `json:"title"`
	Tags       []string      `json:"tags"`
	Body       *string       `json:"body,omitempty"`
	Excerpt    string        `json:"excerpt,omitempty"`
	Images     []store.Image `json:"images,omitempty"`
	ImageCount int           `json:"image_count,omitempty"`
	Created    time.Time     `json:"created"`
	Updated    time.Time     `json:"updated"`
	Views      int           `json:"views"`
	Useful     int           `json:"useful"`
	Replies    int           `json:"replies"`
	Pinned     bool          `json:"pinned"`
	Locked     bool          `json:"locked"`
	Last       *replyRef     `json:"last,omitempty"`
}

func view(t store.Thread, op *store.Post) postView {
	v := postView{Poster: t.Poster, ID: t.ID, Board: t.Board, Title: t.Title, Tags: alsoOrEmpty(t.Tags),
		Created: t.Created, Updated: t.Updated, Views: t.Views, Useful: t.Useful, Replies: t.Replies, Pinned: t.Pinned,
		Locked: t.Locked, Last: ref(t.Last), Excerpt: t.Excerpt, ImageCount: t.ImageCount}
	if op != nil {
		v.Body = &op.Body
		v.Images = imageLinks(op.Images)
	}
	return v
}

type replyView struct {
	store.Poster
	ID      string        `json:"id"`
	PostID  int64         `json:"post_id"`
	N       int           `json:"n"`
	Body    string        `json:"body"`
	Created time.Time     `json:"created"`
	Images  []store.Image `json:"images,omitempty"`
}

func replyOf(p store.Post) replyView {
	return replyView{Poster: p.Poster, ID: p.ID, PostID: p.ThreadID, N: p.N, Body: p.Body, Created: p.Created, Images: imageLinks(p.Images)}
}

type hitView struct {
	Post    postView   `json:"post"`
	N       int        `json:"n"`
	Reply   *replyView `json:"reply,omitempty"`
	Snippet string     `json:"snippet"`
	Score   float64    `json:"score"`
}

type imageRef struct {
	SHA256 string `json:"sha256"`
	Name   string `json:"name"`
}

func imageURL(sha string) string { return "/v1/board/blobs/" + sha }

func imageLinks(images []store.Image) []store.Image {
	out := append([]store.Image(nil), images...)
	for i := range out {
		out[i].URL, out[i].Path = imageURL(out[i].SHA256), ""
	}
	return out
}

func (h *handler) images(refs []imageRef) ([]store.Image, error) {
	if len(refs) > store.MaxImages {
		return nil, store.ErrInvalid
	}
	images := make([]store.Image, 0, len(refs))
	for _, r := range refs {
		if !blob.ValidHash(r.SHA256) || utf8.RuneCountInString(r.Name) > 120 {
			return nil, store.ErrInvalid
		}
		meta, err := h.blobs.Stat(r.SHA256)
		if err != nil {
			return nil, store.ErrInvalid
		}
		images = append(images, store.Image{SHA256: meta.SHA256, Name: store.CleanName(r.Name), Type: meta.Type, Size: meta.Size, Width: meta.Width, Height: meta.Height})
	}
	return images, nil
}

func (h *handler) limits(w http.ResponseWriter, _ *http.Request) {
	respond(w, 200, map[string]any{"max_bytes": h.blobs.MaxBytes(), "max_per_post": store.MaxImages, "types": blob.Accepted})
}

func (h *handler) upload(w http.ResponseWriter, r *http.Request) {
	meta, created, err := h.blobs.PutReader(r.Body)
	if err != nil {
		fail(w, err)
		return
	}
	status := 200
	if created {
		status = 201
	}
	respond(w, status, struct {
		blob.Meta
		URL     string `json:"url"`
		Created bool   `json:"created"`
	}{meta, imageURL(meta.SHA256), created})
}

func (h *handler) download(w http.ResponseWriter, r *http.Request) {
	sha := r.PathValue("sha256")
	if !blob.ValidHash(sha) {
		fail(w, blob.ErrNotFound)
		return
	}
	raw, meta, err := h.blobs.Get(sha)
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("ETag", `"`+sha+`"`)
	w.Header().Set("Content-Type", meta.Type)
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// A stored response cannot satisfy another request without verification.
	w.Header().Set("Vary", "*")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.WriteHeader(200)
	if r.Method != http.MethodHead {
		_, _ = w.Write(raw)
	}
}
