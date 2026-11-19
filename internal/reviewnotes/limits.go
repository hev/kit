package reviewnotes

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Size caps on the agent review path (docs/review.md, "Size caps"). A
// review that needs more than these is not one an agent can act on, and
// every recorded byte is copied into show, --json and the worker's inbox.
const (
	// MaxFindingsBytes caps one recorded review: the findings file hev
	// review record reads, and the notes the host stores.
	MaxFindingsBytes = 1 << 20
	// MaxFindings caps the annotations in one recorded review.
	MaxFindings = 200
	// MaxNotesBytes caps a run's review-notes.json as hev review reads it;
	// a larger file is ignored with a warning, as an invalid one is.
	MaxNotesBytes = 4 << 20
	// MaxMessageBytes caps the anchored message hev review send delivers.
	// Findings past it are left out of the message, counted and named.
	MaxMessageBytes = 64 << 10
	// MaxFindingTextBytes caps one finding's text in that message.
	MaxFindingTextBytes = 4 << 10
)

// CheckSize refuses findings over MaxFindings annotations or
// MaxFindingsBytes encoded.
func CheckSize(n Notes) error {
	count := 0
	for _, f := range n.Files {
		count += len(f.Annotations)
	}
	if count > MaxFindings {
		return fmt.Errorf("%d findings is over the cap of %d per recorded review: record the ones that matter most", count, MaxFindings)
	}
	b, err := json.Marshal(n)
	if err != nil {
		return err
	}
	if len(b) > MaxFindingsBytes {
		return fmt.Errorf("findings are %d bytes, over the cap of %d per recorded review: shorten the rationales", len(b), MaxFindingsBytes)
	}
	return nil
}

// clip shortens s to at most max bytes on a rune boundary, marking the cut.
func clip(s string, max int) string {
	if len(s) <= max {
		return s
	}
	const mark = " […truncated]"
	cut := max - len(mark)
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return strings.TrimRight(s[:cut], " \n") + mark
}
