package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/dispatch"
)

func TestOrchestratorTemplatesKeepValidLayersAndRejectMalformedSpecs(t *testing.T) {
	for _, tc := range []struct {
		name  string
		roles int
		climb bool
	}{
		{"swarm", 1, false},
		{"climb", 0, true},
		{"swarm-climb", 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _, err := runOrchestrate("template", tc.name)
			if err != nil {
				t.Fatal(err)
			}
			spec, err := dispatch.ParseOrchestratorTemplate(out)
			if err != nil {
				t.Fatal(err)
			}
			if len(spec.Roles) != tc.roles || (spec.Climb != nil) != tc.climb {
				t.Fatalf("unexpected template layers: %+v", spec)
			}
			if tc.climb && (spec.Climb.Population.TopK != 2 || spec.Climb.Objective.Scorer != "example-metric") {
				t.Fatalf("climb configuration lost: %+v", spec.Climb)
			}
		})
	}

	for _, tc := range []struct{ name, input, want string }{
		{"kind inferred from roles", strings.TrimPrefix(swarmTemplate, "kind = \"swarm\"\n"), ""},
		{"climb inferred from table", strings.TrimPrefix(swarmClimbTemplate, "kind = \"climb\"\n"), ""},
		{"invalid TOML", "[[roles]", "expected"},
		{"unknown worker field", swarmTemplate + "typo = true\n", "roles.order.typo"},
		{"unknown climb field", climbTemplate + "typo = true\n", "climb.stop.typo"},
		{"wrong type", strings.Replace(climbTemplate, "top_k = 2", "top_k = \"2\"", 1), "top_k"},
		{"missing climb", strings.Split(swarmClimbTemplate, "[climb]")[0], "requires [climb]"},
		{"invalid selection", strings.Replace(swarmClimbTemplate, "top_k = 2", "top_k = 5", 1), "top_k <= width"},
		{"invalid role graph", strings.Replace(swarmClimbTemplate, "shape = \"single\"", "shape = \"single\"\nafter = [\"absent\"]", 1), "after names unknown role"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := dispatch.ParseOrchestratorTemplate(tc.input)
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}
}

func TestOrchestrateTemplateApplyPreservesSwarmCompilationAndRefusesClimb(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HEV_DISPATCH_CONFIG", filepath.Join(home, "none.toml"))
	write := func(name, contents string) string {
		t.Helper()
		file := filepath.Join(home, name)
		if err := os.WriteFile(file, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
		return file
	}
	tomlFile := write("swarm.toml", swarmTemplate)
	markdown := "---\nkind: swarm\nname: example-swarm\ngoal: Deliver the change described in the brief\nreply: owner-session\nevery: 10m\nmax_lifetime: 1d\nbudget: {concurrent_runs: 2}\nroles:\n  - name: implement\n    shape: single\n    order: {repo: example-repo, base: main, validation: none}\n---\n" +
		strings.Split(swarmTemplate, "'''")[1]
	mdFile := write("swarm.md", markdown)
	want, _, err := runOrchestrate("swarm", "up", "-f", mdFile, "--dry-run")
	if err != nil || !strings.Contains(want, "--- order ") {
		t.Fatalf("Markdown swarm preview: %v\n%s", err, want)
	}
	for _, args := range [][]string{{"apply"}, {"swarm", "up"}} {
		out, _, err := runOrchestrate(append(args, "-f", tomlFile, "--dry-run")...)
		if err != nil || out != want {
			t.Fatalf("TOML swarm preview %v: %v\n%s\nwant:\n%s", args, err, out, want)
		}
	}

	for _, file := range []string{write("climb.toml", climbTemplate), write("combined.toml", swarmClimbTemplate)} {
		for _, flags := range [][]string{nil, {"--dry-run"}} {
			out, _, err := runOrchestrate(append([]string{"apply", "-f", file}, flags...)...)
			if err == nil || !strings.Contains(err.Error(), "climb runtime (PRs #322/#323)") || out != "" {
				t.Fatalf("climb apply: %v\n%s", err, out)
			}
		}
	}
	if _, err := os.Stat(dispatch.OrchestratorConfigDir()); !os.IsNotExist(err) {
		t.Fatalf("preview or refused apply created orchestrator config: %v", err)
	}
}

func TestOrchestrateHelpExposesWrapperCommands(t *testing.T) {
	out, _, err := runOrchestrate("--help")
	for _, name := range []string{"swarm", "template"} {
		if err != nil || !strings.Contains(out, "\n  "+name+" ") {
			t.Fatalf("missing %s in orchestrate help: %v\n%s", name, err, out)
		}
	}
	compat, _, err := rootCmd.Find([]string{"swarm"})
	if err != nil || compat.Name() != "swarm" || !compat.Hidden {
		t.Fatalf("swarm compatibility command must remain hidden: %v, %v", compat, err)
	}
}
