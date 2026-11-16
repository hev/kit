package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hev/kit/internal/dispatch"
	"github.com/spf13/cobra"
)

// rfcSwarmExample is RFC 0013's example with its later-phase fields removed.
func rfcSwarmExample(t *testing.T) string {
	t.Helper()
	b, e := os.ReadFile("../../docs/rfcs/0013-swarms.md")
	if e != nil {
		t.Fatal(e)
	}
	_, rest, ok := strings.Cut(string(b), "`logfix.md`:\n\n```markdown\n")
	if !ok {
		t.Fatal("RFC example missing")
	}
	text, _, _ := strings.Cut(rest, "\n```\n")
	for _, pat := range []string{`standby: .*\n`, `host_grace: .*\n`, `plans: .*\n`, `max_runs: .*\n`, `, swarm_limit: 8`, `    placement: \{spread: true, os: linux\}\n`} {
		text = regexp.MustCompile(pat).ReplaceAllString(text, "")
	}
	return text + "\n"
}

func TestSwarmUpDryRunIsAcceptedDownstream(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HEV_DISPATCH_CONFIG", filepath.Join(home, "none.toml"))
	t.Setenv("PEERS_SELF", "test-host")
	t.Setenv("HEV_ORCHESTRATOR", "")
	dir := t.TempDir()
	file := filepath.Join(dir, "logfix.md")
	if e := os.WriteFile(file, []byte(rfcSwarmExample(t)), 0600); e != nil {
		t.Fatal(e)
	}
	cmd := newSwarmCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"up", "-f", file, "--dry-run"})
	if e := cmd.Execute(); e != nil {
		t.Fatal(e)
	}
	// Split the printed spec and orders on their separators.
	parts := regexp.MustCompile(`(?m)^--- (orchestrator|order) ([a-z0-9-]+) ---\n`).Split(out.String(), -1)
	heads := regexp.MustCompile(`(?m)^--- (orchestrator|order) ([a-z0-9-]+) ---\n`).FindAllStringSubmatch(out.String(), -1)
	if len(heads) != 7 || heads[0][1] != "orchestrator" || !strings.Contains(parts[0], "swarm logfix: owner build-1, 2 roles, 6 units") {
		t.Fatalf("dry run output:\n%s", out.String())
	}
	spec := filepath.Join(dir, "spec.md")
	if e := os.WriteFile(spec, []byte(parts[1]), 0600); e != nil {
		t.Fatal(e)
	}
	apply := newOrchestrateCommand()
	var applied bytes.Buffer
	apply.SetOut(&applied)
	apply.SetArgs([]string{"apply", "--dry-run", "-f", spec})
	if e := apply.Execute(); e != nil || !strings.Contains(applied.String(), "valid logfix") {
		t.Fatalf("orchestrate apply --dry-run: %v %s", e, applied.String())
	}
	for i, h := range heads[1:] {
		o, e := dispatch.Parse(parts[i+2])
		if e != nil || o.Name != h[2] {
			t.Fatalf("order %s: %+v %v\n%s", h[2], o, e, parts[i+2])
		}
	}
	for _, p := range []string{dispatch.Root(), dispatch.OrchestratorConfigDir()} {
		if _, e := os.Stat(p); !os.IsNotExist(e) {
			t.Fatalf("dry run wrote %s", p)
		}
	}
	// A later-phase field is refused by name, without a usage dump.
	full := strings.Replace(rfcSwarmExample(t), "every: 30m", "every: 30m\nstandby: [build-2]", 1)
	if e := os.WriteFile(file, []byte(full), 0600); e != nil {
		t.Fatal(e)
	}
	cmd = newSwarmCommand()
	out.Reset()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"up", "-f", file, "--dry-run"})
	if e := cmd.Execute(); e == nil || !strings.Contains(e.Error(), "standby needs swarm phase 4") || strings.Contains(out.String(), "Usage:") {
		t.Fatalf("refusal: %v\n%s", e, out.String())
	}
}

func TestSwarmSurfaceAndHelp(t *testing.T) {
	c := newSwarmCommand()
	for _, n := range []string{"up", "ps", "logs", "say", "pause", "resume", "down"} {
		if sub, _, e := c.Find([]string{n}); e != nil || sub.Name() != n {
			t.Fatalf("missing swarm %s", n)
		}
	}
	ps, _, _ := c.Find([]string{"ps"})
	if ps.Flags().Lookup("json") == nil {
		t.Fatal("ps lacks --json")
	}
	root := &cobra.Command{Use: "hev"}
	root.AddCommand(newSwarmCommand())
	groupFactoryCommands(root)
	if root.Commands()[0].GroupID != "factory" {
		t.Fatal("hev swarm outside the Factory group")
	}
	if !strings.Contains(rootCmd.Long, "hev swarm") {
		t.Fatal("bare hev --help does not list hev swarm")
	}
}
