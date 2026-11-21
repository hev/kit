package evidence

import (
	"bytes"
	"encoding/binary"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// Rule: an item's modality and media type come from its bytes. Every
// accepted type is kept with its dimensions and duration; everything else
// is refused with ErrUnsupported or ErrTooLarge.
func TestInspectAcceptsByBytes(t *testing.T) {
	for _, tt := range []struct {
		name     string
		b        []byte
		modality string
		mime     string
		w, h     int
		duration int64 // -1: none
	}{
		{"png", pngOf(t, 390, 844), "image", "image/png", 390, 844, -1},
		{"jpeg", jpegOf(t, 64, 32), "image", "image/jpeg", 64, 32, -1},
		{"gif", gifOf(t, 10, 20), "image", "image/gif", 10, 20, -1},
		{"webp", webpOf(1920, 1080), "image", "image/webp", 1920, 1080, -1},
		{"mp4", mp4Of("isom", 14200, 390, 844), "video", "video/mp4", 390, 844, 14200},
		{"mov", mp4Of("qt  ", 60000, 1280, 720), "video", "video/quicktime", 1280, 720, 60000},
		{"webm", webmOf("webm", 59999.6, 640, 480), "video", "video/webm", 640, 480, 60000},
		{"webm live recording without duration", webmOf("webm", -1, 640, 480), "video", "video/webm", 640, 480, -1},
		{"plain text", []byte("POST /api/checkout 200 OK\n0 console errors\n"), "text", "text/plain", 0, 0, -1},
		{"json", []byte(`{"status": 200}`), "text", "application/json", 0, 0, -1},
		{"markdown", []byte("# Result\n\nAll good.\n"), "text", "text/markdown", 0, 0, -1},
		{"diff", []byte("diff --git a/x b/x\n--- a/x\n+++ b/x\n"), "text", "text/x-diff", 0, 0, -1},
		{"csv", []byte("a,b,c\n1,2,3\n4,5,6\n"), "text", "text/csv", 0, 0, -1},
		{"html", []byte("<!DOCTYPE html><html><body><svg></svg></body></html>"), "text", "text/html", 0, 0, -1},
		{"utf-8 text", []byte("café ✓ 日本語\n"), "text", "text/plain", 0, 0, -1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, err := Inspect(tt.b, DefaultLimits)
			if err != nil {
				t.Fatal(err)
			}
			if m.Modality != tt.modality || m.MIME != tt.mime || m.Width != tt.w || m.Height != tt.h {
				t.Fatalf("got %+v", m)
			}
			if (tt.duration < 0) != (m.DurationMs == nil) || m.DurationMs != nil && *m.DurationMs != tt.duration {
				t.Fatalf("duration %v, want %d", m.DurationMs, tt.duration)
			}
		})
	}
}

func TestInspectRefuses(t *testing.T) {
	png := pngOf(t, 8, 8)
	mp4 := mp4Of("isom", 1000, 8, 8)
	webm := webmOf("webm", 1000, 8, 8)
	big := func(prefix []byte, n int64) []byte {
		b := make([]byte, n)
		copy(b, prefix)
		return b
	}
	for _, tt := range []struct {
		name string
		b    []byte
		want error
		msg  string
	}{
		{"empty", nil, ErrUnsupported, "empty"},
		{"svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`), ErrUnsupported, "SVG"},
		{"svg with xml prolog", []byte("<?xml version=\"1.0\"?>\n<!-- x -->\n<svg></svg>"), ErrUnsupported, "SVG"},
		{"svg doctype", []byte(`<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN"><svg/>`), ErrUnsupported, "SVG"},
		{"corrupt png", png[:20], ErrUnsupported, "png"},
		{"corrupt jpeg", []byte{0xff, 0xd8, 0xff, 0xe0, 0, 0}, ErrUnsupported, "jpeg"},
		{"bmp", append([]byte("BM"), make([]byte, 60)...), ErrUnsupported, "UTF-8 text"},
		{"invalid utf-8", []byte{0xc3, 0x28, 'a', 'b'}, ErrUnsupported, "UTF-8 text"},
		{"binary with nul", []byte("abc\x00def"), ErrUnsupported, "UTF-8 text"},
		{"truncated mp4", mp4[:len(mp4)-3], ErrUnsupported, "truncated or corrupt"},
		{"mp4 without moov", bmffBox("ftyp", []byte("isom"), []byte{0, 0, 0, 0}), ErrUnsupported, "no moov"},
		{"mp4 zero timescale", func() []byte {
			b := append([]byte(nil), mp4...)
			copy(b[bytes.Index(b, []byte("mvhd"))+4+12:], []byte{0, 0, 0, 0})
			return b
		}(), ErrUnsupported, "timescale"},
		{"unknown iso brand", mp4Of("heic", 1000, 8, 8), ErrUnsupported, "brand"},
		{"truncated webm", webm[:len(webm)-10], ErrUnsupported, "truncated or corrupt"},
		{"matroska", webmOf("matroska", 1000, 8, 8), ErrUnsupported, "only WebM"},
		{"mp4 over 60s", mp4Of("isom", 60001, 8, 8), ErrTooLarge, "60s limit"},
		{"mov over 60s", mp4Of("qt  ", 90000, 8, 8), ErrTooLarge, "60s limit"},
		{"webm over 60s", webmOf("webm", 61000, 8, 8), ErrTooLarge, "60s limit"},
		{"video over 100 MiB", big(mp4, MaxVideoBytes+1), ErrTooLarge, "100.0 MB limit"},
		{"webm over 100 MiB", big(webm, MaxVideoBytes+1), ErrTooLarge, "100.0 MB limit"},
		{"image over 10 MiB", big(png, MaxImageBytes+1), ErrTooLarge, "10.0 MB limit"},
		{"text over 5 MiB", bytes.Repeat([]byte("a"), int(MaxTextBytes+1)), ErrTooLarge, "5.0 MB limit"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, err := Inspect(tt.b, DefaultLimits)
			if !errors.Is(err, tt.want) || !strings.Contains(err.Error(), tt.msg) {
				t.Fatalf("Inspect = %+v, %v; want %v containing %q", m, err, tt.want, tt.msg)
			}
		})
	}
	// The caps are inclusive: exactly 100 MiB and exactly 60s are kept.
	// (The zero padding after the boxes reads as a final box that runs to
	// the end of the file, which ISO BMFF allows.)
	if _, err := Inspect(big(mp4Of("isom", 60000, 8, 8), MaxVideoBytes), DefaultLimits); err != nil {
		t.Fatalf("exactly 100 MiB refused: %v", err)
	}
	if _, err := Inspect(mp4Of("isom", 60000, 8, 8), DefaultLimits); err != nil {
		t.Fatalf("exactly 60s refused: %v", err)
	}
}

// Rule: an item name is one safe path segment. Names with separators, ".."
// or control characters are refused; other unsafe characters are replaced.
func TestSafeName(t *testing.T) {
	for in, want := range map[string]string{
		"checkout-390px.png": "checkout-390px.png",
		"Screen Shot 1.png":  "Screen-Shot-1.png",
		".hidden.log":        "hidden.log",
		"résumé.txt":         "r-sum-.txt",
		"export-status.txt":  "export-status.txt",
		"report.v2.final.md": "report.v2.final.md",
	} {
		got, err := SafeName(in)
		if err != nil || got != want {
			t.Errorf("SafeName(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "../etc/passwd", "..", ".", "a/b.png", `a\b.png`, "/abs.png", "x..y", "C:evil", "tab\there", "nl\n", strings.Repeat("a", MaxNameLen+1), "\xff.png"} {
		if got, err := SafeName(bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("SafeName(%q) = %q, %v; want refused", bad, got, err)
		}
	}
}

// Rule: an image or video is kept only when its container is whole: every
// chunk, segment, block, box or element it declares lies inside the file
// and the format's end marker is present. Whole files of every type are
// kept; each kind of cut or overrun is refused with ErrUnsupported, and a
// refused file writes nothing.
func TestInspectStructure(t *testing.T) {
	png8 := pngOf(t, 8, 8)
	ihdr := png8[8:33] // the 25-byte IHDR chunk
	jpg := jpegOf(t, 16, 16)
	gf := gifOf(t, 10, 20)
	webp := webpOf(64, 64)
	mp4 := mp4Of("isom", 1000, 8, 8)

	keep := map[string][]byte{
		"png":                      png8,
		"png with data after IEND": append(append([]byte(nil), png8...), "trailer"...),
		"png with an ancillary chunk": func() []byte {
			b := append([]byte(nil), png8[:33]...)
			b = append(b, pngChunk("tEXt", []byte("Comment\x00ok"))...)
			return append(b, png8[33:]...)
		}(),
		"jpeg":                     jpg,
		"gif":                      gf,
		"webp":                     webp,
		"mp4":                      mp4,
		"mov":                      mp4Of("qt  ", 1000, 8, 8),
		"webm":                     webmOf("webm", 1000, 8, 8),
		"live webm, unknown sizes": webmLive(0),
		"mp4 with nested stbl":     mp4WithStbl(0),
	}
	for name, b := range keep {
		if _, err := Inspect(b, DefaultLimits); err != nil {
			t.Errorf("%s refused: %v", name, err)
		}
	}

	refuse := map[string]struct {
		b   []byte
		msg string
	}{
		// The validator's 33-byte PNG: signature + IHDR, nothing else.
		"png signature and IHDR only": {png8[:33], "no IEND"},
		"png without IDAT":            {bytes.Join([][]byte{png8[:8], ihdr, pngChunk("IEND", nil)}, nil), "no IDAT"},
		"png without IEND":            {png8[:len(png8)-12], "no IEND"},
		"png chunk past end of file":  {png8[:len(png8)-14], "past the end"},
		"png bad crc": {func() []byte {
			b := append([]byte(nil), png8...)
			b[len(b)-13] ^= 0xff // last byte of the IDAT CRC
			return b
		}(), "CRC"},
		"png IHDR not first": {bytes.Join([][]byte{png8[:8], pngChunk("tEXt", []byte("a\x00b")), png8[8:]}, nil), "not IHDR"},
		"jpeg without EOI":   {jpg[:len(jpg)-2], "no EOI"},
		"jpeg cut in a segment": {func() []byte {
			i := bytes.Index(jpg, []byte{0xff, 0xda})
			return jpg[:i+3]
		}(), "jpeg"},
		"jpeg segment past end": {func() []byte {
			sof := bytes.Index(jpg, []byte{0xff, 0xc0})
			b := append([]byte(nil), jpg[:sof+19]...) // SOF0 for one or three components
			b = append(b, 0xff, 0xc4, 0x40, 0x00, 1, 2, 3)
			return b
		}(), "jpeg"},
		"gif without trailer":   {gf[:len(gf)-1], "no trailer"},
		"gif cut in image data": {gf[:len(gf)-5], "past the end"},
		"gif with no image":     {append(append([]byte(nil), gf[:13+3<<(gf[10]&7+1)]...), 0x3b), "no image"},
		"webp riff larger than file": {func() []byte {
			b := append([]byte(nil), webp...)
			binary.LittleEndian.PutUint32(b[4:], uint32(len(b)))
			return b
		}(), "RIFF header declares"},
		"webp with bytes past riff": {append(append([]byte(nil), webp...), 0, 0), "RIFF header declares"},
		"webp chunk past end": {func() []byte {
			b := append([]byte(nil), webp...)
			binary.LittleEndian.PutUint32(b[16:], 200) // VP8X size
			return b
		}(), "past the end"},
		// The validator's WebM: live Segment, Cluster declaring 126 bytes
		// more than the file contains.
		"webm cluster past end of file": {webmLive(126), "past the end"},
		"webm known-size cluster past end": {func() []byte {
			b := webmOf("webm", 1000, 8, 8)
			return b[:len(b)-1]
		}(), "truncated or corrupt"},
		"webm block past cluster": {func() []byte {
			b := webmLive(0)
			return b[:len(b)-3]
		}(), "past the end"},
		"webm unknown size outside a cluster": {func() []byte {
			b := webmLive(0)
			i := bytes.Index(b, []byte{0x15, 0x49, 0xa9, 0x66}) // Info
			out := append([]byte(nil), b[:i+4]...)
			out = append(out, unknownSize...)
			return append(out, b[i+12:]...)
		}(), "unknown size"},
		"mp4 mdat past end": {func() []byte {
			b := append([]byte(nil), mp4...)
			i := bytes.LastIndex(b, []byte("mdat"))
			binary.BigEndian.PutUint32(b[i-4:], 4096)
			return b
		}(), "past the end"},
		"mp4 nested box past its parent": {mp4WithStbl(64), "past the end"},
	}
	s := testStore(t)
	in := t.TempDir()
	for name, tt := range refuse {
		t.Run(name, func(t *testing.T) {
			if _, err := Inspect(tt.b, DefaultLimits); !errors.Is(err, ErrUnsupported) || !strings.Contains(err.Error(), tt.msg) {
				t.Fatalf("Inspect = %v; want ErrUnsupported containing %q", err, tt.msg)
			}
			f := writeFile(t, in, "bad.bin", tt.b)
			if _, err := s.Put(Meta{Title: "t"}, []ItemInput{{Path: f}}, nil); !errors.Is(err, ErrUnsupported) {
				t.Fatalf("Put = %v", err)
			}
		})
	}
	open, err := s.Create(Meta{Title: "open"})
	if err != nil {
		t.Fatal(err)
	}
	for name, tt := range refuse {
		if _, _, err := s.AddItem(open.ID, ItemInput{Data: tt.b, Name: "bad.bin"}); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("AddItem %s = %v", name, err)
		}
	}
	got := tree(t, s.Root)
	want := []string{".lock", open.ID, filepath.Join(open.ID, ManifestName)}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("refused items left %v; want %v", got, want)
	}
}

// mp4WithStbl is mp4Of with a trak → mdia → minf → stbl chain. extra > 0
// makes the stbl's child declare that many bytes more than stbl holds.
func mp4WithStbl(extra int) []byte {
	stsd := bmffBox("stsd", make([]byte, 8))
	if extra > 0 {
		binary.BigEndian.PutUint32(stsd, uint32(len(stsd)+extra))
	}
	tkhd := make([]byte, 84)
	binary.BigEndian.PutUint32(tkhd[76:], 8<<16)
	binary.BigEndian.PutUint32(tkhd[80:], 8<<16)
	trak := bmffBox("trak", bmffBox("tkhd", tkhd), bmffBox("mdia", bmffBox("minf", bmffBox("stbl", stsd))))
	mvhd := make([]byte, 100)
	binary.BigEndian.PutUint32(mvhd[12:], 1000)
	binary.BigEndian.PutUint32(mvhd[16:], 1000)
	return bytes.Join([][]byte{bmffBox("ftyp", []byte("isom"), []byte{0, 0, 2, 0}, []byte("isom")), bmffBox("moov", bmffBox("mvhd", mvhd), trak), bmffBox("mdat", []byte("frames"))}, nil)
}
