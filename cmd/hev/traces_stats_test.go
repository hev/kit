package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/filearchive"
	"github.com/hev/kit/internal/index"
	"github.com/hev/kit/internal/layerenv"
	tracepkg "github.com/hev/kit/internal/trace"
	"github.com/hev/kit/internal/tracestats"
)

// statsFixture builds a file archive from hand-written transcripts: a
// dispatch worker (Codex) that loads skills by reading SKILL.md, searches
// the board and edits after a failing command, and an interactive Claude
// session that loads a skill through the Skill tool.
func statsFixture(t *testing.T) (*filearchive.Store, []tracepkg.Source, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("HEV_CONFIG", filepath.Join(home, ".hev", "config.toml"))
	for _, name := range layerenv.Names() {
		t.Setenv(name, "")
	}
	claudeRoot, codexRoot := filepath.Join(home, "claude"), filepath.Join(home, "codex")
	write := func(p, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	ts := func(s int) string { return now.Add(-time.Hour + time.Duration(s)*time.Second).Format(time.RFC3339) }
	worker := filepath.Join(home, "dispatch", "fix-bug", "wt")
	write(filepath.Join(codexRoot, "2026", "11", "16", "rollout-w.jsonl"), strings.Join([]string{
		`{"timestamp":"` + ts(0) + `","type":"session_meta","payload":{"id":"0a0a0a0a-0000-4000-8000-000000000001","cwd":"` + worker + `"}}`,
		`{"timestamp":"` + ts(1) + `","type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"<skills_instructions>\n- peers: talk (file: /h/.codex/skills/peers/SKILL.md)\n</skills_instructions>"}]}}`,
		`{"timestamp":"` + ts(2) + `","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"fix the bug"}]}}`,
		`{"timestamp":"` + ts(3) + `","type":"response_item","payload":{"type":"function_call","id":"f1","call_id":"c1","name":"exec_command","arguments":"{\"cmd\":\"cat /h/.codex/skills/peers/SKILL.md /h/.agents/skills/hev-handoff/SKILL.md\"}"}}`,
		`{"timestamp":"` + ts(4) + `","type":"response_item","payload":{"type":"function_call_output","id":"o1","call_id":"c1","output":"Process exited with code 0\nOutput:\nskill text"}}`,
		`{"timestamp":"` + ts(5) + `","type":"response_item","payload":{"type":"function_call","id":"f2","call_id":"c2","name":"exec_command","arguments":"{\"cmd\":\"hev board search flaky port\"}"}}`,
		`{"timestamp":"` + ts(6) + `","type":"response_item","payload":{"type":"function_call_output","id":"o2","call_id":"c2","output":"Process exited with code 1\nOutput:\nno hits"}}`,
		`{"timestamp":"` + ts(7) + `","type":"response_item","payload":{"type":"function_call","id":"f3","call_id":"c3","name":"apply_patch","arguments":"{\"input\":\"*** Begin Patch\"}"}}`,
		`{"timestamp":"` + ts(8) + `","type":"response_item","payload":{"type":"function_call_output","id":"o3","call_id":"c3","output":"Done"}}`,
		`{"timestamp":"` + ts(9) + `","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":1000,"cached_input_tokens":400,"output_tokens":50}}}}`,
	}, "\n")+"\n")
	write(filepath.Join(claudeRoot, "-w", "0b0b0b0b-0000-4000-8000-000000000002.jsonl"), strings.Join([]string{
		`{"type":"user","uuid":"u1","sessionId":"0b0b0b0b-0000-4000-8000-000000000002","timestamp":"` + ts(10) + `","cwd":"/w","message":{"role":"user","content":"what changed?"}}`,
		`{"type":"attachment","uuid":"x1","sessionId":"0b0b0b0b-0000-4000-8000-000000000002","timestamp":"` + ts(10) + `","attachment":{"type":"instructions","files":[{"path":"` + home + `/.claude/CLAUDE.md","type":"User","content":"0123456789"}]}}`,
		`{"type":"assistant","uuid":"a1","parentUuid":"u1","sessionId":"0b0b0b0b-0000-4000-8000-000000000002","timestamp":"` + ts(11) + `","cwd":"/w","requestId":"r1","message":{"role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"tool_use","id":"t1","name":"Skill","input":{"skill":"hev-traces"}},{"type":"tool_use","id":"t2","name":"Bash","input":{"command":"hev traces search flaky"}}],"usage":{"input_tokens":7,"output_tokens":3}}}`,
		`{"type":"user","uuid":"u2","parentUuid":"a1","sessionId":"0b0b0b0b-0000-4000-8000-000000000002","timestamp":"` + ts(12) + `","cwd":"/w","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"loaded"},{"type":"tool_result","tool_use_id":"t2","is_error":true,"content":"Exit code 4\nno archive"}]}}`,
	}, "\n")+"\n")
	store, err := filearchive.Open(filepath.Join(home, "archive"), "")
	if err != nil {
		t.Fatal(err)
	}
	sources := []tracepkg.Source{&tracepkg.CodexSource{Root: codexRoot}, &tracepkg.ClaudeSource{Root: claudeRoot}}
	return store, sources, home
}

func indexFixture(t *testing.T, store *filearchive.Store, sources []tracepkg.Source) {
	t.Helper()
	for _, src := range sources {
		rep, err := index.Run(src, store, index.LoadState(), index.Options{RepoURL: func(string) string { return "" }})
		if err != nil || len(rep.Errors) > 0 {
			t.Fatalf("index: %v %v", err, rep)
		}
	}
}

// show --json carries each tool call's outcome and per-turn usage.
func TestShowJSONToolOutcomes(t *testing.T) {
	store, sources, _ := statsFixture(t)
	indexFixture(t, store, sources)
	turns, err := sessionTurns(store, "0b0b0b0b")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	traceJSON = true
	defer func() { traceJSON = false }()
	if err := showNamespaceTurns(blockTurns(store, turns), &buf); err != nil {
		t.Fatal(err)
	}
	var got []struct {
		Role  string `json:"role"`
		Usage struct {
			Input  int64 `json:"input_tokens"`
			Output int64 `json:"output_tokens"`
		} `json:"usage"`
		Blocks []struct {
			Type        string `json:"type"`
			ToolName    string `json:"tool_name"`
			IsError     bool   `json:"is_error"`
			ExitCode    *int   `json:"exit_code"`
			ResultBytes int64  `json:"result_bytes"`
			Text        string `json:"text"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	var bash, result, usage bool
	for _, turn := range got {
		if turn.Role == "assistant" && turn.Usage.Input == 7 && turn.Usage.Output == 3 {
			usage = true
		}
		for _, b := range turn.Blocks {
			if b.Type == "tool_use" && b.ToolName == "Bash" {
				bash = b.IsError && b.ExitCode != nil && *b.ExitCode == 4 && b.ResultBytes == int64(len("Exit code 4\nno archive"))
			}
			if b.Type == "tool_result" && strings.HasPrefix(b.Text, "Exit code 4") {
				result = b.IsError && b.ExitCode != nil && *b.ExitCode == 4
			}
		}
	}
	if !bash || !result || !usage {
		t.Fatalf("tool outcome on call=%v on result=%v usage=%v\n%s", bash, result, usage, buf.String())
	}
}

func TestTracesStatsOnFixtureArchive(t *testing.T) {
	store, sources, home := statsFixture(t)
	indexFixture(t, store, sources)
	rt := statsRuntime{
		Store: func(string) (traceStore, error) { return store, nil },
		Spans: func(traceStore, int64) ([]tracepkg.SpanRow, error) {
			start := time.Now().Add(-time.Hour).UnixMilli()
			return []tracepkg.SpanRow{
				{ID: "a", Kind: "attempt", Host: "h1", Start: start, RunStart: start, Result: 1, End: start + 30_000, Outcome: "blocked"},
				{ID: "b", Kind: "attempt", Host: "h1", Start: start, RunStart: start, Result: 1, End: start + 3_600_000, Outcome: "done"},
			}, nil
		},
		Installed: func() []string { return []string{"peers", "hev-traces", "oven"} },
		Now:       time.Now,
	}
	rep, err := buildTracesStats(rt, statsOptions{since: "5d", by: "kind", installed: true, scanBlocks: true}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Sessions != 2 || rep.Kinds[tracestats.KindWorker] != 1 || rep.Kinds[tracestats.KindInteractive] != 1 || rep.Capture.WithCaptureFacts != 2 || rep.Capture.WithStartContext != 2 {
		t.Fatalf("report: %+v", rep)
	}
	groups := map[string]tracestats.Group{}
	for _, g := range rep.Groups {
		groups[g.Key] = g
	}
	w, i := groups[tracestats.KindWorker], groups[tracestats.KindInteractive]
	if w.ToolCalls.Median != 3 || w.EditedSessions != 1 || *w.ToolsBeforeEditMedian != 2 || *w.BoardSearchShare != 1 || w.ToolErrors != 1 || w.ToolResults != 3 || w.TokensIn != 1000 || w.TokensOut != 50 {
		t.Fatalf("worker: %+v", w)
	}
	if *i.TracesSearchShare != 1 || i.ToolErrors != 1 || i.EditedSessions != 0 || *i.StartContextBytesMedian != 10 {
		t.Fatalf("interactive: %+v", i)
	}
	names := map[string]tracestats.Skill{}
	for _, s := range rep.Skills {
		names[s.Name] = s
	}
	if names["peers"].ByVia["read"] != 1 || names["hev-handoff"].ByKind[tracestats.KindWorker] != 1 || names["hev-traces"].ByVia["tool"] != 1 {
		t.Fatalf("skills: %+v", rep.Skills)
	}
	if rep.Installed == nil || len(rep.Installed.NeverLoaded) != 1 || rep.Installed.NeverLoaded[0] != "oven" {
		t.Fatalf("installed: %+v", rep.Installed)
	}
	if rep.Dispatch.Runs != 2 || rep.Dispatch.LaunchFailures != 1 {
		t.Fatalf("dispatch: %+v", rep.Dispatch)
	}
	var text bytes.Buffer
	renderTracesStats(&text, rep, 15)
	for _, want := range []string{"2 sessions since", "worker 1", "peers", "never loaded in the window: 1", "dispatch: 2 runs started, 1 launch failures"} {
		if !strings.Contains(text.String(), want) {
			t.Fatalf("text output lacks %q:\n%s", want, text.String())
		}
	}

	// The JSON report keeps its documented top-level keys.
	b, _ := json.Marshal(rep)
	var top map[string]json.RawMessage
	json.Unmarshal(b, &top)
	for _, key := range []string{"schema", "since", "from", "to", "by", "sessions", "kinds", "capture", "groups", "skills", "installed", "dispatch"} {
		if _, ok := top[key]; !ok {
			t.Fatalf("JSON report lacks %q: %s", key, b)
		}
	}
	if _, err := buildTracesStats(rt, statsOptions{since: "5d", by: "nope"}, &bytes.Buffer{}); err == nil {
		t.Fatal("bad --by accepted")
	}
	_ = home
}

// Rows written before the capture facts get them from their block rows.
func TestTracesStatsDerivesOldRows(t *testing.T) {
	store, sources, _ := statsFixture(t)
	indexFixture(t, store, sources)
	rows, err := store.ListSessionRows(-1, nil)
	if err != nil {
		t.Fatal(err)
	}
	old, err := filearchive.Open(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	for i := range rows {
		blocks, err := store.ListBlockRows(rows[i].SessionID)
		if err != nil {
			t.Fatal(err)
		}
		for j := range blocks {
			blocks[j].Resulted, blocks[j].ExitCode, blocks[j].ResultBytes, blocks[j].SkillLoads = false, nil, 0, nil
		}
		if _, err := old.WriteBlocks(blocks); err != nil {
			t.Fatal(err)
		}
		rows[i].CaptureVersion, rows[i].ToolErrors, rows[i].ToolResults, rows[i].ToolsBeforeEdit, rows[i].CommandCounts, rows[i].StartContext, rows[i].SkillLoads = 0, 0, 0, 0, nil, nil, nil
	}
	if _, err := old.WriteSessions(rows); err != nil {
		t.Fatal(err)
	}
	rt := statsRuntime{Store: func(string) (traceStore, error) { return old, nil }, Now: time.Now}
	rep, err := buildTracesStats(rt, statsOptions{since: "5d", by: "harness", scanBlocks: true}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Capture.DerivedFromBlocks != 2 || rep.Capture.WithCaptureFacts != 0 {
		t.Fatalf("capture: %+v", rep.Capture)
	}
	var errs int64
	for _, g := range rep.Groups {
		errs += g.ToolErrors
	}
	if errs != 2 {
		t.Fatalf("derived tool errors = %d, want 2", errs)
	}
}

// The backfill previews per harness without writing, then writes and
// journals each transcript, and a second run resumes with nothing left.
func TestTracesBackfillPreviewThenApply(t *testing.T) {
	store, sources, home := statsFixture(t)
	journal := filepath.Join(home, ".hev", "traces-backfill.json")
	open := func() (index.Archive, *tracepkg.ImageCapture, error) { return store, nil, nil }
	var out bytes.Buffer
	if err := runTracesBackfill(&out, &bytes.Buffer{}, sources, open, journal, "", 30*24*time.Hour, false, false, true, 0, time.Now()); err != nil {
		t.Fatal(err)
	}
	var preview statsBackfillReport
	if err := json.Unmarshal(out.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	cx, cl := preview.Harnesses["codex"], preview.Harnesses["claude_code"]
	if preview.Mode != "preview" || cx == nil || cl == nil || cx.Sessions != 1 || cx.SkillLoadsRead != 2 || cx.ToolCallsExitCode != 2 || cx.ToolCallsErrored != 1 || cx.WithCommands != 1 || cx.EditedSessions != 1 ||
		cl.SkillLoadsTool != 1 || cl.StartTranscript+cl.StartMixed != 1 || cl.ToolCallsResulted != 2 {
		t.Fatalf("preview: %s", out.String())
	}
	if _, err := os.Stat(journal); !os.IsNotExist(err) {
		t.Fatal("preview wrote a journal")
	}
	if rows, _ := store.ListSessionRows(-1, nil); len(rows) != 0 {
		t.Fatal("preview wrote rows")
	}

	out.Reset()
	if err := runTracesBackfill(&out, &bytes.Buffer{}, sources, open, journal, "", 30*24*time.Hour, true, false, true, 1, time.Now()); err != nil {
		t.Fatal(err)
	}
	var applied statsBackfillReport
	json.Unmarshal(out.Bytes(), &applied)
	if applied.Written == nil || applied.Written.Units != 1 {
		t.Fatalf("limited apply: %s", out.String())
	}
	out.Reset()
	if err := runTracesBackfill(&out, &bytes.Buffer{}, sources, open, journal, "", 30*24*time.Hour, true, false, true, 0, time.Now()); err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(out.Bytes(), &applied)
	if applied.Written.Units != 1 {
		t.Fatalf("resume should write only the remaining transcript: %s", out.String())
	}
	rows, _ := store.ListSessionRows(-1, nil)
	if len(rows) != 2 || rows[0].CaptureVersion != tracepkg.CaptureVersion {
		t.Fatalf("rows after apply: %d", len(rows))
	}
	out.Reset()
	runTracesBackfill(&out, &bytes.Buffer{}, sources, open, journal, "", 30*24*time.Hour, false, false, true, 0, time.Now())
	json.Unmarshal(out.Bytes(), &preview)
	if preview.Harnesses["codex"].UnitsDone != 1 || preview.Harnesses["codex"].Sessions != 0 {
		t.Fatalf("journal not honored by preview: %s", out.String())
	}
}
