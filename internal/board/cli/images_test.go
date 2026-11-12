package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
)

func picture(w, h int) *image.RGBA {
	m := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			m.Set(x, y, color.RGBA{uint8(x), uint8(y), uint8(x ^ y), 255})
		}
	}
	return m
}

func writeImage(t *testing.T, dir, name string, enc func(io.Writer, image.Image) error, w, h int) (path, sum string) {
	t.Helper()
	var b bytes.Buffer
	if err := enc(&b, picture(w, h)); err != nil {
		t.Fatal(err)
	}
	path = filepath.Join(dir, name)
	if err := os.WriteFile(path, b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	s := sha256.Sum256(b.Bytes())
	return path, hex.EncodeToString(s[:])
}

func encPNG(w io.Writer, m image.Image) error  { return png.Encode(w, m) }
func encJPEG(w io.Writer, m image.Image) error { return jpeg.Encode(w, m, nil) }
func encGIF(w io.Writer, m image.Image) error  { return gif.Encode(w, m, nil) }

// noisePNG is a PNG that does not compress, about 4*w*h bytes.
func noisePNG(t *testing.T, dir, name string, w, h int) string {
	t.Helper()
	m := image.NewRGBA(image.Rect(0, 0, w, h))
	rand.New(rand.NewSource(1)).Read(m.Pix)
	var b bytes.Buffer
	png.Encode(&b, m)
	p := filepath.Join(dir, name)
	os.WriteFile(p, b.Bytes(), 0o600)
	return p
}

// blobFiles lists the image files under the board directory's blob store.
func blobFiles(t *testing.T, boardDir string) []string {
	t.Helper()
	var out []string
	filepath.WalkDir(filepath.Join(boardDir, "blobs"), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			out = append(out, filepath.Base(p))
		}
		return nil
	})
	sort.Strings(out)
	return out
}

type readOut struct {
	Thread struct {
		ID         int64 `json:"id"`
		ImageCount int   `json:"image_count"`
	} `json:"thread"`
	Posts []struct {
		N      int `json:"n"`
		Images []struct {
			SHA256 string `json:"sha256"`
			Name   string `json:"name"`
			Type   string `json:"type"`
			Size   int64  `json:"size"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
			URL    string `json:"url"`
			Path   string `json:"path"`
		} `json:"images"`
	} `json:"posts"`
}

// imageFlow is the acceptance flow both storage modes must pass: post with
// two images, reply with one, read them back and save them.
func imageFlow(t *testing.T, boardDir string) (threadID string, sums map[string]string) {
	t.Helper()
	src := t.TempDir()
	shot, shotSum := writeImage(t, src, "shot.png", encPNG, 64, 48)
	photo, photoSum := writeImage(t, src, "photo.jpg", encJPEG, 40, 30)
	anim, animSum := writeImage(t, src, "frame.gif", encGIF, 12, 9)
	sums = map[string]string{"shot.png": shotSum, "photo.jpg": photoSum, "frame.gif": animSum}

	var th struct {
		ID         int64 `json:"id"`
		ImageCount int   `json:"image_count"`
	}
	decode(t, must(t, "post", "--board", "research", "--title", "What the login page looked like",
		"-m", "Two captures of the broken login page.", "--image", shot, "--image", photo, "--json"), &th)
	if th.ImageCount != 2 {
		t.Fatalf("post: %+v", th)
	}
	threadID = fmt.Sprint(th.ID)
	if out := must(t, "reply", threadID, "-m", "The next frame.", "--image", anim); !strings.Contains(out, "1 image") {
		t.Fatalf("reply: %s", out)
	}

	var got readOut
	decode(t, must(t, "read", threadID, "--json"), &got)
	if got.Thread.ImageCount != 3 || len(got.Posts) != 2 || len(got.Posts[0].Images) != 2 || len(got.Posts[1].Images) != 1 {
		t.Fatalf("read: %+v", got)
	}
	want := []struct {
		name, typ string
		w, h      int
	}{{"shot.png", "image/png", 64, 48}, {"photo.jpg", "image/jpeg", 40, 30}, {"frame.gif", "image/gif", 12, 9}}
	all := append(got.Posts[0].Images, got.Posts[1].Images...)
	for i, im := range all {
		w := want[i]
		if im.Name != w.name || im.Type != w.typ || im.Width != w.w || im.Height != w.h || im.SHA256 != sums[w.name] || im.Size == 0 ||
			im.URL != os.Getenv("FACTORY_BOARD_URL")+"/blob/"+im.SHA256 {
			t.Errorf("image %d: %+v, want %+v", i, im, w)
		}
	}

	text := must(t, "read", threadID)
	for _, line := range []string{"[image] shot.png · image/png · 64×48", "[image] photo.jpg · image/jpeg · 40×30", "[image] frame.gif · image/gif · 12×9", "/blob/" + animSum} {
		if !strings.Contains(text, line) {
			t.Errorf("read text lacks %q:\n%s", line, text)
		}
	}
	if strings.Index(text, "frame.gif") < strings.Index(text, "The next frame.") {
		t.Errorf("the reply's image is not listed under the reply:\n%s", text)
	}

	dest := filepath.Join(t.TempDir(), "seen")
	paths := strings.Fields(must(t, "read", threadID, "--images", dest))
	if len(paths) != 3 {
		t.Fatalf("read --images printed %q", paths)
	}
	for i, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		s := sha256.Sum256(b)
		if hex.EncodeToString(s[:]) != sums[want[i].name] || filepath.Dir(p) != dest {
			t.Errorf("%s does not match %s", p, want[i].name)
		}
	}
	var saved readOut
	decode(t, must(t, "read", threadID, "--images", dest, "--json"), &saved)
	if saved.Posts[1].Images[0].Path != paths[2] {
		t.Errorf("read --images --json: %+v", saved.Posts[1].Images)
	}

	// ls and search mark threads with images.
	if out := must(t, "ls"); !strings.Contains(out, "3 images") {
		t.Errorf("ls does not mark images:\n%s", out)
	}
	if blobs := blobFiles(t, boardDir); !reflect.DeepEqual(blobs, sorted(shotSum, photoSum, animSum)) {
		t.Errorf("blobs on disk %v", blobs)
	}
	return threadID, sums
}

func sorted(s ...string) []string { sort.Strings(s); return s }

func TestImagesWithoutAKey(t *testing.T) {
	withoutKey(t)
	dir := filepath.Join(t.TempDir(), "board")
	t.Setenv("HEV_BOARD_DIR", dir)
	t.Setenv("FACTORY_CANVAS_DIR", filepath.Join(t.TempDir(), "canvas"))
	t.Setenv("FACTORY_BOARD_LAYER_ENDPOINT", layerTrap(t))
	t.Setenv("FACTORY_BOARD_AS", "agent-eyes")
	t.Setenv("HEV_BOARD_MAX_IMAGE_BYTES", "200000")
	startBoard(t)

	id, sums := imageFlow(t, dir)
	if out := must(t, "search", "login page"); !strings.Contains(out, "3 images") {
		t.Errorf("search does not mark images:\n%s", out)
	}
	var ts []struct {
		ImageCount int `json:"image_count"`
	}
	decode(t, must(t, "ls", "--json"), &ts)
	if len(ts) != 1 || ts[0].ImageCount != 3 {
		t.Errorf("ls --json: %+v", ts)
	}

	// The thread file carries metadata only.
	raw, err := os.ReadFile(filepath.Join(dir, "threads", id+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) > 4096 || !strings.Contains(string(raw), sums["shot.png"]) || strings.Contains(string(raw), `"url"`) {
		t.Errorf("thread file: %s", raw)
	}

	// The same image again is stored once.
	before := blobFiles(t, dir)
	shot, _ := writeImage(t, t.TempDir(), "again.png", encPNG, 64, 48)
	must(t, "reply", id, "-m", "Same capture twice.", "--image", shot, "--image", shot)
	must(t, "post", "--title", "Same capture elsewhere", "-m", "again", "--image", shot)
	if after := blobFiles(t, dir); !reflect.DeepEqual(after, before) {
		t.Fatalf("a repeated image was stored again: %v -> %v", before, after)
	}

	refusals(t, dir)
}

// refusals: each bad file is refused with a clear error and stores nothing.
func refusals(t *testing.T, dir string) {
	t.Helper()
	src := t.TempDir()
	svg := filepath.Join(src, "diagram.svg")
	os.WriteFile(svg, []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`), 0o600)
	notes := filepath.Join(src, "notes.txt")
	os.WriteFile(notes, []byte("plain notes, not a picture\n"), 0o600)
	fake := filepath.Join(src, "screenshot.png")
	os.WriteFile(fake, []byte("#!/bin/sh\necho this is a script named like a picture\n"), 0o600)
	big := noisePNG(t, src, "huge.png", 300, 300) // ~360 KB against a 200 KB cap
	good, _ := writeImage(t, src, "fine.png", encPNG, 8, 8)

	var before []struct{ ID int64 }
	decode(t, must(t, "ls", "--json", "-n", "100"), &before)
	blobsBefore := blobFiles(t, dir)
	var thread1 readOut
	decode(t, must(t, "read", "1", "--json"), &thread1)
	postsBefore := len(thread1.Posts)
	for _, c := range []struct {
		name string
		args []string
		says string
	}{
		{"svg", []string{"--image", svg}, "SVG is not accepted"},
		{"non-image", []string{"--image", notes}, "only PNG, JPEG, GIF and WebP"},
		{"image extension, other content", []string{"--image", fake}, "only PNG, JPEG, GIF and WebP"},
		{"over the cap", []string{"--image", big}, "over the 195.3 KB limit"},
		{"over the cap beside a good one", []string{"--image", good, "--image", big}, "over the"},
		{"nine images", []string{"--image", good, "--image", good, "--image", good, "--image", good, "--image", good, "--image", good, "--image", good, "--image", good, "--image", good}, "at most 8"},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, verb := range [][]string{{"post", "--title", "refused " + c.name, "-m", "x"}, {"reply", "1", "-m", "x"}} {
				_, err := run(t, append(verb, c.args...)...)
				if err == nil || !strings.Contains(err.Error(), c.says) {
					t.Errorf("%s %s: %v", verb[0], c.name, err)
				}
			}
		})
	}
	var after []struct{ ID int64 }
	decode(t, must(t, "ls", "--json", "-n", "100"), &after)
	if len(after) != len(before) {
		t.Errorf("a refused post was stored: %d threads, then %d", len(before), len(after))
	}
	var read readOut
	decode(t, must(t, "read", "1", "--json"), &read)
	if len(read.Posts) != postsBefore {
		t.Errorf("a refused reply was stored: %d posts, then %d", postsBefore, len(read.Posts))
	}
	got := blobFiles(t, dir)
	if !reflect.DeepEqual(got, blobsBefore) {
		t.Errorf("refused images were stored: %v -> %v", blobsBefore, got)
	}

	// The server refuses the same bytes on its own, whatever the client did.
	for name, path := range map[string]string{"svg": svg, "text": notes, "fake png": fake, "big": big} {
		b, _ := os.ReadFile(path)
		resp, err := http.Post(os.Getenv("FACTORY_BOARD_URL")+"/api/blobs", "image/png", bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnsupportedMediaType && resp.StatusCode != http.StatusRequestEntityTooLarge {
			t.Errorf("server took %s: %d %s", name, resp.StatusCode, body)
		}
	}
	if final := blobFiles(t, dir); len(final) != len(got) {
		t.Errorf("the server stored a refused upload: %v -> %v", got, final)
	}
}

// fakeLayer is an in-memory Layer namespace: enough of upsert, patch and
// filtered queries for the board, and nothing else.
type fakeLayer struct {
	mu   sync.Mutex
	rows map[string]map[string]any
}

func (f *fakeLayer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var body map[string]any
	json.NewDecoder(r.Body).Decode(&body)
	w.Header().Set("Content-Type", "application/json")
	if !strings.HasSuffix(r.URL.Path, "/query") {
		for _, x := range asList(body["upsert_rows"]) {
			row := x.(map[string]any)
			f.rows[row["id"].(string)] = row
		}
		for _, x := range asList(body["patch_rows"]) {
			p := x.(map[string]any)
			if row, ok := f.rows[p["id"].(string)]; ok {
				for k, v := range p {
					row[k] = v
				}
			}
		}
		w.Write([]byte(`{"status":"OK"}`))
		return
	}
	var out []map[string]any
	for _, row := range f.rows {
		if match(row, body["filters"]) {
			out = append(out, row)
		}
	}
	if rb := asList(body["rank_by"]); len(rb) == 2 {
		key, desc := rb[0].(string), rb[1] == "desc"
		sort.SliceStable(out, func(i, j int) bool {
			a, b := fmt.Sprint(out[i][key]), fmt.Sprint(out[j][key])
			if x, ok := out[i][key].(float64); ok {
				y, _ := out[j][key].(float64)
				return (x < y) != desc && x != y
			}
			return (a < b) != desc && a != b
		})
	}
	if k, ok := body["top_k"].(float64); ok && len(out) > int(k) {
		out = out[:int(k)]
	}
	json.NewEncoder(w).Encode(map[string]any{"rows": out})
}

func asList(v any) []any { l, _ := v.([]any); return l }

func match(row map[string]any, f any) bool {
	c := asList(f)
	if len(c) == 0 {
		return true
	}
	if c[0] == "And" {
		for _, x := range asList(c[1]) {
			if !match(row, x) {
				return false
			}
		}
		return true
	}
	v := row[c[0].(string)]
	switch c[1] {
	case "Eq":
		return fmt.Sprint(v) == fmt.Sprint(c[2])
	case "In":
		for _, x := range asList(c[2]) {
			if fmt.Sprint(v) == fmt.Sprint(x) {
				return true
			}
		}
	case "Contains":
		for _, x := range asList(v) {
			if fmt.Sprint(x) == fmt.Sprint(c[2]) {
				return true
			}
		}
	}
	return false
}

func TestImagesWithALayerKey(t *testing.T) {
	withoutKey(t)
	dir := filepath.Join(t.TempDir(), "board")
	t.Setenv("HEV_BOARD_DIR", dir)
	t.Setenv("HEV_BOARD_MAX_IMAGE_BYTES", "200000")
	fake := &fakeLayer{rows: map[string]map[string]any{}}
	ts := httptest.NewServer(fake)
	defer ts.Close()
	t.Setenv("FACTORY_BOARD_LAYER_ENDPOINT", ts.URL)
	t.Setenv("FACTORY_BOARD_NAMESPACE", "board-images")
	t.Setenv("HEVLAYER_API_KEY", "test-layer-key")
	t.Setenv("FACTORY_BOARD_AS", "agent-eyes")
	if line := startBoard(t).String(); !strings.Contains(line, "layer mode") {
		t.Fatalf("not in Layer mode: %s", line)
	}

	id, sums := imageFlow(t, dir)

	fake.mu.Lock()
	thread, reply := fake.rows["t:"+id], fake.rows["p:"+id+":2"]
	fake.mu.Unlock()
	if thread == nil || reply == nil {
		t.Fatalf("rows: %v", fake.rows)
	}
	if thread["image_count"] != float64(3) {
		t.Errorf("thread image_count %v", thread["image_count"])
	}
	for _, row := range []map[string]any{thread, reply} {
		s, _ := row["images"].(string)
		var ims []map[string]any
		if err := json.Unmarshal([]byte(s), &ims); err != nil || len(ims) == 0 {
			t.Fatalf("images attribute %q: %v", s, err)
		}
		for _, im := range ims {
			keys := []string{}
			for k := range im {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			if !reflect.DeepEqual(keys, []string{"height", "name", "sha256", "size", "type", "width"}) {
				t.Errorf("row image carries %v", keys)
			}
		}
		if len(s) > 1024 {
			t.Errorf("row carries %d bytes of images; bytes belong on disk", len(s))
		}
	}
	for name, sum := range sums {
		if _, err := os.Stat(filepath.Join(dir, "blobs", "sha256", sum[:2], sum)); err != nil {
			t.Errorf("%s is not on local disk: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "threads")); !os.IsNotExist(err) {
		t.Errorf("Layer mode wrote thread files: %v", err)
	}
}

func TestReadOnlyServeRefusesAnUpload(t *testing.T) {
	withoutKey(t)
	dir := filepath.Join(t.TempDir(), "board")
	t.Setenv("HEV_BOARD_DIR", dir)
	config := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(config, []byte("[board]\naddr = \"127.0.0.1:0\"\nread_only = true\n"), 0o600)
	t.Setenv("HEV_CONFIG", config) // what `hev board serve --read-only` sets
	t.Setenv("FACTORY_BOARD_AS", "agent-eyes")
	startBoard(t)
	shot, _ := writeImage(t, t.TempDir(), "shot.png", encPNG, 8, 8)
	if _, err := run(t, "post", "--title", "t", "-m", "b", "--image", shot); err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("post with an image on a read-only board: %v", err)
	}
	if blobs := blobFiles(t, dir); len(blobs) != 0 {
		t.Fatalf("stored %v", blobs)
	}
}
