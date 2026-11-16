package trace

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/blob"
)

// synthPNG and synthJPEG draw small images in the test; no real screenshot
// is ever committed.
func synthPNG(t testing.TB, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{uint8(x * 7), uint8(y * 11), uint8(x ^ y), 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func synthJPEG(t testing.TB, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		img.Set(x, x%h, color.RGBA{200, 10, 10, 255})
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func sum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

func writeLines(t *testing.T, path string, lines ...any) {
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

// claudeImageTranscript has a pasted image in a user message and a
// screenshot in a tool result, the two places Claude Code puts pictures.
func claudeImageTranscript(t *testing.T, root string, pasted, shot []byte) string {
	t.Helper()
	path := filepath.Join(root, "-w", "11111111-2222-3333-4444-555555555555.jsonl")
	sid := "11111111-2222-3333-4444-555555555555"
	writeLines(t, path,
		map[string]any{"type": "user", "uuid": "u1", "sessionId": sid, "timestamp": "2026-11-10T10:00:00Z", "cwd": "/w",
			"message": map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": b64(pasted)}},
				map[string]any{"type": "text", "text": "why is the header misaligned?"},
			}}},
		map[string]any{"type": "assistant", "uuid": "a1", "parentUuid": "u1", "sessionId": sid, "timestamp": "2026-11-10T10:00:05Z", "cwd": "/w", "requestId": "r1",
			"message": map[string]any{"role": "assistant", "model": "claude-sonnet-4-6", "content": []any{
				map[string]any{"type": "tool_use", "id": "tu1", "name": "mcp__browser__screenshot", "input": map[string]any{}},
			}, "usage": map[string]any{"input_tokens": 5, "output_tokens": 7}}},
		map[string]any{"type": "user", "uuid": "u2", "parentUuid": "a1", "sessionId": sid, "timestamp": "2026-11-10T10:00:06Z", "cwd": "/w",
			"message": map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "tool_result", "tool_use_id": "tu1", "content": []any{
					map[string]any{"type": "text", "text": "screenshot taken"},
					map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/jpeg", "data": b64(shot)}},
				}},
			}}},
	)
	return path
}

func readAll(t *testing.T, src Source) []Turn {
	t.Helper()
	units, err := src.Units()
	if err != nil || len(units) != 1 {
		t.Fatalf("units = %v, %v", units, err)
	}
	turns, err := src.Read(units[0])
	if err != nil {
		t.Fatal(err)
	}
	return turns
}

func imageBlocks(turns []Turn) []Block {
	var out []Block
	for _, turn := range turns {
		for _, b := range turn.Blocks {
			if len(b.Images) > 0 {
				out = append(out, b)
			}
		}
	}
	return out
}

func TestClaudeImagesAreDecodedMarkedAndStored(t *testing.T) {
	root := t.TempDir()
	pasted, shot := synthPNG(t, 40, 30), synthJPEG(t, 64, 48)
	claudeImageTranscript(t, root, pasted, shot)
	turns := readAll(t, &ClaudeSource{Root: root})
	blocks := imageBlocks(turns)
	if len(blocks) != 2 {
		t.Fatalf("image blocks = %+v", blocks)
	}
	user, result := blocks[0], blocks[1]
	if user.Type != "text" || user.Images[0].Index != 0 || user.Images[0].SHA256 != sum(pasted) || user.Images[0].Type != "image/png" ||
		user.Images[0].Width != 40 || user.Images[0].Height != 30 || user.Images[0].Size != int64(len(pasted)) {
		t.Fatalf("user image = %+v", user)
	}
	if want := "[image image/png 40x30 " + blob.Human(int64(len(pasted))) + " sha256:" + sum(pasted)[:12] + "]"; user.Text != want {
		t.Fatalf("user marker = %q, want %q", user.Text, want)
	}
	if result.Type != "tool_result" || result.Images[0].Index != 1 || result.Images[0].SHA256 != sum(shot) || result.Images[0].Type != "image/jpeg" ||
		!strings.HasPrefix(result.Text, "screenshot taken\n[image image/jpeg 64x48 ") {
		t.Fatalf("tool result = %+v", result)
	}
	// The pasted image is not the session's request.
	if s := Sessions(turns, nil, "h"); len(s) != 1 || s[0].FirstPrompt != "why is the header misaligned?" {
		t.Fatalf("first prompt = %+v", s)
	}

	store := blob.New(filepath.Join(t.TempDir(), "blobs"), 0)
	st, err := CaptureImages(turns, &ImageCapture{Store: store})
	if err != nil || st.Seen != 2 || st.Stored != 2 || st.Created != 2 || st.Skipped != 0 {
		t.Fatalf("capture = %+v, %v", st, err)
	}
	for _, want := range [][]byte{pasted, shot} {
		got, err := os.ReadFile(store.Path(sum(want)))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("blob %s: %v", sum(want), err)
		}
	}
	// Bytes are released after capture, and a second pass stores nothing new.
	for _, b := range imageBlocks(turns) {
		if b.Images[0].data != nil {
			t.Fatal("image bytes kept after capture")
		}
	}
	again := readAll(t, &ClaudeSource{Root: root})
	if st, err := CaptureImages(again, &ImageCapture{Store: store}); err != nil || st.Created != 0 || st.Stored != 2 {
		t.Fatalf("second capture = %+v, %v", st, err)
	}
	// Block rows carry the metadata, never the bytes.
	rows := Blocks(again)
	raw, _ := json.Marshal(rows)
	if strings.Contains(string(raw), b64(pasted)[:40]) || !strings.Contains(string(raw), sum(shot)) {
		t.Fatalf("block rows: %s", raw)
	}
}

func TestImagesSkippedOverCapOrNotAnImage(t *testing.T) {
	root := t.TempDir()
	big := synthPNG(t, 120, 120)
	notImage := []byte("this is plain text pretending to be a png")
	claudeImageTranscript(t, root, big, notImage)
	turns := readAll(t, &ClaudeSource{Root: root})
	store := blob.New(filepath.Join(t.TempDir(), "blobs"), 200)
	st, err := CaptureImages(turns, &ImageCapture{Store: store})
	if err != nil || st.Stored != 0 || st.Skipped != 2 {
		t.Fatalf("capture = %+v, %v", st, err)
	}
	blocks := imageBlocks(turns)
	if !strings.Contains(blocks[0].Text, "skipped: image is too large") || blocks[0].Images[0].Stored() {
		t.Fatalf("oversize marker = %q", blocks[0].Text)
	}
	if !strings.Contains(blocks[1].Text, "[image skipped: not an accepted image") || blocks[1].Images[0].SHA256 != "" {
		t.Fatalf("non-image marker = %q", blocks[1].Text)
	}
	if _, err := os.Stat(store.Dir()); !os.IsNotExist(err) {
		t.Fatalf("blob directory created for skipped images: %v", err)
	}
}

func TestImageCaptureOffWritesMarkersOnly(t *testing.T) {
	root := t.TempDir()
	pasted, shot := synthPNG(t, 8, 8), synthPNG(t, 9, 9)
	claudeImageTranscript(t, root, pasted, shot)
	turns := readAll(t, &ClaudeSource{Root: root})
	store := blob.New(filepath.Join(t.TempDir(), "blobs"), 0)
	st, err := CaptureImages(turns, &ImageCapture{Store: store, Off: true})
	if err != nil || st.Stored != 0 || st.Skipped != 2 {
		t.Fatalf("capture = %+v, %v", st, err)
	}
	for _, b := range imageBlocks(turns) {
		if !strings.Contains(b.Text, "sha256:") || !strings.Contains(b.Text, "skipped: image capture is off]") || b.Text == "" {
			t.Fatalf("marker = %q", b.Text)
		}
		if !b.imageOnly() && b.Type == "text" {
			t.Fatalf("pasted image block no longer image-only: %q", b.Text)
		}
	}
	if _, err := os.Stat(store.Dir()); !os.IsNotExist(err) {
		t.Fatalf("blob directory created with capture off: %v", err)
	}
}

func TestCodexImagesAreDecodedMarkedAndStored(t *testing.T) {
	root := t.TempDir()
	pasted, shot := synthPNG(t, 20, 10), synthPNG(t, 33, 17)
	writeLines(t, filepath.Join(root, "2026", "11", "10", "rollout.jsonl"),
		map[string]any{"timestamp": "2026-11-10T10:00:00Z", "type": "session_meta", "payload": map[string]any{"id": "c0dex000-0000-0000-0000-000000000001", "cwd": "/w"}},
		map[string]any{"timestamp": "2026-11-10T10:00:01Z", "type": "response_item", "payload": map[string]any{"type": "message", "role": "user", "id": "m1", "content": []any{
			map[string]any{"type": "input_text", "text": "does this match the mock?"},
			map[string]any{"type": "input_image", "image_url": "data:image/png;base64," + b64(pasted)},
		}}},
		map[string]any{"timestamp": "2026-11-10T10:00:02Z", "type": "response_item", "payload": map[string]any{"type": "custom_tool_call", "id": "f1", "call_id": "c1", "name": "browser", "input": "{\"action\":\"screenshot\"}"}},
		map[string]any{"timestamp": "2026-11-10T10:00:03Z", "type": "response_item", "payload": map[string]any{"type": "custom_tool_call_output", "id": "o1", "call_id": "c1", "output": []any{
			map[string]any{"type": "input_text", "text": "captured"},
			map[string]any{"type": "input_image", "image_url": "data:image/png;base64," + b64(shot), "detail": "auto"},
		}}},
	)
	turns := readAll(t, &CodexSource{Root: root})
	blocks := imageBlocks(turns)
	if len(blocks) != 2 || blocks[0].Type != "text" || blocks[0].Images[0].SHA256 != sum(pasted) ||
		blocks[1].Type != "tool_result" || blocks[1].Images[0].Index != 1 || blocks[1].Images[0].SHA256 != sum(shot) ||
		!strings.HasPrefix(blocks[1].Text, "captured\n[image image/png 33x17 ") {
		t.Fatalf("codex image blocks = %+v", blocks)
	}
	store := blob.New(filepath.Join(t.TempDir(), "blobs"), 0)
	if st, err := CaptureImages(turns, &ImageCapture{Store: store}); err != nil || st.Created != 2 {
		t.Fatalf("capture = %+v, %v", st, err)
	}
	if b, err := os.ReadFile(store.Path(sum(shot))); err != nil || !bytes.Equal(b, shot) {
		t.Fatalf("codex blob: %v", err)
	}
}

// A Codex output with no image reads exactly as it did before images.
func TestCodexOutputWithoutImagesIsUnchanged(t *testing.T) {
	for _, raw := range []string{`"plain"`, `[{"type":"input_text","text":"a"},{"type":"input_text","text":""},{"type":"input_text","text":"b"}]`, `{"x":1}`} {
		text, images := codexOutput(json.RawMessage(raw))
		if text != codexText(json.RawMessage(raw)) || images != nil {
			t.Fatalf("%s: %q %v", raw, text, images)
		}
	}
}

func TestPiImagesAreDecoded(t *testing.T) {
	png := synthPNG(t, 5, 6)
	blocks, err := piBlocks(json.RawMessage(`[{"type":"text","text":"look"},{"type":"image","data":"` + b64(png) + `","mimeType":"image/png"}]`))
	if err != nil || len(blocks) != 2 || len(blocks[1].Images) != 1 || blocks[1].Images[0].SHA256 != sum(png) || !strings.HasPrefix(blocks[1].Text, "[image image/png 5x6 ") {
		t.Fatalf("pi blocks = %+v, %v", blocks, err)
	}
}

// Screenshots arrive in tool results, which are not indexed by default; their
// markers get a small text-tier chunk so search and show can see them.
func TestToolResultImagesGetAMarkerChunk(t *testing.T) {
	root := t.TempDir()
	claudeImageTranscript(t, root, synthPNG(t, 4, 4), synthPNG(t, 7, 3))
	turns := readAll(t, &ClaudeSource{Root: root})
	var found []Chunk
	for _, turn := range turns {
		for _, c := range Chunks(turn) {
			if c.BlockType == ImageBlockType {
				found = append(found, c)
			}
		}
	}
	if len(found) != 1 || found[0].Tier != "text" || !strings.HasPrefix(found[0].Text, "[image image/png 7x3 ") || found[0].Block != 0 {
		t.Fatalf("image chunks = %+v", found)
	}
	// Turns without images chunk exactly as before: no image chunk at all.
	plain := Turn{SessionID: "s", TurnUUID: "t", Blocks: []Block{{Type: "tool_result", Text: "ok"}}}
	if cs := Chunks(plain); len(cs) != 1 || cs[0].BlockType != "tool_result" {
		t.Fatalf("plain chunks = %+v", cs)
	}
}

func TestImagesReadBackFromLayerStringAttribute(t *testing.T) {
	var row BlockRow
	if err := json.Unmarshal([]byte(`{"id":"x","images":"[{\"index\":1,\"sha256\":\"ab\",\"type\":\"image/png\",\"size\":3,\"width\":1,\"height\":2}]"}`), &row); err != nil {
		t.Fatal(err)
	}
	if len(row.Images) != 1 || row.Images[0].Index != 1 || row.Images[0].Height != 2 {
		t.Fatalf("images = %+v", row.Images)
	}
}

func TestDataURLThatIsNotInline(t *testing.T) {
	im := dataURLImage(2, "https://example.com/a.png")
	if im.Skipped == "" || im.Index != 2 || !strings.Contains(im.Marker(), "skipped: the transcript holds a reference") {
		t.Fatalf("remote image = %+v %q", im, im.Marker())
	}
}
