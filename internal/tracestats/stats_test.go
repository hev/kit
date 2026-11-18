package tracestats

import (
	"reflect"
	"testing"

	"github.com/hev/kit/internal/trace"
)

func TestRunKind(t *testing.T) {
	loopPrompt := trace.SessionRow{Workdir: "/Users/u/promo", FirstPromptShort: "Run one iteration of the loop."}
	rows := []trace.SessionRow{loopPrompt, loopPrompt, loopPrompt}
	repeats := Repeats(rows)
	for _, tt := range []struct {
		name string
		row  trace.SessionRow
		want string
	}{
		{"dispatch order", trace.SessionRow{Order: "fix-bug", Workdir: "/elsewhere"}, KindWorker},
		{"trace id", trace.SessionRow{TraceID: "f40e"}, KindWorker},
		{"dispatch workdir", trace.SessionRow{Workdir: "/home/u/dispatch/fix-bug/wt"}, KindWorker},
		{"mac dispatch workdir", trace.SessionRow{Workdir: "/Users/u/dispatch/fix-bug"}, KindWorker},
		{"orchestrator order", trace.SessionRow{Order: "orch-decide"}, KindOrchestrator},
		{"orchestrator round dir", trace.SessionRow{Workdir: "/home/u/dispatch/orch-decide/rounds/049"}, KindOrchestrator},
		{"not a dispatch root", trace.SessionRow{Workdir: "/home/u/src/kit/internal/dispatch/x"}, KindInteractive},
		{"self-scheduling", trace.SessionRow{ToolNames: []string{"Bash", "ScheduleWakeup"}}, KindLoop},
		{"/loop", trace.SessionRow{FirstPrompt: "/loop 5m check CI"}, KindLoop},
		{"expediter", trace.SessionRow{Workdir: "/home/u/.local/state/agentic-expediter-u"}, KindLoop},
		{"recurring prompt", loopPrompt, KindLoop},
		{"one-off", trace.SessionRow{Workdir: "/Users/u/promo", FirstPromptShort: "Something else"}, KindInteractive},
	} {
		if got := RunKind(tt.row, repeats); got != tt.want {
			t.Errorf("%s: RunKind = %q, want %q", tt.name, got, tt.want)
		}
	}
	// Dispatch sessions never feed recurrence.
	if n := Repeats([]trace.SessionRow{{Order: "a", FirstPromptShort: "x"}, {Order: "a", FirstPromptShort: "x"}, {Order: "a", FirstPromptShort: "x"}}); len(n) != 0 {
		t.Fatalf("dispatch prompts counted: %v", n)
	}
}

func TestBuildAggregates(t *testing.T) {
	ctx := func(n int64) *trace.StartContext {
		return &trace.StartContext{Source: "transcript", InstructionBytes: n}
	}
	rows := []trace.SessionRow{
		// Captured worker: edited after 4 calls, searched the board, 1 of 10 results errored.
		{SessionID: "w1", Order: "a", Harness: "codex", Host: "h1", ToolCount: 10, InputTokens: 100, CacheReadTokens: 50, OutputTokens: 10,
			CaptureVersion: 1, ToolResults: 10, ToolErrors: 1, ToolsBeforeEdit: 4, CommandCounts: trace.ToolCounts{"board_search": 2, "dispatch": 1, trace.UnknownCommandKey: 3},
			Skills: []string{"peers", "hev-handoff"}, SkillLoads: trace.SkillLoads{{Name: "peers", Via: "read"}, {Name: "hev-handoff", Via: "read"}},
			StartContext: ctx(1000), StartContextBytes: 1000},
		// Captured worker: never edited, posted to the board.
		{SessionID: "w2", Order: "b", Harness: "claude_code", Host: "h1", ToolCount: 20, OutputTokens: 5,
			CaptureVersion: 1, ToolResults: 20, ToolErrors: 3, ToolsBeforeEdit: -1, CommandCounts: trace.ToolCounts{"board_post": 1},
			Skills: []string{"peers"}, SkillLoads: trace.SkillLoads{{Name: "peers", Via: "tool"}}, StartContext: ctx(3000), StartContextBytes: 3000},
		// Old interactive row: facts derived from blocks.
		{SessionID: "i1", Harness: "claude_code", Host: "h2", ToolCount: 2, Skills: []string{"kit:company"}},
		// Old interactive row with no blocks: no facts.
		{SessionID: "i2", Harness: "pi", Host: "h2", ToolCount: 4},
	}
	derived := map[string]Facts{"i1": {ToolResults: 2, ToolErrors: 0, ToolsBeforeEdit: 0, Commands: map[string]int{"traces_search": 1}, Skills: []string{"company"}}}
	spans := []trace.SpanRow{
		{ID: "1", Kind: "attempt", Host: "h1", Start: 1000, Result: 1, RunStart: 1000, End: 1000 + 60_000, ExitCode: 1},        // failed in 1 min
		{ID: "2", Kind: "attempt", Host: "h1", Start: 2000, Result: 1, RunStart: 2000, End: 2000 + 90_000, Outcome: "blocked"}, // blocked in 1.5 min
		{ID: "3", Kind: "attempt", Host: "h1", Start: 3000, Result: 1, RunStart: 3000, End: 3000 + 10*60_000, ExitCode: 1},     // failed late
		{ID: "4", Kind: "attempt", Host: "h2", Start: 4000, Result: 1, RunStart: 4000, End: 4000 + 30_000, Outcome: "done"},    // fine
		{ID: "5", Kind: "attempt", Host: "h2", Start: 5000},                                                                    // still running
		{ID: "6", Kind: "ask", Host: "h2", Start: 6000},                                                                        // not a run
		{ID: "7", Kind: "attempt", Host: "h2", Start: 10, Result: 1, RunStart: 10, End: 20, ExitCode: 1},                       // before the window
	}
	rep := Build(Input{Since: "5d", From: 500, To: 10_000, By: "kind", Rows: rows, Derived: derived, Spans: spans, Installed: []string{"peers", "company", "oven", "kit:oven"}})

	if rep.Schema != Schema || rep.Sessions != 4 || rep.Kinds[KindWorker] != 2 || rep.Kinds[KindInteractive] != 2 {
		t.Fatalf("header: %+v", rep)
	}
	if rep.Capture != (Capture{Sessions: 4, WithCaptureFacts: 2, DerivedFromBlocks: 1, WithStartContext: 2}) {
		t.Fatalf("capture: %+v", rep.Capture)
	}
	groups := map[string]Group{}
	for _, g := range rep.Groups {
		groups[g.Key] = g
	}
	w := groups[KindWorker]
	f := func(p *float64) float64 {
		if p == nil {
			return -1
		}
		return *p
	}
	if w.Sessions != 2 || w.ToolCalls != (Dist{Median: 10, P90: 20}) || w.FactsSessions != 2 || w.EditedSessions != 1 || f(w.ToolsBeforeEditMedian) != 4 ||
		f(w.BoardSearchShare) != 0.5 || f(w.BoardPostShare) != 0.5 || f(w.TracesSearchShare) != 0 || f(w.DispatchShare) != 0.5 ||
		w.ToolResults != 30 || w.ToolErrors != 4 || f(w.ToolErrorRate) != 0.133 || w.StartContextSessions != 2 || f(w.StartContextBytesMedian) != 1000 ||
		w.TokensIn != 150 || w.TokensOut != 15 || w.UnknownCommands != 3 || w.UnknownCommandSessions != 1 {
		t.Fatalf("worker group: %+v", w)
	}
	i := groups[KindInteractive]
	if i.Sessions != 2 || i.FactsSessions != 1 || f(i.TracesSearchShare) != 1 || f(i.ToolsBeforeEditMedian) != 0 || i.StartContextBytesMedian != nil || f(i.ToolErrorRate) != 0 || i.UnknownCommands != 0 || i.UnknownCommandSessions != 0 {
		t.Fatalf("interactive group: %+v", i)
	}

	skills := map[string]Skill{}
	for _, s := range rep.Skills {
		skills[s.Name] = s
	}
	if rep.Skills[0].Name != "peers" || skills["peers"].Sessions != 2 || !reflect.DeepEqual(skills["peers"].ByVia, map[string]int{"read": 1, "tool": 1}) ||
		!reflect.DeepEqual(skills["peers"].ByHarness, map[string]int{"codex": 1, "claude_code": 1}) || skills["peers"].ByKind[KindWorker] != 2 {
		t.Fatalf("peers: %+v", skills["peers"])
	}
	if skills["company"].Sessions != 1 || skills["company"].ByKind[KindInteractive] != 1 || skills["company"].ByVia != nil {
		t.Fatalf("company: %+v", skills["company"])
	}
	if rep.Installed == nil || !reflect.DeepEqual(rep.Installed.Skills, []string{"company", "oven", "peers"}) || !reflect.DeepEqual(rep.Installed.NeverLoaded, []string{"oven"}) {
		t.Fatalf("installed: %+v", rep.Installed)
	}
	want := Dispatch{Runs: 5, LaunchFailures: 2, Attempts: 5, AttemptLaunchFailures: 2, ByHost: []HostDispatch{{Host: "h1", Runs: 3, LaunchFailures: 2, Attempts: 3, AttemptLaunchFailures: 2}, {Host: "h2", Runs: 2, Attempts: 2}}}
	if !reflect.DeepEqual(rep.Dispatch, want) {
		t.Fatalf("dispatch: %+v", rep.Dispatch)
	}

	byHarness := Build(Input{By: "harness", Rows: rows, Derived: derived})
	if byHarness.Groups[0].Key != "claude_code" || byHarness.Groups[0].Sessions != 2 || byHarness.Installed != nil {
		t.Fatalf("by harness: %+v", byHarness.Groups)
	}
	if g := Build(Input{By: "order", Rows: rows}).Groups; g[0].Key != "(none)" || g[0].Sessions != 2 {
		t.Fatalf("by order: %+v", g)
	}
	if RepoName("git@github.com:hev/kit.git") != "hev/kit" || RepoName("https://github.com/hev/kit") != "hev/kit" {
		t.Fatal("repo name")
	}
}

// Dispatch health counts runs, not attempts: a run is one RunKey whatever
// its attempts, counted where and when its first attempt started, and
// validation runs are counted apart.
func TestDispatchCountsRuns(t *testing.T) {
	fail := func(id, key, host string, start int64) trace.SpanRow {
		return trace.SpanRow{ID: id, Kind: "attempt", RunKey: key, Host: host, Start: start, Result: 1, RunStart: start, End: start + 30_000, ExitCode: 1}
	}
	ok := func(id, key, host string, start int64) trace.SpanRow {
		return trace.SpanRow{ID: id, Kind: "attempt", RunKey: key, Host: host, Start: start, Result: 1, RunStart: start, End: start + 30*60_000, Outcome: "done"}
	}
	spans := []trace.SpanRow{
		// The review's case: one run, two attempts.
		{ID: "attempt1", Kind: "attempt", RunKey: "same-run", Host: "h", Start: 1000},
		{ID: "attempt2", Kind: "attempt", RunKey: "same-run", Host: "h", Start: 2000},
		// A run that failed at launch, then was reworked fine: one run, one launch failure.
		fail("a1", "relaunched", "h", 1500), ok("a2", "relaunched", "h", 9000),
		// The same attempt published by two hosts' indexers.
		ok("b1", "published-twice", "h2", 1200), ok("b1", "published-twice", "h2", 1200),
		// A validation run of "relaunched": counted apart.
		{ID: "v1", Kind: "attempt", RunKey: "review", Source: "relaunched", Host: "h", Start: 3000, Result: 1, RunStart: 3000, End: 3000 + 10_000, ExitCode: 1},
		// Started before the window; its rework inside does not make it a run started.
		ok("c1", "older", "h", 100), fail("c2", "older", "h", 4000),
		// No RunKey: the run directory on its publisher identifies it.
		{ID: "d1", Kind: "attempt", Run: "legacy", Publisher: "h2", Start: 5000},
		{ID: "d2", Kind: "attempt", Run: "legacy", Publisher: "h2", Start: 6000},
	}
	got := Build(Input{By: "kind", From: 500, To: 10_000, Spans: spans}).Dispatch
	want := Dispatch{Runs: 4, LaunchFailures: 1, ValidationRuns: 1, Attempts: 8, AttemptLaunchFailures: 2, ByHost: []HostDispatch{
		{Host: "h", Runs: 2, LaunchFailures: 1, ValidationRuns: 1, Attempts: 5, AttemptLaunchFailures: 2},
		{Host: "h2", Runs: 2, Attempts: 3},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dispatch:\n got %+v\nwant %+v", got, want)
	}
}
