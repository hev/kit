//go:build !windows

package dispatch

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type traceFlow struct {
	t      *testing.T
	name   string
	dir    string
	live   bool
	caller Caller
}

func newTraceFlow(t *testing.T) *traceFlow {
	t.Helper()
	clearTraceEnv(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	source := filepath.Join(t.TempDir(), "source")
	if err := os.MkdirAll(source, 0700); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "init", "-b", "main"},
		{"git", "-c", "user.name=Test", "-c", "user.email=test@example.test", "commit", "--allow-empty", "-m", "initial"},
	} {
		if _, err := command(source, args...); err != nil {
			t.Fatal(err)
		}
	}
	repo := filepath.Join(t.TempDir(), "repo")
	if _, err := command("", "git", "clone", source, repo); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(home, "dispatch.toml")
	if err := os.WriteFile(cfg, []byte("cap = 4\n[validation]\nhost = \"v\"\nharness = \"claude\"\nallow_same_host = true\n[hosts.here]\nlocal = true\nconfig_path = \""+cfg+"\"\n[hosts.v]\nlocal = true\nconfig_path = \""+cfg+"\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEV_DISPATCH_CONFIG", cfg)
	f := &traceFlow{t: t, name: "traced", dir: filepath.Join(Root(), "traced"), caller: Caller{Session: "foh-test", Harness: "claude", HarnessSession: "foh-session-1", Host: "laptop"}}
	oldTmux, oldLaunch, oldAmend := tmuxCommand, runLauncher, amendLaunch
	t.Cleanup(func() { tmuxCommand, runLauncher, amendLaunch = oldTmux, oldLaunch, oldAmend })
	tmuxCommand = func(args ...string) *exec.Cmd {
		if f.live && len(args) > 0 && args[0] == "list-sessions" {
			return exec.Command("echo", "d-"+f.name)
		}
		return exec.Command("true")
	}
	runLauncher = func(dir, name string) error { return nil }
	amendLaunch = func(dir, name string) error { return nil }
	order := strings.Replace(strings.Replace(good, "repo: kit", "repo: "+filepath.ToSlash(repo)+"\nvalidation: browser", 1), "name: example", "name: traced", 1)
	order = strings.Replace(order, "host: buildbox", "host: here", 1)
	o, err := Parse(order)
	if err != nil {
		t.Fatal(err)
	}
	q := Request{Action: "start", Name: f.name, Order: order, Meta: Meta{Order: o}}
	if err := StampTrace(&q.Meta, TraceStart{Caller: f.caller, Text: OrderAskText(order)}); err != nil {
		t.Fatal(err)
	}
	if _, err := Handle(q); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *traceFlow) meta() Meta {
	f.t.Helper()
	var m Meta
	if err := readJSON(filepath.Join(f.dir, "meta.json"), &m); err != nil {
		f.t.Fatal(err)
	}
	return m
}

// finish records the current attempt as done so the next ask may launch.
func (f *traceFlow) finish() {
	f.t.Helper()
	head, err := command(filepath.Join(f.dir, "wt"), "git", "rev-parse", "HEAD")
	if err != nil {
		f.t.Fatal(err)
	}
	m := f.meta()
	if err := WriteJSON(filepath.Join(f.dir, "result.json"), Result{Start: m.Start, End: time.Now().UTC(), Branch: m.Branch, Head: head, TraceID: m.TraceID, SpanID: m.SpanID}); err != nil {
		f.t.Fatal(err)
	}
}

func (f *traceFlow) do(q Request) Response {
	f.t.Helper()
	q.Name = f.name
	c := f.caller
	q.Caller = &c
	r, err := Handle(q)
	if err != nil {
		f.t.Fatalf("%s: %v", q.Action, err)
	}
	return r
}

// lastAsk checks the newest follow-up ask and returns it.
func (f *traceFlow) lastAsk(m Meta, trace, action, text string) AskSpan {
	f.t.Helper()
	a := m.Asks[len(m.Asks)-1]
	root := RootAsk(m)
	if m.TraceID != trace || a.Kind != AskFollowUp || a.Action != action || !strings.Contains(a.Text, text) || a.ParentSpanID != root.SpanID || a.Caller.HarnessSession != "foh-session-1" || !ValidSpanID(a.SpanID) {
		f.t.Fatalf("%s ask: %+v in %+v", action, a, m)
	}
	return a
}

// relaunched checks that a follow-up ask launched a new attempt under it.
func (f *traceFlow) relaunched(m Meta, ask AskSpan, previous string) {
	f.t.Helper()
	if !ValidSpanID(m.SpanID) || m.SpanID == previous || m.ParentSpanID != ask.SpanID {
		f.t.Fatalf("attempt after %s: span %s (was %s) parent %s", ask.Action, m.SpanID, previous, m.ParentSpanID)
	}
	h := history(f.dir, m)
	if got := h.Attempts[len(h.Attempts)-1].Meta.SpanID; got != m.SpanID {
		f.t.Fatalf("history attempt span %s, meta %s", got, m.SpanID)
	}
}

func TestTraceFollowUpAsks(t *testing.T) {
	f := newTraceFlow(t)
	m := f.meta()
	root := RootAsk(m)
	if !ValidTraceID(m.TraceID) || root == nil || !ValidSpanID(m.SpanID) || m.ParentSpanID != root.SpanID || root.Caller.Session != "foh-test" {
		t.Fatalf("start: %+v", m)
	}
	trace := m.TraceID

	f.finish()
	f.do(Request{Action: "rework", Note: "Also cover the error case"})
	next := f.meta()
	f.relaunched(next, f.lastAsk(next, trace, "rework", "error case"), m.SpanID)

	f.finish()
	f.do(Request{Action: "retry"})
	m, next = next, f.meta()
	f.relaunched(next, f.lastAsk(next, trace, "retry", "Retry the existing order."), m.SpanID)

	f.finish()
	f.do(Request{Action: "say", Note: "use plan B"})
	m, next = next, f.meta()
	f.relaunched(next, f.lastAsk(next, trace, "say", "plan B"), m.SpanID)

	// A say to a live run is delivered to its inbox: an ask, no new attempt.
	f.live = true
	f.do(Request{Action: "say", Note: "status please"})
	m, next = next, f.meta()
	f.lastAsk(next, trace, "say", "status please")
	if next.SpanID != m.SpanID {
		t.Fatal("a live say started a new attempt")
	}
	f.live = false

	f.finish()
	b, _ := os.ReadFile(filepath.Join(f.dir, "order.md"))
	f.do(Request{Action: "amend", Order: strings.Replace(string(b), "## Acceptance", "Tighter scope.\n\n## Acceptance", 1)})
	m, next = next, f.meta()
	f.relaunched(next, f.lastAsk(next, trace, "amend", "revision"), m.SpanID)

	// An explicit validate is an ask on the source run, and the validator
	// run joins the same trace under it.
	f.finish()
	wt := filepath.Join(f.dir, "wt")
	if _, err := command(wt, "git", "push", "origin", next.Branch); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(os.Getenv("HEV_DISPATCH_CONFIG"))
	if err != nil {
		t.Fatal(err)
	}
	caller := f.caller
	if _, err := validateAs(context.Background(), cfg, f.name, "v", false, "here", &caller); err != nil {
		t.Fatal(err)
	}
	source := f.meta()
	ask := f.lastAsk(source, trace, "validate", "Validate")
	if source.SpanID != next.SpanID {
		t.Fatal("validate relaunched the source run")
	}
	var v Meta
	if err := readJSON(filepath.Join(Root(), f.name+"-v1", "meta.json"), &v); err != nil {
		t.Fatal(err)
	}
	if v.TraceID != trace || v.ParentSpanID != ask.SpanID || !ValidSpanID(v.SpanID) || v.Parent != f.name {
		t.Fatalf("validator: %+v", v)
	}
	if want := 1 + 6; len(source.Asks) != want {
		t.Fatalf("asks: %d, want %d", len(source.Asks), want)
	}
}

// Follow-up asks on a run with no trace keep it untraced.
func TestTraceUntracedRunStaysUntraced(t *testing.T) {
	f := newTraceFlow(t)
	m := f.meta()
	m.TraceID, m.SpanID, m.ParentSpanID, m.Asks = "", "", "", nil
	if err := WriteJSON(filepath.Join(f.dir, "meta.json"), m); err != nil {
		t.Fatal(err)
	}
	f.finish()
	f.do(Request{Action: "rework", Note: "again"})
	if got := f.meta(); got.TraceID != "" || got.SpanID != "" || len(got.Asks) != 0 {
		t.Fatalf("invented a trace: %+v", got)
	}
}
