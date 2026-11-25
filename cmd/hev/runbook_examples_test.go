package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/dispatch"
)

// Every example spec under docs/runbooks/examples must pass apply --dry-run.
// A sibling NAME.checkpoint.md is validated as that spec's seed checkpoint.
func TestRunbookExamplesDryRun(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	files, e := filepath.Glob("../../docs/runbooks/examples/*.md")
	if e != nil {
		t.Fatal(e)
	}
	specs := 0
	for _, file := range files {
		if strings.HasSuffix(file, ".checkpoint.md") {
			continue
		}
		specs++
		args := []string{"apply", "--dry-run", "-f", file}
		seed := strings.TrimSuffix(file, ".md") + ".checkpoint.md"
		if _, e := os.Stat(seed); e == nil {
			args = append(args, "--seed-checkpoint", seed)
		}
		cmd := newOrchestrateCommand()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		if e := cmd.Execute(); e != nil {
			t.Fatalf("%s: %v\n%s", file, e, out.String())
		}
		if !strings.HasPrefix(out.String(), "valid ") {
			t.Fatalf("%s: %s", file, out.String())
		}
	}
	if specs < 3 {
		t.Fatalf("found %d runbook example specs", specs)
	}
	if _, e := os.Stat(dispatch.OrchestratorConfigDir()); !os.IsNotExist(e) {
		t.Fatal("dry run created config")
	}
	if _, e := os.Stat(dispatch.Root()); !os.IsNotExist(e) {
		t.Fatal("dry run created runtime state")
	}
}
