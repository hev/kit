package dispatch

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// closed takes precedence over every ended state, never over a live run.
func TestStateClosedPrecedence(t *testing.T) {
	closedAt := time.Now().UTC()
	end := closedAt.Add(-time.Minute)
	for name, r := range map[string]Result{
		"halted":       {End: end, Halted: true},
		"needs-review": {End: end, NeedsReview: true},
		"decision":     {End: end, Outcome: OutcomeNeedsDecision, Key: "db"},
		"blocked":      {End: end, Outcome: OutcomeBlocked, Reason: "sudo"},
		"timeout":      {End: end, Timeout: true, ExitCode: 137},
		"failed":       {End: end, ExitCode: 1},
		"done":         {End: end},
		"died":         {},
	} {
		open := r
		if State(&open, false) == StateClosed {
			t.Fatalf("%s: closed without a close", name)
		}
		r.ClosedAt = closedAt
		if got := State(&r, false); got != StateClosed {
			t.Fatalf("%s: %s", name, got)
		}
		if got := State(&r, true); got != "running" {
			t.Fatalf("%s live: %s", name, got)
		}
		if OpenDecision(&r) {
			t.Fatalf("%s: closed run has an open decision", name)
		}
	}
	if State(nil, false) != "died" {
		t.Fatal("nil result")
	}
}

// Older result.json files have no closed fields and keep their state; a
// result without a close writes none.
func TestResultClosedFieldsCompatible(t *testing.T) {
	var r Result
	if err := json.Unmarshal([]byte(`{"exit_code":0,"start":"2026-01-01T00:00:00Z","end":"2026-01-01T00:01:00Z","branch":"d/x","head_sha":"abc","timeout":false,"outcome":"blocked","reason":"sudo"}`), &r); err != nil {
		t.Fatal(err)
	}
	if r.Closed() || State(&r, false) != "blocked" {
		t.Fatalf("%+v", r)
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"closed_at", "closed_by", "closed_reason", "superseded_by"} {
		if strings.Contains(string(b), key) {
			t.Fatalf("open result writes %s: %s", key, b)
		}
	}
}

func TestSplitRunAddress(t *testing.T) {
	for in, want := range map[string][2]string{"old": {"", "old"}, "oven/old": {"oven", "old"}, " mini/old-b ": {"mini", "old-b"}} {
		h, n, err := SplitRunAddress(in)
		if err != nil || h != want[0] || n != want[1] {
			t.Fatalf("%q: %q %q %v", in, h, n, err)
		}
	}
	for _, bad := range []string{"", "/old", "a/b/c", "oven/", "oven/o-orch", "bad name"} {
		if _, _, err := SplitRunAddress(bad); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
}

func TestTakeSupersedesKey(t *testing.T) {
	order := "---\nname: new\nsupersedes: oven/old # replaced\nrepo: app\n---\nBody\nsupersedes: not frontmatter\n"
	text, ref, err := TakeSupersedesKey(order)
	if err != nil || ref != "oven/old" || strings.Contains(text, "supersedes: oven/old") || !strings.Contains(text, "supersedes: not frontmatter") {
		t.Fatalf("%q %q %v", text, ref, err)
	}
	if _, err := Parse(text + "## Acceptance\n- x\n"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := TakeSupersedesKey("---\nname: new\nsupersedes: a/b/c\n---\n"); err == nil {
		t.Fatal("malformed supersedes accepted")
	}
	if _, _, err := TakeSupersedesKey("---\nsupersedes: a\nsupersedes: b\n---\n"); err == nil || !strings.Contains(err.Error(), "duplicate key: supersedes") {
		t.Fatalf("duplicate: %v", err)
	}
	// TakeTraceKey keeps its behavior on the shared helper.
	if _, ref, err := TakeTraceKey("---\ntrace: 0d98d96d1f5acba898a8b922b80f0a92\n---\n"); err != nil || ref != "0d98d96d1f5acba898a8b922b80f0a92" {
		t.Fatalf("%q %v", ref, err)
	}
}

// The closed disposition survives a publish and rebuild of trace facts.
func TestTraceFactsCarryClose(t *testing.T) {
	start := time.Now().UTC().Add(-time.Hour).Truncate(time.Millisecond)
	closedAt := start.Add(30 * time.Minute)
	m := Meta{Order: Order{Name: "old", Repo: "app"}, Start: start, TraceID: "0d98d96d1f5acba898a8b922b80f0a92", SpanID: "f153bb771c2703d7"}
	rec := RunRecord{Name: "old", Meta: m, History: History{Attempts: []Attempt{{Number: 1, Meta: m}}},
		Results: []Result{{Start: start, End: start.Add(time.Minute), Outcome: OutcomeBlocked, Reason: "sudo", SpanID: m.SpanID, ClosedAt: closedAt, SupersededBy: "mini/new"}}}
	rows := TraceSpanRows([]RunRecord{rec}, "oven", nil)
	var found bool
	for _, row := range rows {
		if row.Kind == "attempt" {
			found = true
			if row.Closed != closedAt.UnixMilli() || row.SupersededBy != "mini/new" {
				t.Fatalf("%+v", row)
			}
		}
	}
	if !found {
		t.Fatal("no attempt row")
	}
	back := RecordsFromSpanRows(rows)
	if len(back) != 1 || len(back[0].Results) != 1 {
		t.Fatalf("%+v", back)
	}
	if r := back[0].Results[0]; State(&r, false) != StateClosed || r.SupersededBy != "mini/new" || !r.ClosedAt.Equal(closedAt) {
		t.Fatalf("%+v", r)
	}
}
