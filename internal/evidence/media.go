package evidence

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/hev/kit/internal/blob"
)

// Media is what the bytes say about one item.
type Media struct {
	Modality   string
	MIME       string
	Width      int
	Height     int
	DurationMs *int64
}

// Limits are the per-item caps Inspect applies.
type Limits struct {
	ImageBytes      int64
	VideoBytes      int64
	VideoDurationMs int64
	TextBytes       int64
}

// DefaultLimits are RFC 0022's v1 caps.
var DefaultLimits = Limits{ImageBytes: MaxImageBytes, VideoBytes: MaxVideoBytes, VideoDurationMs: MaxVideoDurationMs, TextBytes: MaxTextBytes}

// MaxAnyBytes is the largest item any modality accepts; a reader is never
// buffered past it.
func (l Limits) MaxAnyBytes() int64 { return max(l.ImageBytes, l.VideoBytes, l.TextBytes) }

// Inspect decides an item's modality and media type from its bytes alone and
// applies the caps. A refused item returns an error wrapping ErrUnsupported
// or ErrTooLarge.
func Inspect(b []byte, lim Limits) (Media, error) {
	if len(b) == 0 {
		return Media{}, fmt.Errorf("%w: the file is empty", ErrUnsupported)
	}
	n := int64(len(b))
	switch {
	case isImage(b):
		if n > lim.ImageBytes {
			return Media{}, tooLarge("image", n, lim.ImageBytes)
		}
		m, err := blob.Inspect(b)
		if err != nil {
			return Media{}, fmt.Errorf("%w: %v", ErrUnsupported, err)
		}
		return Media{Modality: "image", MIME: m.Type, Width: m.Width, Height: m.Height}, nil
	case isBMFF(b):
		if n > lim.VideoBytes {
			return Media{}, tooLarge("video", n, lim.VideoBytes)
		}
		m, err := inspectBMFF(b)
		if err != nil {
			return Media{}, err
		}
		return m, checkDuration(m, lim)
	case bytes.HasPrefix(b, ebmlMagic):
		if n > lim.VideoBytes {
			return Media{}, tooLarge("video", n, lim.VideoBytes)
		}
		m, err := inspectWebM(b)
		if err != nil {
			return Media{}, err
		}
		return m, checkDuration(m, lim)
	}
	if isSVG(b) {
		return Media{}, fmt.Errorf("%w: SVG is not accepted (it can carry scripts); export it as PNG", ErrUnsupported)
	}
	if !utf8.Valid(b) || bytes.IndexByte(b, 0) >= 0 {
		got := strings.SplitN(http.DetectContentType(b), ";", 2)[0]
		return Media{}, fmt.Errorf("%w: the bytes are %s; evidence accepts UTF-8 text, PNG, JPEG, GIF or WebP images, and MP4, WebM or MOV video", ErrUnsupported, got)
	}
	if n > lim.TextBytes {
		return Media{}, tooLarge("text", n, lim.TextBytes)
	}
	return Media{Modality: "text", MIME: textType(b)}, nil
}

func tooLarge(kind string, n, max int64) error {
	return fmt.Errorf("%w: the %s is %s, over the %s limit", ErrTooLarge, kind, blob.Human(n), blob.Human(max))
}

func checkDuration(m Media, lim Limits) error {
	if m.DurationMs != nil && *m.DurationMs > lim.VideoDurationMs {
		return fmt.Errorf("%w: the video runs %.1fs, over the %.0fs limit", ErrTooLarge, float64(*m.DurationMs)/1000, float64(lim.VideoDurationMs)/1000)
	}
	return nil
}

func isImage(b []byte) bool {
	return bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")) ||
		bytes.HasPrefix(b, []byte{0xff, 0xd8, 0xff}) ||
		bytes.HasPrefix(b, []byte("GIF87a")) || bytes.HasPrefix(b, []byte("GIF89a")) ||
		(len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP")
}

// isSVG reports an SVG document: its root element is <svg>, after any XML
// prolog, comments or doctype.
func isSVG(b []byte) bool {
	head := bytes.ToLower(b[:min(len(b), 4096)])
	head = bytes.TrimLeft(head, "\xef\xbb\xbf \t\r\n")
	if bytes.HasPrefix(head, []byte("<svg")) || bytes.Contains(head, []byte("<!doctype svg")) {
		return true
	}
	if !bytes.HasPrefix(head, []byte("<?xml")) && !bytes.HasPrefix(head, []byte("<!--")) {
		return false
	}
	// Skip prolog, comments and processing instructions to the first element.
	for len(head) > 0 {
		head = bytes.TrimLeft(head, " \t\r\n")
		switch {
		case bytes.HasPrefix(head, []byte("<?")):
			i := bytes.Index(head, []byte("?>"))
			if i < 0 {
				return false
			}
			head = head[i+2:]
		case bytes.HasPrefix(head, []byte("<!--")):
			i := bytes.Index(head, []byte("-->"))
			if i < 0 {
				return false
			}
			head = head[i+3:]
		case bytes.HasPrefix(head, []byte("<!")):
			i := bytes.IndexByte(head, '>')
			if i < 0 {
				return false
			}
			head = head[i+1:]
		default:
			return bytes.HasPrefix(head, []byte("<svg"))
		}
	}
	return false
}

// textType names valid UTF-8 text by its content.
func textType(b []byte) string {
	trim := bytes.TrimSpace(bytes.TrimPrefix(b, []byte("\xef\xbb\xbf")))
	lower := bytes.ToLower(trim[:min(len(trim), 512)])
	switch {
	case len(trim) > 0 && (trim[0] == '{' || trim[0] == '[') && json.Valid(trim):
		return "application/json"
	case bytes.HasPrefix(lower, []byte("<!doctype html")) || bytes.HasPrefix(lower, []byte("<html")):
		return "text/html"
	case bytes.HasPrefix(trim, []byte("diff --git ")) || (bytes.HasPrefix(trim, []byte("--- ")) && bytes.Contains(trim[:min(len(trim), 4096)], []byte("\n+++ "))):
		return "text/x-diff"
	case isCSV(trim):
		return "text/csv"
	case isMarkdown(trim):
		return "text/markdown"
	}
	return "text/plain"
}

// isCSV: at least two lines, every one of the first ten with the same
// nonzero number of commas.
func isCSV(b []byte) bool {
	lines := bytes.Split(b[:min(len(b), 8192)], []byte("\n"))
	if len(lines) < 2 {
		return false
	}
	lines = lines[:min(len(lines), 10)]
	if len(b) > 8192 {
		lines = lines[:len(lines)-1] // the last may be cut
	}
	want := bytes.Count(lines[0], []byte(","))
	if want == 0 || len(lines) < 2 {
		return false
	}
	for _, l := range lines[1:] {
		if bytes.Count(l, []byte(",")) != want {
			return false
		}
	}
	return true
}

// isMarkdown: a heading or a fenced code block opens a line.
func isMarkdown(b []byte) bool {
	for _, l := range bytes.Split(b[:min(len(b), 8192)], []byte("\n")) {
		l = bytes.TrimRight(l, "\r")
		if bytes.HasPrefix(l, []byte("# ")) || bytes.HasPrefix(l, []byte("## ")) || bytes.HasPrefix(l, []byte("```")) {
			return true
		}
	}
	return false
}

// ── ISO BMFF (MP4, MOV) ──

func isBMFF(b []byte) bool { return len(b) >= 12 && string(b[4:8]) == "ftyp" }

type box struct {
	typ        string
	start, end int // payload range
}

// boxes parses the boxes in b[start:end]. A box that runs past end is a
// truncated file.
func boxes(b []byte, start, end int) ([]box, error) {
	var out []box
	for off := start; off < end; {
		if end-off < 8 {
			return nil, errors.New("a box header is cut short")
		}
		size := uint64(binary.BigEndian.Uint32(b[off:]))
		typ := string(b[off+4 : off+8])
		hdr := 8
		switch size {
		case 1:
			if end-off < 16 {
				return nil, errors.New("a box header is cut short")
			}
			size = binary.BigEndian.Uint64(b[off+8:])
			hdr = 16
		case 0:
			size = uint64(end - off)
		}
		if size < uint64(hdr) || size > uint64(end-off) {
			return nil, fmt.Errorf("the %q box runs past the end of the file", typ)
		}
		out = append(out, box{typ: typ, start: off + hdr, end: off + int(size)})
		off += int(size)
	}
	return out, nil
}

func find(bs []box, typ string) (box, bool) {
	for _, x := range bs {
		if x.typ == typ {
			return x, true
		}
	}
	return box{}, false
}

func inspectBMFF(b []byte) (Media, error) {
	corrupt := func(why string) (Media, error) {
		return Media{}, fmt.Errorf("%w: the MP4/MOV container is truncated or corrupt (%s)", ErrUnsupported, why)
	}
	top, err := boxes(b, 0, len(b))
	if err != nil {
		return corrupt(err.Error())
	}
	ftyp := top[0]
	if ftyp.end-ftyp.start < 8 {
		return corrupt("short ftyp")
	}
	brand := string(b[ftyp.start : ftyp.start+4])
	brands := []string{brand}
	for off := ftyp.start + 8; off+4 <= ftyp.end; off += 4 {
		brands = append(brands, string(b[off:off+4]))
	}
	mime := ""
	if brand == "qt  " {
		mime = "video/quicktime"
	} else {
		for _, br := range brands {
			if isMP4Brand(br) {
				mime = "video/mp4"
				break
			}
		}
	}
	if mime == "" {
		return Media{}, fmt.Errorf("%w: the ISO media brand %q is not MP4 or QuickTime video", ErrUnsupported, strings.TrimSpace(brand))
	}
	moov, ok := find(top, "moov")
	if !ok {
		return corrupt("no moov box")
	}
	inner, err := boxes(b, moov.start, moov.end)
	if err != nil {
		return corrupt(err.Error())
	}
	mvhd, ok := find(inner, "mvhd")
	if !ok {
		return corrupt("no mvhd box")
	}
	p := b[mvhd.start:mvhd.end]
	var scale uint32
	var dur uint64
	switch {
	case len(p) >= 20 && p[0] == 0:
		scale, dur = binary.BigEndian.Uint32(p[12:]), uint64(binary.BigEndian.Uint32(p[16:]))
		if dur == math.MaxUint32 {
			dur = math.MaxUint64
		}
	case len(p) >= 32 && p[0] == 1:
		scale, dur = binary.BigEndian.Uint32(p[20:]), binary.BigEndian.Uint64(p[24:])
	default:
		return corrupt("bad mvhd")
	}
	if scale == 0 {
		return corrupt("mvhd timescale is zero")
	}
	m := Media{Modality: "video", MIME: mime}
	if dur != 0 && dur != math.MaxUint64 {
		ms := int64(math.MaxInt64)
		if secs := dur / uint64(scale); secs < 1<<40 {
			ms = int64(secs*1000 + dur%uint64(scale)*1000/uint64(scale))
		}
		m.DurationMs = &ms
	}
	for _, trak := range inner {
		if trak.typ != "trak" {
			continue
		}
		tb, err := boxes(b, trak.start, trak.end)
		if err != nil {
			return corrupt(err.Error())
		}
		if tkhd, ok := find(tb, "tkhd"); ok {
			p := b[tkhd.start:tkhd.end]
			off := 76
			if len(p) > 0 && p[0] == 1 {
				off = 88
			}
			if len(p) >= off+8 {
				w, h := int(binary.BigEndian.Uint32(p[off:])>>16), int(binary.BigEndian.Uint32(p[off+4:])>>16)
				if w > 0 && h > 0 {
					m.Width, m.Height = w, h
					break
				}
			}
		}
	}
	return m, nil
}

func isMP4Brand(br string) bool {
	switch br {
	case "isom", "iso2", "iso3", "iso4", "iso5", "iso6", "iso7", "iso8", "iso9", "mp41", "mp42", "avc1", "dash", "M4V ", "M4VP", "mmp4", "msnv", "f4v ", "hvc1", "av01":
		return true
	}
	return false
}

// ── WebM (EBML) ──

var ebmlMagic = []byte{0x1a, 0x45, 0xdf, 0xa3}

const (
	idEBML          = 0x1a45dfa3
	idDocType       = 0x4282
	idSegment       = 0x18538067
	idInfo          = 0x1549a966
	idTimecodeScale = 0x2ad7b1
	idDuration      = 0x4489
	idTracks        = 0x1654ae6b
	idTrackEntry    = 0xae
	idVideo         = 0xe0
	idPixelWidth    = 0xb0
	idPixelHeight   = 0xba
	idCluster       = 0x1f43b675
)

type element struct {
	id         uint64
	start, end int // payload; end is -1 for an unknown size
}

// vint reads an EBML variable-length integer at b[off:]. keepMarker keeps
// the length marker (element IDs); unknown reports an all-ones size.
func vint(b []byte, off int, keepMarker bool) (v uint64, n int, unknown bool, err error) {
	if off >= len(b) {
		return 0, 0, false, errors.New("cut short")
	}
	first := b[off]
	if first == 0 {
		return 0, 0, false, errors.New("bad variable-length integer")
	}
	n = 1
	for mask := byte(0x80); first&mask == 0; mask >>= 1 {
		n++
	}
	if off+n > len(b) {
		return 0, 0, false, errors.New("cut short")
	}
	v = uint64(first)
	if !keepMarker {
		v &= uint64(0xff >> n)
	}
	allOnes := v == uint64(0xff>>n)
	for i := 1; i < n; i++ {
		v = v<<8 | uint64(b[off+i])
		if b[off+i] != 0xff {
			allOnes = false
		}
	}
	return v, n, !keepMarker && allOnes, nil
}

// elements parses the EBML elements in b[start:end]. An element with an
// unknown size (a live recording's Segment or Cluster) runs to end.
func elements(b []byte, start, end int) ([]element, error) {
	var out []element
	for off := start; off < end; {
		id, n, _, err := vint(b, off, true)
		if err != nil || n > 4 {
			return nil, fmt.Errorf("element id: %v", orBad(err))
		}
		size, m, unknown, err := vint(b, off+n, false)
		if err != nil {
			return nil, fmt.Errorf("element size: %v", err)
		}
		p := off + n + m
		if unknown {
			out = append(out, element{id: id, start: p, end: end})
			break
		}
		if size > uint64(end-p) {
			return nil, fmt.Errorf("element %#x runs past the end of the file", id)
		}
		out = append(out, element{id: id, start: p, end: p + int(size)})
		off = p + int(size)
	}
	return out, nil
}

func orBad(err error) error {
	if err == nil {
		return errors.New("id longer than 4 bytes")
	}
	return err
}

func ebmlUint(b []byte) uint64 {
	var v uint64
	for _, c := range b[:min(len(b), 8)] {
		v = v<<8 | uint64(c)
	}
	return v
}

func inspectWebM(b []byte) (Media, error) {
	corrupt := func(why string) (Media, error) {
		return Media{}, fmt.Errorf("%w: the WebM container is truncated or corrupt (%s)", ErrUnsupported, why)
	}
	top, err := elements(b, 0, len(b))
	if err != nil {
		return corrupt(err.Error())
	}
	if len(top) == 0 || top[0].id != idEBML {
		return corrupt("no EBML header")
	}
	head, err := elements(b, top[0].start, top[0].end)
	if err != nil {
		return corrupt(err.Error())
	}
	doc := ""
	for _, e := range head {
		if e.id == idDocType {
			doc = string(bytes.TrimRight(b[e.start:e.end], "\x00"))
		}
	}
	if doc != "webm" {
		return Media{}, fmt.Errorf("%w: the EBML document type is %q; only WebM video is accepted", ErrUnsupported, doc)
	}
	var seg *element
	for i := range top[1:] {
		if top[1+i].id == idSegment {
			seg = &top[1+i]
			break
		}
	}
	if seg == nil {
		return corrupt("no Segment")
	}
	// Scan the segment's children up to the first Cluster: Info and Tracks
	// come before the media data.
	m := Media{Modality: "video", MIME: "video/webm"}
	foundInfo := false
	for off := seg.start; off < seg.end; {
		id, n, _, err := vint(b, off, true)
		if err != nil || n > 4 {
			return corrupt("segment element id")
		}
		if id == idCluster {
			break
		}
		size, sn, unknown, err := vint(b, off+n, false)
		if err != nil {
			return corrupt("segment element size")
		}
		p := off + n + sn
		if unknown || size > uint64(seg.end-p) {
			return corrupt(fmt.Sprintf("element %#x runs past the end of the file", id))
		}
		end := p + int(size)
		switch id {
		case idInfo:
			foundInfo = true
			kids, err := elements(b, p, end)
			if err != nil {
				return corrupt(err.Error())
			}
			scale := uint64(1_000_000)
			var dur float64
			hasDur := false
			for _, k := range kids {
				v := b[k.start:k.end]
				switch k.id {
				case idTimecodeScale:
					scale = ebmlUint(v)
				case idDuration:
					switch len(v) {
					case 4:
						dur, hasDur = float64(math.Float32frombits(binary.BigEndian.Uint32(v))), true
					case 8:
						dur, hasDur = math.Float64frombits(binary.BigEndian.Uint64(v)), true
					default:
						return corrupt("bad Duration")
					}
				}
			}
			if scale == 0 {
				return corrupt("TimecodeScale is zero")
			}
			if hasDur {
				if math.IsNaN(dur) || dur < 0 || math.IsInf(dur, 0) {
					return corrupt("bad Duration")
				}
				ms := dur * float64(scale) / 1e6
				v := int64(math.MaxInt64)
				if ms < float64(math.MaxInt64) {
					v = int64(math.Round(ms))
				}
				m.DurationMs = &v
			}
		case idTracks:
			entries, err := elements(b, p, end)
			if err != nil {
				return corrupt(err.Error())
			}
			for _, te := range entries {
				if te.id != idTrackEntry || m.Width > 0 {
					continue
				}
				fields, err := elements(b, te.start, te.end)
				if err != nil {
					return corrupt(err.Error())
				}
				for _, f := range fields {
					if f.id != idVideo {
						continue
					}
					vf, err := elements(b, f.start, f.end)
					if err != nil {
						return corrupt(err.Error())
					}
					for _, x := range vf {
						switch x.id {
						case idPixelWidth:
							m.Width = int(ebmlUint(b[x.start:x.end]))
						case idPixelHeight:
							m.Height = int(ebmlUint(b[x.start:x.end]))
						}
					}
				}
			}
		}
		off = end
	}
	if !foundInfo {
		return corrupt("no Info")
	}
	if m.Width <= 0 || m.Height <= 0 {
		m.Width, m.Height = 0, 0
	}
	return m, nil
}
