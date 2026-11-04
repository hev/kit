package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/dispatch"
)

func TestOrchestrateDryRunRFCAndDefaultOff(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	b, e := os.ReadFile("../../docs/rfcs/0008-orchestrators.md")
	if e != nil {
		t.Fatal(e)
	}
	_, rest, ok := strings.Cut(string(b), "```yaml\n")
	if !ok {
		t.Fatal("missing RFC example")
	}
	example, _, _ := strings.Cut(rest, "```")
	file := filepath.Join(t.TempDir(), "example.md")
	if e = os.WriteFile(file, []byte("---\n"+example+"---\nStanding brief.\n"), 0600); e != nil {
		t.Fatal(e)
	}
	cmd := newOrchestrateCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"apply", "--dry-run", "-f", file})
	if e = cmd.Execute(); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(out.String(), "valid release-train") {
		t.Fatal(out.String())
	}
	if _, e = os.Stat(dispatch.OrchestratorConfigDir()); !os.IsNotExist(e) {
		t.Fatal("dry run created config")
	}
	if _, e = os.Stat(dispatch.Root()); !os.IsNotExist(e) {
		t.Fatal("dry run created runtime state")
	}
	cmd = newOrchestrateCommand()
	out.Reset()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"ls", "--json"})
	if e = cmd.Execute(); e != nil {
		t.Fatal(e)
	}
	if strings.TrimSpace(out.String()) != "[]" {
		t.Fatal(out.String())
	}
}
func TestOrchestrateSurface(t *testing.T) {
	c := newOrchestrateCommand()
	for _, n := range []string{"apply", "start", "ls", "show", "logs", "wake", "pause", "resume", "stop", "handoff"} {
		sub, _, e := c.Find([]string{n})
		if e != nil || sub == c {
			t.Fatalf("missing %s", n)
		}
	}
}
