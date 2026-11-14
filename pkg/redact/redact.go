// Package redact is kit's secret scrubber for programs outside this module.
// It is the same scrubber hevd runs before rows leave a machine (gitleaks
// rules plus kit's own, replacing each secret with a salted fingerprint), so
// a server can apply it again to rows it receives without trusting the
// sender to have done so.
package redact

import (
	"github.com/hev/kit/internal/layer"
	iredact "github.com/hev/kit/internal/redact"
	"github.com/hev/kit/internal/trace"
)

type (
	// Scrubber is immutable and safe to share across goroutines.
	Scrubber = iredact.Scrubber
	// Counts is redactions per rule ID.
	Counts = iredact.Counts
)

// Version identifies the scrubbing policy.
const Version = iredact.Version

// New is a scrubber with a salt of at least 32 bytes. The salt keys the
// fingerprints that replace secrets, so one salt per tenant keeps tenants'
// fingerprints unlinkable.
func New(salt []byte) (*Scrubber, error) { return iredact.New(salt) }

// Text scrubs one string.
func Text(s *Scrubber, text string) (string, Counts) { return s.Text(text) }

// Session scrubs a session row's free-text fields in place.
func Session(s *Scrubber, r *trace.SessionRow) Counts {
	c := Counts{}
	for _, f := range []*string{&r.Summary, &r.FirstPrompt} {
		if *f == "" {
			continue
		}
		var n Counts
		*f, n = s.Text(*f)
		c.Add(n)
	}
	return c
}

// Block scrubs a block row's text and request text in place.
func Block(s *Scrubber, r *trace.BlockRow) Counts {
	c := Counts{}
	for _, f := range []*string{&r.Text, &r.RequestText} {
		if *f == "" {
			continue
		}
		var n Counts
		*f, n = s.Text(*f)
		c.Add(n)
	}
	return c
}

// Chunk scrubs a chunk row's text in place.
func Chunk(s *Scrubber, r *layer.Row) Counts {
	if r.Text == "" {
		return Counts{}
	}
	var c Counts
	r.Text, c = s.Text(r.Text)
	return c
}
