package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/reviewnotes"
)

func testRecord(t *testing.T, id, head string, anns ...reviewnotes.Annotation) reviewnotes.Record {
	t.Helper()
	r, err := reviewnotes.NewRecord(id, "o-bcc", head, time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC), reviewnotes.Notes{Files: []reviewnotes.File{{Path: "a.go", Annotations: anns}}})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

var recordedFail = reviewnotes.Annotation{Summary: "recorded bug", NewRange: reviewnotes.Range{2, 2}, Tags: []string{"finding", "fail"}, Confidence: "high"}

// hev review RUN --json is the agent path: it fetches the exact head and
// prints the worker's notes merged with review runs' and recorded reviews'
// findings for that head, naming what it dropped, plus the review history.
func TestReviewJSONMergesRecordedReviews(t *testing.T) {
	f := newFakeReview(t)
	f.records = []reviewnotes.Record{testRecord(t, "rv-now", testHead, recordedFail), testRecord(t, "rv-old", "4444444444444444", recordedFail)}
	out, _, err := execReview(t, f, "feat", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var ac struct {
		reviewnotes.Notes
		Run, Ref, Repo, MergeBase string
		Dropped                   []reviewnotes.Dropped
		Reviews                   []reviewnotes.Record
		ReviewRuns                []string `json:"review_runs"`
	}
	if err := json.Unmarshal([]byte(out), &ac); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	// hunk loads the same document.
	if _, err := reviewnotes.Parse([]byte(out)); err != nil {
		t.Fatal(err)
	}
	var summaries []string
	for _, a := range ac.Files[0].Annotations {
		summaries = append(summaries, a.Summary+"/"+a.Author)
	}
	if want := "why/feat opus,bug/feat-review sonnet,recorded bug/o-bcc"; strings.Join(summaries, ",") != want || ac.Head != testHead {
		t.Fatalf("merged %v, want %s", summaries, want)
	}
	var dropped []string
	for _, d := range ac.Dropped {
		dropped = append(dropped, d.Run)
	}
	if strings.Join(dropped, ",") != "kit-review-feat,o-bcc@rv-old" {
		t.Fatalf("dropped %v", dropped)
	}
	if len(ac.Reviews) != 2 || ac.Run != "feat" || ac.Ref != "refs/hev/review/feat" || ac.Repo != "/work/kit" || strings.Join(ac.ReviewRuns, ",") != "feat-review,kit-review-feat" {
		t.Fatalf("context: %+v", ac)
	}
	if len(f.ran) != 0 {
		t.Fatalf("--json ran %v", f.ran)
	}
	// It fetched the branch (update-ref at the head) rather than trusting a cache.
	fetched := false
	for _, q := range f.requests {
		fetched = fetched || q.Action == "review-fetch"
	}
	if !fetched {
		t.Fatal("no review-fetch")
	}
	if _, _, err := execReview(t, f, "feat", "--json", "--print-notes"); err == nil {
		t.Fatal("--json with --print-notes accepted")
	}
}

// hev review (the TUI path) prints the history per head and merges records.
func TestReviewPrintsRecordedHistory(t *testing.T) {
	f := newFakeReview(t)
	f.records = []reviewnotes.Record{testRecord(t, "rv-now", testHead, recordedFail), testRecord(t, "rv-clean", testHead)}
	_, errout, err := execReview(t, f, "feat")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"reviews:", "(current head)", "rv-clean", "clean; nothing to send", "rv-now", "1 finding (1 fail); not sent"} {
		if !strings.Contains(errout, want) {
			t.Errorf("stderr lacks %q:\n%s", want, errout)
		}
	}
}

func TestReviewSendPrefersRecordedFindings(t *testing.T) {
	f := newFakeReview(t)
	f.hunk, f.due = true, 1
	out, _, err := execReview(t, f, "send", "feat", "--dry-run")
	if err != nil || !strings.Contains(out, "dry run: would send 1 recorded review") || len(f.sent) != 0 {
		t.Fatalf("dry run: %q %v %v", out, f.sent, err)
	}
	out, _, err = execReview(t, f, "send", "feat")
	if err != nil || len(f.sent) != 1 || f.sent[0] != "recorded" || !strings.Contains(out, "sent feat") {
		t.Fatalf("send: %q %v %v", out, f.sent, err)
	}
	for _, r := range f.ran {
		if len(r) > 1 && r[1] == "hunk" {
			t.Fatalf("recorded send read hunk: %v", f.ran)
		}
	}
	// Nothing recorded and --recorded: say so, never fall back to hunk.
	f.ran = nil
	out, _, err = execReview(t, f, "send", "feat", "--recorded")
	if err != nil || !strings.Contains(out, "nothing to send") || len(f.ran) != 0 {
		t.Fatalf("--recorded with nothing due: %q %v %v", out, f.ran, err)
	}
	if _, _, err = execReview(t, f, "send", "feat", "--recorded", "--hunk"); err == nil {
		t.Fatal("--recorded --hunk accepted")
	}
	// --hunk skips recorded findings even when some are due.
	f.due = 1
	f.requests = nil
	_, _, _ = execReview(t, f, "send", "feat", "--hunk")
	for _, q := range f.requests {
		if q.Action == "review-send" {
			t.Fatal("--hunk asked for recorded findings")
		}
	}
}

func TestReviewRecordCommand(t *testing.T) {
	f := newFakeReview(t)
	path := filepath.Join(t.TempDir(), "findings.json")
	doc := `{"version":1,"head":"` + testHead + `","files":[{"path":"a.go","annotations":[{"summary":"bug","newRange":[2,2],"tags":["finding","fail"],"confidence":"high"}]}]}`
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _, err := execReview(t, f, "record", "feat", "--head", testHead, "--file", path, "--reviewer", "me")
	if err != nil || !strings.Contains(out, "recorded review") {
		t.Fatalf("%q %v", out, err)
	}
	q := f.requests[len(f.requests)-1]
	if q.Action != "review-record" || q.Name != "feat" || q.ReviewRecord.Head != testHead || q.ReviewRecord.Reviewer != "me" || q.ReviewRecord.Notes.Files[0].Annotations[0].Confidence != "high" {
		t.Fatalf("request %+v", q)
	}
	if _, _, err = execReview(t, f, "record", "feat", "--head", testHead, "--clean"); err != nil {
		t.Fatal(err)
	}
	q = f.requests[len(f.requests)-1]
	if q.ReviewRecord == nil || len(q.ReviewRecord.Notes.Files) != 0 || q.ReviewRecord.Head != testHead {
		t.Fatalf("clean request %+v", q.ReviewRecord)
	}
	for _, args := range [][]string{
		{"record", "feat", "--clean"},
		{"record", "feat", "--head", testHead},
		{"record", "feat", "--head", testHead, "--clean", "--file", path},
	} {
		if _, _, err := execReview(t, f, args...); err == nil {
			t.Errorf("%v accepted", args)
		}
	}
}
