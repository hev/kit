package store

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/hev/kit/internal/board/layer"
)

// MaxImages is how many images one post or reply may carry.
const MaxImages = 8

// Image is an image attached to a post. The bytes live in the blob store on
// the serving host's disk; a post (a thread file, or a Layer row) carries only
// this metadata. Type, size and dimensions come from the stored bytes, never
// from the poster.
type Image struct {
	SHA256 string `json:"sha256"`
	Name   string `json:"name,omitempty"`
	Type   string `json:"type"`
	Size   int64  `json:"size"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	URL    string `json:"url,omitempty"`  // filled on read: ImagePath(SHA256)
	Path   string `json:"path,omitempty"` // filled by `hev board read --images`
}

// ImagePath is the server route that serves an image's bytes.
func ImagePath(sha string) string { return "/blob/" + sha }

// CleanName keeps the base name of an uploaded file, without control
// characters or path separators, at most 120 runes.
func CleanName(name string) string {
	name = filepath.Base(strings.ReplaceAll(strings.TrimSpace(name), `\`, "/"))
	if name == "." || name == "/" {
		return ""
	}
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '/' {
			return -1
		}
		return r
	}, name)
	if r := []rune(name); len(r) > 120 {
		name = string(r[:120])
	}
	return name
}

// stored is the metadata as it is persisted: no URL, no local path.
func stored(in []Image) []Image {
	if len(in) == 0 {
		return nil
	}
	out := make([]Image, len(in))
	for i, im := range in {
		im.URL, im.Path = "", ""
		out[i] = im
	}
	return out
}

// served is the metadata as it is read back: with each image's URL.
func served(in []Image) []Image {
	if len(in) == 0 {
		return nil
	}
	out := make([]Image, len(in))
	for i, im := range in {
		im.URL, im.Path = ImagePath(im.SHA256), ""
		out[i] = im
	}
	return out
}

func checkImages(in []Image) error {
	if len(in) > MaxImages {
		return invalid("at most %d images on one post, got %d", MaxImages, len(in))
	}
	for _, im := range in {
		if im.SHA256 == "" || im.Type == "" {
			return invalid("an image needs its hash and type")
		}
	}
	return nil
}

// Layer rows carry images as one JSON string attribute, "images", present only
// on rows that have images, and threads carry "image_count" for the whole
// thread once any post in it has an image. Rows without images are unchanged.
func imagesAttr(in []Image) (string, error) {
	b, err := json.Marshal(stored(in))
	return string(b), err
}

func imagesOf(r layer.Row) []Image {
	s := str(r, "images")
	if s == "" {
		return nil
	}
	var out []Image
	if json.Unmarshal([]byte(s), &out) != nil {
		return nil
	}
	return served(out)
}
