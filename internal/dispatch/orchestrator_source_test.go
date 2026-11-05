package dispatch

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sourceTemplate = "---\nname: {{.RunName}}\nrepo: app\nissue: {{.Event.ID}}\n---\n{{.Action}} {{.Event.URL}}\n\n## Acceptance\n- Complete requested work.\n"

func sourceFixture(t *testing.T) OrchestratorSpec {
	t.Helper()
	orchHome(t)
	text := strings.Replace(orchestratorExample, "source: none", `agent: off
source:
  plugin: fake
  rules:
    - {status: Ready, action: fix, template: fix.md, mode: live}`, 1)
	s, e := ApplyOrchestrator(text, false)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(OrchestratorDir(s.Name), "fix.md"), []byte(sourceTemplate), 0600); e != nil {
		t.Fatal(e)
	}
	cfg := filepath.Join(t.TempDir(), "dispatch.toml")
	if e = os.WriteFile(cfg, []byte("default_host = 'worker'\n[hosts.worker]\nlocal = true\ncap = 4\n"), 0600); e != nil {
		t.Fatal(e)
	}
	t.Setenv("HEV_DISPATCH_CONFIG", cfg)
	old := sourceCommand
	t.Cleanup(func() { sourceCommand = old })
	sourceCommand = func(_ context.Context, _ OrchestratorSpec, inv sourceInvocation) ([]byte, error) {
		switch inv.Args[0] {
		case "--hev-describe":
			return []byte(`{"source":true,"ack":true}`), nil
		case "events":
			return []byte("{\"id\":\"REL-1\",\"kind\":\"issue\",\"url\":\"https://example.com/1\",\"fields\":{\"status\":\"Ready\"}}\n{\"cursor\":\"next\"}\n"), nil
		case "ack":
			return nil, nil
		}
		return nil, errors.New("unexpected plugin command")
	}
	return s
}
func TestSourceContractRejectsMalformedStreams(t *testing.T) {
	s := sourceFixture(t)
	good := sourceCommand
	for _, raw := range []string{"", `{"cursor":7}`, "{\"cursor\":\"x\"}\n{}", `{"id":"a","kind":"issue"}`, "{\"cursor\":\"x\",\"id\":\"a\"}", "garbage", "{\"id\":\"a\",\"kind\":\"issue\",\"fields\":{}}\n{\"id\":\"a\",\"kind\":\"issue\",\"fields\":{}}\n{\"cursor\":\"x\"}"} {
		sourceCommand = func(ctx context.Context, p OrchestratorSpec, inv sourceInvocation) ([]byte, error) {
			if inv.Args[0] == "events" {
				return []byte(raw), nil
			}
			return good(ctx, p, inv)
		}
		if _, e := fetchSource(context.Background(), s); e == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
	sourceCommand = func(context.Context, OrchestratorSpec, sourceInvocation) ([]byte, error) {
		return []byte(`{"source":false}`), nil
	}
	if _, e := fetchSource(context.Background(), s); e == nil {
		t.Fatal("non source plugin accepted")
	}
}
func TestSourceRulesCursorCapsPauseDryRunIdempotency(t *testing.T) {
	s := sourceFixture(t)
	st, _ := orchestratorState(s.Name)
	ctx := context.Background()
	old := sourceDispatch
	t.Cleanup(func() { sourceDispatch = old })
	launches := 0
	sourceDispatch = func(_ context.Context, _ Host, q Request) (Response, error) {
		if q.Orchestrator != s.Name {
			t.Fatal("identity missing")
		}
		if !q.AdmissionOnly {
			launches++
		}
		return Response{}, nil
	}
	d := OrchestratorDriver{Gather: func(context.Context, OrchestratorSpec) ([]OrchestratorEvidence, error) { return nil, nil }, Session: func(context.Context, OrchestratorSpec, string, OrchestratorPacket) error {
		t.Fatal("agent off called model")
		return nil
	}}
	if e := RunOrchestratorRound(ctx, s, &st, d); e != nil {
		t.Fatal(e)
	}
	if launches != 1 {
		t.Fatal(launches)
	}
	var cursor struct{ Cursor string }
	if e := readJSON(filepath.Join(OrchestratorDir(s.Name), "source-cursor.json"), &cursor); e != nil || cursor.Cursor != "next" {
		t.Fatal(cursor, e)
	}
	s.Idle = "run"
	if e := RunOrchestratorRound(ctx, s, &st, d); e != nil || launches != 1 {
		t.Fatal("duplicate dispatch", launches, e)
	}
	// A separate dry-run receipt must never suppress subsequent live promotion.
	s.Source.Rules[0].Mode = "dry-run"
	if e := RunOrchestratorRound(ctx, s, &st, d); e != nil || launches != 1 {
		t.Fatal(launches, e)
	}
	entries, _ := filepath.Glob(filepath.Join(OrchestratorDir(s.Name), "source-markers", "*.json"))
	if len(entries) != 2 {
		t.Fatal(entries)
	}
	s.PauseFile = "PAUSED"
	pause := filepath.Join(OrchestratorDir(s.Name), "PAUSED")
	_ = os.WriteFile(pause, nil, 0600)
	if e := RunOrchestratorRound(ctx, s, &st, d); e == nil {
		t.Fatal("pause ignored")
	}
	_ = os.Remove(pause)
	// Remove receipts to exercise cap deferral without advancing a cursor.
	_ = os.RemoveAll(filepath.Join(OrchestratorDir(s.Name), "source-markers"))
	s.Source.Rules[0].Mode = "live"
	s.Budget.RunsPerDay = 1
	if e := RunOrchestratorRound(ctx, s, &st, d); e == nil || !strings.Contains(e.Error(), "cap") {
		t.Fatal("daily cap ignored", e)
	}
	s.Budget.RunsPerDay = 20
	s.Budget.ConcurrentRuns = 1
	d.Gather = func(context.Context, OrchestratorSpec) ([]OrchestratorEvidence, error) {
		return []OrchestratorEvidence{{Row: Row{State: "running"}}}, nil
	}
	if e := RunOrchestratorRound(ctx, s, &st, d); e == nil || !strings.Contains(e.Error(), "cap") {
		t.Fatal("concurrency cap ignored", e)
	}
}
func TestSourceAgentPacketAndCursorOnlyAfterCheckpoint(t *testing.T) {
	s := sourceFixture(t)
	s.Agent = "on"
	st, _ := orchestratorState(s.Name)
	d := OrchestratorDriver{Gather: func(context.Context, OrchestratorSpec) ([]OrchestratorEvidence, error) { return nil, nil }, Session: func(_ context.Context, _ OrchestratorSpec, dir string, p OrchestratorPacket) error {
		if len(p.Source.Events) != 1 || p.Source.Cursor != "next" {
			t.Fatal(p.Source)
		}
		return nil
	}}
	if e := RunOrchestratorRound(context.Background(), s, &st, d); e == nil {
		t.Fatal("missing checkpoint accepted")
	}
	if exists(filepath.Join(OrchestratorDir(s.Name), "source-cursor.json")) {
		t.Fatal("failed round advanced cursor")
	}
	d.Session = func(_ context.Context, _ OrchestratorSpec, dir string, p OrchestratorPacket) error {
		return os.WriteFile(filepath.Join(dir, "checkpoint.md"), []byte("pending"), 0600)
	}
	if e := RunOrchestratorRound(context.Background(), s, &st, d); e != nil {
		t.Fatal(e)
	}
}
func TestSourceUncertainAttemptIsHeld(t *testing.T) {
	s := sourceFixture(t)
	st, _ := orchestratorState(s.Name)
	old := sourceDispatch
	t.Cleanup(func() { sourceDispatch = old })
	calls := 0
	sourceDispatch = func(_ context.Context, _ Host, q Request) (Response, error) {
		if q.AdmissionOnly || q.Action == "list" {
			return Response{}, nil
		}
		calls++
		return Response{}, transportError{errors.New("connection lost")}
	}
	d := OrchestratorDriver{Gather: func(context.Context, OrchestratorSpec) ([]OrchestratorEvidence, error) { return nil, nil }}
	if e := RunOrchestratorRound(context.Background(), s, &st, d); e == nil {
		t.Fatal("uncertain attempt accepted")
	}
	if exists(filepath.Join(OrchestratorDir(s.Name), "source-cursor.json")) {
		t.Fatal("uncertain round advanced cursor")
	}
	// Later rounds hold the receipt; they never dispatch it again.
	if e := RunOrchestratorRound(context.Background(), s, &st, d); e != nil {
		t.Fatal(e)
	}
	if calls != 1 {
		t.Fatal("retried uncertain dispatch", calls)
	}
}

func TestSourceModelRulesRemainGuardrails(t *testing.T) {
	s := sourceFixture(t)
	s.Agent = "on"
	st, _ := orchestratorState(s.Name)
	var rendered string
	d := OrchestratorDriver{Gather: func(context.Context, OrchestratorSpec) ([]OrchestratorEvidence, error) { return nil, nil }, Session: func(_ context.Context, _ OrchestratorSpec, dir string, p OrchestratorPacket) error {
		event := p.Source.Events[0]
		rule := s.Source.Rules[0]
		key := fmt.Sprintf("%x", sha256.Sum256([]byte(s.Source.Plugin+"\x00"+event.ID+"\x00"+rule.Action+"\x00"+rule.Mode)))
		name := "src-" + s.Name + "-" + key[:16]
		rendered = strings.NewReplacer("{{.RunName}}", name, "{{.Event.ID}}", event.ID, "{{.Action}}", rule.Action, "{{.Event.URL}}", event.URL).Replace(sourceTemplate)
		q := Request{Action: "start", Name: name, Order: rendered}
		if e := admitSourceOrder(s, q); e != nil {
			t.Fatal(e)
		}
		q.Order = strings.Replace(rendered, "repo: app", "repo: other", 1)
		if e := admitSourceOrder(s, q); e == nil {
			t.Fatal("model changed rule order")
		}
		q.Order = rendered
		s.Source.Rules[0].Mode = "dry-run"
		if e := admitSourceOrder(s, q); e == nil {
			t.Fatal("model promoted dry-run rule")
		}
		s.Source.Rules[0].Mode = "live"
		return os.WriteFile(filepath.Join(dir, "checkpoint.md"), []byte("pending"), 0600)
	}}
	if e := RunOrchestratorRound(context.Background(), s, &st, d); e != nil {
		t.Fatal(e)
	}
}

func TestSourceSpecStrictRules(t *testing.T) {
	orchHome(t)
	for _, source := range []string{
		"source: {plugin: fake, typo: true}",
		"source: {plugin: fake, rules: [{status: Ready, action: fix, template: fix.md, mode: unknown}]}",
		"source: {plugin: fake, rules: [{status: Ready, action: fix, template: fix.md, mode: live, typo: true}]}",
		"source: {plugin: fake, rules: [{status: Ready, action: fix, template: fix.md, mode: live}, {status: Ready, action: other, template: fix.md, mode: live}]}",
		"source: none\nagent: off",
	} {
		if _, e := ParseOrchestrator(strings.Replace(orchestratorExample, "source: none", source, 1)); e == nil {
			t.Fatal("accepted invalid source", source)
		}
	}
}
