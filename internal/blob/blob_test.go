package blob

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func pic(w, h int) image.Image {
	m := image.NewPaletted(image.Rect(0, 0, w, h), []color.Color{color.Black, color.White})
	m.SetColorIndex(w/2, h/2, 1)
	return m
}

func pngOf(t *testing.T, w, h int) []byte {
	var b bytes.Buffer
	if err := png.Encode(&b, pic(w, h)); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func jpegOf(t *testing.T, w, h int) []byte {
	var b bytes.Buffer
	if err := jpeg.Encode(&b, pic(w, h), nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func gifOf(t *testing.T, w, h int) []byte {
	var b bytes.Buffer
	if err := gif.Encode(&b, pic(w, h), nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// webpVP8X is the smallest extended-format WebP header for a w×h canvas.
func webpVP8X(w, h int) []byte {
	b := []byte("RIFF\x00\x00\x00\x00WEBPVP8X\x0a\x00\x00\x00\x00\x00\x00\x00")
	b = append(b, byte(w-1), byte((w-1)>>8), byte((w-1)>>16), byte(h-1), byte((h-1)>>8), byte((h-1)>>16))
	binary.LittleEndian.PutUint32(b[4:], uint32(len(b)-8))
	return b
}

func webpVP8L(w, h int) []byte {
	b := []byte("RIFF\x00\x00\x00\x00WEBPVP8L\x05\x00\x00\x00\x2f")
	v := uint32(w-1) | uint32(h-1)<<14
	b = binary.LittleEndian.AppendUint32(b, v)
	b = append(b, make([]byte, 8)...)
	binary.LittleEndian.PutUint32(b[4:], uint32(len(b)-8))
	return b
}

func webpVP8(w, h int) []byte {
	b := []byte("RIFF\x00\x00\x00\x00WEBPVP8 \x0a\x00\x00\x00\x00\x00\x00\x9d\x01\x2a")
	b = binary.LittleEndian.AppendUint16(b, uint16(w))
	b = binary.LittleEndian.AppendUint16(b, uint16(h))
	binary.LittleEndian.PutUint32(b[4:], uint32(len(b)-8))
	return b
}

func TestInspectAcceptsTheFourTypes(t *testing.T) {
	for _, c := range []struct {
		name string
		b    []byte
		typ  string
		w, h int
	}{
		{"png", pngOf(t, 64, 48), "image/png", 64, 48},
		{"jpeg", jpegOf(t, 33, 17), "image/jpeg", 33, 17},
		{"gif", gifOf(t, 5, 9), "image/gif", 5, 9},
		{"webp vp8x", webpVP8X(1920, 1080), "image/webp", 1920, 1080},
		{"webp vp8l", webpVP8L(300, 200), "image/webp", 300, 200},
		{"webp vp8", webpVP8(640, 480), "image/webp", 640, 480},
	} {
		t.Run(c.name, func(t *testing.T) {
			m, err := Inspect(c.b)
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(c.b)
			if m.Type != c.typ || m.Width != c.w || m.Height != c.h || m.Size != int64(len(c.b)) || m.SHA256 != hex.EncodeToString(sum[:]) {
				t.Fatalf("%+v", m)
			}
		})
	}
}

// A real encoder's WebP, when one is installed, reads the same as ours.
func TestInspectRealWebP(t *testing.T) {
	cwebp, err := exec.LookPath("cwebp")
	if err != nil {
		t.Skip("cwebp not installed")
	}
	dir := t.TempDir()
	in, out := filepath.Join(dir, "in.png"), filepath.Join(dir, "out.webp")
	os.WriteFile(in, pngOf(t, 37, 21), 0o600)
	if b, err := exec.Command(cwebp, "-quiet", in, "-o", out).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, b)
	}
	b, _ := os.ReadFile(out)
	m, err := Inspect(b)
	if err != nil || m.Width != 37 || m.Height != 21 {
		t.Fatalf("%+v %v", m, err)
	}
}

func TestStoreRefusesWithoutWriting(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "blobs")
	s := New(dir, 1000)
	for _, c := range []struct {
		name string
		b    []byte
		want error
		says string
	}{
		{"too large", jpegOf(t, 600, 600), ErrTooLarge, "over the"},
		{"text", []byte("just some notes\n"), ErrUnsupported, "text/plain"},
		{"svg", []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`), ErrUnsupported, "SVG"},
		{"empty", nil, ErrUnsupported, "empty"},
		{"pdf", []byte("%PDF-1.7\n..."), ErrUnsupported, "application/pdf"},
		{"truncated png", pngOf(t, 4, 4)[:12], ErrUnsupported, "image/png"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if len(c.b) > 1000 != (c.want == ErrTooLarge) {
				t.Fatalf("fixture size %d does not suit the case", len(c.b))
			}
			_, _, err := s.Put(c.b)
			if !errors.Is(err, c.want) || !strings.Contains(err.Error(), c.says) {
				t.Fatalf("Put: %v", err)
			}
			_, _, err = s.PutReader(bytes.NewReader(c.b))
			if !errors.Is(err, c.want) {
				t.Fatalf("PutReader: %v", err)
			}
		})
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("a refused blob created the store directory: %v", err)
	}
}

func TestPutDedupesAndGetVerifies(t *testing.T) {
	dir := t.TempDir()
	s := New(dir, 0)
	if s.MaxBytes() != DefaultMaxBytes {
		t.Fatalf("default cap %d", s.MaxBytes())
	}
	b := pngOf(t, 10, 10)
	m, created, err := s.Put(b)
	if err != nil || !created {
		t.Fatalf("first put: %v %v", created, err)
	}
	m2, created, err := s.PutReader(bytes.NewReader(b))
	if err != nil || created || m2 != m {
		t.Fatalf("second put: %+v %v %v", m2, created, err)
	}
	var files []string
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if !d.IsDir() {
			files = append(files, p)
		}
		return nil
	})
	if len(files) != 1 || files[0] != filepath.Join(dir, "sha256", m.SHA256[:2], m.SHA256) {
		t.Fatalf("files on disk: %v", files)
	}
	got, gm, err := s.Get(m.SHA256)
	if err != nil || !bytes.Equal(got, b) || gm != m {
		t.Fatalf("get: %+v %v", gm, err)
	}
	if st, err := s.Stat(m.SHA256); err != nil || st != m {
		t.Fatalf("stat: %+v %v", st, err)
	}

	for _, h := range []string{"", "../etc/passwd", strings.Repeat("A", 64), strings.Repeat("0", 64)} {
		if _, _, err := s.Get(h); !errors.Is(err, ErrNotFound) {
			t.Errorf("get %q: %v", h, err)
		}
	}

	// Bytes that no longer match their name are not served.
	os.WriteFile(s.Path(m.SHA256), pngOf(t, 11, 11), 0o600)
	if _, _, err := s.Get(m.SHA256); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("tampered blob: %v", err)
	}
}

func TestExt(t *testing.T) {
	for _, typ := range Accepted {
		if Ext(typ) == "" {
			t.Errorf("no extension for %s", typ)
		}
	}
	if Ext("image/svg+xml") != "" {
		t.Error("svg has an extension")
	}
}
