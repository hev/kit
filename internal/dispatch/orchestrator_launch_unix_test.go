//go:build !windows

package dispatch

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOrchestratorTmuxWorkerClearsServerIdentity(t *testing.T) {
	orchHome(t)
	t.Setenv("HEV_ORCHESTRATOR", "parent")
	dir := t.TempDir()
	if e := WriteJSON(filepath.Join(dir, "meta.json"), Meta{LaunchResolved: true}); e != nil {
		t.Fatal(e)
	}
	old := tmuxCommand
	t.Cleanup(func() { tmuxCommand = old })
	var command *exec.Cmd
	var args []string
	tmuxCommand = func(a ...string) *exec.Cmd { args = a; command = exec.Command("true"); return command }
	if e := launch(dir, "work"); e != nil {
		t.Fatal(e)
	}
	if !strings.HasPrefix(args[len(args)-1], "unset HEV_ORCHESTRATOR; exec ") {
		t.Fatal(args)
	}
	for _, v := range command.Env {
		if strings.HasPrefix(v, "HEV_ORCHESTRATOR=") {
			t.Fatal("tmux server inherited identity")
		}
	}
}
