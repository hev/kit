//go:build !windows

package dispatch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/attention"
)

// An orchestrator that repeats the same [needs you] decision every round
// raises it once: one notice, one hook run, one Slack post. A changed
// question raises again; a non-decision [needs you] reply raises no
// needs-you at all (a done-kind record, not posted by default).
func TestOrchestratorRepeatEveryRoundNeedsYouOnce(t *testing.T) {
	orchHome(t)
	slack := &fakeSlack{}
	slackHost(t, slack, "")
	bin, out := t.TempDir(), filepath.Join(t.TempDir(), "hooks.log")
	if err := os.WriteFile(filepath.Join(bin, "hev-hook-record"), []byte("#!/bin/sh\necho \"$1\" >> '"+out+"'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	// Only the recording handler and system tools: never the developer's
	// installed hev-hook-* handlers.
	t.Setenv("PATH", bin+string(os.PathListSeparator)+"/usr/bin:/bin")
	t.Setenv("HEV_HOOKS", "")
	s, err := ApplyOrchestrator(strings.Replace(orchestratorExample, "idle: skip", "idle: run", 1), false)
	if err != nil {
		t.Fatal(err)
	}
	st, _ := orchestratorState(s.Name)
	reply := "[needs you] needs-decision db: Postgres or SQLite for kit-x?\nDetails follow."
	d := noticeDriver(t, nil)
	d.Session = func(_ context.Context, _ OrchestratorSpec, dir string, _ OrchestratorPacket) error {
		if err := os.WriteFile(filepath.Join(dir, "reply.md"), []byte(reply), 0600); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, "checkpoint.md"), []byte("Waiting on the owner."), 0600)
	}
	rounds := func(n int) {
		t.Helper()
		for i := 0; i < n; i++ {
			if err := RunOrchestratorRound(context.Background(), s, &st, d); err != nil {
				t.Fatal(err)
			}
		}
	}
	hookRuns := func() int {
		b, _ := os.ReadFile(out)
		return strings.Count(string(b), "needs-you\n")
	}
	rounds(4)
	if st.Rounds != 4 {
		t.Fatalf("rounds %d", st.Rounds)
	}
	if slack.count() != 1 || hookRuns() != 1 || len(attention.ReadStatus(OrchestratorDir(s.Name))) != 1 {
		t.Fatalf("an unchanged decision every round: posts=%d hooks=%d records=%d", slack.count(), hookRuns(), len(attention.ReadStatus(OrchestratorDir(s.Name))))
	}
	// Reflowed whitespace is the same question.
	reply = "[needs you] needs-decision db:   Postgres or SQLite   for kit-x?"
	rounds(1)
	if slack.count() != 1 || hookRuns() != 1 {
		t.Fatalf("reflowed question raised again: posts=%d hooks=%d", slack.count(), hookRuns())
	}
	// A changed question raises again, once.
	reply = "[needs you] needs-decision db: Postgres or MySQL for kit-x?"
	rounds(2)
	if slack.count() != 2 || hookRuns() != 2 {
		t.Fatalf("a changed question: posts=%d hooks=%d", slack.count(), hookRuns())
	}
	if text := slack.posts[1]["text"].(string); !strings.HasPrefix(text, "[needs you] "+s.Name+" on test-host: needs-decision db: Postgres or MySQL") {
		t.Fatalf("text %q", text)
	}
	// A [needs you] about a failed run is not a decision: no needs-you.
	reply = "[needs you] kit-x failed (exit 1); retry?"
	rounds(2)
	if slack.count() != 2 || hookRuns() != 2 {
		t.Fatalf("a non-decision reply raised needs-you: posts=%d hooks=%d", slack.count(), hookRuns())
	}
	b, _ := os.ReadFile(out)
	if !strings.Contains(string(b), "done\n") {
		t.Fatalf("the non-decision reply is not on the done channel: %q", b)
	}
}

// A rework resets a one-shot run's start, but not its lineage: the same open
// decision raised again after a real rework (through Handle) is the same
// notice, with no new hook run or Slack post, and stays so across a second
// rework. A changed question after a rework raises again.
func TestUnchangedDecisionAcrossRealRework(t *testing.T) {
	const name = "rework-dedupe"
	dir, _, _ := outcomeFixture(t, name)
	slack := &fakeSlack{}
	slackHost(t, slack, "")
	old := runLauncher
	runLauncher = func(string, string) error { return nil }
	t.Cleanup(func() { runLauncher = old })
	meta := func() Meta {
		t.Helper()
		var m Meta
		if err := readJSON(filepath.Join(dir, "meta.json"), &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	round := func(marker string) {
		t.Helper()
		t.Setenv("FINAL_MARKER", marker)
		if err := superviseRound(name); err != nil {
			t.Fatal(err)
		}
	}
	rework := func() {
		t.Helper()
		if _, err := Handle(Request{Action: "rework", Name: name, Note: "Keep examining the open decision."}); err != nil {
			t.Fatal(err)
		}
	}
	first := meta()
	round("needs-decision db: Postgres or SQLite?")
	if slack.count() != 1 {
		t.Fatalf("first question posts %d", slack.count())
	}
	for i := 1; i <= 2; i++ {
		rework()
		m := meta()
		if m.Start.Equal(first.Start) || m.Lineage != first.Start.UTC().Format(time.RFC3339Nano) {
			t.Fatalf("rework %d: start %s lineage %q, first start %s", i, m.Start, m.Lineage, first.Start)
		}
		round("needs-decision db: Postgres   or SQLite?")
		if slack.count() != 1 || len(attention.ReadStatus(dir)) != 1 {
			t.Fatalf("rework %d: unchanged question raised again: posts=%d records=%+v", i, slack.count(), attention.ReadStatus(dir))
		}
	}
	rework()
	round("needs-decision db: Postgres or MySQL?")
	if slack.count() != 2 {
		t.Fatalf("a changed question after rework: posts=%d", slack.count())
	}
}

// An amend's new attempt keeps the lineage the same way, and a later
// attempt never moves it.
func TestNewAttemptKeepsLineage(t *testing.T) {
	start := time.Date(2026, 11, 17, 1, 0, 0, 0, time.UTC)
	m := Meta{Order: Order{Name: "kit-x", Host: "oven"}, Start: start}
	ask := func(m Meta) string {
		n, _ := runAttention(m, Result{End: m.Start.Add(time.Hour), Outcome: OutcomeNeedsDecision, Key: "db", Question: "choose"})
		return n.ID
	}
	want := ask(m)
	for i := 0; i < 2; i++ {
		newAttempt(&m)
		if m.Start.Equal(start) || m.Lineage != start.Format(time.RFC3339Nano) || ask(m) != want {
			t.Fatalf("attempt %d: %+v id %s want %s", i, m, ask(m), want)
		}
	}
}

// Reapplying an orchestrator starts a new lifetime but keeps its first
// start as the subject scope: the same open decision after a reapply (twice)
// is not raised again; a changed one is.
func TestOrchestratorDecisionAcrossReapply(t *testing.T) {
	orchHome(t)
	slack := &fakeSlack{}
	slackHost(t, slack, "")
	spec := strings.Replace(orchestratorExample, "idle: skip", "idle: run", 1)
	s, err := ApplyOrchestrator(spec, false)
	if err != nil {
		t.Fatal(err)
	}
	reply := "[needs you] needs-decision db: Postgres or SQLite for kit-x?"
	d := noticeDriver(t, nil)
	d.Session = func(_ context.Context, _ OrchestratorSpec, dir string, _ OrchestratorPacket) error {
		if err := os.WriteFile(filepath.Join(dir, "reply.md"), []byte(reply), 0600); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, "checkpoint.md"), []byte("Waiting on the owner."), 0600)
	}
	round := func() {
		t.Helper()
		st, _ := orchestratorState(s.Name)
		if err := RunOrchestratorRound(context.Background(), s, &st, d); err != nil {
			t.Fatal(err)
		}
	}
	round()
	first, _ := orchestratorState(s.Name)
	if slack.count() != 1 {
		t.Fatalf("first decision posts %d", slack.count())
	}
	for i := 1; i <= 2; i++ {
		if _, err := ApplyOrchestrator(spec, false); err != nil {
			t.Fatal(err)
		}
		st, _ := orchestratorState(s.Name)
		if st.Start.Equal(first.Start) || st.FirstStart != first.Start.Format(time.RFC3339Nano) {
			t.Fatalf("reapply %d: start %s first_start %q, first %s", i, st.Start, st.FirstStart, first.Start)
		}
		round()
		if slack.count() != 1 {
			t.Fatalf("reapply %d: unchanged decision raised again: posts=%d", i, slack.count())
		}
	}
	reply = "[needs you] needs-decision db: Postgres or MySQL for kit-x?"
	round()
	if slack.count() != 2 {
		t.Fatalf("a changed decision after reapply: posts=%d", slack.count())
	}
}
