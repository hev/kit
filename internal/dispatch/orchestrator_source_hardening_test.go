package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// fakeTarget is an in-process dispatch target for the source runner.
type fakeTarget struct {
	t       *testing.T
	probe   func(Request) error
	start   func(Request) error
	listed  map[string]bool
	starts  []string
	listErr error
}

func newFakeTarget(t *testing.T) *fakeTarget {
	t.Helper()
	f := &fakeTarget{t: t, listed: map[string]bool{}}
	old := sourceDispatch
	t.Cleanup(func() { sourceDispatch = old })
	sourceDispatch = func(_ context.Context, _ Host, q Request) (Response, error) {
		switch {
		case q.Action == "list":
			if f.listErr != nil {
				return Response{}, f.listErr
			}
			r := Response{}
			for n := range f.listed {
				r.Rows = append(r.Rows, Row{Meta: Meta{Order: Order{Name: n}}})
			}
			return r, nil
		case q.AdmissionOnly:
			if f.probe != nil {
				return Response{}, f.probe(q)
			}
			return Response{}, nil
		}
		f.starts = append(f.starts, q.Name)
		if f.start != nil {
			if e := f.start(q); e != nil {
				return Response{}, e
			}
		}
		f.listed[q.Name] = true
		return Response{}, nil
	}
	return f
}

// setEvents replaces the fake plugin's event stream and records ack calls.
func setEvents(t *testing.T, cursor string, events ...string) *[]sourceInvocation {
	t.Helper()
	calls := &[]sourceInvocation{}
	sourceCommand = func(_ context.Context, _ OrchestratorSpec, inv sourceInvocation) ([]byte, error) {
		*calls = append(*calls, inv)
		switch inv.Args[0] {
		case "--hev-describe":
			return []byte(`{"source":true,"ack":true}`), nil
		case "events":
			return []byte(strings.Join(append(events, `{"cursor":"`+cursor+`"}`), "\n") + "\n"), nil
		case "ack":
			return nil, nil
		}
		return nil, errors.New("unexpected plugin command")
	}
	return calls
}

func sourceEvent(id, status string) string {
	b, _ := json.Marshal(SourceEvent{ID: id, Kind: "issue", URL: "https://example.com/" + id, Fields: map[string]any{"status": status}})
	return string(b)
}

func noEvidence() OrchestratorDriver {
	return OrchestratorDriver{Gather: func(context.Context, OrchestratorSpec) ([]OrchestratorEvidence, error) { return nil, nil }}
}

func receiptsByEvent(t *testing.T, s OrchestratorSpec) map[string]sourceReceipt {
	t.Helper()
	all, e := loadSourceReceipts(s)
	if e != nil {
		t.Fatal(e)
	}
	out := map[string]sourceReceipt{}
	for _, r := range all {
		out[r.Event] = r
	}
	return out
}

func cursorOf(s OrchestratorSpec) string {
	var c struct{ Cursor string }
	_ = readJSON(filepath.Join(OrchestratorDir(s.Name), "source-cursor.json"), &c)
	return c.Cursor
}

func fakeClock(t *testing.T) *time.Time {
	t.Helper()
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	old := sourceNow
	t.Cleanup(func() { sourceNow = old })
	sourceNow = func() time.Time { return now }
	return &now
}

func TestOrchestratorAdmissionProbeRunsFullChecks(t *testing.T) {
	orchHome(t)
	s := applyOrch(t)
	order := strings.NewReplacer("{{.RunName}}", "src-x", "{{.Event.ID}}", "REL-1", "{{.Action}}", "fix", "{{.Event.URL}}", "u").Replace(sourceTemplate)
	probe := Request{Action: "start", Name: "src-x", Order: strings.Replace(order, "repo: app", "repo: other", 1), Orchestrator: s.Name, AdmissionOnly: true}
	if e := admitOrchestrator(probe); e == nil || !strings.Contains(e.Error(), "outside manages scope") {
		t.Fatal("out-of-scope probe admitted", e)
	}
	text := strings.Replace(orchestratorExample, "authority: [say, rework, retry, validate, dispatch]", "authority: [say]", 1)
	if _, e := ApplyOrchestrator(text, false); e != nil {
		t.Fatal(e)
	}
	probe.Order = order
	if e := admitOrchestrator(probe); e == nil || !strings.Contains(e.Error(), "lacks authority") {
		t.Fatal("probe without dispatch authority admitted", e)
	}
	// Lifecycle probes for other verbs still need no run metadata.
	if e := admitOrchestrator(Request{Action: "show", Name: "absent", Orchestrator: s.Name, AdmissionOnly: true}); e != nil {
		t.Fatal(e)
	}
}

func TestSourceLiveRuleRequiresDispatchAuthority(t *testing.T) {
	orchHome(t)
	text := strings.Replace(orchestratorExample, "source: none", "agent: off\nsource: {plugin: fake, rules: [{status: Ready, action: fix, template: fix.md, mode: live}]}", 1)
	if _, e := ParseOrchestrator(strings.Replace(text, "authority: [say, rework, retry, validate, dispatch]", "authority: [say]", 1)); e == nil || !strings.Contains(e.Error(), "dispatch authority") {
		t.Fatal("live rule without dispatch authority accepted", e)
	}
	dry := strings.Replace(text, "mode: live", "mode: dry-run", 1)
	if _, e := ParseOrchestrator(strings.Replace(dry, "authority: [say, rework, retry, validate, dispatch]", "authority: [say]", 1)); e != nil {
		t.Fatal("dry-run rule needs no dispatch authority", e)
	}
}

func TestSourceDefinitiveRejectionNeverLeavesAttempting(t *testing.T) {
	for _, tc := range []struct {
		name         string
		probe, start error
		want         string
	}{
		{"probe authority", errors.New("orchestrator intake lacks authority dispatch"), nil, "rejected"},
		{"budget race", nil, errors.New("orchestrator runs_per_day budget exhausted"), "rejected"},
		{"remote answered", nil, fmt.Errorf("target said no"), "rejected"},
		{"launch unconfirmed", nil, errLaunchUnconfirmed, "attempting"},
		{"transport", nil, transportError{errors.New("ssh: connection lost")}, "attempting"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := sourceFixture(t)
			st, _ := orchestratorState(s.Name)
			f := newFakeTarget(t)
			f.probe = func(Request) error { return tc.probe }
			f.start = func(Request) error { return tc.start }
			e := RunOrchestratorRound(context.Background(), s, &st, noEvidence())
			r := receiptsByEvent(t, s)["REL-1"]
			if r.Mode != tc.want {
				t.Fatalf("receipt %q, want %q (round error %v)", r.Mode, tc.want, e)
			}
			if tc.want == "rejected" && (e != nil || r.Error == "" || r.RetryPoll == 0) {
				t.Fatalf("definitive rejection must not fail intake: %v %+v", e, r)
			}
			if tc.want == "attempting" && (e == nil || cursorOf(s) != "") {
				t.Fatal("uncertain dispatch advanced", e)
			}
		})
	}
}

func TestSourceAttemptPromotesWhenRunExistsAndReconcileAfterGrace(t *testing.T) {
	s := sourceFixture(t)
	st, _ := orchestratorState(s.Name)
	now := fakeClock(t)
	f := newFakeTarget(t)
	f.start = func(Request) error { return transportError{errors.New("ssh: lost")} }
	if e := RunOrchestratorRound(context.Background(), s, &st, noEvidence()); e == nil {
		t.Fatal("uncertain dispatch accepted")
	}
	// The crash window: dispatch landed, but the live write never happened.
	f.listed[receiptsByEvent(t, s)["REL-1"].Run] = true
	f.start = nil
	if e := RunOrchestratorRound(context.Background(), s, &st, noEvidence()); e != nil {
		t.Fatal(e)
	}
	if r := receiptsByEvent(t, s)["REL-1"]; r.Mode != "live" || len(f.starts) != 1 {
		t.Fatal("not promoted or relaunched", r.Mode, f.starts)
	}
	if cursorOf(s) != "next" {
		t.Fatal("promotion kept cursor")
	}

	// An absent run stays attempting within grace, then needs reconciliation.
	s2 := sourceFixture(t)
	st2, _ := orchestratorState(s2.Name)
	f2 := newFakeTarget(t)
	f2.start = func(Request) error { return errLaunchUnconfirmed }
	_ = RunOrchestratorRound(context.Background(), s2, &st2, noEvidence())
	f2.start = func(Request) error { t.Fatal("uncertain receipt relaunched"); return nil }
	s2.Idle = "run"
	if e := RunOrchestratorRound(context.Background(), s2, &st2, noEvidence()); e != nil || receiptsByEvent(t, s2)["REL-1"].Mode != "attempting" {
		t.Fatal("within grace", e, receiptsByEvent(t, s2)["REL-1"].Mode)
	}
	*now = now.Add(sourceAttemptGrace + time.Minute)
	if e := RunOrchestratorRound(context.Background(), s2, &st2, noEvidence()); e != nil {
		t.Fatal(e)
	}
	if r := receiptsByEvent(t, s2)["REL-1"]; r.Mode != "needs-reconcile" {
		t.Fatal(r.Mode)
	}
	if _, e := ReconcileSourceReceipt(s2.Name, "REL-404", true); e == nil {
		t.Fatal("reconciled unknown event")
	}
	if n, e := ReconcileSourceReceipt(s2.Name, "REL-1", false); e != nil || n != 1 {
		t.Fatal(n, e)
	}
	if r := receiptsByEvent(t, s2)["REL-1"]; r.Mode != "rejected" || !exists(filepath.Join(OrchestratorDir(s2.Name), "wake")) {
		t.Fatal("not-admitted must retry", r.Mode)
	}
	// Not admitted retries from the stored event on the next round.
	starts := len(f2.starts)
	f2.start = func(Request) error { return errLaunchUnconfirmed }
	_ = RunOrchestratorRound(context.Background(), s2, &st2, noEvidence())
	if len(f2.starts) != starts+1 || receiptsByEvent(t, s2)["REL-1"].Mode != "attempting" {
		t.Fatal("not-admitted receipt was not retried", f2.starts)
	}
	if n, e := ReconcileSourceReceipt(s2.Name, "REL-1", true); e != nil || n != 1 || receiptsByEvent(t, s2)["REL-1"].Mode != "live" {
		t.Fatal("admitted", n, e)
	}
	if _, e := ReconcileSourceReceipt(s2.Name, "REL-1", true); e == nil {
		t.Fatal("live receipt reconciled again")
	}
}

func TestSourceBadEventRejectedWithBackoffAndBatchContinues(t *testing.T) {
	s := sourceFixture(t)
	if e := os.WriteFile(filepath.Join(OrchestratorDir(s.Name), "fix.md"), []byte(strings.Replace(sourceTemplate, "repo: app", "repo: {{.Event.Fields.repo}}", 1)), 0600); e != nil {
		t.Fatal(e)
	}
	bad, _ := json.Marshal(SourceEvent{ID: "REL-1", Kind: "issue", URL: "u", Fields: map[string]any{"status": "Ready", "repo": "elsewhere"}})
	good, _ := json.Marshal(SourceEvent{ID: "REL-2", Kind: "issue", URL: "u", Fields: map[string]any{"status": "Ready", "repo": "app"}})
	missing, _ := json.Marshal(SourceEvent{ID: "REL-3", Kind: "issue", URL: "u", Fields: map[string]any{"status": "Ready"}})
	setEvents(t, "c1", string(bad), string(good), string(missing))
	st, _ := orchestratorState(s.Name)
	now := fakeClock(t)
	f := newFakeTarget(t)
	for i := 0; i < 2; i++ {
		s.Idle = "run"
		if e := RunOrchestratorRound(context.Background(), s, &st, noEvidence()); e != nil {
			t.Fatal("one bad event failed the round", e)
		}
	}
	r := receiptsByEvent(t, s)
	if r["REL-1"].Mode != "rejected" || !strings.Contains(r["REL-1"].Error, ErrSourceScope.Error()) {
		t.Fatal(r["REL-1"])
	}
	if r["REL-3"].Mode != "rejected" || r["REL-2"].Mode != "live" || len(f.starts) != 1 || cursorOf(s) != "c1" {
		t.Fatal("batch did not continue", r, f.starts)
	}
	// Rejected at poll 1, retried at poll 2, next due at poll 4.
	if r["REL-1"].Attempts != 2 || r["REL-1"].RetryPoll != 4 {
		t.Fatal("backoff", r["REL-1"].Attempts, r["REL-1"].RetryPoll)
	}
	// After backoff a fixed template admits the stored event, even off-cursor.
	setEvents(t, "c2")
	_ = os.WriteFile(filepath.Join(OrchestratorDir(s.Name), "fix.md"), []byte(sourceTemplate), 0600)
	for poll := 3; poll <= 4; poll++ {
		if e := RunOrchestratorRound(context.Background(), s, &st, noEvidence()); e != nil {
			t.Fatal(e)
		}
		if poll == 3 && len(f.starts) != 1 {
			t.Fatal("retried before backoff", f.starts)
		}
	}
	r = receiptsByEvent(t, s)
	if r["REL-1"].Mode != "live" || r["REL-3"].Mode != "live" || len(f.starts) != 3 {
		t.Fatal("rejected events not retried", r, f.starts)
	}
	// A replayed event whose status moved on is not retried under its old rule.
	_ = os.WriteFile(filepath.Join(OrchestratorDir(s.Name), "fix.md"), []byte(strings.Replace(sourceTemplate, "repo: app", "repo: {{.Event.Fields.repo}}", 1)), 0600)
	setEvents(t, "c3", `{"id":"REL-4","kind":"issue","url":"u","fields":{"status":"Ready","repo":"elsewhere"}}`)
	*now = now.Add(time.Minute)
	_ = RunOrchestratorRound(context.Background(), s, &st, noEvidence())
	if receiptsByEvent(t, s)["REL-4"].Mode != "rejected" {
		t.Fatal("setup: REL-4 must be rejected")
	}
	setEvents(t, "c4", sourceEvent("REL-4", "Done"))
	starts := len(f.starts)
	_ = os.WriteFile(filepath.Join(OrchestratorDir(s.Name), "fix.md"), []byte(sourceTemplate), 0600)
	if e := RunOrchestratorRound(context.Background(), s, &st, noEvidence()); e != nil || len(f.starts) != starts {
		t.Fatal("retried an event whose status changed", e, f.starts)
	}
}

func TestSourceCapAndPauseDeferWithoutRoundDir(t *testing.T) {
	s := sourceFixture(t)
	st, _ := orchestratorState(s.Name)
	_ = WriteJSON(filepath.Join(OrchestratorDir(s.Name), "source-cursor.json"), map[string]string{"cursor": "kept"})
	f := newFakeTarget(t)
	s.Budget.RunsPerDay = 1
	_ = WriteJSON(filepath.Join(OrchestratorDir(s.Name), "source-budget.json"), map[string]any{"day": sourceNow().UTC().Format("2006-01-02"), "runs": 1})
	e := RunOrchestratorRound(context.Background(), s, &st, noEvidence())
	if !errors.Is(e, ErrSourceDeferred) || st.State != "waiting" || st.Rounds != 0 || cursorOf(s) != "kept" || len(f.starts) != 0 {
		t.Fatal("cap", e, st.State, st.Rounds, cursorOf(s))
	}
	if exists(filepath.Join(OrchestratorDir(s.Name), "rounds")) {
		t.Fatal("deferred round created a round directory")
	}
	s.PauseFile = filepath.Join(t.TempDir(), "PAUSED")
	_ = os.WriteFile(s.PauseFile, nil, 0600)
	if e = RunOrchestratorRound(context.Background(), s, &st, noEvidence()); !errors.Is(e, ErrSourceDeferred) || st.State != "waiting" || st.Deferred == "" {
		t.Fatal("pause", e, st)
	}
	if exists(filepath.Join(OrchestratorDir(s.Name), "rounds")) || cursorOf(s) != "kept" {
		t.Fatal("paused round left state")
	}
	_ = os.Remove(s.PauseFile)
	// Dry-run events behind the cap still process; the capped live event keeps the cursor.
	s.Source.Rules = append(s.Source.Rules, SourceRule{Status: "Preview", Action: "fix", Template: "fix.md", Mode: "dry-run"})
	setEvents(t, "c2", sourceEvent("REL-1", "Ready"), sourceEvent("REL-2", "Preview"))
	if e = RunOrchestratorRound(context.Background(), s, &st, noEvidence()); e != nil {
		t.Fatal(e)
	}
	r := receiptsByEvent(t, s)
	if r["REL-2"].Mode != "dry-run" || r["REL-1"].Mode != "" || cursorOf(s) != "kept" || st.State != "waiting" || st.Deferred == "" || st.Digest != "" {
		t.Fatal("partial deferral", r, cursorOf(s), st)
	}
}

func TestSourceAckOnlyReceiptEventsWithPayload(t *testing.T) {
	s := sourceFixture(t)
	s.Manages.Runs.Selector = "issue=*"
	st, _ := orchestratorState(s.Name)
	newFakeTarget(t)
	calls := setEvents(t, "c1", sourceEvent("-flag-like", "Ready"), sourceEvent("REL-9", "Unmatched"))
	if e := RunOrchestratorRound(context.Background(), s, &st, noEvidence()); e != nil {
		t.Fatal(e)
	}
	var acks [][]string
	for _, c := range *calls {
		if c.Args[0] == "ack" {
			acks = append(acks, c.Args)
		}
	}
	run := receiptsByEvent(t, s)["-flag-like"].Run
	if len(acks) != 1 || !slices.Equal(acks[0], []string{"ack", "--run", run, "--mode", "live", "--", "-flag-like"}) {
		t.Fatal(acks)
	}
	var got []sourceInvocation
	sourceCommand = func(_ context.Context, _ OrchestratorSpec, inv sourceInvocation) ([]byte, error) {
		got = append(got, inv)
		return nil, nil
	}
	if e := ackSource(context.Background(), s, SourceBatch{Ack: true, AckBatch: true}, []sourceAck{{"a", "r1", "live"}, {"b", "r2", "dry-run"}}); e != nil {
		t.Fatal(e)
	}
	if len(got) != 1 || !slices.Equal(got[0].Args, []string{"ack", "--batch"}) || string(got[0].Stdin) != `{"id":"a","run":"r1","mode":"live"}`+"\n"+`{"id":"b","run":"r2","mode":"dry-run"}`+"\n" {
		t.Fatal(got)
	}
}

func TestSourceReceiptPrecedesBudgetCharge(t *testing.T) {
	s := sourceFixture(t)
	st, _ := orchestratorState(s.Name)
	f := newFakeTarget(t)
	budget := filepath.Join(OrchestratorDir(s.Name), "source-budget.json")
	f.probe = func(Request) error { return os.MkdirAll(filepath.Join(budget, "blocked"), 0700) }
	if e := RunOrchestratorRound(context.Background(), s, &st, noEvidence()); e == nil {
		t.Fatal("budget write failure ignored")
	}
	if r := receiptsByEvent(t, s)["REL-1"]; r.Mode != "attempting" || len(f.starts) != 0 {
		t.Fatal("budget charged before receipt", r.Mode, f.starts)
	}
}

func TestValidNameReservesOrchestratorPrefix(t *testing.T) {
	if ValidName("o-release") || !ValidName("release") || !ValidName("orch-release") || !ValidAddress("o-release") || ValidAddress("o-o-x") || !ValidLabel("o-host") {
		t.Fatal("o- prefix")
	}
	if _, e := Parse("---\nname: o-release\nrepo: app\n---\n## Acceptance\n- x\n"); e == nil {
		t.Fatal("run named o- accepted")
	}
}

func TestSourceTemplateRejectsLineBreaksAndKeyInjection(t *testing.T) {
	s := sourceFixture(t)
	rule := s.Source.Rules[0]
	render := func(e SourceEvent) error {
		name, _ := sourceRunName(s, rule, []SourceEvent{e})
		_, _, err := renderSourceOrder(s, rule, []SourceEvent{e}, name)
		return err
	}
	for _, id := range []string{"REL-1\nhost: other", "REL-1\rhost: other", "REL-1\x00"} {
		if e := render(SourceEvent{ID: id, Kind: "issue", URL: "u", Fields: map[string]any{"status": "Ready"}}); !errors.Is(e, ErrSourceUnsafeValue) {
			t.Fatalf("%q: %v", id, e)
		}
	}
	if e := render(SourceEvent{ID: "REL-1", Kind: "issue", URL: "https://example.com/\nhost: other", Fields: map[string]any{"status": "Ready"}}); !errors.Is(e, ErrSourceUnsafeValue) {
		t.Fatal("body value", e)
	}
	// Strings that are never printed may contain line breaks.
	if e := render(SourceEvent{ID: "REL-1", Kind: "issue", URL: "u", Fields: map[string]any{"status": "Ready", "description": "line one\nline two"}}); e != nil {
		t.Fatal(e)
	}
	nested := strings.Replace(sourceTemplate, "{{.Action}}", "{{range $k, $v := .Event.Fields}}{{$k}}={{$v}} {{end}}", 1)
	_ = os.WriteFile(filepath.Join(OrchestratorDir(s.Name), "fix.md"), []byte(nested), 0600)
	if e := render(SourceEvent{ID: "REL-1", Kind: "issue", URL: "u", Fields: map[string]any{"status": "Ready", "note": "a\nb"}}); !errors.Is(e, ErrSourceUnsafeValue) {
		t.Fatal("range value", e)
	}
	keyed := strings.Replace(sourceTemplate, "issue: {{.Event.ID}}", "{{.Event.Fields.key}}: {{.Event.ID}}", 1)
	_ = os.WriteFile(filepath.Join(OrchestratorDir(s.Name), "fix.md"), []byte(keyed), 0600)
	if e := render(SourceEvent{ID: "REL-1", Kind: "issue", URL: "u", Fields: map[string]any{"status": "Ready", "key": "host"}}); !errors.Is(e, ErrSourceFrontmatter) {
		t.Fatal("injected key", e)
	}
}

func TestSourceBatchRuleHashesMembersAndBlocksChangedSet(t *testing.T) {
	s := sourceFixture(t)
	s.Source.Rules[0].Batch = true
	s.Source.Rules[0].Cooldown = "1h"
	tpl := "---\nname: {{.RunName}}\nrepo: app\nissue: REL-batch\n---\nFix:{{range .Events}} {{.ID}}{{end}}\n\n## Acceptance\n- Done.\n"
	_ = os.WriteFile(filepath.Join(OrchestratorDir(s.Name), "fix.md"), []byte(tpl), 0600)
	st, _ := orchestratorState(s.Name)
	now := fakeClock(t)
	f := newFakeTarget(t)
	setEvents(t, "c1", sourceEvent("REL-2", "Ready"), sourceEvent("REL-1", "Ready"))
	if e := RunOrchestratorRound(context.Background(), s, &st, noEvidence()); e != nil {
		t.Fatal(e)
	}
	want, _ := sourceRunName(s, s.Source.Rules[0], []SourceEvent{{ID: "REL-1"}, {ID: "REL-2"}})
	other, _ := sourceRunName(s, s.Source.Rules[0], []SourceEvent{{ID: "REL-2"}, {ID: "REL-1"}})
	r := receiptsByEvent(t, s)
	if want != other || len(f.starts) != 1 || f.starts[0] != want || r["REL-1"].Run != want || r["REL-2"].Run != want || !slices.Equal(r["REL-2"].Members, []string{"REL-1", "REL-2"}) {
		t.Fatal(f.starts, r)
	}
	order, _ := os.ReadFile(filepath.Join(OrchestratorDir(s.Name), "rounds", "001", want+".md"))
	if !strings.Contains(string(order), "Fix: REL-1 REL-2") {
		t.Fatal(string(order))
	}
	// A changed set waits while the batch run is active.
	setEvents(t, "c2", sourceEvent("REL-1", "Ready"), sourceEvent("REL-3", "Ready"))
	active := OrchestratorDriver{Gather: func(context.Context, OrchestratorSpec) ([]OrchestratorEvidence, error) {
		return []OrchestratorEvidence{{Row: Row{Meta: Meta{Order: Order{Name: want}}, State: "running"}}}, nil
	}}
	*now = now.Add(2 * time.Hour)
	if e := RunOrchestratorRound(context.Background(), s, &st, active); !errors.Is(e, ErrSourceDeferred) || !strings.Contains(st.Deferred, "blocks a changed set") || cursorOf(s) != "c1" {
		t.Fatal("active batch", e, st.Deferred)
	}
	// Inactive, but within cooldown of the last batch.
	*now = now.Add(-90 * time.Minute)
	if e := RunOrchestratorRound(context.Background(), s, &st, noEvidence()); !errors.Is(e, ErrSourceDeferred) || !strings.Contains(st.Deferred, "cooldown") {
		t.Fatal("cooldown", e, st.Deferred)
	}
	*now = now.Add(2 * time.Hour)
	if e := RunOrchestratorRound(context.Background(), s, &st, noEvidence()); e != nil {
		t.Fatal(e)
	}
	r = receiptsByEvent(t, s)
	if len(f.starts) != 2 || r["REL-3"].Mode != "live" || r["REL-1"].Run != want || !slices.Equal(r["REL-3"].Members, []string{"REL-3"}) {
		t.Fatal("new batch", f.starts, r)
	}
	text := strings.Replace(orchestratorExample, "source: none", "agent: off\nsource: {plugin: fake, rules: [{status: Ready, action: fix, template: fix.md, mode: live, cooldown: 1h}]}", 1)
	if _, e := ParseOrchestrator(text); e == nil {
		t.Fatal("cooldown without batch accepted")
	}
}

func TestRenderSourcePreviewWritesNothing(t *testing.T) {
	s := sourceFixture(t)
	before, _ := os.ReadDir(OrchestratorDir(s.Name))
	out, e := RenderSourcePreview(s.Name, []SourceEvent{{ID: "REL-1", Kind: "issue", URL: "https://example.com/1", Fields: map[string]any{"status": "Ready"}}, {ID: "REL-2\nhost: x", Kind: "issue", URL: "u", Fields: map[string]any{"status": "Ready"}}, {ID: "REL-3", Fields: map[string]any{"status": "Other"}}})
	if e != nil {
		t.Fatal(e)
	}
	name, _ := sourceRunName(s, s.Source.Rules[0], []SourceEvent{{ID: "REL-1"}})
	if len(out) != 2 || out[0].Run != name || !strings.Contains(out[0].Order, "issue: REL-1") || !strings.Contains(out[1].Error, ErrSourceUnsafeValue.Error()) {
		t.Fatal(out)
	}
	after, _ := os.ReadDir(OrchestratorDir(s.Name))
	if len(before) != len(after) {
		t.Fatal("render wrote state", before, after)
	}
}

func TestIntakeSignalsEdgeTriggeredAndListed(t *testing.T) {
	s := sourceFixture(t)
	st, _ := orchestratorState(s.Name)
	now := fakeClock(t)
	f := newFakeTarget(t)
	var hooked []IntakeSignal
	old := IntakeSignalHook
	t.Cleanup(func() { IntakeSignalHook = old })
	IntakeSignalHook = func(_ OrchestratorSpec, sig IntakeSignal) error { hooked = append(hooked, sig); return nil }
	f.start = func(Request) error { return errors.New("target refused") }
	// Retries at polls 1, 2 and 4.
	for i := 0; i < 4; i++ {
		s.Idle = "run"
		_ = RunOrchestratorRound(context.Background(), s, &st, noEvidence())
	}
	if len(f.starts) != 3 || len(hooked) != 1 || hooked[0].Kind != "rejected" || len(st.Signals) != 1 {
		t.Fatal("rejected signal must fire once per receipt", f.starts, hooked)
	}
	f.start = func(Request) error { return errLaunchUnconfirmed }
	f.listErr = transportError{errors.New("ssh: down")}
	for i := 0; i < 8 && len(f.starts) == 3; i++ {
		_ = RunOrchestratorRound(context.Background(), s, &st, noEvidence())
	}
	*now = now.Add(sourceAttemptGrace + time.Minute)
	for i := 0; i < 2; i++ {
		_ = RunOrchestratorRound(context.Background(), s, &st, noEvidence())
	}
	f.listErr = nil
	for i := 0; i < 2; i++ {
		_ = RunOrchestratorRound(context.Background(), s, &st, noEvidence())
	}
	kinds := []string{}
	for _, sig := range hooked {
		kinds = append(kinds, sig.Kind)
	}
	if !slices.Equal(kinds, []string{"rejected", "attempt-stale", "needs-reconcile"}) {
		t.Fatal(kinds)
	}
	if e := WriteJSON(filepath.Join(OrchestratorDir(s.Name), "state.json"), st); e != nil {
		t.Fatal(e)
	}
	rows, e := ListOrchestrators()
	if e != nil || len(rows) != 1 || len(rows[0].Signals) != 3 {
		t.Fatal(rows, e)
	}
	b, _ := json.Marshal(rows[0])
	if !strings.Contains(string(b), `"signals":[{"kind":"rejected"`) {
		t.Fatal(string(b))
	}
}

func TestSourceEventNamedRuns(t *testing.T) {
	s := sourceFixture(t)
	s.NamePattern = "rel-[0-9]+"
	s.Manages.Runs.Selector = "issue=rel-*"
	s.Source.Rules[0].Name = "event"
	st, _ := orchestratorState(s.Name)
	f := newFakeTarget(t)
	f.start = func(q Request) error {
		if q.Name == "rel-2" {
			return fmt.Errorf("%w: rel-2", errRunExists)
		}
		return nil
	}
	setEvents(t, "c1", sourceEvent("rel-1", "Ready"), sourceEvent("rel-2", "Ready"), sourceEvent("REL-3", "Ready"), sourceEvent("other-4", "Ready"))
	if e := RunOrchestratorRound(context.Background(), s, &st, noEvidence()); e != nil {
		t.Fatal(e)
	}
	r := receiptsByEvent(t, s)
	if r["rel-1"].Run != "rel-1" || r["rel-1"].Mode != "live" || r["rel-2"].Mode != "live" {
		t.Fatal("event names", r)
	}
	for _, id := range []string{"REL-3", "other-4"} {
		if r[id].Mode != "rejected" || !strings.Contains(r[id].Error, ErrSourceEventName.Error()) {
			t.Fatal(id, r[id])
		}
	}
	if _, e := ParseOrchestrator(strings.Replace(orchestratorExample, "source: none", "agent: off\nsource: {plugin: fake, rules: [{status: Ready, action: fix, template: fix.md, mode: live, name: event}]}", 1)); e == nil {
		t.Fatal("name: event without name_pattern accepted")
	}
}

func TestSourceCancellationKeepsAttemptAndCursor(t *testing.T) {
	s := sourceFixture(t)
	st, _ := orchestratorState(s.Name)
	f := newFakeTarget(t)
	ctx, cancel := context.WithCancel(context.Background())
	f.start = func(Request) error { cancel(); return ctx.Err() }
	if e := RunOrchestratorRound(ctx, s, &st, noEvidence()); e == nil {
		t.Fatal("cancelled round accepted")
	}
	if r := receiptsByEvent(t, s)["REL-1"]; r.Mode != "attempting" || cursorOf(s) != "" {
		t.Fatal("cancellation", r.Mode, cursorOf(s))
	}
	// The next round never re-dispatches; the landed run is promoted.
	f.start = func(Request) error { t.Fatal("relaunched after cancellation"); return nil }
	f.listed[receiptsByEvent(t, s)["REL-1"].Run] = true
	if e := RunOrchestratorRound(context.Background(), s, &st, noEvidence()); e != nil || receiptsByEvent(t, s)["REL-1"].Mode != "live" {
		t.Fatal(e)
	}
}

func TestSourceCursorTravelsOutsideArgv(t *testing.T) {
	s := sourceFixture(t)
	s.Source.Args = []string{"--config", "fake.toml"}
	s.Source.Team = "core"
	cursor := `%(x)!^&"`
	_ = WriteJSON(filepath.Join(OrchestratorDir(s.Name), "source-cursor.json"), map[string]string{"cursor": cursor})
	calls := setEvents(t, "c1")
	if _, e := fetchSource(context.Background(), s); e != nil {
		t.Fatal(e)
	}
	events := (*calls)[1]
	if !slices.Equal(events.Args, []string{"events", "--config", "fake.toml", "--team", "core"}) || !slices.Contains(events.Env, "HEV_SOURCE_CURSOR="+cursor) {
		t.Fatal(events)
	}
	if _, e := CmdPluginLine(`C:\plugins\hev-fake.cmd`, events.Args); e != nil {
		t.Fatal("batch plugin rejects cursor-free argv", e)
	}
}

func TestSourceAgentOffAdmissionMatchesRoundArtifact(t *testing.T) {
	s := sourceFixture(t)
	st, _ := orchestratorState(s.Name)
	fakeClock(t)
	f := newFakeTarget(t)
	f.start = func(Request) error { return errors.New("target refused") }
	_ = RunOrchestratorRound(context.Background(), s, &st, noEvidence())
	// The retry is off-cursor: the event is no longer in the packet.
	setEvents(t, "c2")
	var admitted []error
	f.start = func(q Request) error {
		admitted = append(admitted, admitSourceOrder(s, q))
		q.Order += "\nextra"
		admitted = append(admitted, admitSourceOrder(s, q))
		return nil
	}
	if e := RunOrchestratorRound(context.Background(), s, &st, noEvidence()); e != nil {
		t.Fatal(e)
	}
	if len(admitted) != 2 || admitted[0] != nil || admitted[1] == nil {
		t.Fatal("agent off admission", admitted)
	}
}

func TestSupervisorWakesWhenPauseFileRemoved(t *testing.T) {
	s := sourceFixture(t)
	pause := filepath.Join(t.TempDir(), "PAUSED")
	if e := os.WriteFile(pause, nil, 0600); e != nil {
		t.Fatal(e)
	}
	text := strings.Replace(orchestratorExample, "source: none", "agent: off\npause_file: "+pause+"\nsource:\n  plugin: fake\n  rules:\n    - {status: Ready, action: fix, template: fix.md, mode: live}", 1)
	if _, e := ApplyOrchestrator(text, false); e != nil {
		t.Fatal(e)
	}
	newFakeTarget(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- SuperviseOrchestrator(ctx, s.Name, OrchestratorDriver{Gather: noEvidence().Gather, Now: time.Now})
	}()
	t.Cleanup(func() { cancel(); <-done })
	waitFor := func(what string, ok func() bool) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for !ok() {
			if time.Now().After(deadline) {
				t.Fatal("timed out waiting for", what)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	waitFor("deferred state", func() bool {
		st, _ := orchestratorState(s.Name)
		return st.State == "waiting" && st.Deferred == "pause_file present" && !st.Next.IsZero()
	})
	if exists(filepath.Join(OrchestratorDir(s.Name), "rounds")) {
		t.Fatal("paused round created a directory")
	}
	if e := os.Remove(pause); e != nil {
		t.Fatal(e)
	}
	waitFor("dispatch after pause_file removal", func() bool {
		all, _ := loadSourceReceipts(s)
		for _, r := range all {
			if r.Mode == "live" {
				return true
			}
		}
		return false
	})
}
