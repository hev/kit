package dispatch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// A plugin that gives each status entry its own transition ID dispatches a
// returning item again, while replaying an ID stays idempotent. A stable item
// ID is one transition, which is why the contract requires transition IDs.
func TestSourceTransitionIDsDispatchEachEntry(t *testing.T) {
	s := sourceFixture(t)
	st, _ := orchestratorState(s.Name)
	fakeClock(t)
	f := newFakeTarget(t)
	for i, poll := range [][]string{
		{sourceEvent("REL-7@ready-1", "Ready"), sourceEvent("REL-8", "Ready")},
		{sourceEvent("REL-7@done-2", "Done"), sourceEvent("REL-8", "Done")},
		{sourceEvent("REL-7@ready-3", "Ready"), sourceEvent("REL-8", "Ready")},
		{sourceEvent("REL-7@ready-3", "Ready"), sourceEvent("REL-8", "Ready")},
	} {
		setEvents(t, "c"+string(rune('1'+i)), poll...)
		s.Idle = "run"
		if e := RunOrchestratorRound(context.Background(), s, &st, noEvidence()); e != nil {
			t.Fatal(e)
		}
	}
	r := receiptsByEvent(t, s)
	if len(f.starts) != 3 || r["REL-7@ready-1"].Mode != "live" || r["REL-7@ready-3"].Mode != "live" || r["REL-7@ready-1"].Run == r["REL-7@ready-3"].Run {
		t.Fatal("re-entry with a new transition ID must dispatch once more", f.starts)
	}
	if n := slices.Index(f.starts, r["REL-8"].Run); n < 0 || slices.Index(f.starts[n+1:], r["REL-8"].Run) >= 0 {
		t.Fatal("a stable item ID is one transition", f.starts)
	}
}

func TestSourceTransitionIdentityDocumented(t *testing.T) {
	for file, want := range map[string][]string{
		filepath.Join("..", "..", "docs", "rfcs", "0008-orchestrators.md"): {"identifies the item plus the transition", "stable on replay", "new ID on re-entry"},
		filepath.Join("..", "..", "docs", "orchestrators.md"):              {"transition ID"},
	} {
		b, e := os.ReadFile(file)
		if e != nil {
			t.Fatal(e)
		}
		text := strings.Join(strings.Fields(string(b)), " ")
		for _, w := range want {
			if !strings.Contains(text, w) {
				t.Errorf("%s does not document %q", file, w)
			}
		}
	}
}

// A crash after the receipt transition but before delivery or the state save
// must not lose the signal: the outbox in the receipt is replayed.
func TestSourceSignalSurvivesCrash(t *testing.T) {
	s := sourceFixture(t)
	st, _ := orchestratorState(s.Name)
	now := fakeClock(t)
	f := newFakeTarget(t)
	f.start = func(Request) error { return errors.New("refused") }
	old := IntakeSignalHook
	t.Cleanup(func() { IntakeSignalHook = old })
	IntakeSignalHook = func(OrchestratorSpec, IntakeSignal) error { panic("crash before delivery and state save") }
	func() {
		defer func() { _ = recover() }()
		_ = RunOrchestratorRound(context.Background(), s, &st, noEvidence())
		t.Fatal("hook did not run")
	}()
	st, _ = orchestratorState(s.Name)
	if len(st.Signals) != 0 {
		t.Fatal("setup: state must predate the signal")
	}
	if rows, e := ListOrchestrators(); e != nil || len(rows) != 1 || len(rows[0].Signals) != 1 {
		t.Fatal("inventory must rebuild signals from the outbox", rows, e)
	}
	// A failed delivery stays queued; a successful one is acknowledged once.
	var delivered []IntakeSignal
	IntakeSignalHook = func(_ OrchestratorSpec, sig IntakeSignal) error {
		delivered = append(delivered, sig)
		if len(delivered) == 1 {
			return errors.New("transport down")
		}
		return nil
	}
	for i := 0; i < 3; i++ {
		*now = now.Add(7 * time.Hour)
		s.Idle = "run"
		if e := RunOrchestratorRound(context.Background(), s, &st, noEvidence()); e != nil {
			t.Fatal(e)
		}
	}
	if len(delivered) != 2 || delivered[0].ID != delivered[1].ID || delivered[0].ID == "" || delivered[0].Kind != "rejected" {
		t.Fatal("signal must be redelivered with a stable ID until acknowledged", delivered)
	}
	if len(st.Signals) != 1 || st.Signals[0].ID != delivered[0].ID {
		t.Fatal("state signals must be rebuilt from the outbox", st.Signals)
	}
}

// Rejected receipts retry after 1, 2, 4 ... 64 polls, independent of the clock.
func TestSourceRetryBacksOffInPolls(t *testing.T) {
	s := sourceFixture(t)
	st, _ := orchestratorState(s.Name)
	fakeClock(t)
	f := newFakeTarget(t)
	var polls []int64
	f.start = func(Request) error {
		polls = append(polls, sourcePoll(s))
		return errors.New("refused")
	}
	for i := 0; i < 260; i++ {
		if e := RunOrchestratorRound(context.Background(), s, &st, noEvidence()); e != nil {
			t.Fatal(e)
		}
	}
	if want := []int64{1, 2, 4, 8, 16, 32, 64, 128, 192, 256}; !slices.Equal(polls, want) {
		t.Fatalf("retry polls %v, want %v", polls, want)
	}
	for attempts, want := range map[int]int64{1: 1, 2: 2, 6: 32, 7: 64, 8: 64, 40: 64} {
		if got := sourceRetryPolls(attempts); got != want {
			t.Errorf("sourceRetryPolls(%d) = %d, want %d", attempts, got, want)
		}
	}
}

func TestSourceInjectionValuesRejected(t *testing.T) {
	s := sourceFixture(t)
	rule := s.Source.Rules[0]
	for _, value := range []string{"REL-1\nhost: redirected", "REL-1\rrepo: redirected", "REL-1\nauthority: [dispatch]"} {
		event := SourceEvent{ID: value, Kind: "issue", URL: "u", Fields: map[string]any{"status": "Ready"}}
		name, _ := sourceRunName(s, rule, []SourceEvent{event})
		if _, _, e := renderSourceOrder(s, rule, []SourceEvent{event}, name); !errors.Is(e, ErrSourceUnsafeValue) {
			t.Errorf("%q: %v", value, e)
		}
	}
}
