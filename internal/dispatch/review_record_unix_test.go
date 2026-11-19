//go:build !windows

package dispatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/reviewnotes"
	"github.com/hev/kit/internal/steer"
)

// liveRun makes the fake tmux report run name as a live session, so say
// reaches its inbox instead of reworking it.
func liveRun(t *testing.T, name string) {
	t.Helper()
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if p := filepath.Join(dir, "tmux"); exists(p) {
			if err := os.WriteFile(p, []byte("#!/bin/sh\nfor a; do [ \"$a\" = list-sessions ] && echo d-"+name+"; done\nexit 0\n"), 0700); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatal("no fake tmux on PATH")
}

// endedRun makes the fake tmux report no sessions, so the run has ended and
// a send reworks it, and gives it an order to append the rework to.
func endedRun(t *testing.T, dir string) {
	t.Helper()
	for _, d := range filepath.SplitList(os.Getenv("PATH")) {
		if p := filepath.Join(d, "tmux"); exists(p) {
			if err := os.WriteFile(p, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "order.md"), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
}

// reworkText is what the ended run's order.md gained as its rework.
func reworkText(t *testing.T, dir string) string {
	t.Helper()
	b, _ := os.ReadFile(filepath.Join(dir, "order.md"))
	return string(b)
}

// assertNothingDelivered checks a refused send wrote nothing to the run and
// marked no record sent.
func assertNothingDelivered(t *testing.T, dir string) {
	t.Helper()
	if exists(filepath.Join(dir, "inbox.md")) {
		t.Fatal("refused send wrote the inbox")
	}
	if msgs, _ := steer.RunStore(Root(), filepath.Base(dir)).List(); len(msgs) != 0 {
		t.Fatalf("refused send queued steer notes: %+v", msgs)
	}
	recs, _ := loadReviewRecords(dir)
	for _, rec := range recs {
		if rec.Sent != nil {
			t.Fatalf("refused send marked %s sent", rec.ID)
		}
	}
}

func findings(anns ...reviewnotes.Annotation) reviewnotes.Notes {
	return reviewnotes.Notes{Summary: "review", Files: []reviewnotes.File{{Path: "a.txt", Annotations: anns}}}
}

var (
	failFinding = reviewnotes.Annotation{Summary: "two is wrong", NewRange: reviewnotes.Range{2, 2}, Rationale: "the order said three", Tags: []string{"finding", "fail"}, Confidence: "high"}
	noteFinding = reviewnotes.Annotation{Summary: "consider a comment", NewRange: reviewnotes.Range{1, 1}, Tags: []string{"finding", "note"}, Confidence: "medium"}
)

func record(t *testing.T, q Request, head string, n reviewnotes.Notes) (Response, error) {
	t.Helper()
	q.Action, q.Name = "review-record", "example"
	q.ReviewRecord = &ReviewRecordRequest{Head: head, Reviewer: "tester", Notes: n}
	return Handle(q)
}

// Record writes findings with tags, confidence and head; a clean review is
// the head and no findings; both survive, show lists them per head, and send
// delivers only the current head's unsent findings, once.
func TestReviewRecordShowAndSend(t *testing.T) {
	dir, _, base, head := reviewFixture(t)
	r, err := record(t, Request{}, head[:12], findings(failFinding, noteFinding))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.Message, "fail, 2 findings") {
		t.Fatalf("record message %q", r.Message)
	}
	if _, err = record(t, Request{}, head, reviewnotes.Notes{}); err != nil {
		t.Fatal(err)
	}
	// Findings for an earlier head are history, never sent for this one.
	if _, err = record(t, Request{}, base, findings(failFinding)); err != nil {
		t.Fatal(err)
	}
	for name, bad := range map[string]reviewnotes.Notes{
		"untagged":      findings(reviewnotes.Annotation{Summary: "x", Confidence: "low"}),
		"no confidence": findings(reviewnotes.Annotation{Summary: "x", Tags: []string{"finding", "fail"}}),
	} {
		if _, err := record(t, Request{}, head, bad); err == nil {
			t.Errorf("%s finding recorded", name)
		}
	}
	if _, err := record(t, Request{}, "abcdef0123456", reviewnotes.Notes{}); err == nil || !strings.Contains(err.Error(), "not a commit") {
		t.Fatalf("unknown head recorded: %v", err)
	}

	// Durable: a fresh show reads the records from disk.
	entries, _ := os.ReadDir(filepath.Join(dir, "reviews"))
	var files int
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			files++
		}
	}
	if files != 3 {
		t.Fatalf("%d record files", files)
	}
	r, err = Handle(Request{Action: "show", Name: "example"})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Reviews) != 3 {
		t.Fatalf("show reviews: %+v", r.Reviews)
	}
	var fail, clean *reviewnotes.Record
	for i, rec := range r.Reviews {
		switch {
		case rec.Head == head && rec.Verdict == "fail":
			fail = &r.Reviews[i]
		case rec.Head == head && rec.Verdict == "clean":
			clean = &r.Reviews[i]
		}
	}
	if fail == nil || clean == nil {
		t.Fatalf("records: %+v", r.Reviews)
	}
	a := fail.Notes.Files[0].Annotations
	if fail.Notes.Head != head || fail.Findings != 2 || a[0].Confidence != "high" || strings.Join(a[0].Tags, ",") != "finding,fail" || a[1].Confidence != "medium" || a[0].Author != "tester" || a[0].Source != "tester" {
		t.Fatalf("fail record: %+v", fail)
	}
	if clean.Findings != 0 || clean.Notes.Head != head || clean.Sent != nil {
		t.Fatalf("clean record: %+v", clean)
	}
	h := reviewnotes.History(r.Reviews, head)
	if !strings.Contains(h, "at "+reviewnotes.Short(head)+" (current head):") || !strings.Contains(h, "at "+reviewnotes.Short(base)+" (earlier head):") || !strings.Contains(h, "clean; nothing to send") {
		t.Fatalf("history:\n%s", h)
	}

	// A dry run shows the message and marks nothing.
	r, err = Handle(Request{Action: "review-send", Name: "example", DryRun: true})
	if err != nil || r.ReviewDue != 1 || !strings.Contains(r.Message, "a.txt:2: [fail, high confidence] two is wrong") {
		t.Fatalf("dry run: %+v %v", r, err)
	}
	if exists(filepath.Join(dir, "inbox.md")) {
		t.Fatal("dry run delivered")
	}

	// A running run refuses the send the way it refuses hev dispatch
	// rework: nothing reaches it and nothing is marked sent.
	liveRun(t, "example")
	if _, err = Handle(Request{Action: "review-send", Name: "example"}); err == nil || !strings.Contains(err.Error(), "stop running order before rework: example is still running") || !strings.Contains(err.Error(), "nothing marked sent") {
		t.Fatalf("live send: %v", err)
	}
	assertNothingDelivered(t, dir)
	if r, err = Handle(Request{Action: "review-send", Name: "example", DryRun: true}); err != nil || !strings.Contains(r.Message, "example is running: a real send is refused until it ends") {
		t.Fatalf("live dry run: %+v %v", r, err)
	}

	// Once it has ended, the same records are sent as its rework.
	endedRun(t, dir)
	r, err = Handle(Request{Action: "review-send", Name: "example"})
	if err != nil {
		t.Fatal(err)
	}
	inbox := reworkText(t, dir)
	for _, want := range []string{"## Rework", "Review findings for example at " + reviewnotes.Short(head), "1. a.txt:2: [fail, high confidence] two is wrong\n   the order said three", "2. a.txt:1: [note, medium confidence] consider a comment"} {
		if !strings.Contains(inbox, want) {
			t.Errorf("rework lacks %q:\n%s", want, inbox)
		}
	}
	if strings.Contains(inbox, reviewnotes.Short(base)) {
		t.Errorf("an earlier head's findings were sent:\n%s", inbox)
	}
	if r.ReviewDue != 1 || !strings.Contains(r.Message, "1 unsent for earlier heads") {
		t.Fatalf("send: %+v", r)
	}
	r, _ = Handle(Request{Action: "show", Name: "example"})
	for _, rec := range r.Reviews {
		if rec.Verdict == "fail" && rec.Head == head && rec.Sent == nil {
			t.Fatal("sent record not marked")
		}
	}
	r, err = Handle(Request{Action: "review-send", Name: "example"})
	if err != nil || r.ReviewDue != 0 || !strings.Contains(r.Message, "nothing to send") {
		t.Fatalf("second send: %+v %v", r, err)
	}
}

// An orchestrator records and sends under the rework authority; one with only
// say can read the branch but neither record nor send.
func TestReviewRecordSendNeedRework(t *testing.T) {
	orchHome(t)
	dir, _, _, head := reviewFixture(t)
	m := Meta{Order: Order{Name: "example", Base: "main", Model: "opus", Repo: "app", Issue: "REL-1"}, Branch: "d/example"}
	var old Meta
	_ = readJSON(filepath.Join(dir, "meta.json"), &old)
	m.ResolvedRepo = old.ResolvedRepo
	if err := WriteJSON(filepath.Join(dir, "meta.json"), m); err != nil {
		t.Fatal(err)
	}
	s := applyOrch(t)
	reader := strings.Replace(strings.Replace(orchestratorExample, "name: release-train", "name: reader", 1), "authority: [say, rework, retry, validate, dispatch]", "authority: [say, notify]", 1)
	if _, err := ApplyOrchestrator(reader, false); err != nil {
		t.Fatal(err)
	}
	endedRun(t, dir)

	if _, err := Handle(Request{Action: "review-fetch", Name: "example", Orchestrator: "reader"}); err != nil {
		t.Fatalf("read-only fetch refused: %v", err)
	}
	if _, err := record(t, Request{Orchestrator: "reader"}, head, findings(failFinding)); err == nil || !strings.Contains(err.Error(), "lacks authority rework") {
		t.Fatalf("record without rework: %v", err)
	}
	if exists(filepath.Join(dir, "reviews")) {
		t.Fatal("refused record wrote state")
	}

	r, err := record(t, Request{Orchestrator: s.Name}, head, findings(failFinding))
	if err != nil {
		t.Fatal(err)
	}
	if r.Reviews[0].Reviewer != "o-"+s.Name {
		t.Fatalf("reviewer %q, want the orchestrator", r.Reviews[0].Reviewer)
	}
	if _, err := Handle(Request{Action: "review-send", Name: "example", Orchestrator: "reader"}); err == nil || !strings.Contains(err.Error(), "lacks authority rework") {
		t.Fatalf("send without rework: %v", err)
	}
	assertNothingDelivered(t, dir)
	if strings.Contains(reworkText(t, dir), "Review findings") {
		t.Fatal("refused send reworked")
	}
	if _, err := Handle(Request{Action: "review-send", Name: "example", Orchestrator: s.Name}); err != nil {
		t.Fatal(err)
	}
	rework := reworkText(t, dir)
	if !strings.Contains(rework, "Review findings for example") || !strings.Contains(rework, "from o-"+s.Name) {
		t.Fatalf("order.md:\n%s", rework)
	}
}

// Rework 1, finding 1: an orchestrator with rework and no say records and
// sends. The say that send delivers through is admitted as the review-send it
// is; it grants no say of its own.
func TestReviewSendNeedsOnlyRework(t *testing.T) {
	orchHome(t)
	dir, _, _, head := reviewFixture(t)
	m := Meta{Order: Order{Name: "example", Base: "main", Model: "opus", Repo: "app", Issue: "REL-1"}, Branch: "d/example"}
	var old Meta
	_ = readJSON(filepath.Join(dir, "meta.json"), &old)
	m.ResolvedRepo = old.ResolvedRepo
	if err := WriteJSON(filepath.Join(dir, "meta.json"), m); err != nil {
		t.Fatal(err)
	}
	spec := strings.Replace(strings.Replace(orchestratorExample, "name: release-train", "name: rwonly", 1), "authority: [say, rework, retry, validate, dispatch]", "authority: [rework]", 1)
	if !strings.Contains(spec, "authority: [rework]") {
		t.Fatal("fixture spec did not change")
	}
	if _, err := ApplyOrchestrator(spec, false); err != nil {
		t.Fatal(err)
	}
	if _, err := record(t, Request{Orchestrator: "rwonly"}, head, findings(failFinding)); err != nil {
		t.Fatal(err)
	}
	// Rework 2: rework authority does not reach a running run's inbox
	// either; the send is refused there as rework is.
	liveRun(t, "example")
	if _, err := Handle(Request{Action: "review-send", Name: "example", Orchestrator: "rwonly"}); err == nil || !strings.Contains(err.Error(), "stop running order before rework") {
		t.Fatalf("live send with rework only: %v", err)
	}
	assertNothingDelivered(t, dir)
	endedRun(t, dir)
	r, err := Handle(Request{Action: "review-send", Name: "example", Orchestrator: "rwonly"})
	if err != nil {
		t.Fatalf("send with rework only: %v", err)
	}
	if rework := reworkText(t, dir); r.ReviewDue != 1 || !strings.Contains(rework, "from o-rwonly") {
		t.Fatalf("send: %+v\norder.md:\n%s", r, rework)
	}
	// A plain say still needs say.
	if _, err := Handle(Request{Action: "say", Name: "example", Note: "hi", Orchestrator: "rwonly"}); err == nil || !strings.Contains(err.Error(), "lacks authority say") {
		t.Fatalf("plain say without say authority: %v", err)
	}
}

// Rework 1, finding 2: a head the run's branch never had is refused; an
// earlier head of the branch is recorded and named as earlier.
func TestReviewRecordRefusesHeadOffBranch(t *testing.T) {
	dir, repo, base, head := reviewFixture(t)
	other := git(t, repo, "rev-parse", "main") // main moved on past base
	_, err := record(t, Request{}, other, findings(failFinding))
	if err == nil || !strings.Contains(err.Error(), "is not on example's branch d/example") {
		t.Fatalf("off-branch head: %v", err)
	}
	if entries, _ := os.ReadDir(filepath.Join(dir, "reviews")); len(entries) != 0 {
		t.Fatalf("refused record wrote %v", entries)
	}
	r, err := record(t, Request{}, base, findings(failFinding))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.Message, "an earlier head of d/example; the branch head is now "+reviewnotes.Short(head)) {
		t.Fatalf("earlier head message %q", r.Message)
	}
	if r, err = record(t, Request{}, head, reviewnotes.Notes{}); err != nil || strings.Contains(r.Message, "earlier head") {
		t.Fatalf("current head: %q %v", r.Message, err)
	}
}

// Rework 1, finding 3: the host refuses a recorded review over the caps,
// whatever client sent it.
func TestReviewRecordRefusesOversize(t *testing.T) {
	dir, _, _, head := reviewFixture(t)
	many := make([]reviewnotes.Annotation, reviewnotes.MaxFindings+1)
	for i := range many {
		many[i] = failFinding
	}
	if _, err := record(t, Request{}, head, findings(many...)); err == nil || !strings.Contains(err.Error(), "over the cap of 200") {
		t.Fatalf("too many findings: %v", err)
	}
	big := failFinding
	big.Rationale = strings.Repeat("x", reviewnotes.MaxFindingsBytes)
	if _, err := record(t, Request{}, head, findings(big)); err == nil || !strings.Contains(err.Error(), "bytes, over the cap") {
		t.Fatalf("oversized findings: %v", err)
	}
	if entries, _ := os.ReadDir(filepath.Join(dir, "reviews")); len(entries) != 0 {
		t.Fatalf("refused records wrote %v", entries)
	}
}

// Rework 1, note 8: a dry-run send previews without spending budget, so it
// never loads the orchestrator for a reservation.
func TestReviewSendDryRunReservesNothing(t *testing.T) {
	orchHome(t)
	release, err := reserveOrchestratorOwner(Request{Action: "review-send", Name: "example", DryRun: true, Orchestrator: "no-such-orchestrator"}, Host{Local: true})
	if err != nil {
		t.Fatalf("dry run reserved: %v", err)
	}
	release()
	if _, err := reserveOrchestratorOwner(Request{Action: "review-send", Name: "example", Orchestrator: "no-such-orchestrator"}, Host{Local: true}); err == nil {
		t.Fatal("a real send skipped the reservation")
	}
}

// Rework 1, note 7: show reports the branch head for the review history,
// not result.json's, which is stale during a rework.
func TestShowReportsReviewBranchHead(t *testing.T) {
	dir, _, _, head := reviewFixture(t)
	if err := WriteJSON(filepath.Join(dir, "result.json"), map[string]string{"head_sha": "1111111111111111111111111111111111111111"}); err != nil {
		t.Fatal(err)
	}
	r, _ := Handle(Request{Action: "show", Name: "example"})
	if r.ReviewHead != "" {
		t.Fatalf("review head without records: %q", r.ReviewHead)
	}
	if _, err := record(t, Request{}, head, reviewnotes.Notes{}); err != nil {
		t.Fatal(err)
	}
	if r, _ = Handle(Request{Action: "show", Name: "example"}); r.ReviewHead != head {
		t.Fatalf("review head %q, want %s", r.ReviewHead, head)
	}
}
