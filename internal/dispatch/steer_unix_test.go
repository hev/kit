//go:build !windows

package dispatch

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/hooks"
	"github.com/hev/kit/internal/steer"
)

func TestStateHalted(t *testing.T) {
	r := &Result{End: time.Now(), ExitCode: 0, Halted: true}
	if got := State(r, false); got != "halted" {
		t.Fatal(got)
	}
	if got := State(r, true); got != "running" {
		t.Fatal(got)
	}
	r.Halted = false
	if got := State(r, false); got != "done" {
		t.Fatal(got)
	}
}

func TestWireHooksPerInvocation(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "steer")
	args, env := wireHooks("claude", dir, "dispatch:x", []string{"-p", "--verbose", "PROMPT"}, nil)
	if args[len(args)-1] != "PROMPT" || args[len(args)-3] != "--settings" {
		t.Fatalf("%q", args)
	}
	var settings struct {
		Hooks map[string][]struct {
			Hooks []struct{ Command string }
		}
	}
	if err := json.Unmarshal([]byte(args[len(args)-2]), &settings); err != nil {
		t.Fatal(err)
	}
	if cmd := settings.Hooks["PreToolUse"][0].Hooks[0].Command; !strings.HasSuffix(cmd, " hook pre-tool --harness claude") {
		t.Fatalf("%q", cmd)
	}
	if len(settings.Hooks) != 6 {
		t.Fatalf("%v", settings.Hooks)
	}
	joined := strings.Join(env, "\n")
	for _, want := range []string{steer.EnvDir + "=" + dir, steer.EnvTarget + "=dispatch:x", "HEV_HOOK_INVOCATION=1"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("env %q missing %s", env, want)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, wiredFile)); err != nil {
		t.Fatal("launch not recorded as wired")
	}
	args, _ = wireHooks("codex", dir, "dispatch:x", []string{"exec", "--json", "PROMPT"}, nil)
	if args[2] != "--dangerously-bypass-hook-trust" || args[len(args)-1] != "PROMPT" {
		t.Fatalf("%q", args)
	}
	if args, env := wireHooks("pi", dir, "x", []string{"PROMPT"}, nil); len(args) != 1 || env != nil {
		t.Fatal("pi has no verified hook mechanism")
	}
}

func TestAfterHaltMarksHaltedResult(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "run")
	store := steer.Store{Dir: filepath.Join(dir, "steer")}
	m, _ := store.Enqueue(steer.HaltL, "stop", "", steer.ToolBoundary, time.Minute)
	// Not injected yet and not escalated: the run was not halted by it.
	var r Result
	afterHalt(dir, "run", &r)
	if r.Halted {
		t.Fatal("halted by a halt it never saw")
	}
	m2, _ := store.Enqueue(steer.HaltL, "stop again", "", steer.ToolBoundary, time.Minute)
	store.Update(m2.ID, func(x *steer.Message) { x.State = steer.Injected; x.Injected = &steer.Mark{At: time.Now()} })
	r = Result{End: time.Now()}
	afterHalt(dir, "run", &r)
	if !r.Halted || r.HaltID != m2.ID || State(&r, false) != "halted" {
		t.Fatalf("%+v", r)
	}
	if got, _ := store.Get(m.ID); got.State != steer.Acked {
		t.Fatal("first halt not closed")
	}
	if c := store.Counts(); !c.Halted || c.Halting {
		t.Fatalf("%+v", c)
	}
}

func TestAfterHaltHonoursHaltClosedByStopHook(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "run")
	store := steer.Store{Dir: filepath.Join(dir, "steer")}
	start := time.Now().UTC()
	m, _ := store.Enqueue(steer.HaltL, "stop", "", steer.ToolBoundary, time.Minute)
	ev := func(name, raw string) {
		e, err := hooks.Normalize("codex", name, []byte(raw), time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.Handle(e, "ack", time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	ev("post-tool", `{"session_id":"s","hook_event_name":"PostToolUse","tool_name":"Bash","tool_input":{"command":"./fake-ci"}}`)
	ev("stop", `{"session_id":"s","hook_event_name":"Stop"}`)
	r := Result{Start: start, End: time.Now()}
	afterHalt(dir, "run", &r)
	if !r.Halted || r.HaltID != m.ID {
		t.Fatalf("%+v", r)
	}
	// A halt from an earlier attempt does not mark a later one.
	r = Result{Start: time.Now().Add(time.Minute), End: time.Now()}
	afterHalt(dir, "run", &r)
	if r.Halted {
		t.Fatal("earlier attempt's halt marked this one")
	}
}

func TestEscalateHaltAfterTimeout(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "run")
	store := steer.Store{Dir: filepath.Join(dir, "steer")}
	m, _ := store.Enqueue(steer.HaltL, "stop", "", steer.ToolBoundary, time.Hour)
	if escalateHalt(dir) {
		t.Fatal("escalated early")
	}
	store.Update(m.ID, func(x *steer.Message) { x.HaltTimeout = "1ms" })
	time.Sleep(5 * time.Millisecond)
	if !escalateHalt(dir) {
		t.Fatal("not escalated")
	}
	got, _ := store.Get(m.ID)
	if got.Escalated == nil || !strings.Contains(got.Escalated.By, "1ms") {
		t.Fatalf("%+v", got.Escalated)
	}
	r := Result{End: time.Now(), HaltEscalated: true}
	afterHalt(dir, "run", &r)
	if !r.Halted {
		t.Fatal("escalated halt not shown as halted")
	}
}

func TestCheckpointCommitsDirtyWorktree(t *testing.T) {
	wt := t.TempDir()
	run := func(args ...string) string {
		c := exec.Command("git", args...)
		c.Dir = wt
		c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		b, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %s", err, b)
		}
		return strings.TrimSpace(string(b))
	}
	run("init", "-q")
	run("config", "user.name", "t")
	run("config", "user.email", "t@t")
	os.WriteFile(filepath.Join(wt, "a"), []byte("1"), 0600)
	run("add", "a")
	run("commit", "-qm", "init")
	if note, err := checkpoint(wt, "st-x"); err != nil || !strings.Contains(note, "clean") {
		t.Fatal(note, err)
	}
	os.WriteFile(filepath.Join(wt, "b"), []byte("work in progress"), 0600)
	note, err := checkpoint(wt, "st-x")
	if err != nil || !strings.Contains(note, "checkpoint commit") {
		t.Fatal(note, err)
	}
	if msg := run("log", "-1", "--format=%s"); msg != "checkpoint: redirect st-x (hev steer)" {
		t.Fatal(msg)
	}
	if st := run("status", "--porcelain"); st != "" {
		t.Fatal(st)
	}
}

func TestSteerRPCRefusesHaltOnEndedRunAndCountsReceipts(t *testing.T) {
	name := "steer-ended"
	dir := filepath.Join(Root(), name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	m := Meta{Order: Order{Name: name, Host: "here", Repo: "x"}, Start: time.Now()}
	if err := WriteJSON(filepath.Join(dir, "meta.json"), m); err != nil {
		t.Fatal(err)
	}
	WriteJSON(filepath.Join(dir, "result.json"), Result{End: time.Now()})
	if _, err := Handle(Request{Action: "steer", Name: name, Note: "stop", Level: "halt"}); err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("%v", err)
	}
	if _, err := Handle(Request{Action: "steer", Name: name, Note: "x", Level: "bogus"}); err == nil {
		t.Fatal("bogus level accepted")
	}
	if _, err := Handle(Request{Action: "steer", Name: name, Level: "urgent"}); err == nil {
		t.Fatal("empty message accepted")
	}
	store := steer.RunStore(Root(), name)
	store.Enqueue(steer.Urgent, "a", "", steer.ToolBoundary, 0)
	r, err := Handle(Request{Action: "list"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range r.Rows {
		if row.Name == name {
			found = true
			if row.Steer == nil || row.Steer.Queued != 1 || row.Steer.String() != "q1 i0 a0" {
				t.Fatalf("%+v", row.Steer)
			}
		}
	}
	if !found {
		t.Fatal("row missing")
	}
	show, err := Handle(Request{Action: "show", Name: name})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(show.Message, "steer: 1 queued, 0 injected, 0 acked") || !strings.Contains(show.Files["steer.json"], `"level": "urgent"`) {
		t.Fatalf("%q %q", show.Message, show.Files["steer.json"])
	}
}

// gitWorktree makes dir/wt a git repository with one commit.
func gitWorktree(t *testing.T, dir string) string {
	t.Helper()
	wt := filepath.Join(dir, "wt")
	os.MkdirAll(wt, 0700)
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.name", "t"}, {"config", "user.email", "t@t"}, {"commit", "-q", "--allow-empty", "-m", "init"}} {
		c := exec.Command("git", args...)
		c.Dir = wt
		if b, err := c.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, b)
		}
	}
	return wt
}

// Review finding 2: a halt delivered by the harness interrupt (no kit hooks)
// must leave the run halted, and a redirect must checkpoint and continue.
func TestInterruptFallbackHaltsAndRedirects(t *testing.T) {
	var followed []string
	defer func(f func(string, string, string) error) { startRedirectFollower = f }(startRedirectFollower)
	startRedirectFollower = func(_, name, id string) error { followed = append(followed, name+" "+id); return nil }

	dir := filepath.Join(t.TempDir(), "run")
	store := steer.Store{Dir: filepath.Join(dir, "steer"), Target: "dispatch:run"}
	m, _ := store.Enqueue(steer.HaltL, "stop", "", steer.TurnBoundary, time.Minute)
	sent := 0
	if err := deliverByInterrupt(store, m.ID, func() error { sent++; return nil }); err != nil || sent != 1 {
		t.Fatal(err, sent)
	}
	if got, _ := store.Get(m.ID); got.State != steer.Injected || got.Injected == nil || got.Injected.Event != "interrupt" {
		t.Fatalf("%+v", got)
	}
	start := time.Now()
	r := Result{Start: start, End: time.Now()}
	afterHalt(dir, "run", &r)
	if !r.Halted || r.HaltID != m.ID || State(&r, false) != "halted" {
		t.Fatalf("%+v", r)
	}
	if got, _ := store.Get(m.ID); got.State != steer.Acked || got.Acked.Event != "exit" {
		t.Fatalf("%+v", got)
	}

	// A failed interrupt leaves the halt queued for the hard stop.
	m2, _ := store.Enqueue(steer.HaltL, "stop", "", steer.TurnBoundary, time.Minute)
	if err := deliverByInterrupt(store, m2.ID, func() error { return os.ErrProcessDone }); err == nil {
		t.Fatal("interrupt error lost")
	}
	if got, _ := store.Get(m2.ID); got.State != steer.Queued || got.Injected != nil {
		t.Fatalf("%+v", got)
	}
	store.Update(m2.ID, func(x *steer.Message) { x.State = steer.Acked })

	// Redirect by interrupt: checkpoint and follower.
	wt := gitWorktree(t, dir)
	os.WriteFile(filepath.Join(wt, "wip"), []byte("x"), 0600)
	m3, _ := store.Enqueue(steer.Redirect, "do y instead", "", steer.TurnBoundary, time.Minute)
	if err := deliverByInterrupt(store, m3.ID, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	r = Result{Start: time.Now(), End: time.Now()}
	afterHalt(dir, "run", &r)
	if !r.Halted || r.HaltID != m3.ID || !strings.Contains(r.Redirect, "checkpoint commit") || len(followed) != 1 || followed[0] != "run "+m3.ID {
		t.Fatalf("%+v %v", r, followed)
	}

	// Redirect whose interrupt failed and which the hard stop ended.
	os.WriteFile(filepath.Join(wt, "wip2"), []byte("x"), 0600)
	m4, _ := store.Enqueue(steer.Redirect, "and z", "", steer.TurnBoundary, time.Minute)
	r = Result{Start: time.Now(), End: time.Now(), HaltEscalated: true}
	afterHalt(dir, "run", &r)
	if !r.Halted || r.HaltID != m4.ID || len(followed) != 2 {
		t.Fatalf("%+v %v", r, followed)
	}
	if got, _ := store.Get(m4.ID); got.Injected == nil || got.Injected.Event != "hard-stop" || got.State != steer.Acked {
		t.Fatalf("%+v", got)
	}
}

// Review finding 4: an orchestrator round escalates a halt past its timeout
// and closes halt receipts when the harness exits.
func TestOrchestratorRoundHaltEscalatesAndCloses(t *testing.T) {
	defer func(d time.Duration) { roundHaltPoll = d }(roundHaltPoll)
	roundHaltPoll = 5 * time.Millisecond
	orch := t.TempDir()
	store := steer.Store{Dir: filepath.Join(orch, "steer"), Target: "orch:x"}

	// A halt the round never honours: hard stop after its timeout.
	m, _ := store.Enqueue(steer.HaltL, "stop", "", steer.ToolBoundary, time.Hour)
	store.Update(m.ID, func(x *steer.Message) { x.HaltTimeout = "20ms" })
	killed := make(chan struct{})
	start := time.Now()
	err := superviseOrchestratorRound(orch, func() error { <-killed; return os.ErrProcessDone }, func() { close(killed) })
	if err == nil || !strings.Contains(err.Error(), "hard-stopped") || time.Since(start) > 2*time.Second {
		t.Fatalf("%v after %s", err, time.Since(start))
	}
	got, _ := store.Get(m.ID)
	if got.State != steer.Acked || got.Escalated == nil || got.Injected == nil || got.Injected.Event != "hard-stop" {
		t.Fatalf("%+v", got)
	}
	if c := store.Counts(); !c.Halted || c.Halting {
		t.Fatalf("%+v", c)
	}

	// A halt the round honoured (injected by its hook) is closed on exit.
	m2, _ := store.Enqueue(steer.HaltL, "stop", "", steer.ToolBoundary, time.Hour)
	store.Update(m2.ID, func(x *steer.Message) { x.State = steer.Injected; x.Injected = &steer.Mark{At: time.Now()} })
	if err := superviseOrchestratorRound(orch, func() error { return nil }, func() { t.Fatal("killed") }); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.Get(m2.ID); got.State != steer.Acked || got.Escalated != nil {
		t.Fatalf("%+v", got)
	}
	if st := store.LoadState(); !st.Halted || st.HaltID != m2.ID {
		t.Fatalf("%+v", st)
	}
}

// An unwired orchestrator round gets the harness interrupt, not a
// tool-boundary label.
func TestSteerOrchestratorHaltWithoutHooksInterrupts(t *testing.T) {
	orchHome(t)
	s, e := ApplyOrchestrator(strings.Replace(orchestratorExample, "release-train", "steer-halt", 1), false)
	if e != nil {
		t.Fatal(e)
	}
	child := exec.Command("sleep", "30")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- child.Wait() }()
	defer child.Process.Kill()
	if err := recordHarness(OrchestratorDir(s.Name), child); err != nil {
		t.Fatal(err)
	}
	r, err := Handle(Request{Action: "steer", Name: "o-" + s.Name, Note: "stop now", Level: steer.HaltL, HaltTimeout: "1m"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Delivery != steer.TurnBoundary || !strings.Contains(r.Message, "interrupt (SIGINT)") || !strings.Contains(r.Message, "hard stop after 1m0s") {
		t.Fatalf("%+v", r)
	}
	select {
	case err := <-exited:
		if ee, ok := err.(*exec.ExitError); !ok || ee.String() != "signal: interrupt" {
			t.Fatalf("harness ended with %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("harness not interrupted")
	}
	store := steer.OrchestratorStore(Root(), s.Name)
	if got, _ := store.Get(r.SteerID); got.State != steer.Injected || got.Injected.Event != "interrupt" {
		t.Fatalf("%+v", got)
	}
}
