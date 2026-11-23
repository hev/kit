//go:build !windows

package dispatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Closing a run waiting on a decision settles the decision without a
// rework, keeps the worker's record, and a later rework reopens the run.
func TestCloseNeedsDecisionWithoutRework(t *testing.T) {
	dir, prompts, _ := outcomeFixture(t, "pick-db")
	t.Setenv("FINAL_MARKER", "needs-decision db: Postgres or SQLite?")
	launched := 0
	original := runLauncher
	t.Cleanup(func() { runLauncher = original })
	runLauncher = func(string, string) error { launched++; return nil }
	if err := superviseRound("pick-db"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "report.md"), []byte("report\n"), 0600); err != nil {
		t.Fatal(err)
	}
	before := readResult(t, dir)

	r, err := Handle(Request{Action: "close", Name: "pick-db", Note: "merged by hand", Meta: Meta{Sender: "local:hev"}})
	if err != nil || r.Message != "closed pick-db: merged by hand" || !strings.Contains(r.Files["result.json"], `"closed_reason": "merged by hand"`) {
		t.Fatalf("%+v %v", r, err)
	}
	if launched != 0 {
		t.Fatal("close started a rework")
	}
	after := readResult(t, dir)
	if State(&after, false) != StateClosed || after.ClosedBy != "local:hev" || after.ClosedAt.IsZero() || after.ClosedReason != "merged by hand" {
		t.Fatalf("%+v", after)
	}
	// The worker's own record stays.
	if after.Outcome != before.Outcome || after.Key != "db" || after.Question != before.Question || after.Head != before.Head || !after.Start.Equal(before.Start) || !after.End.Equal(before.End) || after.Branch != "d/pick-db" {
		t.Fatalf("worker record changed:\nbefore %+v\nafter  %+v", before, after)
	}
	for _, f := range []string{"log", "status.md", "report.md", "wt", "order.md"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatalf("close removed %s: %v", f, err)
		}
	}
	if OpenDecision(&after) {
		t.Fatal("decision still open")
	}
	ledger := loadDecisions(dir)
	if len(ledger) != 1 || !ledger[0].Closed || ledger[0].Answer != "merged by hand" || ledger[0].ResolvedBy != "local:hev" || ledger[0].Resolved.IsZero() {
		t.Fatalf("ledger %+v", ledger)
	}
	if len(after.Decisions) != 1 || !after.Decisions[0].Closed {
		t.Fatalf("result decisions %+v", after.Decisions)
	}

	list, err := Handle(Request{Action: "list"})
	if err != nil || len(list.Rows) != 1 {
		t.Fatalf("%+v %v", list.Rows, err)
	}
	row := list.Rows[0]
	if row.State != StateClosed || row.ClosedReason != "merged by hand" || row.ClosedBy != "local:hev" || row.ClosedAt.IsZero() || VisibleByDefault(row) {
		t.Fatalf("ls row %+v", row)
	}

	if _, err := Handle(Request{Action: "say", Name: "pick-db", Note: "SQLite", Resolve: "db"}); err == nil || !strings.Contains(err.Error(), "already resolved") {
		t.Fatalf("resolve after close: %v", err)
	}
	if _, err := Handle(Request{Action: "close", Name: "pick-db"}); err == nil || !strings.Contains(err.Error(), "already closed") {
		t.Fatalf("second close: %v", err)
	}

	// A rework starts a fresh attempt; its new result reopens the run.
	if _, err := Handle(Request{Action: "rework", Name: "pick-db", Note: "Pick SQLite after all"}); err != nil || launched != 1 {
		t.Fatalf("rework: %v (%d launches)", err, launched)
	}
	t.Setenv("FINAL_MARKER", "done")
	if err := superviseRound("pick-db"); err != nil {
		t.Fatal(err)
	}
	reopened := readResult(t, dir)
	if State(&reopened, false) != "done" || reopened.Closed() {
		t.Fatalf("reopened %+v", reopened)
	}
	b, _ := os.ReadFile(prompts)
	if !strings.Contains(string(b), "Decision db (Postgres or SQLite?) was closed without an answer: merged by hand.") {
		t.Fatalf("rework prompt %s", b)
	}
}

func TestCloseBlockedRecordsSupersededBy(t *testing.T) {
	dir, _, _ := outcomeFixture(t, "stuck")
	t.Setenv("FINAL_MARKER", "blocked: needs sudo")
	if err := superviseRound("stuck"); err != nil {
		t.Fatal(err)
	}
	if _, err := Handle(Request{Action: "close", Name: "stuck", SupersededBy: "bad/host/name"}); err == nil {
		t.Fatal("malformed superseded_by accepted")
	}
	r, err := Handle(Request{Action: "close", Name: "stuck", SupersededBy: "mini/stuck-b"})
	if err != nil || r.Message != "closed stuck: superseded by mini/stuck-b" {
		t.Fatalf("%+v %v", r, err)
	}
	res := readResult(t, dir)
	if State(&res, false) != StateClosed || res.SupersededBy != "mini/stuck-b" || res.ClosedReason != "superseded by mini/stuck-b" || res.Reason != "needs sudo" || res.Outcome != OutcomeBlocked || res.ClosedBy == "" {
		t.Fatalf("%+v", res)
	}
	if _, err := os.Stat(filepath.Join(dir, "decisions.json")); !os.IsNotExist(err) {
		t.Fatal("closing a blocked run wrote a decision ledger")
	}
}

func TestCloseRefusesRunningAndUnknown(t *testing.T) {
	dir, _, _ := outcomeFixture(t, "busy")
	if err := WriteJSON(filepath.Join(dir, "result.json"), Result{Start: time.Now().UTC(), Branch: "d/busy"}); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "tmux"), []byte("#!/bin/sh\necho d-busy\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if _, err := Handle(Request{Action: "close", Name: "busy", Note: "x"}); err == nil || !strings.Contains(err.Error(), "stop running order before close") {
		t.Fatalf("running: %v", err)
	}
	if res := readResult(t, dir); res.Closed() {
		t.Fatal("running run closed")
	}
	if _, err := Handle(Request{Action: "close", Name: "nowhere", Note: "x"}); err == nil || !strings.Contains(err.Error(), "order not found") {
		t.Fatalf("unknown: %v", err)
	}
}

// A run whose supervisor died (no end) can still be closed.
func TestCloseDiedRun(t *testing.T) {
	dir, _, _ := outcomeFixture(t, "gone")
	if err := WriteJSON(filepath.Join(dir, "result.json"), Result{Start: time.Now().UTC(), Branch: "d/gone"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Handle(Request{Action: "close", Name: "gone", Note: "picked up by hand"}); err != nil {
		t.Fatal(err)
	}
	if res := readResult(t, dir); State(&res, false) != StateClosed {
		t.Fatalf("%+v", res)
	}
}

func TestTeammateCloseIsOwnRunOnly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := filepath.Join(Root(), "theirs")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := WriteJSON(filepath.Join(dir, "meta.json"), Meta{Sender: "teammate", Order: Order{Name: "theirs"}}); err != nil {
		t.Fatal(err)
	}
	q := Request{Action: "close", Name: "theirs", Meta: Meta{Sender: "someone-else"}}
	if err := authorizeSender(&q, "teammate"); err != nil || q.Meta.Sender != "teammate" {
		t.Fatalf("%v %+v", err, q.Meta)
	}
	q = Request{Action: "close", Name: "theirs"}
	if err := authorizeSender(&q, "other"); err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("%v", err)
	}
}
