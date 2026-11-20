package notify_test

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/notify"
	"github.com/hev/kit/internal/redact"
)

var update = flag.Bool("update", false, "rewrite golden files")

func scrubber(t *testing.T) notify.Scrub {
	t.Helper()
	s, err := redact.New([]byte(strings.Repeat("k", 32)))
	if err != nil {
		t.Fatal(err)
	}
	return func(text string) (string, map[string]int) { out, c := s.Text(text); return out, c }
}

// templateEvents has one fixture per event, using the RFC's example names.
func templateEvents() map[string]notify.Event {
	next := time.Date(2026, 11, 15, 14, 20, 0, 0, time.UTC)
	return map[string]notify.Event{
		notify.EventStarted:       {Kind: notify.EventStarted, Name: "release-train", Host: "build-1", Goal: "Every PR labelled release-0.9 is merged with green CI.", Mode: "loop", Every: "20m", Until: "goal"},
		notify.EventRound:         {Kind: notify.EventRound, Name: "release-train", Round: 4, Managed: 3, Live: 1, Decisions: 1, Blocked: 0, NextWake: next},
		notify.EventNeedsDecision: {Kind: notify.EventNeedsDecision, Run: "rel-42", Key: "db", Question: "Postgres or SQLite for the cache?"},
		notify.EventResolved:      {Kind: notify.EventResolved, Run: "rel-42", Key: "db", ResolvedBy: "local:owner"},
		notify.EventBlocked:       {Kind: notify.EventBlocked, Run: "rel-43", Reason: "CI secret missing for the deploy job"},
		notify.EventDone:          {Kind: notify.EventDone, Run: "rel-42", PR: "https://github.com/example/app/pull/42"},
		notify.EventFailed:        {Kind: notify.EventFailed, Run: "rel-44", State: "timeout", Exit: 124},
		notify.EventVerdict:       {Kind: notify.EventVerdict, Run: "rel-42", Verdict: "pass"},
		notify.EventRoundFailures: {Kind: notify.EventRoundFailures, Name: "release-train", Failures: 2},
		notify.EventBudget:        {Kind: notify.EventBudget, Name: "release-train", Cap: "runs_per_day"},
		notify.EventHandoff:       {Kind: notify.EventHandoff, Name: "release-train", To: "next-owner"},
		notify.EventComplete:      {Kind: notify.EventComplete, Name: "release-train", Rounds: 9},
		notify.EventStopped:       {Kind: notify.EventStopped, Name: "release-train"},
		notify.EventPaused:        {Kind: notify.EventPaused, Name: "release-train"},
		notify.EventResumed:       {Kind: notify.EventResumed, Name: "release-train"},
		notify.EventSourceCap:     {Kind: notify.EventSourceCap, Name: "release-train", Signal: "source cap reached"},
		notify.EventIntake:        {Kind: notify.EventIntake, Name: "release-train", Signal: "fetch-failed", IntakeEvent: "evt-17"},
	}
}

func render(t *testing.T, e notify.Event, external bool) string {
	m, err := notify.Compose(e, external, scrubber(t))
	if err == notify.ErrNotAvailable {
		return "(not posted)"
	}
	if err != nil {
		t.Fatal(err)
	}
	out := m.Text
	if len(m.Withheld) > 0 {
		out += "\n  withheld: " + strings.Join(m.Withheld, ",")
	}
	return out
}

func TestTemplateGoldens(t *testing.T) {
	events := templateEvents()
	if len(events) != len(notify.Events) {
		t.Fatalf("fixture per event: %d of %d", len(events), len(notify.Events))
	}
	for _, name := range notify.Events {
		t.Run(name, func(t *testing.T) {
			e := events[name]
			got := fmt.Sprintf("internal: %s\nexternal: %s\n", render(t, e, false), render(t, e, true))
			path := filepath.Join("testdata", "templates", name+".golden")
			if *update {
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(got), 0600); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if strings.ReplaceAll(string(want), "\r\n", "\n") != got {
				t.Fatalf("golden mismatch\nwant:\n%s\ngot:\n%s", want, got)
			}
		})
	}
}

func TestDefaultEvents(t *testing.T) {
	for _, e := range []string{"started", "needs-decision", "resolved", "blocked", "done", "failed", "verdict", "round-failures", "budget", "handoff", "complete", "stopped"} {
		if !notify.Enabled(nil, e) || !notify.Enabled([]string{"default"}, e) {
			t.Fatal(e)
		}
	}
	for _, e := range []string{"round", "paused", "resumed", "source-cap", "intake"} {
		if notify.Enabled(nil, e) || !notify.Enabled([]string{"all"}, e) || !notify.Enabled([]string{"default", e}, e) {
			t.Fatal(e)
		}
	}
	if notify.Enabled([]string{"done"}, "started") || notify.ValidateEvents([]string{"nope"}) == nil {
		t.Fatal("explicit list")
	}
}

func TestAgentFieldsCleanedScrubbedCapped(t *testing.T) {
	secret := "ghp_" + strings.Repeat("A1b2C3d4E5", 4)[:36]
	q := "Use\x00 this\n\tkey " + secret + " ?" + strings.Repeat(" long", 200)
	m, err := notify.Compose(notify.Event{Kind: notify.EventNeedsDecision, Run: "rel-42", Key: "db", Question: q}, false, scrubber(t))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(m.Text, secret) || strings.ContainsAny(m.Text, "\x00\n\t") || len(m.Redactions) == 0 {
		t.Fatalf("%q %v", m.Text, m.Redactions)
	}
	question := strings.TrimPrefix(strings.SplitN(m.Text, " Answer from", 2)[0], "rel-42 needs a decision (db): ")
	if r := []rune(question); len(r) != 281 || r[280] != '…' {
		t.Fatalf("question not capped at 280 runes then …: %d", len(r))
	}
	if len([]rune(m.Text)) > notify.MessageLimit {
		t.Fatal("message over limit")
	}
	bad, _ := notify.Compose(notify.Event{Kind: notify.EventDone, Run: "rel-42", PR: "javascript:alert(1)"}, false, nil)
	if bad.Text != "rel-42 done." {
		t.Fatal(bad.Text)
	}
	key, _ := notify.Compose(notify.Event{Kind: notify.EventResolved, Run: "rel-42", Key: "<!here>"}, false, nil)
	if strings.Contains(key.Text, "<") {
		t.Fatal(key.Text)
	}
}
