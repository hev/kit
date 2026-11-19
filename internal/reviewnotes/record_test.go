package reviewnotes

import (
	"strings"
	"testing"
	"time"
)

const recHead = "0123456789abcdef0123456789abcdef01234567"

func finding(path, kind, conf string, r Range) File {
	return File{Path: path, Annotations: []Annotation{{Summary: kind + " in " + path, NewRange: r, Tags: []string{TagFinding, kind}, Confidence: conf, Rationale: "because"}}}
}

func TestNewRecordValidatesFindings(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	r, err := NewRecord("rv-1", "o-bcc", recHead, now, Notes{Files: []File{finding("a.go", TagFail, "high", Range{3, 4}), finding("b.go", TagNote, "low", Range{1, 1})}})
	if err != nil {
		t.Fatal(err)
	}
	a := r.Notes.Files[0].Annotations[0]
	if r.Verdict != TagFail || r.Findings != 2 || r.Notes.Head != recHead || r.Head != recHead || a.Author != "o-bcc" || a.Source != "o-bcc" || a.CreatedAt == "" {
		t.Fatalf("record: %+v", r)
	}
	if err := r.Notes.Validate(); err != nil {
		t.Fatal(err)
	}
	clean, err := NewRecord("rv-2", "o-bcc", recHead, now, Notes{})
	if err != nil || clean.Verdict != "clean" || clean.Findings != 0 || clean.Notes.Files == nil {
		t.Fatalf("clean: %+v %v", clean, err)
	}
	note, _ := NewRecord("rv-3", "x", recHead, now, Notes{Files: []File{finding("a.go", TagNote, "medium", nil)}})
	if note.Verdict != TagNote {
		t.Fatalf("note verdict %q", note.Verdict)
	}
	for name, c := range map[string]struct {
		head  string
		notes Notes
		want  string
	}{
		"untagged":      {recHead, Notes{Files: []File{{Path: "a", Annotations: []Annotation{{Summary: "x", Confidence: "low"}}}}}, `want ["finding","fail"]`},
		"no kind":       {recHead, Notes{Files: []File{{Path: "a", Annotations: []Annotation{{Summary: "x", Tags: []string{"finding"}, Confidence: "low"}}}}}, "tags"},
		"no confidence": {recHead, Notes{Files: []File{{Path: "a", Annotations: []Annotation{{Summary: "x", Tags: []string{"finding", "fail"}}}}}}, "confidence: required"},
		"bad head":      {"main", Notes{}, "not a commit"},
		"other head":    {recHead, Notes{Head: "fedcba9"}, "findings name head"},
		"bad range":     {recHead, Notes{Files: []File{finding("a", TagFail, "high", Range{4, 3})}}, "before start"},
	} {
		if _, err := NewRecord("rv", "x", c.head, now, c.notes); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", name, err, c.want)
		}
	}
}

func TestUnsentHistoryAndFindingsMessage(t *testing.T) {
	t0 := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	old, _ := NewRecord("rv-old", "o-bcc", "fedcba9876543210", t0, Notes{Files: []File{finding("a.go", TagFail, "high", Range{1, 1})}})
	fail, _ := NewRecord("rv-fail", "o-bcc", recHead, t0.Add(time.Hour), Notes{Files: []File{finding("a.go", TagFail, "high", Range{3, 4})}})
	clean, _ := NewRecord("rv-clean", "o-bcc", recHead, t0.Add(2*time.Hour), Notes{})
	sent, _ := NewRecord("rv-sent", "o-bcc", recHead, t0.Add(30*time.Minute), Notes{Files: []File{{Path: "z.go", Annotations: []Annotation{{Summary: "gone", OldRange: Range{7, 9}, Tags: []string{"finding", "note"}, Confidence: "low"}}}}})
	sent.Sent = &Delivery{Time: t0}
	all := []Record{clean, fail, old, sent}
	due, stale := Unsent(all, recHead[:12])
	if len(due) != 1 || due[0].ID != "rv-fail" || stale != 1 {
		t.Fatalf("unsent %+v stale %d", due, stale)
	}
	msg := FormatFindings("feat", recHead, append(due, sent))
	for _, want := range []string{"Review findings for feat at 0123456789ab from o-bcc (2):", "1. a.go:3-4: [fail, high confidence] fail in a.go\n   because", "2. z.go:7-9 (old side): [note, low confidence] gone", "rv-fail, rv-sent", "hev review feat --json"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message lacks %q:\n%s", want, msg)
		}
	}
	h := History(all, recHead)
	for _, want := range []string{"at fedcba987654 (earlier head):", "at 0123456789ab (current head):", "rv-fail 2026-11-18T13:00:00Z by o-bcc: 1 finding (1 fail); not sent", "rv-clean 2026-11-18T14:00:00Z by o-bcc: clean; nothing to send", "rv-sent", "; sent 2026-11-18T12:00:00Z"} {
		if !strings.Contains(h, want) {
			t.Errorf("history lacks %q:\n%s", want, h)
		}
	}
	if strings.Index(h, "earlier head") > strings.Index(h, "current head") {
		t.Errorf("heads out of order:\n%s", h)
	}
	if History(nil, recHead) != "" {
		t.Fatal("empty history printed")
	}
	// Recorded reviews merge as review parts; another head's are dropped by name.
	merged, dropped, _ := MergeDetail(recHead, RecordParts(all))
	if len(dropped) != 1 || dropped[0].Run != "o-bcc@rv-old" || dropped[0].Kind != "review" || len(merged.Files) != 2 {
		t.Fatalf("merge: %+v dropped %+v", merged, dropped)
	}
}
