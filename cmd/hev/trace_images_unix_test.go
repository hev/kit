//go:build !windows

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func cliPNG(t *testing.T, w, h int, seed uint8) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{seed, uint8(x), uint8(y), 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func cliSum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// TestTracesImagesThroughCLI is the read-side acceptance through the real
// binary with no Layer key: hev index captures a Claude Code session with a
// pasted image and a screenshot, hev traces show marks them, and hev traces
// images --out saves files whose sha256 match the originals, in transcript
// order, with --json listing the same.
func TestTracesImagesThroughCLI(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs the hev binary")
	}
	tr := newTrap(t)
	bin := buildHev(t)
	home := t.TempDir()
	env := noKeyEnv(home, tr)
	mustMkdir(t, filepath.Join(home, ".hev"))
	mustWrite(t, filepath.Join(home, ".hev", "config.toml"), []byte(fmt.Sprintf("[layer]\nendpoint = %q\napi_key = \"\"\n[capture]\nscan_interval = \"1h\"\ninstructions = false\n", tr.server.URL)))

	session := "9d8c7b6a-1111-4222-8333-444455556666"
	pasted, shot := cliPNG(t, 64, 40, 7), cliPNG(t, 120, 80, 9)
	enc := base64.StdEncoding.EncodeToString
	at := time.Now().Add(-time.Hour).UTC()
	ts := func(d time.Duration) string { return at.Add(d).Format(time.RFC3339) }
	lines := []map[string]any{
		{"type": "user", "uuid": "u1", "sessionId": session, "cwd": "/work/kit", "timestamp": ts(0), "message": map[string]any{"role": "user", "content": []any{
			map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": enc(pasted)}},
			map[string]any{"type": "text", "text": "the login button overlaps the logo"},
		}}},
		{"type": "assistant", "uuid": "a1", "parentUuid": "u1", "sessionId": session, "cwd": "/work/kit", "timestamp": ts(time.Second), "requestId": "r1", "message": map[string]any{
			"role": "assistant", "model": "claude-sonnet-4-6", "content": []any{map[string]any{"type": "tool_use", "id": "tu1", "name": "mcp__browser__screenshot", "input": map[string]any{}}},
			"usage": map[string]any{"input_tokens": 10, "output_tokens": 20}}},
		{"type": "user", "uuid": "u2", "parentUuid": "a1", "sessionId": session, "cwd": "/work/kit", "timestamp": ts(2 * time.Second), "message": map[string]any{"role": "user", "content": []any{
			map[string]any{"type": "tool_result", "tool_use_id": "tu1", "content": []any{
				map[string]any{"type": "text", "text": "page captured"},
				map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": enc(shot)}},
			}},
		}}},
		{"type": "assistant", "uuid": "a2", "parentUuid": "u2", "sessionId": session, "cwd": "/work/kit", "timestamp": ts(3 * time.Second), "requestId": "r2", "message": map[string]any{
			"role": "assistant", "model": "claude-sonnet-4-6", "content": []any{map[string]any{"type": "text", "text": "the logo has a negative margin"}},
			"usage": map[string]any{"input_tokens": 10, "output_tokens": 20}}},
	}
	var transcript strings.Builder
	for _, l := range lines {
		raw, _ := json.Marshal(l)
		transcript.Write(raw)
		transcript.WriteByte('\n')
	}
	project := filepath.Join(home, ".claude", "projects", "-work-kit")
	mustMkdir(t, project)
	mustWrite(t, filepath.Join(project, session+".jsonl"), []byte(transcript.String()))

	out := runHev(t, bin, env, "index")
	if !strings.Contains(out, "images  2 seen, 2 stored (2 new), 0 skipped") {
		t.Fatalf("index:\n%s", out)
	}
	blobs := filepath.Join(home, ".hev", "archive", "hev-traces", "blobs", "sha256")
	for _, b := range [][]byte{pasted, shot} {
		if _, err := os.Stat(filepath.Join(blobs, cliSum(b)[:2], cliSum(b))); err != nil {
			t.Fatalf("blob not under the archive: %v", err)
		}
	}

	show := runHev(t, bin, env, "traces", "show", session[:8])
	if !strings.Contains(show, "[image image/png 64x40 ") || !strings.Contains(show, "▣ [image image/png 120x80 ") || !strings.Contains(show, "hev traces images "+session) {
		t.Fatalf("show does not mark the images:\n%s", show)
	}

	list := runHev(t, bin, env, "traces", "images", session[:8])
	if !strings.Contains(list, "2 images in session "+session) || !strings.Contains(list, cliSum(pasted)[:12]) || !strings.Contains(list, "120x80") {
		t.Fatalf("images:\n%s", list)
	}

	dir := filepath.Join(t.TempDir(), "seen")
	saved := runHev(t, bin, env, "traces", "images", session[:8], "--out", dir)
	files, err := filepath.Glob(filepath.Join(dir, "*.png"))
	if err != nil || len(files) != 2 {
		t.Fatalf("saved files = %v, %v\n%s", files, err, saved)
	}
	for i, want := range [][]byte{pasted, shot} {
		got, err := os.ReadFile(files[i])
		if err != nil || cliSum(got) != cliSum(want) {
			t.Fatalf("file %d (%s) sha256 does not match the original", i, files[i])
		}
		if !strings.Contains(saved, files[i]) {
			t.Fatalf("path %s not printed:\n%s", files[i], saved)
		}
	}

	raw := runHev(t, bin, env, "traces", "images", session[:8], "--json")
	var listed []sessionImage
	if err := json.Unmarshal([]byte(raw[strings.Index(raw, "["):]), &listed); err != nil {
		t.Fatalf("json: %v\n%s", err, raw)
	}
	if len(listed) != 2 || listed[0].SHA256 != cliSum(pasted) || listed[0].Role != "user" || listed[0].BlockType != "text" ||
		listed[1].SHA256 != cliSum(shot) || listed[1].BlockType != "tool_result" || listed[1].Index != 1 ||
		listed[1].Width != 120 || listed[1].Height != 80 || listed[1].Type != "image/png" || !listed[1].OnHost || listed[0].Seq >= listed[1].Seq {
		t.Fatalf("listed = %+v", listed)
	}
	tr.check(t)
}
