// Package blob is a content-addressed store for images on the host's disk.
//
// Bytes are kept by their sha256 under one directory:
//
//	<dir>/sha256/<first two hex digits>/<64 hex digits>
//
// A blob is written once, atomically, and never changes, so the same bytes
// put twice are stored once. The media type is decided by the bytes, never by
// a file name: PNG, JPEG, GIF and WebP are accepted, everything else (SVG
// included) is refused. Pixel dimensions come from the image header.
//
// The package depends on nothing in the board, so trace capture can keep
// screenshots in its own archive directory with the same code.
package blob

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/gif" // register decoders for DecodeConfig
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/hev/kit/internal/platformops"
)

// DefaultMaxBytes is the per-blob cap when a Store is given none.
const DefaultMaxBytes int64 = 10 << 20

// Accepted lists the media types a Store keeps.
var Accepted = []string{"image/png", "image/jpeg", "image/gif", "image/webp"}

var (
	ErrTooLarge    = errors.New("image is too large")
	ErrUnsupported = errors.New("not an accepted image")
	ErrNotFound    = errors.New("no such image")
)

// Meta is what the store knows about a blob. All of it is derived from the
// bytes; nothing comes from the caller.
type Meta struct {
	SHA256 string `json:"sha256"`
	Type   string `json:"type"`
	Size   int64  `json:"size"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// Store keeps blobs under Dir. The directory is created on the first Put, so
// a store that is never written to leaves no trace on disk.
type Store struct {
	dir      string
	maxBytes int64
}

// New is a store rooted at dir. maxBytes <= 0 means DefaultMaxBytes.
func New(dir string, maxBytes int64) *Store {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	return &Store{dir: dir, maxBytes: maxBytes}
}

func (s *Store) Dir() string     { return s.dir }
func (s *Store) MaxBytes() int64 { return s.maxBytes }

// ValidHash reports whether h is a lowercase hex sha256.
func ValidHash(h string) bool {
	if len(h) != 64 {
		return false
	}
	for _, c := range h {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// Path is where the blob with hash h lives (or would live).
func (s *Store) Path(h string) string {
	return filepath.Join(s.dir, "sha256", h[:2], h)
}

// Inspect sniffs b and reads its dimensions. It does not apply a size cap.
func Inspect(b []byte) (Meta, error) {
	typ, err := sniff(b)
	if err != nil {
		return Meta{}, err
	}
	w, h, err := dimensions(typ, b)
	if err != nil {
		return Meta{}, fmt.Errorf("%w: the %s header cannot be read (%v)", ErrUnsupported, typ, err)
	}
	if w <= 0 || h <= 0 {
		return Meta{}, fmt.Errorf("%w: the %s has no pixels", ErrUnsupported, typ)
	}
	sum := sha256.Sum256(b)
	return Meta{SHA256: hex.EncodeToString(sum[:]), Type: typ, Size: int64(len(b)), Width: w, Height: h}, nil
}

// Check is Inspect with the store's size cap: what Put would accept, without
// writing anything.
func (s *Store) Check(b []byte) (Meta, error) {
	if int64(len(b)) > s.maxBytes {
		return Meta{}, tooLarge(int64(len(b)), s.maxBytes)
	}
	return Inspect(b)
}

func tooLarge(n, max int64) error {
	return fmt.Errorf("%w: %s is over the %s limit", ErrTooLarge, Human(n), Human(max))
}

// Put stores b and returns its metadata. created is false when the same bytes
// were already stored. A refused blob writes nothing.
func (s *Store) Put(b []byte) (m Meta, created bool, err error) {
	if m, err = s.Check(b); err != nil {
		return Meta{}, false, err
	}
	p := s.Path(m.SHA256)
	if fi, err := os.Stat(p); err == nil && fi.Size() == m.Size {
		return m, false, nil
	}
	if err := platformops.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return Meta{}, false, fmt.Errorf("blob directory: %w", err)
	}
	if err := platformops.WriteFileAtomic(p, b, 0o600); err != nil {
		return Meta{}, false, err
	}
	return m, true, nil
}

// PutReader reads at most the size cap from r and stores it. A reader with
// more than the cap is refused without reading the rest.
func (s *Store) PutReader(r io.Reader) (Meta, bool, error) {
	b, err := io.ReadAll(io.LimitReader(r, s.maxBytes+1))
	if err != nil {
		return Meta{}, false, err
	}
	if int64(len(b)) > s.maxBytes {
		return Meta{}, false, fmt.Errorf("%w: it is over the %s limit", ErrTooLarge, Human(s.maxBytes))
	}
	return s.Put(b)
}

// Get returns the blob's bytes and metadata, verifying that the bytes on disk
// still hash to h.
func (s *Store) Get(h string) ([]byte, Meta, error) {
	if !ValidHash(h) {
		return nil, Meta{}, ErrNotFound
	}
	b, err := os.ReadFile(s.Path(h))
	if errors.Is(err, os.ErrNotExist) {
		return nil, Meta{}, ErrNotFound
	}
	if err != nil {
		return nil, Meta{}, err
	}
	m, err := Inspect(b)
	if err != nil {
		return nil, Meta{}, fmt.Errorf("blob %s: %w", h, err)
	}
	if m.SHA256 != h {
		return nil, Meta{}, fmt.Errorf("blob %s: content does not match its hash", h)
	}
	return b, m, nil
}

// Stat is Get without the bytes.
func (s *Store) Stat(h string) (Meta, error) {
	_, m, err := s.Get(h)
	return m, err
}

// Ext is the file extension for an accepted media type, with the dot.
func Ext(typ string) string {
	switch typ {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	}
	return ""
}

func sniff(b []byte) (string, error) {
	switch {
	case bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")):
		return "image/png", nil
	case bytes.HasPrefix(b, []byte{0xff, 0xd8, 0xff}):
		return "image/jpeg", nil
	case bytes.HasPrefix(b, []byte("GIF87a")), bytes.HasPrefix(b, []byte("GIF89a")):
		return "image/gif", nil
	case len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP":
		return "image/webp", nil
	}
	if len(b) == 0 {
		return "", fmt.Errorf("%w: the file is empty", ErrUnsupported)
	}
	head := b[:min(len(b), 4096)]
	if bytes.Contains(bytes.ToLower(head), []byte("<svg")) {
		return "", fmt.Errorf("%w: SVG is not accepted (it can carry scripts); export it as PNG", ErrUnsupported)
	}
	got := strings.SplitN(http.DetectContentType(b), ";", 2)[0]
	return "", fmt.Errorf("%w: the bytes are %s, and only PNG, JPEG, GIF and WebP are accepted", ErrUnsupported, got)
}

func dimensions(typ string, b []byte) (int, int, error) {
	if typ == "image/webp" {
		return webpSize(b)
	}
	c, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return 0, 0, err
	}
	return c.Width, c.Height, nil
}

// webpSize reads the canvas size from the first chunk of a WebP file: VP8X
// (extended), VP8L (lossless) or VP8 (lossy).
func webpSize(b []byte) (int, int, error) {
	if len(b) < 30 {
		return 0, 0, errors.New("truncated")
	}
	switch string(b[12:16]) {
	case "VP8X":
		w := int(b[24]) | int(b[25])<<8 | int(b[26])<<16
		h := int(b[27]) | int(b[28])<<8 | int(b[29])<<16
		return w + 1, h + 1, nil
	case "VP8L":
		if b[20] != 0x2f {
			return 0, 0, errors.New("bad VP8L signature")
		}
		v := binary.LittleEndian.Uint32(b[21:25])
		return int(v&0x3fff) + 1, int(v>>14&0x3fff) + 1, nil
	case "VP8 ":
		if b[23] != 0x9d || b[24] != 0x01 || b[25] != 0x2a {
			return 0, 0, errors.New("bad VP8 start code")
		}
		return int(binary.LittleEndian.Uint16(b[26:28]) & 0x3fff), int(binary.LittleEndian.Uint16(b[28:30]) & 0x3fff), nil
	}
	return 0, 0, errors.New("unknown WebP chunk")
}

// Human formats a byte count the way the store's errors do.
func Human(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d bytes", n)
}
