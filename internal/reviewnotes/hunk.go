package reviewnotes

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// HunkComment is one note from `hunk session comment list --json` (hunk 0.25).
// Source is "user" for notes the reviewer wrote in the hunk window, "agent"
// for live comments added through `hunk session comment add`, and "ai" for
// sidecar annotations. Replies carry ParentID.
type HunkComment struct {
	NoteID    string `json:"noteId"`
	ParentID  string `json:"parentId,omitempty"`
	Source    string `json:"source"`
	FilePath  string `json:"filePath"`
	HunkIndex *int   `json:"hunkIndex,omitempty"`
	NewRange  Range  `json:"newRange,omitempty"`
	OldRange  Range  `json:"oldRange,omitempty"`
	Body      string `json:"body"`
	Author    string `json:"author,omitempty"`
	CreatedAt string `json:"createdAt,omitempty"`
}

// Item is one anchored piece of reviewer feedback.
type Item struct {
	Path string
	// Line is the first line of the note's range; Old marks the old side.
	Line, End int
	Old       bool
	Text      string
	// ReplyTo is the note this replies to, when it is a reply.
	ReplyTo *HunkComment
	ID      string
	Created string
}

// Anchor renders path:line, path:line-end, or the old-side equivalent.
func (it Item) Anchor() string {
	a := it.Path
	if it.Line > 0 {
		a += fmt.Sprintf(":%d", it.Line)
		if it.End > it.Line {
			a += fmt.Sprintf("-%d", it.End)
		}
		if it.Old {
			a += " (old side)"
		}
	}
	return a
}

// ParseHunkComments reads hunk's comment-list JSON and returns the reviewer's
// notes as anchored items: every note with source "user", including replies
// to agent or sidecar notes. includeLive also takes "agent" notes, which is
// what `hunk session comment add` creates when a person or script adds notes
// from a shell. Sidecar ("ai") notes are never feedback. Items are ordered by
// path, line and creation time.
func ParseHunkComments(b []byte, includeLive bool) ([]Item, error) {
	var doc struct {
		Comments []HunkComment `json:"comments"`
		Error    any           `json:"error"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("hunk comment list: %w", err)
	}
	if doc.Error != nil {
		e, _ := json.Marshal(doc.Error)
		return nil, fmt.Errorf("hunk comment list: %s", e)
	}
	byID := map[string]*HunkComment{}
	for i := range doc.Comments {
		byID[doc.Comments[i].NoteID] = &doc.Comments[i]
	}
	var items []Item
	for _, c := range doc.Comments {
		if c.Source != "user" && !(includeLive && c.Source == "agent") {
			continue
		}
		text := strings.TrimSpace(c.Body)
		if text == "" {
			continue
		}
		it := Item{Path: c.FilePath, Text: text, ID: c.NoteID, Created: c.CreatedAt}
		r := c.NewRange
		if len(r) == 0 && len(c.OldRange) > 0 {
			r, it.Old = c.OldRange, true
		}
		if len(r) > 0 {
			it.Line = r[0]
			if len(r) > 1 {
				it.End = r[1]
			}
		}
		if c.ParentID != "" {
			if p, ok := byID[c.ParentID]; ok {
				parent := *p
				it.ReplyTo = &parent
			} else {
				it.ReplyTo = &HunkComment{NoteID: c.ParentID}
			}
		}
		items = append(items, it)
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Created < b.Created
	})
	return items, nil
}

// FormatItems renders items as one message for a run.
func FormatItems(run, head string, items []Item) string {
	var b strings.Builder
	at := ""
	if head != "" {
		at = " at " + Short(head)
	}
	fmt.Fprintf(&b, "Review notes for %s%s from hunk (%d):\n", run, at, len(items))
	for i, it := range items {
		fmt.Fprintf(&b, "\n%d. %s", i+1, it.Anchor())
		if it.ReplyTo != nil {
			who := it.ReplyTo.Author
			if who == "" {
				who = it.ReplyTo.Source
			}
			if it.ReplyTo.Body != "" {
				fmt.Fprintf(&b, " (reply to %s note %q)", who, oneLine(it.ReplyTo.Body))
			} else {
				fmt.Fprintf(&b, " (reply to note %s)", it.ReplyTo.NoteID)
			}
		}
		fmt.Fprintf(&b, ": %s", indentRest(it.Text))
	}
	b.WriteString("\n")
	return b.String()
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 80 {
		s = s[:77] + "..."
	}
	return s
}

func indentRest(s string) string { return strings.ReplaceAll(s, "\n", "\n   ") }

var verdictLine = regexp.MustCompile(`^VERDICT:\s*(\S+)(?:.*\bhead=([0-9a-fA-F]{7,40})\b)?`)

// ReportVerdict reads the `VERDICT: PASS|FAIL head=SHA` first line a review
// run writes in report.md. It returns empty strings when the first non-empty
// line is not a verdict.
func ReportVerdict(report string) (verdict, head string) {
	for _, line := range strings.Split(report, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		m := verdictLine.FindStringSubmatch(line)
		if m == nil {
			return "", ""
		}
		return m[1], m[2]
	}
	return "", ""
}
