package dispatch

import (
	"context"
	"errors"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestOrchestratorForwardedRetryPreservesAuthority(t *testing.T) {
	orchHome(t)
	s := applyOrch(t)
	text := strings.Replace(orchestratorExample, "authority: [say, rework, retry, validate, dispatch]", "authority: [say, retry]", 1)
	if _, e := ApplyOrchestrator(text, false); e != nil {
		t.Fatal(e)
	}
	dir := filepath.Join(Root(), "work")
	_ = os.MkdirAll(dir, 0700)
	_ = WriteJSON(filepath.Join(dir, "meta.json"), Meta{Order: Order{Name: "work", Repo: "app", Issue: "REL-1"}, Forward: &Host{SSH: "target"}})
	old := routeRPC
	t.Cleanup(func() { routeRPC = old })
	called := false
	routeRPC = func(_ context.Context, _ Host, q Request) (Response, error) {
		called = true
		if q.Action != "retry" {
			t.Fatalf("forwarded %s", q.Action)
		}
		if e := admitOrchestrator(q); e != nil {
			t.Fatal(e)
		}
		return Response{}, nil
	}
	if _, e := Handle(Request{Action: "retry", Name: "work", Orchestrator: s.Name}); e != nil {
		t.Fatal(e)
	}
	if !called {
		t.Fatal("not forwarded")
	}
}

func TestOrchestratorNestedSubsetAndSharedBudget(t *testing.T) {
	orchHome(t)
	parent := applyOrch(t)
	parent.Name = "parent"
	parent.AllowNested = true
	parent.Manages.Runs.Selector = ""
	parent.Manages.Runs.Names = []string{"child", "work"}
	child := parent
	child.Name = "child"
	child.AllowNested = false
	child.Manages.Runs.Names = []string{"work"}
	if e := orchestratorSubset(parent, child); e != nil {
		t.Fatal(e)
	}
	for _, change := range []func(*OrchestratorSpec){func(s *OrchestratorSpec) { s.Authority = append(s.Authority, "extra") }, func(s *OrchestratorSpec) { s.Manages.Runs.Names = []string{"other"} }, func(s *OrchestratorSpec) { s.Manages.Repos = nil }, func(s *OrchestratorSpec) { s.Budget.RunsPerDay = 0 }, func(s *OrchestratorSpec) { s.Budget.ConcurrentRuns = 4 }} {
		c := child
		change(&c)
		if e := orchestratorSubset(parent, c); e == nil {
			t.Fatal("escalation admitted")
		}
	}
	// Persist specs without invoking a harness. Parent's daily allowance includes child attempts.
	write := func(s OrchestratorSpec) {
		t.Helper()
		b, e := marshalOrchestratorForTest(s)
		if e != nil {
			t.Fatal(e)
		}
		_ = os.MkdirAll(OrchestratorDir(s.Name), 0700)
		_ = os.WriteFile(filepath.Join(OrchestratorConfigDir(), s.Name+".md"), b, 0600)
	}
	parent.Budget.RunsPerDay = 1
	child.Budget.RunsPerDay = 1
	write(child)
	write(parent)
	if e := reserveOrchestratorBudget(Request{Orchestrator: child.Name}, nil); e != nil {
		t.Fatal(e)
	}
	if e := reserveOrchestratorBudget(Request{Orchestrator: parent.Name}, nil); e == nil {
		t.Fatal("child did not consume parent budget")
	}
}

func TestOrchestratorWorkerInternalIdentityCleared(t *testing.T) {
	orchHome(t)
	s := applyOrch(t)
	t.Setenv("HEV_ORCHESTRATOR", s.Name)
	_ = OrchestratorControl(s.Name, "pause", "")
	for _, action := range []string{"validation-reserve", "validation-result"} {
		q := Request{Action: action, Orchestrator: s.Name}
		requestOrchestratorIdentity(&q)
		if q.Orchestrator != "" {
			t.Fatal("worker inherited identity")
		}
		if e := admitOrchestrator(q); e != nil {
			t.Fatal(e)
		}
	}
	for _, v := range orchestratorEnvironment("") {
		if strings.HasPrefix(v, "HEV_ORCHESTRATOR=") {
			t.Fatal("identity inherited")
		}
	}
}

func TestOrchestratorWorkerLaunchEnvironmentCleared(t *testing.T) {
	orchHome(t)
	t.Setenv("HEV_ORCHESTRATOR", "parent")
	dir := t.TempDir()
	if e := WriteJSON(filepath.Join(dir, "meta.json"), Meta{LaunchResolved: true, LaunchEnv: []string{"HEV_ORCHESTRATOR", "HOME"}}); e != nil {
		t.Fatal(e)
	}
	env, e := launchEnvironment(dir)
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range env {
		if strings.HasPrefix(v, "HEV_ORCHESTRATOR=") {
			t.Fatal("worker harness inherited identity")
		}
	}
}

func TestOrchestratorScopeOnlyDefersLifecycle(t *testing.T) {
	orchHome(t)
	s, e := RegisterOrchestratorScope(orchestratorExample, Host{SSH: "owner"}, false)
	if e != nil {
		t.Fatal(e)
	}
	if e := LaunchOrchestrator(s.Name); e == nil {
		t.Fatal("scope-only launched")
	}
	if e := SuperviseOrchestrator(context.Background(), s.Name, OrchestratorDriver{}); e == nil {
		t.Fatal("scope-only supervised")
	}
	_ = WriteJSON(filepath.Join(OrchestratorDir(s.Name), "state.json"), OrchestratorState{State: "complete"})
	dir := filepath.Join(Root(), "work")
	_ = os.MkdirAll(dir, 0700)
	_ = WriteJSON(filepath.Join(dir, "meta.json"), Meta{Order: Order{Name: "work", Repo: "app", Issue: "REL-1"}})
	old := routeRPC
	t.Cleanup(func() { routeRPC = old })
	paused := false
	calls := 0
	routeRPC = func(_ context.Context, h Host, q Request) (Response, error) {
		calls++
		if h.SSH != "owner" || !q.AdmissionOnly {
			t.Fatal("missing owner check")
		}
		if paused {
			return Response{}, errors.New("owner paused")
		}
		return Response{}, nil
	}
	q := Request{Action: "say", Name: "work", Orchestrator: s.Name}
	if e := admitOrchestrator(q); e != nil {
		t.Fatal(e)
	}
	paused = true
	if e := admitOrchestrator(q); e == nil {
		t.Fatal("owner pause ignored")
	}
	if calls != 2 {
		t.Fatal(calls)
	}
}

func TestOrchestratorBudgetIgnoresUnrelatedHosts(t *testing.T) {
	orchHome(t)
	s := applyOrch(t)
	target := Host{SSH: "target"}
	known := Host{SSH: "known"}
	hosts := map[string]Host{"target": target, "known": known, "unrelated": {SSH: "offline"}}
	dir := filepath.Join(Root(), "work")
	_ = os.MkdirAll(dir, 0700)
	_ = WriteJSON(filepath.Join(dir, "meta.json"), Meta{Order: Order{Name: "work", Repo: "app", Issue: "REL-1"}, Forward: &known})
	got := orchestratorBudgetHosts(s, hosts, target)
	if len(got) != 2 || !reflect.DeepEqual(got["known"], known) {
		t.Fatalf("hosts: %+v", got)
	}
	inv := map[string]Response{}
	for n := range got {
		inv[n] = Response{}
	}
	if e := reserveOrchestratorInventoryCounts(s, inv); e != nil {
		t.Fatal(e)
	}
	inv["known"] = Response{Error: "offline"}
	if e := reserveOrchestratorInventoryCounts(s, inv); e == nil {
		t.Fatal("known host failure hidden")
	}
}

func TestOrchestratorRoundAncestryIdentity(t *testing.T) {
	orchHome(t)
	s := applyOrch(t)
	path := filepath.Join(OrchestratorDir(s.Name), "harness.json")
	if e := writeProcessRecord(path, os.Getpid()); e != nil {
		t.Fatal(e)
	}
	t.Setenv("HEV_ORCHESTRATOR", "")
	if n := roundIdentityForAncestors(map[int]bool{os.Getpid(): true}); n != s.Name {
		t.Fatalf("identity %q", n)
	}
	if n := roundIdentityForAncestors(map[int]bool{}); n != "" {
		t.Fatal(n)
	}
}

func TestOrchestratorConsumesOnlyAcceptedInbox(t *testing.T) {
	orchHome(t)
	s := applyOrch(t)
	st, _ := orchestratorState(s.Name)
	path := filepath.Join(OrchestratorDir(s.Name), "inbox.md")
	_ = os.WriteFile(path, []byte("first"), 0600)
	fail := true
	d := OrchestratorDriver{Gather: func(context.Context, OrchestratorSpec) ([]OrchestratorEvidence, error) { return nil, nil }, Session: func(_ context.Context, _ OrchestratorSpec, dir string, p OrchestratorPacket) error {
		if p.Inbox != "first" {
			t.Fatal(p.Inbox)
		}
		if fail {
			return errors.New("failed")
		}
		_ = os.WriteFile(path, []byte("firstlater"), 0600)
		return os.WriteFile(filepath.Join(dir, "checkpoint.md"), []byte("accepted"), 0600)
	}}
	if e := RunOrchestratorRound(context.Background(), s, &st, d); e == nil {
		t.Fatal("failure accepted")
	}
	b, _ := os.ReadFile(path)
	if string(b) != "first" {
		t.Fatal("failed inbox consumed")
	}
	fail = false
	if e := RunOrchestratorRound(context.Background(), s, &st, d); e != nil {
		t.Fatal(e)
	}
	b, _ = os.ReadFile(path)
	if string(b) != "later" {
		t.Fatal(string(b))
	}
	b, _ = os.ReadFile(filepath.Join(OrchestratorDir(s.Name), "rounds", "002", "inbox.md"))
	if string(b) != "first" {
		t.Fatal("archive missing")
	}
}

func TestOrchestratorRoundTimeout(t *testing.T) {
	orchHome(t)
	text := strings.Replace(orchestratorExample, "mode: loop", "round_timeout: 1ms\nmode: once", 1)
	s, e := ApplyOrchestrator(text, false)
	if e != nil {
		t.Fatal(e)
	}
	d := OrchestratorDriver{Now: time.Now, Gather: func(context.Context, OrchestratorSpec) ([]OrchestratorEvidence, error) { return nil, nil }, Session: func(ctx context.Context, _ OrchestratorSpec, _ string, _ OrchestratorPacket) error {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > time.Second {
			t.Fatal("round not bounded")
		}
		<-ctx.Done()
		return ctx.Err()
	}}
	if e := SuperviseOrchestrator(context.Background(), s.Name, d); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatalf("%v", e)
	}
	st, _ := orchestratorState(s.Name)
	if st.State != "failed" {
		t.Fatal(st.State)
	}
	if _, e := ParseOrchestrator(strings.Replace(text, "1ms", "invalid", 1)); e == nil {
		t.Fatal("invalid timeout accepted")
	}
}

func TestOrchestratorTerminalControlErrors(t *testing.T) {
	orchHome(t)
	s := applyOrch(t)
	for _, state := range []string{"complete", "handed-off"} {
		_ = WriteJSON(filepath.Join(OrchestratorDir(s.Name), "state.json"), OrchestratorState{State: state})
		for _, action := range []string{"wake", "handoff", "resume"} {
			if e := OrchestratorControl(s.Name, action, "owner"); e == nil {
				t.Fatalf("%s %s succeeded", state, action)
			}
		}
		if e := LaunchOrchestrator(s.Name); e == nil {
			t.Fatal("terminal launch succeeded")
		}
	}
}

func TestOrchestratorPausedStateWrittenOnce(t *testing.T) {
	orchHome(t)
	s := applyOrch(t)
	_ = OrchestratorControl(s.Name, "pause", "")
	polls := 0
	var initial time.Time
	d := OrchestratorDriver{Now: time.Now, Wait: func(context.Context) error {
		polls++
		info, e := os.Stat(filepath.Join(OrchestratorDir(s.Name), "state.json"))
		if e != nil {
			t.Fatal(e)
		}
		if polls == 1 {
			initial = info.ModTime()
		} else if !initial.Equal(info.ModTime()) {
			t.Fatal("paused state rewritten")
		}
		if polls == 3 {
			return errors.New("end test")
		}
		return nil
	}}
	if e := SuperviseOrchestrator(context.Background(), s.Name, d); e == nil {
		t.Fatal("expected test stop")
	}
}

func marshalOrchestratorForTest(s OrchestratorSpec) ([]byte, error) {
	b, e := yaml.Marshal(s)
	return append(append([]byte("---\n"), b...), []byte("---\n")...), e
}

func TestOrchestratorOwnerLifecycleProbeNeedsNoLocalRun(t *testing.T) {
	orchHome(t)
	s := applyOrch(t)
	q := Request{Action: "say", Name: "remote-work", Orchestrator: s.Name, AdmissionOnly: true}
	if _, e := Handle(q); e != nil {
		t.Fatal(e)
	}
	if exists(filepath.Join(Root(), q.Name)) {
		t.Fatal("probe mutated run state")
	}
	if e := OrchestratorControl(s.Name, "pause", ""); e != nil {
		t.Fatal(e)
	}
	if _, e := Handle(q); e == nil {
		t.Fatal("paused owner admitted probe")
	}
}
