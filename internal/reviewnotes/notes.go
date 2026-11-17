// Package reviewnotes owns review-notes.json, the anchored review notes a
// dispatched run writes beside report.md. The format is hunk's agent-context
// sidecar, version 1 (https://hunk.dev/docs/agents/agent-context-and-stml/),
// so the file loads into `hunk diff --agent-context` unchanged. Kit owns and
// validates the file whether or not hunk is installed.
//
// Kit adds one optional top-level field, head: the commit SHA the notes
// describe. hunk ignores unknown top-level keys.
package reviewnotes

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// FileName is the artifact's name in a run directory, beside report.md.
const FileName = "review-notes.json"

// Version is the only sidecar version hunk and kit understand.
const Version = 1

// Notes is one review-notes.json document.
type Notes struct {
	Version int    `json:"version,omitempty"`
	Summary string `json:"summary,omitempty"`
	// Head is kit's extension: the commit the notes describe. Review runs set
	// it to the SHA they reviewed so stale findings can be dropped.
	Head  string `json:"head,omitempty"`
	Files []File `json:"files"`
}

// File describes one changed file. Path is the file's current path, or its
// previous path for a rename.
type File struct {
	Path        string       `json:"path"`
	Summary     string       `json:"summary,omitempty"`
	Annotations []Annotation `json:"annotations,omitempty"`
}

// Annotation is one note pinned to a line range.
type Annotation struct {
	Summary    string   `json:"summary"`
	NewRange   Range    `json:"newRange,omitempty"`
	OldRange   Range    `json:"oldRange,omitempty"`
	Rationale  string   `json:"rationale,omitempty"`
	Author     string   `json:"author,omitempty"`
	Markup     string   `json:"markup,omitempty"`
	ID         string   `json:"id,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	Confidence string   `json:"confidence,omitempty"`
	Source     string   `json:"source,omitempty"`
	CreatedAt  string   `json:"createdAt,omitempty"`
}

// Range is a 1-based inclusive [start, end] line range. Decoding rejects
// non-integer elements; Validate checks the shape.
type Range []int

// Start is the range's first line, or 0 when absent.
func (r Range) Start() int {
	if len(r) == 0 {
		return 0
	}
	return r[0]
}

// Parse decodes and validates a document. Unknown fields are allowed, as hunk
// allows them. An empty or whitespace-only input is an error.
func Parse(b []byte) (Notes, error) {
	var n Notes
	if len(bytes.TrimSpace(b)) == 0 {
		return n, errors.New("empty review notes")
	}
	if err := json.Unmarshal(b, &n); err != nil {
		return n, fmt.Errorf("review notes: %w", err)
	}
	return n, n.Validate()
}

// Load reads and parses path. A missing file returns an error satisfying
// errors.Is(err, os.ErrNotExist).
func Load(path string) (Notes, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Notes{}, err
	}
	return Parse(b)
}

// Validate applies hunk's sidecar rules: version 1, a non-empty path on every
// file, a summary on every annotation, and ranges that are two positive line
// numbers with end >= start. It also checks confidence against hunk's
// documented values. All problems are reported together.
func (n Notes) Validate() error {
	var problems []string
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }
	if n.Version != 0 && n.Version != Version {
		add("version: unsupported %d (want %d)", n.Version, Version)
	}
	for i, f := range n.Files {
		if strings.TrimSpace(f.Path) == "" {
			add("files[%d].path: required", i)
		}
		for j, a := range f.Annotations {
			at := fmt.Sprintf("files[%d].annotations[%d]", i, j)
			if strings.TrimSpace(a.Summary) == "" {
				add("%s.summary: required", at)
			}
			for _, r := range []struct {
				name string
				r    Range
			}{{"newRange", a.NewRange}, {"oldRange", a.OldRange}} {
				if r.r == nil {
					continue
				}
				if err := checkRange(r.r); err != nil {
					add("%s.%s: %v", at, r.name, err)
				}
			}
			switch a.Confidence {
			case "", "low", "medium", "high":
			default:
				add("%s.confidence: %q is not low, medium or high", at, a.Confidence)
			}
		}
	}
	if len(problems) == 0 {
		return nil
	}
	return &ValidationError{Problems: problems}
}

func checkRange(r Range) error {
	if len(r) != 2 {
		return fmt.Errorf("want [start, end], got %d numbers", len(r))
	}
	if r[0] < 1 || r[1] < 1 {
		return errors.New("lines are 1-based")
	}
	if r[1] < r[0] {
		return fmt.Errorf("end %d is before start %d", r[1], r[0])
	}
	return nil
}

// ValidationError lists every rule a document breaks.
type ValidationError struct{ Problems []string }

func (e *ValidationError) Error() string {
	return "invalid review notes: " + strings.Join(e.Problems, "; ")
}

// Marshal renders a document as indented JSON with a trailing newline.
func Marshal(n Notes) ([]byte, error) {
	if n.Version == 0 {
		n.Version = Version
	}
	if n.Files == nil {
		n.Files = []File{}
	}
	b, err := json.MarshalIndent(n, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// Status summarizes a run's notes for `hev dispatch show`.
type Status struct {
	Path        string `json:"path"`
	State       string `json:"state"` // missing, valid or invalid
	Error       string `json:"error,omitempty"`
	Head        string `json:"head,omitempty"`
	Files       int    `json:"files,omitempty"`
	Annotations int    `json:"annotations,omitempty"`
}

// Inspect reports the state of the notes at path. It never fails: a missing
// or invalid file is a state, not an error, so it never fails a run.
func Inspect(path string) Status {
	s := Status{Path: path}
	n, err := Load(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		s.State = "missing"
	case err != nil:
		s.State, s.Error = "invalid", err.Error()
	default:
		s.State, s.Head, s.Files = "valid", n.Head, len(n.Files)
		for _, f := range n.Files {
			s.Annotations += len(f.Annotations)
		}
	}
	return s
}

// Line describes s as one human line.
func (s Status) Line() string {
	switch s.State {
	case "valid":
		head := ""
		if s.Head != "" {
			head = " at " + Short(s.Head)
		}
		return fmt.Sprintf("review notes: %s (%s, %s%s)", s.Path, plural(s.Files, "file"), plural(s.Annotations, "annotation"), head)
	case "invalid":
		return fmt.Sprintf("review notes: %s is invalid: %s", s.Path, s.Error)
	default:
		return fmt.Sprintf("review notes: none (%s missing)", s.Path)
	}
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// Short abbreviates a commit SHA for display.
func Short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}
