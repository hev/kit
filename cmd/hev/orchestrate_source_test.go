package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/dispatch"
)

const sourceSpecForCLI = "---\nkind: orchestrator\nname: intake\ngoal: Process test events\nagent: off\npause_file: state/PAUSED\nmanages: {runs: {selector: 'issue=TASK-*'}, repos: [app]}\nauthority: [dispatch]\nsource: {plugin: fake, rules: [{status: Ready, action: fix, template: fix.md, mode: live}]}\n---\nBrief\n"

func TestOrchestrateApplyResolvesPathsBesideSpec(t *testing.T) {
	dir := t.TempDir()
	if e := os.WriteFile(filepath.Join(dir, "fix.md"), []byte("x"), 0600); e != nil {
		t.Fatal(e)
	}
	b, e := resolveSpecPaths(filepath.Join(dir, "spec.md"), []byte(sourceSpecForCLI))
	if e != nil {
		t.Fatal(e)
	}
	s, e := dispatch.ParseOrchestrator(string(b))
	if e != nil {
		t.Fatal(e)
	}
	if s.PauseFile != filepath.Join(dir, "state", "PAUSED") || s.Source.Rules[0].Template != filepath.Join(dir, "fix.md") {
		t.Fatal(s.PauseFile, s.Source.Rules[0].Template)
	}
	// A spec without a source still resolves its relative pause_file.
	plain := "---\nkind: orchestrator\nname: plain\ngoal: g\npause_file: P\nmanages: {runs: {names: [task]}}\n---\nBrief\n"
	if b, e = resolveSpecPaths(filepath.Join(dir, "plain.md"), []byte(plain)); e != nil || !strings.Contains(string(b), filepath.Join(dir, "P")) {
		t.Fatal(string(b), e)
	}
}

func applySourceForCLI(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HEV_ORCHESTRATOR", "")
	s, e := dispatch.ApplyOrchestrator(sourceSpecForCLI, false)
	if e != nil {
		t.Fatal(e)
	}
	tpl := "---\nname: {{.RunName}}\nrepo: app\nissue: {{.Event.ID}}\n---\n{{.Action}} {{.Event.URL}}\n\n## Acceptance\n- Done.\n"
	if e = os.WriteFile(filepath.Join(dispatch.OrchestratorDir(s.Name), "fix.md"), []byte(tpl), 0600); e != nil {
		t.Fatal(e)
	}
}

func TestOrchestrateRenderPreviewCLI(t *testing.T) {
	applySourceForCLI(t)
	before, _ := os.ReadDir(dispatch.OrchestratorDir("intake"))
	cmd := newOrchestrateCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetIn(strings.NewReader(`{"id":"TASK-1","kind":"issue","url":"https://example.com/1","fields":{"status":"Ready"}}`))
	cmd.SetArgs([]string{"render", "intake", "--event", "-"})
	if e := cmd.Execute(); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(out.String(), "issue: TASK-1") || !strings.Contains(out.String(), "fix https://example.com/1") {
		t.Fatal(out.String())
	}
	after, _ := os.ReadDir(dispatch.OrchestratorDir("intake"))
	if len(before) != len(after) {
		t.Fatal("render wrote state")
	}
	cmd = newOrchestrateCommand()
	out.Reset()
	cmd.SetOut(&out)
	cmd.SetIn(strings.NewReader("{\"id\":\"TASK-2\\nhost: x\",\"kind\":\"issue\",\"url\":\"u\",\"fields\":{\"status\":\"Ready\"}}\n"))
	cmd.SetArgs([]string{"render", "intake", "--event", "-"})
	if e := cmd.Execute(); e == nil || !strings.Contains(out.String(), dispatch.ErrSourceUnsafeValue.Error()) {
		t.Fatal("unsafe value rendered", out.String(), e)
	}
}

func TestOrchestrateReconcileCLI(t *testing.T) {
	applySourceForCLI(t)
	for _, args := range [][]string{
		{"reconcile", "intake", "--event", "TASK-1"},
		{"reconcile", "intake", "--event", "TASK-1", "--admitted", "--not-admitted"},
	} {
		cmd := newOrchestrateCommand()
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs(args)
		if e := cmd.Execute(); e == nil || !strings.Contains(e.Error(), "exactly one") {
			t.Fatal(args, e)
		}
	}
	cmd := newOrchestrateCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"reconcile", "intake", "--event", "TASK-1", "--admitted"})
	if e := cmd.Execute(); e == nil || !strings.Contains(e.Error(), "no attempting or needs-reconcile receipt") {
		t.Fatal(e)
	}
}
