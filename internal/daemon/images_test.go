package daemon

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/filearchive"
	"github.com/hev/kit/internal/index"
	"github.com/hev/kit/internal/storagemode"
	"github.com/hev/kit/internal/trace"
)

const switchSession = "5e5e5e5e-0000-4000-8000-00000000cafe"

// switchPNG is a synthetic image drawn in the test.
func switchPNG(t *testing.T, w, h int, seed uint8) []byte {
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

// claudeImageSession writes a Claude Code session with an image in a user
// message and one in a tool result, and returns its projects root.
func claudeImageSession(t *testing.T) string {
	t.Helper()
	enc := base64.StdEncoding.EncodeToString
	img := func(b []byte) map[string]any {
		return map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": enc(b)}}
	}
	lines := []map[string]any{
		{"type": "user", "uuid": "u1", "sessionId": switchSession, "timestamp": "2026-11-10T10:00:00Z", "cwd": "/w",
			"message": map[string]any{"role": "user", "content": []any{img(switchPNG(t, 16, 9, 3)), map[string]any{"type": "text", "text": "look at this"}}}},
		{"type": "assistant", "uuid": "a1", "parentUuid": "u1", "sessionId": switchSession, "timestamp": "2026-11-10T10:00:05Z", "cwd": "/w", "requestId": "r1",
			"message": map[string]any{"role": "assistant", "model": "claude-sonnet-4-6", "content": []any{
				map[string]any{"type": "tool_use", "id": "tu1", "name": "mcp__browser__screenshot", "input": map[string]any{}},
			}, "usage": map[string]any{"input_tokens": 5, "output_tokens": 7}}},
		{"type": "user", "uuid": "u2", "parentUuid": "a1", "sessionId": switchSession, "timestamp": "2026-11-10T10:00:06Z", "cwd": "/w",
			"message": map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "tool_result", "tool_use_id": "tu1", "content": []any{map[string]any{"type": "text", "text": "page"}, img(switchPNG(t, 20, 12, 5))}},
			}}},
		{"type": "assistant", "uuid": "a2", "parentUuid": "u2", "sessionId": switchSession, "timestamp": "2026-11-10T10:00:09Z", "cwd": "/w", "requestId": "r2",
			"message": map[string]any{"role": "assistant", "model": "claude-sonnet-4-6", "content": []any{
				map[string]any{"type": "text", "text": "done"},
			}, "usage": map[string]any{"input_tokens": 9, "output_tokens": 11}}},
	}
	var b strings.Builder
	for _, l := range lines {
		raw, err := json.Marshal(l)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(raw)
		b.WriteByte('\n')
	}
	root := t.TempDir()
	path := filepath.Join(root, "-w", switchSession+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// indexThroughTarget indexes the image session with the capture the host's
// config produced, the way hevd and hev index do, and returns the block
// rows' images and the report.
func indexThroughTarget(t *testing.T, target Target) ([]trace.BlockRow, *index.Report) {
	t.Helper()
	if target.Files == nil || target.Images == nil {
		t.Fatalf("target = %+v", target)
	}
	src := &trace.ClaudeSource{Root: claudeImageSession(t)}
	rep, err := index.Run(src, target.Files, &index.State{Units: map[string]string{}}, index.Options{RepoURL: func(string) string { return "" }, Images: target.Images})
	if err != nil || len(rep.Errors) != 0 {
		t.Fatalf("index: %+v, %v", rep, err)
	}
	blocks, err := target.Files.ListBlockRows(switchSession)
	if err != nil {
		t.Fatal(err)
	}
	return blocks, rep
}

// assertCaptureOff checks that every image is skipped with the capture-off
// marker and that no blobs directory was created.
func assertCaptureOff(t *testing.T, target Target) {
	t.Helper()
	if !target.Images.Off {
		t.Fatalf("capture is on: %+v", target.Images)
	}
	blocks, rep := indexThroughTarget(t, target)
	if rep.Images.Seen != 2 || rep.Images.Stored != 0 || rep.Images.Skipped != 2 {
		t.Fatalf("images = %+v", rep.Images)
	}
	n := 0
	for _, b := range blocks {
		for _, im := range b.Images {
			n++
			if im.Skipped != trace.SkipCaptureOff || !strings.Contains(b.Text, "sha256:"+im.SHA256[:12]+" skipped: image capture is off]") {
				t.Fatalf("block = %+v", b)
			}
		}
	}
	if n != 2 {
		t.Fatalf("images on blocks = %d, want 2", n)
	}
	if _, err := os.Stat(filepath.Join(target.Files.Dir(), "blobs")); !os.IsNotExist(err) {
		t.Fatalf("blobs directory exists with capture off: %v", err)
	}
}

// TestCaptureImagesOnByDefault is the control for the two switch tests:
// with nothing set, the same flow stores both images.
func TestCaptureImagesOnByDefault(t *testing.T) {
	t.Setenv("HEVD_CAPTURE_IMAGES", "")
	cfg, _ := isolate(t, "")
	target, err := OpenTarget(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, rep := indexThroughTarget(t, target); rep.Images.Stored != 2 || rep.Images.Created != 2 {
		t.Fatalf("images = %+v", rep.Images)
	}
	if _, err := os.Stat(filepath.Join(target.Files.Dir(), "blobs")); err != nil {
		t.Fatalf("blobs directory: %v", err)
	}
}

// TestCaptureImagesOffInConfig: [capture] images = false reaches
// ImageCapture, so markers are written and no blobs directory is created.
func TestCaptureImagesOffInConfig(t *testing.T) {
	t.Setenv("HEVD_CAPTURE_IMAGES", "")
	cfg, _ := isolate(t, "[capture]\nimages = false\n")
	if cfg.CaptureImages {
		t.Fatal("[capture] images = false was not read")
	}
	target, err := OpenTarget(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	assertCaptureOff(t, target)
}

// TestCaptureImagesOffInEnvironment: HEVD_CAPTURE_IMAGES=0 reaches
// ImageCapture, and wins over a config that leaves capture on.
func TestCaptureImagesOffInEnvironment(t *testing.T) {
	t.Setenv("HEVD_CAPTURE_IMAGES", "0")
	cfg, _ := isolate(t, "[capture]\nimages = true\n")
	if cfg.CaptureImages {
		t.Fatal("HEVD_CAPTURE_IMAGES=0 was not applied")
	}
	target, err := OpenTarget(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	assertCaptureOff(t, target)
}

// TestLayerNamespaceOutsideArchiveCharset: Layer accepts namespaces the
// file archive cannot use as a directory name. Opening such a target in
// Layer mode must not fail (it did when the blobs directory was resolved
// eagerly); an image that arrives under it is skipped with a marker and
// nothing is stored.
func TestLayerNamespaceOutsideArchiveCharset(t *testing.T) {
	t.Setenv("HEVD_CAPTURE_IMAGES", "")
	cfg, root := isolate(t, "")
	t.Setenv("HEVLAYER_API_KEY", "fake")
	t.Setenv("HEVLAYER_ENDPOINT", "http://127.0.0.1:9")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	target, err := OpenTarget(cfg, "team:traces")
	if err != nil {
		t.Fatalf("OpenTarget in Layer mode: %v", err)
	}
	if target.Mode != storagemode.Layer || target.Layer == nil || target.Layer.Namespace != "team:traces" {
		t.Fatalf("target = %+v", target)
	}
	if target.Images == nil || target.Images.Store != nil || target.Images.Unavailable == "" {
		t.Fatalf("images = %+v", target.Images)
	}

	// Index into a file archive with that capture, to see what the image
	// blocks get.
	files, err := filearchive.Open(t.TempDir(), "check")
	if err != nil {
		t.Fatal(err)
	}
	blocks, rep := indexThroughTarget(t, Target{Files: files, Images: target.Images})
	if rep.Images.Seen != 2 || rep.Images.Stored != 0 || rep.Images.Skipped != 2 {
		t.Fatalf("images = %+v", rep.Images)
	}
	for _, b := range blocks {
		for _, im := range b.Images {
			if im.Skipped != target.Images.Unavailable || !strings.Contains(b.Text, `skipped: namespace "team:traces" cannot hold image files on this host]`) {
				t.Fatalf("block = %+v", b)
			}
		}
	}
	for _, dir := range []string{filepath.Join(files.Dir(), "blobs"), root} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("%s exists: %v", dir, err)
		}
	}
}
