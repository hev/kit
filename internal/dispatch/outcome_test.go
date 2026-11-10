package dispatch

import (
	"strings"
	"testing"
	"time"
)

func TestParseOutcome(t *testing.T) {
	for _, tc := range []struct{ status, outcome, key, text string }{
		{"progress\ndone\n", OutcomeDone, "", ""},
		{"done: shipped\n\n", OutcomeDone, "", "shipped"},
		{"blocked: no credentials for staging", OutcomeBlocked, "", "no credentials for staging"},
		{"- `needs-decision db: Postgres or SQLite?`", OutcomeNeedsDecision, "db", "Postgres or SQLite?"},
		{"needs-decision schema.v2: Keep column?", OutcomeNeedsDecision, "schema.v2", "Keep column?"},
		// No marker, or a malformed one, keeps today's behaviour.
		{"", "", "", ""},
		{"blocked: fixed later\nstill working", "", "", ""},
		{"blocked:", "", "", ""},
		{"needs-decision: which db?", "", "", ""},
		{"needs-decision two words: which db?", "", "", ""},
		{"needs-decision db:", "", "", ""},
		{"doneish", "", "", ""},
	} {
		outcome, key, text := ParseOutcome(tc.status)
		if outcome != tc.outcome || key != tc.key || text != tc.text {
			t.Errorf("%q: got %q %q %q", tc.status, outcome, key, text)
		}
	}
}

func TestOutcomeState(t *testing.T) {
	end := time.Now()
	for _, tc := range []struct {
		r    Result
		want string
	}{
		{Result{End: end, Outcome: OutcomeDone}, "done"},
		{Result{End: end, Outcome: OutcomeBlocked, Reason: "x"}, "blocked"},
		{Result{End: end, Outcome: OutcomeNeedsDecision, Key: "db"}, "needs-review"},
		{Result{End: end}, "done"},
		{Result{End: end, ExitCode: 1}, "failed"},
		// A marker never hides a failed, timed-out or stopped run.
		{Result{End: end, ExitCode: 1, Outcome: OutcomeNeedsDecision, Key: "db"}, "failed"},
		{Result{End: end, Timeout: true, Outcome: OutcomeBlocked}, "timeout"},
	} {
		if got := State(&tc.r, false); got != tc.want {
			t.Errorf("%+v: got %s want %s", tc.r, got, tc.want)
		}
	}
	if !VisibleByDefault(Row{State: "blocked"}) {
		t.Fatal("blocked run hidden from ls")
	}
}

func TestWorkerPromptAsksForOneFinalMarker(t *testing.T) {
	p := workerPrompt(t.TempDir(), Meta{})
	for _, marker := range []string{"`done`", "`blocked: <reason>`", "`needs-decision <KEY>: <question>`", "last line of ../status.md"} {
		if !strings.Contains(p, marker) {
			t.Fatalf("worker prompt missing %q", marker)
		}
	}
	if strings.Contains(workerPrompt("", Meta{Parent: "impl"}), "needs-decision") {
		t.Fatal("validator prompt asks for a marker")
	}
	if strings.Contains(workerPrompt("", Meta{Order: Order{Every: "1h"}}), "needs-decision") {
		t.Fatal("repeat round prompt asks for a marker")
	}
}
