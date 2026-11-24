package foh

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, G: 107, B: 51, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestTUIMultiLinePasteCollapsesAndExpandsOnSubmit(t *testing.T) {
	p := &scripted{replies: []ChatResult{{Content: "got it"}}}
	m, _ := newTestTUI(t, p)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Short paste (<= 3 lines) stays inline.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("line 1\nline 2"), Paste: true})
	if got := m.input.Value(); got != "line 1\nline 2" {
		t.Fatalf("short paste collapsed unexpectedly: %q", got)
	}
	m.input.Reset()

	// Multi-line paste (>= 4 lines) collapses to [X chars, Y lines pasted].
	big := "alpha\nbeta\ngamma\ndelta\nepsilon"
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("check this: ")})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(big), Paste: true})
	gotInput := m.input.Value()
	if !strings.Contains(gotInput, "[30 chars, 5 lines pasted]") {
		t.Fatalf("expected collapsed placeholder in input, got: %q", gotInput)
	}
	if strings.Contains(gotInput, "epsilon") {
		t.Fatalf("full pasted body leaked into input box: %q", gotInput)
	}

	// Submitting expands the full payload for the model while keeping the
	// compact placeholder in the user transcript entry.
	drain(t, m, submit(m, ""))
	p.mu.Lock()
	reqs := append([]ChatRequest(nil), p.requests...)
	p.mu.Unlock()
	if len(reqs) == 0 {
		t.Fatal("agent did not receive request")
	}
	lastMsg := reqs[0].Messages[len(reqs[0].Messages)-1]
	if !strings.Contains(lastMsg.Content, "check this: alpha\nbeta\ngamma\ndelta\nepsilon") {
		t.Fatalf("model did not receive expanded paste: %q", lastMsg.Content)
	}
	if len(m.pastedTexts) != 0 {
		t.Fatalf("pastedTexts not cleared after submit: %v", m.pastedTexts)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "[30 chars, 5 lines pasted]") {
		t.Fatalf("transcript should show compact paste placeholder:\n%s", view)
	}
}

func TestTUIImagePasteClipboardAndFilePath(t *testing.T) {
	pngBytes := tinyPNG(t)
	att, err := buildImageAttachment(pngBytes, "shot.png")
	if err != nil {
		t.Fatal(err)
	}

	// 1. Clipboard image paste via Ctrl+V.
	origReader := clipboardImageReader
	clipboardImageReader = func(context.Context) (ImageAttachment, error) { return att, nil }
	defer func() { clipboardImageReader = origReader }()

	p := &scripted{replies: []ChatResult{{Content: "nice screenshot"}}}
	m, _ := newTestTUI(t, p)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlV})
	if len(m.pendingImages) != 1 || m.pendingImages[0].MediaType != "image/png" {
		t.Fatalf("Ctrl+V did not attach image: %+v", m.pendingImages)
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "1 image(s) attached") {
		t.Fatalf("pending attachment bar missing:\n%s", v)
	}

	// 2. Pasted image file path (e.g. Finder drag-and-drop with escaped space).
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "my screen.png")
	if err := os.WriteFile(imgPath, pngBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	escaped := strings.ReplaceAll(imgPath, " ", `\ `)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(escaped), Paste: true})
	if len(m.pendingImages) != 2 || m.pendingImages[1].Label != "my screen.png" {
		t.Fatalf("file path paste did not attach second image: %+v", m.pendingImages)
	}
	if m.input.Value() != "" {
		t.Fatalf("image path should not be dumped into input text: %q", m.input.Value())
	}

	// Submit with a prompt: both images reach the model request.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("what is in these?")})
	drain(t, m, submit(m, ""))
	p.mu.Lock()
	reqs := append([]ChatRequest(nil), p.requests...)
	p.mu.Unlock()
	if len(reqs) == 0 {
		t.Fatal("agent did not receive image request")
	}
	userMsg := reqs[0].Messages[len(reqs[0].Messages)-1]
	if len(userMsg.Images) != 2 {
		t.Fatalf("expected 2 images in model message, got %d", len(userMsg.Images))
	}

	// Verify OpenRouter JSON marshaling produces multimodal content parts.
	rawJSON, err := json.Marshal(userMsg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rawJSON), `"type":"image_url"`) || !strings.Contains(string(rawJSON), `"data:image/png;base64,`) {
		t.Fatalf("OpenRouter multimodal JSON missing image_url: %s", string(rawJSON))
	}

	// Verify Codex Responses API conversion produces input_image items.
	_, items := responsesInput([]Message{userMsg})
	itemsJSON, _ := json.Marshal(items)
	if !strings.Contains(string(itemsJSON), `"type":"input_image"`) || !strings.Contains(string(itemsJSON), `"data:image/png;base64,`) {
		t.Fatalf("Codex Responses API missing input_image: %s", string(itemsJSON))
	}

	// Refusal: non-image bytes with .png extension must be rejected.
	badPath := filepath.Join(dir, "fake.png")
	os.WriteFile(badPath, []byte("not a png at all"), 0o600)
	if _, ok, err := loadImageFile(badPath); !ok || err == nil {
		t.Fatalf("expected refusal for fake.png, got ok=%v err=%v", ok, err)
	}
}
