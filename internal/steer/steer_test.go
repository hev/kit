package steer

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hev/kit/internal/hooks"
)

const ack = "/x/hev steer ack"

func event(t *testing.T, name, raw string) hooks.Event {
	t.Helper()
	ev, err := hooks.Normalize("claude", name, []byte(raw), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return ev
}

var (
	prompt  = `{"session_id":"s1","hook_event_name":"UserPromptSubmit","prompt":"work"}`
	readPre = `{"session_id":"s1","hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"sleep 20; echo pending"}}`
	readPst = `{"session_id":"s1","hook_event_name":"PostToolUse","tool_name":"Bash","tool_input":{"command":"sleep 20; echo pending"},"tool_response":"pending"}`
	writePr = `{"session_id":"s1","hook_event_name":"PreToolUse","tool_name":"Write","tool_input":{"file_path":"progress.md","content":"x"}}`
	ackPre  = `{"session_id":"s1","hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"/x/hev steer ack st-1"}}`
	stopEv  = `{"session_id":"s1","hook_event_name":"Stop"}`
)

func handle(t *testing.T, s Store, name, raw string) hooks.Response {
	t.Helper()
	r, err := s.Handle(event(t, name, raw), ack, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestUrgentInjectedOnceAndBlocksMutationsUntilAck(t *testing.T) {
	s := Store{Dir: t.TempDir(), Target: "dispatch:demo"}
	handle(t, s, "prompt-submit", prompt)
	handle(t, s, "pre-tool", readPre)
	handle(t, s, "post-tool", readPst)

	note, _ := s.Enqueue(Note, "also update the docs", "foh", TurnBoundary, 0)
	urgent, err := s.Enqueue(Urgent, "scope changed: use candidate B", "foh", ToolBoundary, 0)
	if err != nil {
		t.Fatal(err)
	}
	// A read before the next boundary is allowed.
	if r := handle(t, s, "pre-tool", readPre); r.Decision != "" || r.Context != "" {
		t.Fatalf("read held: %+v", r)
	}
	// The post-tool boundary injects the urgent, once.
	r := handle(t, s, "post-tool", readPst)
	if !strings.Contains(r.Context, "URGENT steering message "+urgent.ID) || !strings.Contains(r.Context, "scope changed") || !strings.Contains(r.Context, ack+" "+urgent.ID) {
		t.Fatalf("injection: %+v", r)
	}
	if strings.Contains(r.Context, "also update the docs") {
		t.Fatal("note injected mid-turn")
	}
	m, _ := s.Get(urgent.ID)
	if m.State != Injected || m.Injected.Turn != 1 || m.Injected.ToolCall != 2 || m.Injected.Event != "post-tool" || m.Injected.Session != "s1" {
		t.Fatalf("receipt: %+v %+v", m, m.Injected)
	}
	if r := handle(t, s, "post-tool", readPst); r.Context != "" {
		t.Fatalf("injected twice: %+v", r)
	}
	// Reads continue; mutations are blocked with a reason naming the ack.
	if r := handle(t, s, "pre-tool", readPre); r.Decision != "" {
		t.Fatalf("read blocked: %+v", r)
	}
	r = handle(t, s, "pre-tool", writePr)
	if r.Decision != hooks.Block || !strings.Contains(r.Reason, ack+" "+urgent.ID) || strings.Contains(r.Reason, "scope changed") {
		t.Fatalf("mutation not held (or message repeated): %+v", r)
	}
	if r := handle(t, s, "pre-tool", ackPre); r.Decision != "" {
		t.Fatalf("ack command blocked: %+v", r)
	}
	// Acking a queued note is refused: the agent was never shown it.
	if _, err := s.Ack(note.ID, "agent"); !errors.Is(err, ErrNotInjected) {
		t.Fatalf("queued ack: %v", err)
	}
	if m, err = s.Ack(urgent.ID, "agent"); err != nil || m.State != Acked || m.Acked.Turn != 1 || m.Blocked != 1 {
		t.Fatalf("ack: %+v %v", m, err)
	}
	if r := handle(t, s, "pre-tool", writePr); r.Decision != "" {
		t.Fatalf("mutation still blocked after ack: %+v", r)
	}
	// The note waits for the next turn, then is injected there.
	if m, _ := s.Get(note.ID); m.State != Queued {
		t.Fatalf("note state %s before next turn", m.State)
	}
	r = handle(t, s, "prompt-submit", prompt)
	if !strings.Contains(r.Context, "also update the docs") {
		t.Fatalf("note not injected at next turn: %+v", r)
	}
	if m, _ := s.Get(note.ID); m.State != Injected || m.Injected.Turn != 2 || m.Injected.Event != "prompt-submit" {
		t.Fatalf("note receipt %+v", m.Injected)
	}
	c := s.Counts()
	if c.Queued != 0 || c.Injected != 1 || c.Acked != 1 || c.String() != "q0 i1 a1" {
		t.Fatalf("counts %+v %s", c, c)
	}
}

func TestUrgentInjectedByBlockWhenNextCallMutates(t *testing.T) {
	s := Store{Dir: t.TempDir(), Target: "x"}
	handle(t, s, "prompt-submit", prompt)
	u, _ := s.Enqueue(Urgent, "stop pushing", "", ToolBoundary, 0)
	r := handle(t, s, "pre-tool", writePr)
	if r.Decision != hooks.Block || !strings.Contains(r.Reason, "stop pushing") {
		t.Fatalf("%+v", r)
	}
	if m, _ := s.Get(u.ID); m.State != Injected || m.Injected.Event != "pre-tool" || m.Injected.ToolCall != 1 {
		t.Fatalf("%+v", m.Injected)
	}
	if r := handle(t, s, "post-tool", readPst); r.Context != "" {
		t.Fatal("re-injected at post-tool")
	}
}

func TestParallelHooksInjectOnce(t *testing.T) {
	s := Store{Dir: t.TempDir(), Target: "x"}
	u, _ := s.Enqueue(Urgent, "once", "", ToolBoundary, 0)
	var wg sync.WaitGroup
	var mu sync.Mutex
	injected := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := s.Handle(event(t, "post-tool", readPst), ack, time.Now())
			if err != nil {
				t.Error(err)
				return
			}
			if strings.Contains(r.Context, u.ID) {
				mu.Lock()
				injected++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if injected != 1 {
		t.Fatalf("injected %d times", injected)
	}
}

func TestHaltBlocksEveryToolAndEndsTurn(t *testing.T) {
	s := Store{Dir: t.TempDir(), Target: "dispatch:demo"}
	handle(t, s, "prompt-submit", prompt)
	h, _ := s.Enqueue(HaltL, "stop now", "foh", ToolBoundary, 10*time.Minute)
	if h.HaltTimeout != "10m0s" {
		t.Fatal(h.HaltTimeout)
	}
	for _, raw := range []string{readPre, writePr, ackPre} {
		r := handle(t, s, "pre-tool", raw)
		if r.Decision != hooks.Halt || !strings.Contains(r.Reason, "End your turn now") || !strings.Contains(r.Reason, "stop now") {
			t.Fatalf("not halted: %+v", r)
		}
	}
	m, _ := s.Get(h.ID)
	if m.State != Injected || m.Injected.ToolCall != 1 || m.Blocked != 3 {
		t.Fatalf("%+v", m)
	}
	if c := s.Counts(); !c.Halting || c.Halted {
		t.Fatalf("%+v", c)
	}
	handle(t, s, "stop", stopEv)
	m, _ = s.Get(h.ID)
	if m.State != Acked || m.Acked.By != "turn ended" {
		t.Fatalf("%+v", m.Acked)
	}
	if c := s.Counts(); c.Halting || !c.Halted || !strings.Contains(c.String(), "halted") {
		t.Fatalf("%+v", c)
	}
	// The next turn runs normally.
	if r := handle(t, s, "prompt-submit", prompt); r.Decision != "" {
		t.Fatalf("%+v", r)
	}
	if r := handle(t, s, "pre-tool", writePr); r.Decision != "" {
		t.Fatalf("%+v", r)
	}
	if s.LoadState().Halted {
		t.Fatal("halted survived a new turn")
	}
}

func TestHaltAtPostToolAndEndHaltsOnExit(t *testing.T) {
	s := Store{Dir: t.TempDir(), Target: "dispatch:demo"}
	h, _ := s.Enqueue(Redirect, "switch to plan B", "", ToolBoundary, time.Minute)
	if r := handle(t, s, "post-tool", readPst); r.Decision != hooks.Halt || !strings.Contains(r.Reason, "REDIRECT") {
		t.Fatalf("%+v", r)
	}
	if m, ok := s.ActiveHalt(); !ok || m.ID != h.ID {
		t.Fatal("no active halt")
	}
	ended, found, err := s.EndHalts("turn ended (harness exited)")
	if err != nil || !found || ended.ID != h.ID || ended.State != Acked || ended.Acked.By != "turn ended (harness exited)" {
		t.Fatalf("%+v %v %v", ended, found, err)
	}
	if _, ok := s.ActiveHalt(); ok || !s.LoadState().Halted {
		t.Fatal("halt not closed")
	}
}

var startEv = `{"session_id":"s2","hook_event_name":"SessionStart","source":"startup"}`

// supervised is a target whose supervisor wires kit's hooks into each launch.
func supervised(t *testing.T) Store {
	t.Helper()
	s := Store{Dir: t.TempDir(), Target: "dispatch:demo"}
	if err := writeJSON(filepath.Join(s.Dir, WiredFile), map[string]any{"harness": "claude"}); err != nil {
		t.Fatal(err)
	}
	return s
}

// A halt or redirect the supervisor accepts while it launches the harness
// (wiring marker written, no hook fired yet) survives SessionStart and stops
// the first prompt; the supervisor then sees the halt that ended the attempt.
func TestHaltDuringStartupStopsFirstPrompt(t *testing.T) {
	for _, level := range []string{HaltL, Redirect} {
		t.Run(level, func(t *testing.T) {
			s := supervised(t)
			h, _ := s.Enqueue(level, "stop before any work", "foh", ToolBoundary, time.Minute)
			note, _ := s.Enqueue(Note, "later", "foh", TurnBoundary, 0)
			if r := handle(t, s, "session-start", startEv); r.Decision != "" {
				t.Fatalf("session-start: %+v", r)
			}
			if m, _ := s.Get(h.ID); m.State != Queued {
				t.Fatalf("startup discarded the halt: %+v", m)
			}
			r := handle(t, s, "prompt-submit", prompt)
			if r.Decision != hooks.Halt || !strings.Contains(r.Reason, "stop before any work") || !strings.Contains(r.Reason, "End your turn now") || r.Context != "" {
				t.Fatalf("first prompt not halted: %+v", r)
			}
			m, _ := s.Get(h.ID)
			if m.State != Injected || m.Injected.Event != "prompt-submit" || m.Injected.Turn != 1 {
				t.Fatalf("%+v %+v", m, m.Injected)
			}
			if n, _ := s.Get(note.ID); n.State != Queued {
				t.Fatalf("note injected into a halted prompt: %+v", n)
			}
			ended, found, err := s.EndHalts("turn ended (harness exited)")
			if err != nil || !found || ended.ID != h.ID || ended.Level != level {
				t.Fatalf("%+v %v %v", ended, found, err)
			}
			if !s.LoadState().Halted {
				t.Fatal("not halted")
			}
		})
	}
}

// A halt still queued from an attempt that ended (closed by EndHalts on
// exit, so only a crash or race could leave one) is closed at the next start,
// not delivered to the new attempt; one admitted after that closure is.
func TestStaleHaltFromEndedAttemptIsClosedAtStart(t *testing.T) {
	s := supervised(t)
	stale, _ := s.Enqueue(HaltL, "old", "", ToolBoundary, time.Minute)
	if _, _, err := s.EndHalts("turn ended (harness exited)"); err != nil {
		t.Fatal(err)
	}
	// Simulate a halt the closure missed: still queued, but queued before it.
	m, _ := s.Get(stale.ID)
	m.State, m.Acked = Queued, nil
	if err := s.Put(m); err != nil {
		t.Fatal(err)
	}
	fresh, _ := s.Enqueue(HaltL, "new", "", ToolBoundary, time.Minute)
	handle(t, s, "session-start", startEv)
	if m, _ := s.Get(stale.ID); m.State != Acked || m.Acked.By != "new session" {
		t.Fatalf("stale halt kept: %+v", m)
	}
	if m, _ := s.Get(fresh.ID); m.State != Queued {
		t.Fatalf("current halt discarded: %+v", m)
	}
	if r := handle(t, s, "prompt-submit", prompt); r.Decision != hooks.Halt || !strings.Contains(r.Reason, fresh.ID) {
		t.Fatalf("%+v", r)
	}
}

// Without a supervisor (a peers session) there is no attempt boundary: a
// halt queued while nothing ran is closed when the next turn starts.
func TestUnsupervisedHaltQueuedWhileIdleIsClosedAtNextPrompt(t *testing.T) {
	s := Store{Dir: t.TempDir(), Target: "peers:demo"}
	h, _ := s.Enqueue(HaltL, "stop", "", ToolBoundary, time.Minute)
	if r := handle(t, s, "prompt-submit", prompt); r.Decision != "" {
		t.Fatalf("%+v", r)
	}
	if m, _ := s.Get(h.ID); m.State != Acked || m.Acked.By != "turn ended" {
		t.Fatalf("%+v", m)
	}
}

func TestIDsAndResolve(t *testing.T) {
	id := NewID()
	if !ValidID(id) || ValidID("st-../../etc") || ValidID("x") {
		t.Fatal(id)
	}
	home, root := t.TempDir(), t.TempDir()
	env := map[string]string{}
	getenv := func(k string) string { return env[k] }
	env[EnvDir], env[EnvTarget] = "/d/steer", "dispatch:a"
	if s, ok := Resolve(getenv, "", home, root, nil); !ok || s.Dir != "/d/steer" || s.Target != "dispatch:a" {
		t.Fatal(s)
	}
	delete(env, EnvDir)
	if _, ok := Resolve(getenv, "/elsewhere", home, root, nil); ok {
		t.Fatal("resolved outside any target")
	}
	env["TMUX_PANE"] = "%1"
	if s, ok := Resolve(getenv, "/elsewhere", home, root, func() string { return "foh-x" }); !ok || s.Target != "peers:foh-x" {
		t.Fatal(s)
	}
	if _, ok := Resolve(getenv, "/elsewhere", home, root, func() string { return "../x" }); ok {
		t.Fatal("unsafe session name")
	}
	s, _ := PeersStore(home, "foh-x")
	m, _ := s.Enqueue(Urgent, "hi", "", ToolBoundary, 0)
	if got, _, err := Find(m.ID, nil, home, root); err != nil || got.Target != "peers:foh-x" {
		t.Fatal(err)
	}
}
