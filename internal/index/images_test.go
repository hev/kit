package index

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/hev/kit/internal/blob"
	"github.com/hev/kit/internal/filearchive"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
)

// Synthetic images, drawn in the test. No real screenshot is committed.
func drawPNG(t testing.TB, w, h int, seed uint8) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{uint8(x) + seed, uint8(y) * seed, seed, 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func hexSum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func jsonl(t *testing.T, path string, lines ...any) {
	t.Helper()
	var b strings.Builder
	for _, l := range lines {
		raw, err := json.Marshal(l)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(raw)
		b.WriteByte('\n')
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

const imageSession = "1a2b3c4d-0000-4000-8000-00000000beef"

// claudeWithImages writes a Claude Code session with an image pasted into a
// user message and a screenshot in a tool result.
func claudeWithImages(t *testing.T, pasted, shot []byte) string {
	t.Helper()
	root := t.TempDir()
	enc := base64.StdEncoding.EncodeToString
	jsonl(t, filepath.Join(root, "-w", imageSession+".jsonl"),
		map[string]any{"type": "user", "uuid": "u1", "sessionId": imageSession, "timestamp": "2026-11-10T10:00:00Z", "cwd": "/w",
			"message": map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": enc(pasted)}},
				map[string]any{"type": "text", "text": "the header is misaligned in this mock"},
			}}},
		map[string]any{"type": "assistant", "uuid": "a1", "parentUuid": "u1", "sessionId": imageSession, "timestamp": "2026-11-10T10:00:05Z", "cwd": "/w", "requestId": "r1",
			"message": map[string]any{"role": "assistant", "model": "claude-sonnet-4-6", "content": []any{
				map[string]any{"type": "tool_use", "id": "tu1", "name": "mcp__browser__screenshot", "input": map[string]any{}},
			}, "usage": map[string]any{"input_tokens": 5, "output_tokens": 7}}},
		map[string]any{"type": "user", "uuid": "u2", "parentUuid": "a1", "sessionId": imageSession, "timestamp": "2026-11-10T10:00:06Z", "cwd": "/w",
			"message": map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "tool_result", "tool_use_id": "tu1", "content": []any{
					map[string]any{"type": "text", "text": "screenshot of the page"},
					map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": enc(shot)}},
				}},
			}}},
		map[string]any{"type": "assistant", "uuid": "a2", "parentUuid": "u2", "sessionId": imageSession, "timestamp": "2026-11-10T10:00:09Z", "cwd": "/w", "requestId": "r2",
			"message": map[string]any{"role": "assistant", "model": "claude-sonnet-4-6", "content": []any{
				map[string]any{"type": "text", "text": "the flex container is missing align-items"},
			}, "usage": map[string]any{"input_tokens": 9, "output_tokens": 11}}},
	)
	return root
}

const codexImageSession = "c0dec0de-0000-4000-8000-00000000beef"

func codexWithImages(t *testing.T, pasted, shot []byte) string {
	t.Helper()
	root := t.TempDir()
	url := func(b []byte) string { return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b) }
	jsonl(t, filepath.Join(root, "2026", "11", "10", "rollout-"+codexImageSession+".jsonl"),
		map[string]any{"timestamp": "2026-11-10T10:00:00Z", "type": "session_meta", "payload": map[string]any{"id": codexImageSession, "cwd": "/w"}},
		map[string]any{"timestamp": "2026-11-10T10:00:01Z", "type": "response_item", "payload": map[string]any{"type": "message", "role": "user", "id": "m1", "content": []any{
			map[string]any{"type": "input_text", "text": "does the page match this mock?"},
			map[string]any{"type": "input_image", "image_url": url(pasted)},
		}}},
		map[string]any{"timestamp": "2026-11-10T10:00:02Z", "type": "response_item", "payload": map[string]any{"type": "custom_tool_call", "id": "f1", "call_id": "c1", "name": "browser", "input": "{\"action\":\"screenshot\"}"}},
		map[string]any{"timestamp": "2026-11-10T10:00:03Z", "type": "response_item", "payload": map[string]any{"type": "custom_tool_call_output", "id": "o1", "call_id": "c1", "output": []any{
			map[string]any{"type": "input_text", "text": "captured the page"},
			map[string]any{"type": "input_image", "image_url": url(shot), "detail": "auto"},
		}}},
		map[string]any{"timestamp": "2026-11-10T10:00:04Z", "type": "response_item", "payload": map[string]any{"type": "message", "role": "assistant", "id": "m2", "content": []any{
			map[string]any{"type": "output_text", "text": "it matches"},
		}}},
	)
	return root
}

// snapshot is every file under dir with its bytes, so a second run can be
// shown to have written nothing new.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		b, _ := os.ReadFile(p)
		rel, _ := filepath.Rel(dir, p)
		out[rel] = hexSum(b)
		return nil
	})
	return out
}

func blobFiles(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			out = append(out, filepath.Base(p))
		}
		return nil
	})
	return out
}

type imageCase struct {
	name    string
	session string
	src     func(root string) trace.Source
	write   func(t *testing.T, pasted, shot []byte) string
	shotTyp string // block type of the screenshot
}

var imageCases = []imageCase{
	{"claude", imageSession, func(r string) trace.Source { return &trace.ClaudeSource{Root: r} }, claudeWithImages, "tool_result"},
	{"codex", codexImageSession, func(r string) trace.Source { return &trace.CodexSource{Root: r} }, codexWithImages, "tool_result"},
}

// TestIndexImagesIntoFileArchive is the file-mode acceptance for Claude Code
// and Codex: an image in a user message and one in a tool result are both
// stored as blobs whose sha256 matches, the block rows carry the metadata,
// the block text carries the markers, and indexing the session again stores
// and writes nothing new.
func TestIndexImagesIntoFileArchive(t *testing.T) {
	for _, tc := range imageCases {
		t.Run(tc.name, func(t *testing.T) {
			pasted, shot := drawPNG(t, 48, 32, 3), drawPNG(t, 80, 50, 5)
			src := tc.src(tc.write(t, pasted, shot))
			store, err := filearchive.Open(t.TempDir(), "hev-traces")
			if err != nil {
				t.Fatal(err)
			}
			blobsDir := filepath.Join(store.Dir(), "blobs")
			capture := &trace.ImageCapture{Store: blob.New(blobsDir, 0)}
			opt := Options{RepoURL: func(string) string { return "" }, Images: capture, Tiers: trace.AllTiers[:]}
			rep, err := Run(src, store, &State{Units: map[string]string{}}, opt)
			if err != nil || len(rep.Errors) != 0 || rep.UnitsIndexed != 1 || rep.Images.Created != 2 || rep.Images.Stored != 2 {
				t.Fatalf("report = %+v, %v", rep, err)
			}
			for _, want := range [][]byte{pasted, shot} {
				got, err := os.ReadFile(capture.Store.Path(hexSum(want)))
				if err != nil || hexSum(got) != hexSum(want) {
					t.Fatalf("blob %s: %v", hexSum(want), err)
				}
			}
			blocks, err := store.ListBlockRows(tc.session)
			if err != nil {
				t.Fatal(err)
			}
			var got []trace.BlockRow
			for _, b := range blocks {
				if len(b.Images) > 0 {
					got = append(got, b)
				}
			}
			if len(got) != 2 {
				t.Fatalf("blocks with images = %+v", got)
			}
			user, result := got[0], got[1]
			if user.Seq > result.Seq {
				user, result = result, user
			}
			im := user.Images[0]
			if user.Role != "user" || user.BlockType != "text" || im.SHA256 != hexSum(pasted) || im.Type != "image/png" ||
				im.Width != 48 || im.Height != 32 || im.Size != int64(len(pasted)) || im.Index != 0 || !im.Stored() {
				t.Fatalf("user image block = %+v", user)
			}
			if user.Text != im.Marker() || !strings.Contains(user.Text, "sha256:"+hexSum(pasted)[:12]) {
				t.Fatalf("user marker = %q", user.Text)
			}
			im = result.Images[0]
			if result.BlockType != tc.shotTyp || im.SHA256 != hexSum(shot) || im.Width != 80 || im.Height != 50 || im.Index != 1 ||
				!strings.Contains(result.Text, "[image image/png 80x50 ") {
				t.Fatalf("tool result image block = %+v", result)
			}
			// The archive holds metadata only: the base64 is in no file
			// outside the blob store.
			enc := base64.StdEncoding.EncodeToString(shot)[:60]
			filepath.WalkDir(store.Dir(), func(p string, d fs.DirEntry, err error) error {
				if err == nil && !d.IsDir() && !strings.HasPrefix(p, blobsDir) {
					if b, _ := os.ReadFile(p); bytes.Contains(b, []byte(enc)) {
						t.Errorf("image bytes in %s", p)
					}
				}
				return nil
			})
			// Lexical search finds the session by its markers.
			if err := store.Refresh(); err != nil {
				t.Fatal(err)
			}
			hits, err := store.SearchHits("image png", 10, nil)
			if err != nil || len(hits) == 0 || hits[0].SessionID != tc.session {
				t.Fatalf("search for images = %+v, %v", hits, err)
			}

			// Idempotence: re-indexing (forced past the unchanged-unit
			// cache) stores no new blob and changes no file.
			before := snapshot(t, store.Dir())
			again, err := Run(src, store, &State{Units: map[string]string{}}, opt)
			if err != nil || len(again.Errors) != 0 || again.Images.Created != 0 || again.Images.Stored != 2 {
				t.Fatalf("second report = %+v, %v", again, err)
			}
			after := snapshot(t, store.Dir())
			if len(before) != len(after) {
				t.Fatalf("files changed: %d -> %d", len(before), len(after))
			}
			for k, v := range before {
				if after[k] != v {
					t.Errorf("%s rewritten with different bytes", k)
				}
			}
			if n := len(blobFiles(t, blobsDir)); n != 2 {
				t.Fatalf("blobs = %d, want 2", n)
			}
		})
	}
}

// The same image in two sessions is one blob.
func TestSameImageInTwoSessionsIsOneBlob(t *testing.T) {
	shot := drawPNG(t, 30, 30, 9)
	claude := &trace.ClaudeSource{Root: claudeWithImages(t, shot, shot)}
	codex := &trace.CodexSource{Root: codexWithImages(t, shot, shot)}
	store, err := filearchive.Open(t.TempDir(), "hev-traces")
	if err != nil {
		t.Fatal(err)
	}
	capture := &trace.ImageCapture{Store: blob.New(filepath.Join(store.Dir(), "blobs"), 0)}
	created := 0
	for _, src := range []trace.Source{claude, codex} {
		rep, err := Run(src, store, &State{Units: map[string]string{}}, Options{RepoURL: func(string) string { return "" }, Images: capture})
		if err != nil || len(rep.Errors) != 0 || rep.Images.Stored != 2 {
			t.Fatalf("report = %+v, %v", rep, err)
		}
		created += rep.Images.Created
	}
	if files := blobFiles(t, capture.Store.Dir()); created != 1 || len(files) != 1 || files[0] != hexSum(shot) {
		t.Fatalf("created = %d, blobs = %v", created, files)
	}
}

func TestIndexSkipsOversizeAndNonImages(t *testing.T) {
	big := drawPNG(t, 200, 200, 1)
	notImage := []byte("GIF? no: this is a shell transcript labelled image/png")
	src := &trace.ClaudeSource{Root: claudeWithImages(t, big, notImage)}
	store, err := filearchive.Open(t.TempDir(), "hev-traces")
	if err != nil {
		t.Fatal(err)
	}
	capture := &trace.ImageCapture{Store: blob.New(filepath.Join(store.Dir(), "blobs"), 512)}
	rep, err := Run(src, store, &State{Units: map[string]string{}}, Options{RepoURL: func(string) string { return "" }, Images: capture})
	if err != nil || len(rep.Errors) != 0 || rep.Images.Stored != 0 || rep.Images.Skipped != 2 {
		t.Fatalf("report = %+v, %v", rep, err)
	}
	blocks, _ := store.ListBlockRows(imageSession)
	var texts []string
	for _, b := range blocks {
		for _, im := range b.Images {
			if im.Stored() || im.Skipped == "" {
				t.Fatalf("image stored: %+v", im)
			}
			texts = append(texts, b.Text)
		}
	}
	if len(texts) != 2 || !strings.Contains(texts[0], "skipped: image is too large: ") || !strings.Contains(texts[1], "[image skipped: not an accepted image") {
		t.Fatalf("markers = %q", texts)
	}
	if _, err := os.Stat(capture.Store.Dir()); !os.IsNotExist(err) {
		t.Fatalf("blobs directory exists: %v", err)
	}
}

func TestIndexWithImageCaptureOff(t *testing.T) {
	src := &trace.ClaudeSource{Root: claudeWithImages(t, drawPNG(t, 10, 10, 2), drawPNG(t, 12, 12, 4))}
	store, err := filearchive.Open(t.TempDir(), "hev-traces")
	if err != nil {
		t.Fatal(err)
	}
	capture := &trace.ImageCapture{Store: blob.New(filepath.Join(store.Dir(), "blobs"), 0), Off: true}
	rep, err := Run(src, store, &State{Units: map[string]string{}}, Options{RepoURL: func(string) string { return "" }, Images: capture})
	if err != nil || len(rep.Errors) != 0 || rep.Images.Stored != 0 || rep.Images.Skipped != 2 {
		t.Fatalf("report = %+v, %v", rep, err)
	}
	blocks, _ := store.ListBlockRows(imageSession)
	n := 0
	for _, b := range blocks {
		for _, im := range b.Images {
			n++
			if im.SHA256 == "" || im.Skipped != trace.SkipCaptureOff || !strings.Contains(b.Text, "sha256:"+im.SHA256[:12]+" skipped: image capture is off]") {
				t.Fatalf("block = %+v", b)
			}
		}
	}
	if n != 2 {
		t.Fatalf("images = %d", n)
	}
	if _, err := os.Stat(filepath.Join(store.Dir(), "blobs")); !os.IsNotExist(err) {
		t.Fatalf("blobs directory created with capture off: %v", err)
	}
}

// TestIndexImagesInLayerMode runs the same Claude flow against a Layer test
// endpoint: block rows carry the images as metadata only, no request carries
// the bytes, and the bytes are under the local archive directory.
func TestIndexImagesInLayerMode(t *testing.T) {
	pasted, shot := drawPNG(t, 48, 32, 3), drawPNG(t, 80, 50, 5)
	src := &trace.ClaudeSource{Root: claudeWithImages(t, pasted, shot)}
	var (
		mu     sync.Mutex
		bodies = map[string][]map[string]any{}
		raw    []string
	)
	srv := httptest.NewServer(withSessionPersistence(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		raw = append(raw, string(b))
		var body map[string]any
		json.Unmarshal(b, &body)
		bodies[r.URL.Path] = append(bodies[r.URL.Path], body)
		mu.Unlock()
		if strings.HasSuffix(r.URL.Path, "/query") {
			io.WriteString(w, `{"rows":[]}`)
			return
		}
		io.WriteString(w, `{"status":"OK","rows_upserted":1}`)
	}))
	defer srv.Close()
	cl := layer.New(srv.URL, "key", "ns", "")
	archiveDir := t.TempDir()
	blobsDir, err := filearchive.BlobsDir(archiveDir, "ns")
	if err != nil {
		t.Fatal(err)
	}
	capture := &trace.ImageCapture{Store: blob.New(blobsDir, 0)}
	rep, err := Run(src, cl, &State{Units: map[string]string{}}, Options{RepoURL: func(string) string { return "" }, Images: capture})
	if err != nil || len(rep.Errors) != 0 || rep.Images.Created != 2 {
		t.Fatalf("report = %+v, %v", rep, err)
	}
	for _, want := range [][]byte{pasted, shot} {
		if got, err := os.ReadFile(filepath.Join(archiveDir, "ns", "blobs", "sha256", hexSum(want)[:2], hexSum(want))); err != nil || !bytes.Equal(got, want) {
			t.Fatalf("local blob %s: %v", hexSum(want), err)
		}
	}
	for _, body := range raw {
		for _, img := range [][]byte{pasted, shot} {
			if strings.Contains(body, base64.StdEncoding.EncodeToString(img)[:60]) {
				t.Fatal("image bytes sent to Layer")
			}
		}
	}
	writes := bodies["/v2/namespaces/ns-blocks"]
	if len(writes) != 1 {
		t.Fatalf("block writes = %d", len(writes))
	}
	if _, ok := writes[0]["schema"].(map[string]any)["images"]; !ok {
		t.Fatal("images attribute not declared on a batch with images")
	}
	var found []trace.Image
	for _, row := range writes[0]["upsert_rows"].([]any) {
		obj := row.(map[string]any)
		s, ok := obj["images"].(string)
		if !ok {
			if _, present := obj["images"]; present {
				t.Fatalf("images is not a string attribute: %T", obj["images"])
			}
			continue
		}
		var images trace.Images
		if err := json.Unmarshal([]byte(s), &images); err != nil {
			t.Fatal(err)
		}
		found = append(found, images...)
		if !strings.Contains(obj["text"].(string), "[image image/png ") {
			t.Fatalf("block text without marker: %v", obj["text"])
		}
	}
	if len(found) != 2 || found[0].SHA256 != hexSum(pasted) || found[1].SHA256 != hexSum(shot) || found[1].Width != 80 || found[1].Index != 1 {
		t.Fatalf("images on Layer rows = %+v", found)
	}
}
