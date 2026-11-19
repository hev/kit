package reviewnotes

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// ReviewsDir is the directory, in a run directory beside review-notes.json,
// that holds recorded reviews: one JSON file per review, named by its ID.
const ReviewsDir = "reviews"

// Record is one recorded review of a run at one head: the reviewer's findings
// in the agent-context format, or none for a clean review. Records are
// written by hev review record and never rewritten except to mark delivery.
type Record struct {
	ID string `json:"id"`
	// Reviewer is who reviewed: the orchestrator (o-NAME) when one recorded
	// it, else the name the caller gave.
	Reviewer string    `json:"reviewer"`
	Time     time.Time `json:"time"`
	// Head is the commit reviewed. Notes.Head always equals it.
	Head string `json:"head"`
	// Verdict is derived from the findings: clean (none), fail (any finding
	// tagged fail) or note (findings, none tagged fail).
	Verdict  string `json:"verdict"`
	Findings int    `json:"findings"`
	Notes    Notes  `json:"notes"`
	// Sent records delivery to the run as a rework, by hev review send.
	Sent *Delivery `json:"sent,omitempty"`
}

// Delivery is when and how a record's findings reached the run.
type Delivery struct {
	Time    time.Time `json:"time"`
	Message string    `json:"message,omitempty"`
}

// Finding tags: every recorded annotation carries TagFinding and one of
// TagFail or TagNote.
const (
	TagFinding = "finding"
	TagFail    = "fail"
	TagNote    = "note"
)

// NewRecord validates findings for head and returns the record to store.
// Every annotation must be a finding: tagged finding plus fail or note, with
// a confidence. A head in the findings must agree with head. Empty author and
// source are filled with reviewer. No annotations is a clean review.
func NewRecord(id, reviewer, head string, now time.Time, findings Notes) (Record, error) {
	head = strings.TrimSpace(head)
	if !isCommit(head) {
		return Record{}, fmt.Errorf("head %q is not a commit SHA", head)
	}
	if findings.Head != "" && !SameHead(findings.Head, head) {
		return Record{}, fmt.Errorf("findings name head %s, not %s", Short(findings.Head), Short(head))
	}
	if strings.TrimSpace(reviewer) == "" {
		return Record{}, fmt.Errorf("reviewer required")
	}
	if err := findings.Validate(); err != nil {
		return Record{}, err
	}
	var problems []string
	r := Record{ID: id, Reviewer: reviewer, Time: now.UTC(), Head: head, Verdict: "clean"}
	findings.Version, findings.Head = Version, head
	if findings.Files == nil {
		findings.Files = []File{}
	}
	fail := false
	for i := range findings.Files {
		f := &findings.Files[i]
		for j := range f.Annotations {
			a := &f.Annotations[j]
			at := fmt.Sprintf("files[%d].annotations[%d]", i, j)
			kind := findingKind(a.Tags)
			if kind == "" {
				problems = append(problems, at+`.tags: want ["finding","fail"] or ["finding","note"]`)
			}
			if a.Confidence == "" {
				problems = append(problems, at+".confidence: required (low, medium or high)")
			}
			fail = fail || kind == TagFail
			if a.Author == "" {
				a.Author = reviewer
			}
			if a.Source == "" {
				a.Source = reviewer
			}
			if a.CreatedAt == "" {
				a.CreatedAt = r.Time.Format(time.RFC3339)
			}
			r.Findings++
		}
	}
	if len(problems) > 0 {
		return Record{}, &ValidationError{Problems: problems}
	}
	switch {
	case fail:
		r.Verdict = TagFail
	case r.Findings > 0:
		r.Verdict = TagNote
	}
	r.Notes = findings
	return r, nil
}

func findingKind(tags []string) string {
	finding, kind := false, ""
	for _, t := range tags {
		switch t {
		case TagFinding:
			finding = true
		case TagFail:
			kind = TagFail
		case TagNote:
			if kind == "" {
				kind = TagNote
			}
		}
	}
	if !finding {
		return ""
	}
	return kind
}

func isCommit(s string) bool {
	if len(s) < 7 || len(s) > 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

// SortRecords orders records oldest first, then by ID.
func SortRecords(rs []Record) {
	sort.SliceStable(rs, func(i, j int) bool {
		if !rs[i].Time.Equal(rs[j].Time) {
			return rs[i].Time.Before(rs[j].Time)
		}
		return rs[i].ID < rs[j].ID
	})
}

// RecordParts turns records into merge parts, one per review, named
// REVIEWER@ID so the merge can name a dropped one.
func RecordParts(rs []Record) []Part {
	var parts []Part
	for _, r := range rs {
		parts = append(parts, Part{Run: r.Reviewer + "@" + r.ID, Review: true, Head: r.Head, Notes: r.Notes})
	}
	return parts
}

// Line describes a record as one line of review history.
func (r Record) Line() string {
	what := "clean"
	if r.Findings > 0 {
		fails := 0
		for _, f := range r.Notes.Files {
			for _, a := range f.Annotations {
				if findingKind(a.Tags) == TagFail {
					fails++
				}
			}
		}
		what = fmt.Sprintf("%s (%d fail)", plural(r.Findings, "finding"), fails)
	}
	sent := "not sent"
	switch {
	case r.Sent != nil:
		sent = "sent " + r.Sent.Time.Format(time.RFC3339)
	case r.Findings == 0:
		sent = "nothing to send"
	}
	return fmt.Sprintf("%s %s by %s: %s; %s", r.ID, r.Time.Format(time.RFC3339), r.Reviewer, what, sent)
}

// History renders records grouped by head, oldest head first, with current
// marking the run's present branch head. It is empty without records.
func History(rs []Record, current string) string {
	if len(rs) == 0 {
		return ""
	}
	rs = append([]Record(nil), rs...)
	SortRecords(rs)
	var heads []string
	byHead := map[string][]Record{}
	for _, r := range rs {
		key := strings.ToLower(r.Head)
		if _, ok := byHead[key]; !ok {
			heads = append(heads, key)
		}
		byHead[key] = append(byHead[key], r)
	}
	var b strings.Builder
	b.WriteString("reviews:\n")
	for _, h := range heads {
		mark := ""
		if current != "" {
			if SameHead(h, current) {
				mark = " (current head)"
			} else {
				mark = " (earlier head)"
			}
		}
		fmt.Fprintf(&b, "  at %s%s:\n", Short(h), mark)
		for _, r := range byHead[h] {
			fmt.Fprintf(&b, "    %s\n", r.Line())
		}
	}
	return b.String()
}

// Unsent returns the records for head whose findings were never delivered,
// and how many unsent records with findings name another head.
func Unsent(rs []Record, head string) (due []Record, stale int) {
	for _, r := range rs {
		if r.Sent != nil || r.Findings == 0 {
			continue
		}
		if SameHead(r.Head, head) {
			due = append(due, r)
		} else {
			stale++
		}
	}
	SortRecords(due)
	return due, stale
}

// FormatFindings renders records' findings as the one anchored message a run
// gets as its rework: each finding is path:line with its kind, confidence,
// headline and rationale.
func FormatFindings(run, head string, rs []Record) string {
	var items []string
	var reviewers []string
	seen := map[string]bool{}
	for _, r := range rs {
		if !seen[r.Reviewer] {
			seen[r.Reviewer] = true
			reviewers = append(reviewers, r.Reviewer)
		}
		for _, f := range r.Notes.Files {
			for _, a := range f.Annotations {
				it := Item{Path: f.Path}
				rg := a.NewRange
				if len(rg) == 0 && len(a.OldRange) > 0 {
					rg, it.Old = a.OldRange, true
				}
				if len(rg) == 2 {
					it.Line, it.End = rg[0], rg[1]
				}
				kind := findingKind(a.Tags)
				text := fmt.Sprintf("[%s, %s confidence] %s", kind, a.Confidence, strings.TrimSpace(a.Summary))
				if r := strings.TrimSpace(a.Rationale); r != "" {
					text += "\n" + indentRest(r)
				}
				items = append(items, fmt.Sprintf("%s: %s", it.Anchor(), indentRest(text)))
			}
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Review findings for %s at %s from %s (%d):\n", run, Short(head), strings.Join(reviewers, ", "), len(items))
	for i, it := range items {
		fmt.Fprintf(&b, "\n%d. %s", i+1, it)
	}
	var ids []string
	for _, r := range rs {
		ids = append(ids, r.ID)
	}
	fmt.Fprintf(&b, "\n\nRecorded beside the run as %s; `hev review %s --json` shows them merged with your review-notes.json. Address each fail finding, push, and update ../review-notes.json for the new head.\n", strings.Join(ids, ", "), run)
	return b.String()
}
