//go:build !windows

package dispatch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
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
