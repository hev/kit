package trace

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/hev/kit/internal/blob"
)

// Image is one picture a block carried. A parser decodes the bytes and
// describes them; CaptureImages stores them in the host's blob store and
// drops them from memory. Everything here is derived from the bytes, never
// from what the transcript claims they are.
type Image struct {
	// Index is the image's position among the block's content parts: in a
	// tool result whose content is [text, image, text] the image is 1. An
	// image that stood alone in a message becomes its own block, at 0.
	Index  int    `json:"index"`
	SHA256 string `json:"sha256,omitempty"`
	Type   string `json:"type,omitempty"`
	Size   int64  `json:"size"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
	// Skipped says why the bytes were not stored: over the size cap, not an
	// accepted image, not inline in the transcript, or capture switched off.
	// Empty means the bytes are in the capturing host's blob store.
	Skipped string `json:"skipped,omitempty"`

	data []byte
}

// Images is a block's pictures. On Layer it travels as a JSON string
// attribute, as skill loads do, so it reads back from either form.
type Images []Image

func (l *Images) UnmarshalJSON(raw []byte) error {
	b, err := unquoted(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, (*[]Image)(l))
}

// MarkerPrefix opens every image marker, so a reader of block text (and a
// word search for "image") can find where a picture was.
const MarkerPrefix = "[image"

// hashPrefix is how much of the sha256 a marker shows: enough to tell images
// apart and to find the blob, short enough to read.
const hashPrefix = 12

// Marker is the short stable text that stands where the image was.
//
//	[image image/png 390x700 7.5 KB sha256:3f2a9c1b04de]
//	[image image/png 390x700 7.5 KB sha256:3f2a9c1b04de skipped: image capture is off]
//	[image skipped: not an accepted image: the bytes are text/plain; ...]
func (im Image) Marker() string {
	var b strings.Builder
	b.WriteString(MarkerPrefix)
	if im.SHA256 != "" {
		fmt.Fprintf(&b, " %s %dx%d %s sha256:%s", im.Type, im.Width, im.Height, blob.Human(im.Size), im.SHA256[:min(hashPrefix, len(im.SHA256))])
	}
	if im.Skipped != "" {
		b.WriteString(" skipped: ")
		b.WriteString(oneLine(im.Skipped))
	}
	b.WriteString("]")
	return b.String()
}

// Stored reports whether the capturing host kept the bytes.
func (im Image) Stored() bool { return im.Skipped == "" && im.SHA256 != "" }

// Skip reasons that are not blob store errors.
const (
	SkipCaptureOff = "image capture is off"
	skipNotInline  = "the transcript holds a reference, not the image bytes"
	skipBadBase64  = "the payload is not valid base64"
)

// decodeImage describes inline image bytes for block index i. It applies
// the default size cap, which CaptureImages re-applies with the store's own.
func decodeImage(i int, b []byte) Image {
	im := Image{Index: i, Size: int64(len(b))}
	m, err := blob.Inspect(b)
	if err != nil {
		im.Skipped = err.Error()
		return im
	}
	im.SHA256, im.Type, im.Width, im.Height = m.SHA256, m.Type, m.Width, m.Height
	if im.Size > blob.DefaultMaxBytes {
		im.Skipped = fmt.Sprintf("%v: %s is over the %s limit", blob.ErrTooLarge, blob.Human(im.Size), blob.Human(blob.DefaultMaxBytes))
		return im
	}
	im.data = b
	return im
}

// base64Image decodes a bare base64 payload (Claude's source.data, Pi's
// data). Transcripts use standard padding; unpadded input is accepted too.
func base64Image(i int, payload string) Image {
	payload = strings.TrimSpace(payload)
	b, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		if b, err = base64.RawStdEncoding.DecodeString(strings.TrimRight(payload, "=")); err != nil {
			return Image{Index: i, Size: int64(base64.StdEncoding.DecodedLen(len(payload))), Skipped: skipBadBase64}
		}
	}
	return decodeImage(i, b)
}

// dataURLImage decodes "data:<type>;base64,<payload>". Anything else (an
// http URL, a file path, a store reference) is recorded as not inline.
func dataURLImage(i int, url string) Image {
	rest, ok := strings.CutPrefix(strings.TrimSpace(url), "data:")
	if !ok {
		return Image{Index: i, Skipped: skipNotInline}
	}
	header, payload, ok := strings.Cut(rest, ",")
	if !ok || !strings.HasSuffix(strings.ToLower(header), ";base64") {
		return Image{Index: i, Skipped: skipBadBase64}
	}
	return base64Image(i, payload)
}

// fileImage reads an attachment that a harness keeps as a file part with a
// media type and a URL or inline data (OpenCode). Only image/* parts are
// images; anything else stays a file the caller names and omits.
func fileImage(i int, mime, data, url string) (Image, bool) {
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(mime)), "image/") {
		return Image{}, false
	}
	if data != "" {
		return base64Image(i, data), true
	}
	return dataURLImage(i, url), true
}

// imageBlock is an image that stood alone in a message: a text block whose
// text is its marker.
func imageBlock(im Image) Block {
	return Block{Type: "text", Text: im.Marker(), Images: []Image{im}}
}

// imageOnly reports a block that is nothing but one image's marker, which is
// not something a person typed and so is never the session's first prompt.
func (b Block) imageOnly() bool {
	return len(b.Images) == 1 && b.Text == b.Images[0].Marker()
}

// ImageCapture is where a host keeps the images it sees, and whether it
// keeps them at all. A nil *ImageCapture stores nothing and changes nothing.
type ImageCapture struct {
	Store *blob.Store
	// Off writes markers and metadata but stores no bytes.
	Off bool
	// Unavailable, when set, is why this host has nowhere to keep image
	// bytes (a Layer namespace that cannot be a directory name). Each image
	// is skipped with this reason as its marker; nothing fails.
	Unavailable string
}

// ImageStats counts one capture pass.
type ImageStats struct {
	Seen    int // images in the transcript, stored or not
	Stored  int // kept in the blob store (new or already there)
	Created int // newly written blobs
	Skipped int // over the cap, not an image, not inline, or capture off
}

// Add folds another pass's counts into s.
func (s *ImageStats) Add(o ImageStats) {
	s.Seen += o.Seen
	s.Stored += o.Stored
	s.Created += o.Created
	s.Skipped += o.Skipped
}

// CaptureImages stores the bytes each block carries, records what was kept
// and rewrites each marker if the outcome changed it (capture off, or a cap
// smaller than the default). It runs before redaction and releases the bytes
// either way, so nothing downstream ever holds pixels. Storing is idempotent:
// the same bytes are one blob however often they are seen.
func CaptureImages(turns []Turn, c *ImageCapture) (ImageStats, error) {
	var st ImageStats
	for ti := range turns {
		for bi := range turns[ti].Blocks {
			b := &turns[ti].Blocks[bi]
			for ii := range b.Images {
				im := &b.Images[ii]
				st.Seen++
				if im.data == nil {
					if im.Skipped != "" {
						st.Skipped++
					}
					continue
				}
				before := im.Marker()
				data := im.data
				im.data = nil
				switch {
				case c == nil || (c.Store == nil && !c.Off && c.Unavailable == ""):
					continue
				case c.Off:
					im.Skipped = SkipCaptureOff
				case c.Unavailable != "":
					im.Skipped = c.Unavailable
				default:
					if _, created, err := c.Store.Put(data); err != nil {
						if !errors.Is(err, blob.ErrTooLarge) && !errors.Is(err, blob.ErrUnsupported) {
							return st, fmt.Errorf("store image %s: %w", im.SHA256, err)
						}
						im.Skipped = err.Error()
					} else if created {
						st.Created++
					}
				}
				if im.Skipped != "" {
					st.Skipped++
				} else {
					st.Stored++
				}
				if after := im.Marker(); after != before {
					b.Text = strings.Replace(b.Text, before, after, 1)
				}
			}
		}
	}
	return st, nil
}

// joinParts renders a block's content parts in order, an image's marker
// standing where the image was. Empty text parts are dropped as before.
func joinParts(parts []string) string {
	var kept []string
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, "\n")
}
