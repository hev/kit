package trace

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const (
	// MaxRunes is the chunk ceiling. Big enough that an ordinary prose turn or
	// a tool call stays whole, small enough that a hit points at something a
	// person can read rather than a screenful to scan.
	MaxRunes = 1200
	// Overlap is carried between consecutive splits of one block, which bounds
	// what reassembly has to match when stitching cited chunks back together.
	Overlap = 150
)

// Chunk is one row in the namespace: searchable text plus everything needed to
// filter it, order it, and find its way back to the turn it came from.
type Chunk struct {
	ID   string
	Text string

	SessionID  string
	TurnUUID   string
	ParentUUID string
	Seq        int64
	Block      int // position within the turn; older rows default to zero
	Part       int // which split of the block this is
	TS         string
	Role       string
	BlockType  string
	Tier       string
	ToolName   string

	Workdir     string
	Branch      string
	Harness     string
	SourcePath  string
	IsSidechain bool
}

// Chunks renders one turn's blocks into chunks.
//
// The id is a hash of the text and the coordinates that place it, which is what
// makes re-indexing idempotent: the same block chunked again produces the same
// ids and upserts over itself. Two turns that genuinely say the same thing in
// the same position collapse to one row, which is the dedup half of the same
// property. This is the content-addressing issue #1 asked for, arrived at from
// the direction that also solved chunking.
func Chunks(t Turn) []Chunk {
	var out []Chunk
	for block, b := range t.Blocks {
		tier := TierOf(b.Type)
		for part, text := range split(b.Text) {
			c := Chunk{
				Text:        text,
				SessionID:   t.SessionID,
				TurnUUID:    t.TurnUUID,
				ParentUUID:  t.ParentUUID,
				Seq:         t.Seq,
				Block:       block,
				Part:        part,
				TS:          t.TS,
				Role:        t.Role,
				BlockType:   b.Type,
				Tier:        tier.String(),
				ToolName:    b.ToolName,
				Workdir:     t.Workdir,
				Branch:      t.Branch,
				Harness:     t.Harness,
				SourcePath:  t.SourcePath,
				IsSidechain: t.IsSidechain,
			}
			c.ID = chunkID(t.SessionID, t.TurnUUID, b.Type, part, text)
			out = append(out, c)
		}
	}
	return out
}

// chunkID hashes the text together with the coordinates that place it. Text
// alone would collapse two identical tool results from different turns into one
// row and lose the second's threading; coordinates alone would not be content
// addressing at all.
func chunkID(session, turn, blockType string, part int, text string) string {
	h := sha256.New()
	for _, s := range []string{session, turn, blockType, string(rune('0' + part%10)), text} {
		h.Write([]byte(s))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:32]
}

// split cuts text into overlapping windows by rune, never by byte — a split
// that lands mid-codepoint produces a chunk that is invalid UTF-8 and a row
// turbopuffer will reject.
func split(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	r := []rune(text)
	if len(r) <= MaxRunes {
		return []string{text}
	}
	var out []string
	step := MaxRunes - Overlap
	for start := 0; start < len(r); start += step {
		end := start + MaxRunes
		if end > len(r) {
			end = len(r)
		}
		out = append(out, string(r[start:end]))
		if end == len(r) {
			break
		}
	}
	return out
}
