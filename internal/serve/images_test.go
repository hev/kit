package serve

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/blob"
	"github.com/hev/kit/internal/filearchive"
	"github.com/hev/kit/internal/trace"
)

func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		img.Set(x, x%h, color.RGBA{10, 200, 30, 255})
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// imageArchive is a file archive holding one session with a pasted image in
// the prompt and a screenshot in a tool result, its bytes in the blob store.
func imageArchive(t *testing.T) (*filearchive.Store, *blob.Store, blob.Meta, blob.Meta) {
	t.Helper()
	store, err := filearchive.Open(t.TempDir(), "hev-traces")
	if err != nil {
		t.Fatal(err)
	}
	blobs := blob.New(filepath.Join(store.Dir(), "blobs"), 0)
	pasted, _, err := blobs.Put(testPNG(t, 30, 20))
	if err != nil {
		t.Fatal(err)
	}
	shot, _, err := blobs.Put(testPNG(t, 64, 40))
	if err != nil {
		t.Fatal(err)
	}
	img := func(m blob.Meta, index int) trace.Image {
		return trace.Image{Index: index, SHA256: m.SHA256, Type: m.Type, Size: m.Size, Width: m.Width, Height: m.Height}
	}
	start := time.Now().Add(-time.Hour).UnixMilli()
	p, s := img(pasted, 0), img(shot, 1)
	rows := []trace.BlockRow{
		{ID: "b0", SessionID: "imgs0001", TurnUUID: "u1", Seq: 0, Role: "user", BlockType: "text", Text: p.Marker(), Images: trace.Images{p}, Start: start, End: start, OK: true},
		{ID: "b1", SessionID: "imgs0001", TurnUUID: "u1", Seq: 1, Role: "user", BlockType: "text", Text: "fix the header", Start: start, End: start, OK: true},
		{ID: "b2", SessionID: "imgs0001", TurnUUID: "a1", Seq: 2, Role: "assistant", BlockType: "tool_use", ToolName: "screenshot", ToolUseID: "t1", Text: "screenshot", Start: start + 1000, End: start + 2000, OK: true, RequestID: "r1"},
		{ID: "b3", SessionID: "imgs0001", TurnUUID: "u2", Seq: 3, Role: "user", BlockType: "tool_result", ToolUseID: "t1", Text: "taken\n" + s.Marker(), Images: trace.Images{s}, Start: start + 2000, End: start + 2000, OK: true},
	}
	if _, err := store.WriteBlocks(rows); err != nil {
		t.Fatal(err)
	}
	if _, err := store.WriteSessions([]trace.SessionRow{{ID: "imgs0001", SessionID: "imgs0001", Harness: "claude_code", Host: "capture-host", Start: start, End: start + 3000, FirstPrompt: "fix the header", PromptCount: 1}}); err != nil {
		t.Fatal(err)
	}
	return store, blobs, pasted, shot
}

func sessionImages(t *testing.T, body []byte) []map[string]any {
	t.Helper()
	var data struct {
		Turns []struct {
			Images []map[string]any `json:"images"`
			Items  []map[string]any `json:"items"`
		} `json:"turns"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		t.Fatal(err)
	}
	var out []map[string]any
	for _, turn := range data.Turns {
		out = append(out, turn.Images...)
		for _, it := range turn.Items {
			if list, ok := it["images"].([]any); ok {
				for _, im := range list {
					out = append(out, im.(map[string]any))
				}
			}
		}
	}
	return out
}

// TestServeShowsSessionImages: the session view carries the images inline
// when the bytes are on this host, and the image route serves them with the
// sniffed type and nosniff.
func TestServeShowsSessionImages(t *testing.T) {
	store, blobs, pasted, shot := imageArchive(t)
	h := New(store).WithCacheTTL(0).WithImages(blobs).Handler()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/session/imgs0001", nil))
	if w.Code != 200 {
		t.Fatalf("session %d: %s", w.Code, w.Body.String())
	}
	images := sessionImages(t, w.Body.Bytes())
	if len(images) != 2 {
		t.Fatalf("images = %+v", images)
	}
	for i, want := range []blob.Meta{pasted, shot} {
		im := images[i]
		if im["sha256"] != want.SHA256 || im["local"] != true || im["url"] != "/api/image/"+want.SHA256 || im["width"].(float64) != float64(want.Width) {
			t.Fatalf("image %d = %+v", i, im)
		}
		r := httptest.NewRecorder()
		h.ServeHTTP(r, httptest.NewRequest("GET", im["url"].(string), nil))
		got, _, _ := blobs.Get(want.SHA256)
		if r.Code != 200 || r.Header().Get("Content-Type") != "image/png" || r.Header().Get("X-Content-Type-Options") != "nosniff" ||
			!strings.Contains(r.Header().Get("Content-Security-Policy"), "default-src 'none'") || !bytes.Equal(r.Body.Bytes(), got) {
			t.Fatalf("image route %d %v", r.Code, r.Header())
		}
	}
	// An unknown or malformed hash is a 404, never a path.
	for _, path := range []string{"/api/image/" + strings.Repeat("0", 64), "/api/image/..%2f..%2fmeta.json"} {
		r := httptest.NewRecorder()
		h.ServeHTTP(r, httptest.NewRequest("GET", path, nil))
		if r.Code != 404 {
			t.Fatalf("%s = %d", path, r.Code)
		}
	}
	// The page renders those entries as image elements.
	page := httptest.NewRecorder()
	h.ServeHTTP(page, httptest.NewRequest("GET", "/", nil))
	body := page.Body.String()
	for _, want := range []string{"function renderImages", "el('img',{src:im.url", "renderImages(it.images)", "renderImages(t.images)"} {
		if !strings.Contains(body, want) {
			t.Fatalf("page does not render images: missing %q", want)
		}
	}
}

// Without the bytes on this host, the view keeps the marker and names the
// host that captured them; nothing points at an image URL.
func TestServeImagesFromAnotherHost(t *testing.T) {
	store, _, _, _ := imageArchive(t)
	h := New(store).WithCacheTTL(0).Handler()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/session/imgs0001", nil))
	images := sessionImages(t, w.Body.Bytes())
	if len(images) != 2 {
		t.Fatalf("images = %+v", images)
	}
	for _, im := range images {
		if im["local"] != false || im["url"] != nil || im["host"] != "capture-host" || !strings.HasPrefix(im["marker"].(string), "[image image/png ") {
			t.Fatalf("remote image = %+v", im)
		}
	}
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest("GET", "/api/image/"+strings.Repeat("a", 64), nil))
	if r.Code != 404 {
		t.Fatalf("image route without a store = %d", r.Code)
	}
}
