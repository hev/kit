package foh

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxPasteImageBytes    = 10 << 20 // 10 MiB cap per attached image
	maxPendingImages      = 8
	maxPendingPastes      = 32
	collapsePasteMinLines = 4
	clipboardTimeout      = 5 * time.Second
)

// errNoClipboardImage reports that the OS clipboard does not currently hold an image.
var errNoClipboardImage = errors.New("clipboard does not contain an image")

// ImageAttachment is a base64-encoded image attached to a user turn.
type ImageAttachment struct {
	MediaType string
	Data      string
	Bytes     int
	Label     string
}

// DataURL returns a data: URI suitable for OpenRouter and Codex Responses APIs.
func (a ImageAttachment) DataURL() string {
	return "data:" + a.MediaType + ";base64," + a.Data
}

func formatByteSize(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%d KB", max(1, (n+512)/(1<<10)))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func normalizePasteText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

// shouldCollapsePaste reports whether a pasted text block is large enough
// (more than a few lines) to collapse into a compact placeholder token.
func shouldCollapsePaste(text string) bool {
	norm := normalizePasteText(text)
	return strings.Count(norm, "\n")+1 >= collapsePasteMinLines
}

// makePastePlaceholder builds a unique `[X chars, Y lines pasted]` token.
func makePastePlaceholder(text string, existing map[string]string) string {
	norm := normalizePasteText(text)
	chars := utf8.RuneCountInString(norm)
	lines := strings.Count(norm, "\n") + 1
	base := fmt.Sprintf("[%d chars, %d lines pasted]", chars, lines)
	if _, clash := existing[base]; !clash {
		return base
	}
	for i := 2; i < 1000; i++ {
		tok := fmt.Sprintf("[%d chars, %d lines pasted #%d]", chars, lines, i)
		if _, clash := existing[tok]; !clash {
			return tok
		}
	}
	return base
}

// expandPastedTokens replaces every collapsed paste token present in text with
// its stored full payload.
func expandPastedTokens(text string, pastes map[string]string) string {
	if len(pastes) == 0 {
		return text
	}
	out := text
	for tok, full := range pastes {
		if strings.Contains(out, tok) {
			out = strings.ReplaceAll(out, tok, full)
		}
	}
	return out
}

func imageMediaTypeByExt(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return ""
	}
}

// unquotePastedPath strips surrounding shell quotes and unescapes backslash
// spaces produced by terminal drag-and-drop (e.g. macOS Finder drops).
func unquotePastedPath(raw string) string {
	s := strings.TrimSpace(raw)
	if len(s) >= 2 && ((s[0] == '\'' && s[len(s)-1] == '\'') || (s[0] == '"' && s[len(s)-1] == '"')) {
		s = s[1 : len(s)-1]
	}
	// macOS terminals escape spaces and parentheses with backslashes on drop.
	s = strings.NewReplacer(`\ `, " ", `\(`, "(", `\)`, ")", `\[`, "[", `\]`, "]").Replace(s)
	return expandHome(s)
}

// loadImageFile checks whether raw text is a path to an existing local image
// file and loads it into an ImageAttachment. ok=false means raw is not an image
// file path (not an error); ok=true with err!=nil means it looked like an image
// path but could not be loaded (e.g. too large or invalid image bytes).
func loadImageFile(raw string) (att ImageAttachment, ok bool, err error) {
	if strings.Contains(raw, "\n") {
		return ImageAttachment{}, false, nil
	}
	p := unquotePastedPath(raw)
	if p == "" || imageMediaTypeByExt(p) == "" {
		return ImageAttachment{}, false, nil
	}
	info, statErr := os.Stat(p)
	if statErr != nil || !info.Mode().IsRegular() {
		return ImageAttachment{}, false, nil
	}
	if info.Size() <= 0 {
		return ImageAttachment{}, true, errors.New("image file is empty")
	}
	if info.Size() > maxPasteImageBytes {
		return ImageAttachment{}, true, fmt.Errorf("image exceeds %s limit", formatByteSize(maxPasteImageBytes))
	}
	f, openErr := os.Open(p)
	if openErr != nil {
		return ImageAttachment{}, true, errors.New("could not open image file")
	}
	defer f.Close()
	data, readErr := io.ReadAll(io.LimitReader(f, maxPasteImageBytes+1))
	if readErr != nil {
		return ImageAttachment{}, true, errors.New("could not read image file")
	}
	if len(data) > maxPasteImageBytes {
		return ImageAttachment{}, true, fmt.Errorf("image exceeds %s limit", formatByteSize(maxPasteImageBytes))
	}
	att, buildErr := buildImageAttachment(data, filepath.Base(p))
	return att, true, buildErr
}

func buildImageAttachment(data []byte, label string) (ImageAttachment, error) {
	if len(data) == 0 {
		return ImageAttachment{}, errors.New("image is empty")
	}
	if len(data) > maxPasteImageBytes {
		return ImageAttachment{}, fmt.Errorf("image exceeds %s limit", formatByteSize(maxPasteImageBytes))
	}
	mt := http.DetectContentType(data)
	switch mt {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
	default:
		return ImageAttachment{}, fmt.Errorf("unsupported image type %s", mt)
	}
	if label == "" {
		label = "clipboard.png"
	}
	return ImageAttachment{
		MediaType: mt,
		Data:      base64.StdEncoding.EncodeToString(data),
		Bytes:     len(data),
		Label:     label,
	}, nil
}

// clipboardImageReader is swappable in tests so unit tests never touch the host clipboard.
var clipboardImageReader = readClipboardImage

func readClipboardImage(ctx context.Context) (ImageAttachment, error) {
	ctx, cancel := context.WithTimeout(ctx, clipboardTimeout)
	defer cancel()
	switch runtime.GOOS {
	case "darwin":
		return readDarwinClipboardImage(ctx)
	case "linux":
		return readLinuxClipboardImage(ctx)
	case "windows":
		return readWindowsClipboardImage(ctx)
	default:
		return ImageAttachment{}, errNoClipboardImage
	}
}

func readDarwinClipboardImage(ctx context.Context) (ImageAttachment, error) {
	if path, err := exec.LookPath("pngpaste"); err == nil {
		out, err := exec.CommandContext(ctx, path, "-").Output()
		if err == nil && len(out) > 0 {
			return buildImageAttachment(out, "clipboard.png")
		}
	}
	dir, err := os.MkdirTemp("", "hev-foh-clip-")
	if err != nil {
		return ImageAttachment{}, err
	}
	defer os.RemoveAll(dir)
	pngPath := filepath.Join(dir, "clip.png")
	// Try PNGf first, then TIFF (converted to PNG via sips).
	script := fmt.Sprintf(`
try
	set imgData to the clipboard as «class PNGf»
	set fp to open for access POSIX file %q with write permission
	write imgData to fp
	close access fp
	return "png"
on error
	try
		set imgData to the clipboard as TIFF picture
		set fp to open for access POSIX file %q with write permission
		write imgData to fp
		close access fp
		return "tiff"
	on error
		return "none"
	end try
end try
`, pngPath, filepath.Join(dir, "clip.tiff"))
	out, err := exec.CommandContext(ctx, "osascript", "-e", script).Output()
	if err != nil {
		return ImageAttachment{}, errNoClipboardImage
	}
	kind := strings.TrimSpace(string(out))
	if kind == "tiff" {
		if err := exec.CommandContext(ctx, "sips", "-s", "format", "png", filepath.Join(dir, "clip.tiff"), "--out", pngPath).Run(); err != nil {
			return ImageAttachment{}, errNoClipboardImage
		}
	} else if kind != "png" {
		return ImageAttachment{}, errNoClipboardImage
	}
	data, err := os.ReadFile(pngPath)
	if err != nil || len(data) == 0 {
		return ImageAttachment{}, errNoClipboardImage
	}
	return buildImageAttachment(data, "clipboard.png")
}

func readLinuxClipboardImage(ctx context.Context) (ImageAttachment, error) {
	if path, err := exec.LookPath("wl-paste"); err == nil {
		out, err := exec.CommandContext(ctx, path, "--no-newline", "--type", "image/png").Output()
		if err == nil && len(out) > 0 {
			return buildImageAttachment(out, "clipboard.png")
		}
	}
	if path, err := exec.LookPath("xclip"); err == nil {
		out, err := exec.CommandContext(ctx, path, "-selection", "clipboard", "-t", "image/png", "-o").Output()
		if err == nil && len(out) > 0 {
			return buildImageAttachment(out, "clipboard.png")
		}
	}
	return ImageAttachment{}, errNoClipboardImage
}

func readWindowsClipboardImage(ctx context.Context) (ImageAttachment, error) {
	dir, err := os.MkdirTemp("", "hev-foh-clip-")
	if err != nil {
		return ImageAttachment{}, err
	}
	defer os.RemoveAll(dir)
	pngPath := filepath.Join(dir, "clip.png")
	ps := fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms; $img = [System.Windows.Forms.Clipboard]::GetImage(); if ($img -eq $null) { exit 1 }; $img.Save('%s', [System.Drawing.Imaging.ImageFormat]::Png)`, strings.ReplaceAll(pngPath, "'", "''"))
	if err := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", ps).Run(); err != nil {
		return ImageAttachment{}, errNoClipboardImage
	}
	data, err := os.ReadFile(pngPath)
	if err != nil || len(data) == 0 {
		return ImageAttachment{}, errNoClipboardImage
	}
	return buildImageAttachment(data, "clipboard.png")
}
