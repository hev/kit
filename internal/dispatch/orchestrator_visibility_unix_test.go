//go:build !windows

package dispatch

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOrchestratorProcessHelper(t *testing.T) {
	if os.Getenv("ORCH_PROCESS_HELPER") != "1" {
		return
	}
	d := DefaultOrchestratorDriver()
	d.Gather = func(context.Context, OrchestratorSpec) ([]OrchestratorEvidence, error) {
		return []OrchestratorEvidence{}, nil
	}
	if e := SuperviseOrchestrator(context.Background(), "release-train", d); e != nil {
		t.Fatal(e)
	}
}
func waitOrch(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("orchestrator boundary timed out")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
func TestOrchestratorTmuxRestSayAndStopLifecycle(t *testing.T) {
	if _, e := exec.LookPath("tmux"); e != nil {
		t.Skip("tmux unavailable")
	}
	orchHome(t)
	s := applyOrch(t)
	s.Harness = "codex"
	// Update the applied harness without changing other validated defaults.
	text := strings.Replace(orchestratorExample, "harness: claude", "harness: codex", 1)
	if _, e := ApplyOrchestrator(text, false); e != nil {
		t.Fatal(e)
	}
	bin := t.TempDir()
	harness := filepath.Join(bin, "codex")
	if e := os.WriteFile(harness, []byte("#!/bin/sh\nprintf 'pending\\n' > checkpoint.md\n"), 0700); e != nil {
		t.Fatal(e)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	exe := filepath.Join(bin, "helper")
	script := "#!/bin/sh\nexport ORCH_PROCESS_HELPER=1\nexec " + Quote(os.Args[0]) + " -test.run '^TestOrchestratorProcessHelper$'\n"
	if e := os.WriteFile(exe, []byte(script), 0700); e != nil {
		t.Fatal(e)
	}
	if _, e := launchOrchestratorTmux(exe, s.Name); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = tmuxCommand("kill-session", "-t", "=o-"+s.Name).Run() })
	waitOrch(t, func() bool {
		st, e := orchestratorState(s.Name)
		return e == nil && st.Rounds == 1 && st.State == "waiting" && !st.Next.IsZero()
	})
	out, e := tmuxCommand("display-message", "-p", "-t", "=o-"+s.Name+":", "#{@peers-role}|#{pane_title}").Output()
	if e != nil || strings.TrimSpace(string(out)) != "orchestrator|"+s.Goal {
		t.Fatal(string(out), e)
	}
	pane, e := tmuxCommand("capture-pane", "-p", "-t", "=o-"+s.Name+":").Output()
	if e != nil || !strings.Contains(string(pane), "next wake:") {
		t.Fatal("rest placeholder missing", string(pane), e)
	}
	if e := SayOrchestrator(s.Name, "inspect early"); e != nil {
		t.Fatal(e)
	}
	waitOrch(t, func() bool {
		st, e := orchestratorState(s.Name)
		return e == nil && st.Rounds == 2 && st.State == "waiting"
	})
	b, e := os.ReadFile(filepath.Join(OrchestratorDir(s.Name), "rounds", "002", "inbox.md"))
	if e != nil || !strings.Contains(string(b), "inspect early") {
		t.Fatal(string(b), e)
	}
	if e := OrchestratorControl(s.Name, "stop", ""); e != nil {
		t.Fatal(e)
	}
	waitOrch(t, func() bool { return tmuxCommand("has-session", "-t", "=o-"+s.Name).Run() != nil })
	st, e := orchestratorState(s.Name)
	if e != nil || st.State != "stopped" {
		t.Fatal(st, e)
	}
}
func TestOrchestratorInventoryKindAndManagingNames(t *testing.T) {
	orchHome(t)
	s := applyOrch(t)
	r := Response{Rows: []Row{{Kind: "run", Meta: Meta{Order: Order{Name: "work", Repo: "app", Issue: "REL-1"}}}}}
	if e := addOrchestratorRows(&r); e != nil {
		t.Fatal(e)
	}
	if len(r.Rows) != 2 || r.Rows[0].Orchestrator != s.Name || r.Rows[1].Kind != "orch" || r.Rows[1].Name != "o-"+s.Name || !VisibleByDefault(r.Rows[1]) {
		t.Fatal(r.Rows)
	}
	b, e := json.Marshal(r)
	if e != nil || !strings.Contains(string(b), `"kind":"run"`) || !strings.Contains(string(b), `"orchestrator":"release-train"`) {
		t.Fatal(string(b), e)
	}
	if e := writePID(OrchestratorDir(s.Name)); e != nil {
		t.Fatal(e)
	}
	if _, e := Handle(Request{Action: "say", Name: "o-" + s.Name, Note: "hello"}); e != nil {
		t.Fatal(e)
	}
	b, e = os.ReadFile(filepath.Join(OrchestratorDir(s.Name), "inbox.md"))
	if e != nil || !strings.Contains(string(b), "hello") {
		t.Fatal(string(b), e)
	}
}
