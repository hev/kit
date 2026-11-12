package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hev/kit/internal/blob"
	"github.com/hev/kit/internal/board/store"
)

// bulk moves image bytes; it allows longer than the JSON calls.
var bulk = &http.Client{Timeout: 2 * time.Minute}

type upload struct {
	name string
	data []byte
}

// readImages reads the files named by --image and refuses, before anything
// is sent, more than a post may carry, a file that is not an accepted image,
// or one over the server's size cap, so a refused post leaves nothing on the
// board. The server checks every upload again.
func readImages(paths []string) ([]upload, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	if len(paths) > store.MaxImages {
		return nil, fmt.Errorf("at most %d images on one post, got %d", store.MaxImages, len(paths))
	}
	var limits struct {
		MaxBytes int64 `json:"max_bytes"`
	}
	_ = call("GET", "/api/blobs", nil, &limits) // an older server has no limits route; it still refuses on upload
	check := blob.New("", limits.MaxBytes)
	var out []upload
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		if limits.MaxBytes > 0 {
			_, err = check.Check(b)
		} else {
			_, err = blob.Inspect(b)
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		out = append(out, upload{name: filepath.Base(p), data: b})
	}
	return out, nil
}

// sendImages uploads each image and returns the references a post carries.
func sendImages(ups []upload) ([]map[string]string, error) {
	var refs []map[string]string
	for _, u := range ups {
		var got struct {
			SHA256 string `json:"sha256"`
		}
		if err := send("POST", "/api/blobs", "application/octet-stream", bytes.NewReader(u.data), &got, bulk); err != nil {
			return nil, fmt.Errorf("%s: %w", u.name, err)
		}
		refs = append(refs, map[string]string{"sha256": got.SHA256, "name": u.name})
	}
	return refs, nil
}

// absolute makes each image URL a full URL on the board the client talks to.
func absolute(posts []store.Post) {
	for i := range posts {
		for j := range posts[i].Images {
			if u := posts[i].Images[j].URL; strings.HasPrefix(u, "/") {
				posts[i].Images[j].URL = baseURL() + u
			}
		}
	}
}

// saveImages downloads every image in the thread into dir, checks each one's
// sha256, and records where it went in Path.
func saveImages(posts []store.Post, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for i := range posts {
		for j := range posts[i].Images {
			im := &posts[i].Images[j]
			b, err := fetch(im.URL)
			if err != nil {
				return fmt.Errorf("image %s: %w", im.SHA256, err)
			}
			sum := sha256.Sum256(b)
			if hex.EncodeToString(sum[:]) != im.SHA256 {
				return fmt.Errorf("image %s: the board returned different bytes", im.SHA256)
			}
			p := filepath.Join(dir, localName(posts[i].N, j+1, *im))
			if err := os.WriteFile(p, b, 0o644); err != nil {
				return err
			}
			if abs, err := filepath.Abs(p); err == nil {
				p = abs
			}
			im.Path = p
		}
	}
	return nil
}

// localName is p<post>-<image>-<name>, so names never collide in one thread
// and sort in reading order. A name without the type's extension gets one.
func localName(n, i int, im store.Image) string {
	name := store.CleanName(im.Name)
	if name == "" || strings.HasPrefix(name, ".") {
		name = im.SHA256[:12] + name
	}
	if ext := blob.Ext(im.Type); ext != "" && !strings.EqualFold(filepath.Ext(name), ext) &&
		!(ext == ".jpg" && strings.EqualFold(filepath.Ext(name), ".jpeg")) {
		name += ext
	}
	return fmt.Sprintf("p%d-%d-%s", n, i, name)
}

func fetch(u string) ([]byte, error) {
	resp, err := bulk.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<30))
}

// printImages lists a message's images under it.
func printImages(ims []store.Image) {
	if len(ims) == 0 {
		return
	}
	fmt.Println()
	for _, im := range ims {
		name := im.Name
		if name == "" {
			name = im.SHA256[:12]
		}
		fmt.Printf("[image] %s · %s · %d×%d · %s\n        %s\n", name, im.Type, im.Width, im.Height, blob.Human(im.Size), im.URL)
	}
}

func imagesNote(n int) string {
	switch n {
	case 0:
		return ""
	case 1:
		return " · 1 image"
	}
	return fmt.Sprintf(" · %d images", n)
}
