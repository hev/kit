package foh

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestDispatchWatchAndAutoRespond(t *testing.T) {
	p := &scripted{replies: []ChatResult{
		{ToolCalls: []ToolCall{call("d1", "hev_dispatch", `{"action":"send","name":"smoke-1","host":"mini","repo":"kit","body":"## Acceptance\n1. echo ok"}`)}},
		{Content: "Dispatched mini/smoke-1."},
		{Content: "mini/smoke-1 finished cleanly (PR #999)."},
	}}
	m, a := newTestTUI(t, p)
	state := "running"
	a.Tools = NewToolset(t.TempDir(), func(_ context.Context, args []string, _ string) ([]byte, error) {
		if len(args) >= 2 && args[0] == "dispatch" && args[1] == "show" {
			return []byte(`{"name":"smoke-1","host":"mini","state":"` + state + `","pr":"https://github.com/hev/kit/pull/999","report":"All good."}`), nil
		}
		return []byte(`{"name":"smoke-1","host":"mini","state":"running"}`), nil
	})

	// 1. /dispatch sends the order and automatically registers a watch.
	drain(t, m, submit(m, "/dispatch smoke test on mini"))
	if len(m.watched) != 1 || m.watched[0].Label() != "mini/smoke-1" {
		t.Fatalf("watched = %+v, want mini/smoke-1", m.watched)
	}
	if !strings.Contains(plain(m), "dispatched to mini/smoke-1") || !strings.Contains(plain(m), "◉ watching 1") {
		t.Fatalf("missing watch indicators:\n%s", plain(m))
	}
	if !strings.Contains(plain(m), "dispatched → mini/smoke-1") {
		t.Fatalf("dispatch card missing host/name:\n%s", plain(m))
	}

	// 2. While still running, poll emits no terminal results.
	if res := pollWatchedRuns(m.watched, a.Tools.Hev); len(res) != 0 {
		t.Fatalf("expected 0 results while running, got %+v", res)
	}

	// 3. Once done, poll returns the finished run and watchPollMsg auto-responds.
	state = "done"
	res := pollWatchedRuns(m.watched, a.Tools.Hev)
	if len(res) != 1 || res[0].State != "done" {
		t.Fatalf("poll results = %+v", res)
	}
	_, cmd := m.Update(watchPollMsg{results: res})
	drain(t, m, cmd)
	if len(m.watched) != 0 {
		t.Fatalf("watched not cleared: %+v", m.watched)
	}
	if !strings.Contains(plain(m), "mini/smoke-1 finished cleanly") {
		t.Fatalf("auto-response missing:\n%s", plain(m))
	}
}

func TestParseRealDispatchShowEnvelope(t *testing.T) {
	// Matches actual `hev dispatch show NAME --host HOST --json` output.
	raw := []byte(`{"files":{"result.json":"{\"outcome\":\"done\",\"exit_code\":0,\"head_sha\":\"abc1234\"}","status.md":"working\ndone\n","report.md":"All checks passed. PR https://github.com/hev/kit/pull/345"}}`)
	wr, ok := parseShowResponse(watchedRun{Name: "my-run", Host: "oven"}, raw)
	if !ok || wr.State != "done" || wr.HeadSHA != "abc1234" || wr.PR != "https://github.com/hev/kit/pull/345" {
		t.Fatalf("parsed = %+v ok=%v", wr, ok)
	}
	// Still-running envelope (no result.json, non-terminal status.md) must not trigger completion.
	running := []byte(`{"files":{"status.md":"Running tests...\n"}}`)
	if _, ok := parseShowResponse(watchedRun{Name: "my-run", Host: "oven"}, running); ok {
		t.Fatal("running envelope should not be terminal")
	}
	// Pre-written in-flight result.json with zero Go time ("0001-01-01T00:00:00Z")
	// and default exit_code=1 must NOT be treated as failed.
	inFlight := []byte(`{"files":{"result.json":"{\"harness\":\"codex\",\"exit_code\":1,\"start\":\"2026-11-23T01:38:59Z\",\"end\":\"0001-01-01T00:00:00Z\"}","status.md":"READY: holding for steering\n"}}`)
	if wr, ok := parseShowResponse(watchedRun{Name: "my-run", Host: "mini"}, inFlight); ok {
		t.Fatalf("in-flight zero-end result.json falsely reported terminal: %+v", wr)
	}
}

func TestTUIScrollAndSlashTypeahead(t *testing.T) {
	m, _ := newTestTUI(t, &scripted{})
	// 1. Typing "/" shows all slash commands.
	typeText(m, "/")
	v := plain(m)
	for _, want := range []string{"/dispatch", "/model", "/prompt", "/help"} {
		if !strings.Contains(v, want) {
			t.Fatalf("typeahead for '/' missing %q:\n%s", want, v)
		}
	}
	// 2. Typing "dis" narrows to /dispatch and Tab autocompletes "/dispatch ".
	typeText(m, "dis")
	if matches := m.matchingCommands(); len(matches) != 1 || matches[0].Name != "/dispatch" {
		t.Fatalf("matches for /dis = %+v", matches)
	}
	m.Update(key(tea.KeyTab))
	if got := m.input.Value(); got != "/dispatch " {
		t.Fatalf("Tab completion = %q, want \"/dispatch \"", got)
	}
	m.input.Reset()

	// 2b. Vim-style ":" command palette and :q exit.
	typeText(m, ":q")
	if matches := m.matchingCommands(); len(matches) != 1 || matches[0].Name != "/quit" {
		t.Fatalf("matches for :q = %+v", matches)
	}
	m.input.Reset()
	if cmd := submit(m, ":q"); cmd == nil || !m.quitting {
		t.Fatalf(":q did not quit (quitting=%v)", m.quitting)
	}
	m.quitting = false

	// 3. Scrollback works with PgUp / mouse wheel / End.
	for i := 0; i < 40; i++ {
		m.system("scroll-line-" + string(rune('a'+(i%26))))
	}
	m.Update(key(tea.KeyPgUp))
	if m.scroll == 0 || !strings.Contains(plain(m), "scrolled") {
		t.Fatalf("PgUp did not scroll: scroll=%d", m.scroll)
	}
	m.Update(key(tea.KeyEnd))
	if m.scroll != 0 {
		t.Fatalf("End did not reset scroll: scroll=%d", m.scroll)
	}
}

func TestPeerBackgroundVsInterruptModes(t *testing.T) {
	// 1. Background mode (default): shows in lower pane, does NOT touch main thread history or entries.
	p := &scripted{replies: []ChatResult{
		{ToolCalls: []ToolCall{call("p1", "hev_peers", `{"action":"send","target":"foh-kit-6","message":"pong"}`)}},
		{Content: "sent pong"},
	}}
	m, a := newTestTUI(t, p)
	var calls []hevCall
	a.Tools = NewToolset(t.TempDir(), fakeHev(&calls, `ok`, nil))
	cmd := submit(m, "[peer message from foh-kit-6 (agent)]\nping")
	if !strings.Contains(plain(m), "⇄ ← foh-kit-6: ping") {
		t.Fatalf("lower pane missing incoming peer message:\n%s", plain(m))
	}
	if len(m.entries) != 0 || len(a.History()) != 0 {
		t.Fatalf("background peer message polluted main thread: entries=%d history=%d", len(m.entries), len(a.History()))
	}
	// Run the background sidecar command to completion.
	if batch, ok := cmd().(tea.BatchMsg); ok {
		for _, sub := range batch {
			if sub == nil {
				continue
			}
			if done, ok := sub().(peerBgDoneMsg); ok {
				m.Update(done)
			}
		}
	}
	if !strings.Contains(plain(m), "⇄ → foh-kit-6: pong") {
		t.Fatalf("lower pane missing outgoing peer reply:\n%s", plain(m))
	}
	if len(m.entries) != 0 || len(a.History()) != 0 {
		t.Fatalf("background peer reply polluted main thread: entries=%d history=%d", len(m.entries), len(a.History()))
	}

	// 2. Interrupt mode ([interrupt]): shows in lower pane AND interrupts main thread.
	p2 := &scripted{replies: []ChatResult{{Content: "Surfaced to user."}}}
	m2, a2 := newTestTUI(t, p2)
	drain(t, m2, submit(m2, "[peer message from foh-kit-6 (agent)]\n[interrupt] need approval"))
	v2 := plain(m2)
	if !strings.Contains(v2, "⇄ ← foh-kit-6: [interrupt] need approval") || !strings.Contains(v2, "Surfaced to user.") {
		t.Fatalf("interrupt mode missing lower pane or main thread output:\n%s", v2)
	}
	if len(a2.History()) == 0 {
		t.Fatal("interrupt mode did not update main thread history")
	}
}

func TestSteerOverlayAndDirectSend(t *testing.T) {
	m, a := newTestTUI(t, &scripted{})
	var calls []hevCall
	a.Tools = NewToolset(t.TempDir(), fakeHev(&calls, `{"oven":{"rows":[{"name":"run-1","kind":"run","state":"running"},{"name":"old-1","kind":"run","state":"done"}]}}`, nil))
	// Typeahead /st matches /steer.
	typeText(m, "/st")
	if matches := m.matchingCommands(); len(matches) == 0 || matches[0].Name != "/steer" {
		t.Fatalf("matches for /st = %+v", matches)
	}
	m.input.Reset()
	// /steer --urgent oven/run-1 stop and rebase sends via hev steer and logs receipt + tracks ACK.
	a.Tools = NewToolset(t.TempDir(), fakeHev(&calls, `st-01TESTACK urgent queued for oven/run-1`, nil))
	cmd := submit(m, "/steer --urgent oven/run-1 stop and rebase")
	if cmd == nil {
		t.Fatal("expected steer send cmd")
	}
	msg := cmd()
	m.Update(msg)
	if !strings.Contains(plain(m), "st-01TESTACK") {
		t.Fatalf("lower bar missing steer receipt:\n%s", plain(m))
	}
	if len(m.watchedSteers) != 1 || m.watchedSteers[0].ID != "st-01TESTACK" {
		t.Fatalf("watchedSteers = %+v", m.watchedSteers)
	}
	// When poll reports acked, lower bar updates to ✓ acked and clears watchedSteers.
	m.Update(steerPollMsg{updates: map[string]string{"st-01TESTACK": "acked"}})
	if len(m.watchedSteers) != 0 || !strings.Contains(plain(m), "✓ acked") {
		t.Fatalf("steer ACK not reflected: watched=%+v\n%s", m.watchedSteers, plain(m))
	}
	if len(m.entries) != 0 {
		t.Fatalf("direct /steer should stay in lower bar, not main entries: %+v", m.entries)
	}
}
