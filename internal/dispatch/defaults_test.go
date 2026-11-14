package dispatch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResolveDefaultsLayersAndPairing(t *testing.T) {
	for _, role := range []Role{RoleWorker, RoleOrchestrator, RoleValidation} {
		for _, tc := range []struct {
			name                   string
			node, explicit         HarnessModel
			harness, model, hs, ms string
			inferred               bool
		}{
			{"builtin", HarnessModel{}, HarnessModel{}, "claude", "claude-opus-5-5", "builtin", "builtin", false},
			{"node", HarnessModel{"codex", "gpt-6.1-sol"}, HarnessModel{}, "codex", "gpt-6.1-sol", "node", "node", false},
			{"explicit", HarnessModel{"codex", "gpt-6.1-sol"}, HarnessModel{"claude", "claude-fable-5-2"}, "claude", "claude-fable-5-2", "explicit", "explicit", false},
			{"same-harness", HarnessModel{"codex", "gpt-6.1-sol"}, HarnessModel{"codex", ""}, "codex", "gpt-6.1-sol", "explicit", "node", false},
			{"different-harness", HarnessModel{"codex", "gpt-6.1-sol"}, HarnessModel{"claude", ""}, "claude", "claude-opus-5-5", "explicit", "builtin", false},
			{"codex-own-model", HarnessModel{"claude", "claude-fable-5-2"}, HarnessModel{"codex", ""}, "codex", "", "explicit", "builtin", false},
			{"infer-claude", HarnessModel{"codex", "gpt-6.1-sol"}, HarnessModel{"", "claude-fable-5-2"}, "claude", "claude-fable-5-2", "explicit", "explicit", true},
			{"infer-codex", HarnessModel{}, HarnessModel{"", "gpt-6.1-sol"}, "codex", "gpt-6.1-sol", "explicit", "explicit", true},
			{"node-inference", HarnessModel{"", "gpt-6.1-sol"}, HarnessModel{}, "codex", "gpt-6.1-sol", "node", "node", true},
			{"node-harness-only", HarnessModel{"codex", ""}, HarnessModel{}, "codex", "", "node", "builtin", false},
			{"model-overrides-node", HarnessModel{"claude", "claude-fable-5-2"}, HarnessModel{"", "claude-opus-5-5"}, "claude", "claude-opus-5-5", "explicit", "explicit", true},
		} {
			t.Run(string(role)+"/"+tc.name, func(t *testing.T) {
				got, err := ResolveDefaults(Config{Defaults: map[Role]HarnessModel{role: tc.node}}, role, tc.explicit)
				if err != nil {
					t.Fatal(err)
				}
				if got.Harness != tc.harness || got.Model != tc.model || got.HarnessSource != tc.hs || got.ModelSource != tc.ms || got.HarnessInferred != tc.inferred {
					t.Fatalf("%+v", got)
				}
			})
		}
		for _, cfg := range []Config{{Defaults: map[Role]HarnessModel{role: {Harness: "bogus"}}}, {}} {
			explicit := HarnessModel{}
			if len(cfg.Defaults) == 0 {
				explicit.Harness = "bogus"
			}
			if _, err := ResolveDefaults(cfg, role, explicit); err == nil {
				t.Fatal("invalid harness accepted")
			}
		}
	}
	if _, err := ResolveDefaults(Config{}, "bogus", HarnessModel{}); err == nil {
		t.Fatal("invalid role accepted")
	}
}
func TestDefaultsConfigAliasAndTarget(t *testing.T) {
	for _, tc := range []struct{ text, harness, model string }{
		{"[validation]\nharness='codex'\nmodel='gpt-6.1-sol'", "codex", "gpt-6.1-sol"},
		{"[validation]\nharness='codex'\nmodel='old'\n[defaults.validation]\nharness='claude'\nmodel='claude-fable-5-2'", "claude", "claude-fable-5-2"},
	} {
		p := filepath.Join(t.TempDir(), "dispatch.toml")
		if err := os.WriteFile(p, []byte(tc.text), 0600); err != nil {
			t.Fatal(err)
		}
		for _, target := range []bool{false, true} {
			cfg, err := loadConfig(p, "", target)
			if err != nil {
				t.Fatal(err)
			}
			r, err := ResolveDefaults(cfg, RoleValidation, HarnessModel{})
			if err != nil || r.Harness != tc.harness || r.Model != tc.model || r.HarnessSource != "node" {
				t.Fatalf("%+v %v", r, err)
			}
		}
	}
}
func TestRoleParsersLeaveDefaultsUnset(t *testing.T) {
	o, err := Parse("---\nname: defaults\nrepo: app\n---\n## Acceptance\n- Done\n")
	if err != nil || o.Harness != "" || o.Model != "" {
		t.Fatalf("%+v %v", o, err)
	}
	s, err := ParseOrchestrator(strings.Replace(orchestratorExample, "harness: claude\n", "", 1))
	if err != nil || s.Harness != "" || s.Model != "" {
		t.Fatalf("%+v %v", s, err)
	}
}
func TestHarnessPreflightError(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	err := preflightHarness("codex", "node")
	for _, part := range []string{"codex", "PATH=", os.Getenv("PATH"), "source=node", "config="} {
		if err == nil || !strings.Contains(err.Error(), part) {
			t.Fatalf("missing %q: %v", part, err)
		}
	}
}

func TestDispatchResolvesOnExecutingNode(t *testing.T) {
	for _, tc := range []struct{ name, config, fields, harness, model, hs, ms string }{
		{"builtin", "", "", "claude", "claude-opus-5-5", "builtin", "builtin"},
		{"node", "[defaults.worker]\nharness='codex'\nmodel='gpt-6.1-sol'\n", "", "codex", "gpt-6.1-sol", "node", "node"},
		{"explicit", "[defaults.worker]\nharness='codex'\nmodel='gpt-6.1-sol'\n", "harness: claude\nmodel: claude-fable-5-2\n", "claude", "claude-fable-5-2", "explicit", "explicit"},
		{"inferred", "", "model: gpt-6.1-sol\n", "codex", "gpt-6.1-sol", "explicit", "explicit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newContextHarness(t)
			if tc.config != "" {
				p := filepath.Join(t.TempDir(), "node.toml")
				if err := os.WriteFile(p, []byte(tc.config), 0600); err != nil {
					t.Fatal(err)
				}
				t.Setenv("HEV_DISPATCH_CONFIG", p)
			}
			text := "---\nname: defaults-test\nrepo: proj\n" + tc.fields + "---\nTask.\n## Acceptance\n- Done\n"
			if err := h.start(text); err != nil {
				t.Fatal(err)
			}
			m := runMeta(t, "defaults-test")
			if m.Harness != tc.harness || m.Model != tc.model || m.HarnessSource != tc.hs || m.ModelSource != tc.ms || m.HarnessInferred != (tc.name == "inferred") {
				t.Fatalf("%+v", m)
			}
			if err := superviseRound(m.Name); err != nil {
				t.Fatal(err)
			}
			_, args := h.captured(1, tc.harness)
			if !strings.Contains(strings.Join(args, " "), "--model "+tc.model) {
				t.Fatal(args)
			}
			r := readResultForDefaults(t, m.Name)
			if r.ExitCode != 0 {
				t.Fatalf("%+v", r)
			}
			// Rework must use the original explicit fields, not promote the previous
			// node/builtin resolution into the explicit layer.
			if _, err := Handle(Request{Action: "rework", Name: m.Name, Note: "Check again"}); err != nil {
				t.Fatal(err)
			}
			after := runMeta(t, m.Name)
			if after.HarnessSource != m.HarnessSource || after.ModelSource != m.ModelSource {
				t.Fatalf("rework changed provenance: %+v", after)
			}
		})
	}
}
func readResultForDefaults(t *testing.T, name string) Result {
	t.Helper()
	var r Result
	if err := readJSON(filepath.Join(Root(), name, "result.json"), &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestOrchestratorUsesNodeResolutionAndRecordsSources(t *testing.T) {
	orchHome(t)
	t.Setenv("HEV_DISPATCH_CONFIG", filepath.Join(t.TempDir(), "dispatch.toml"))
	if err := os.WriteFile(os.Getenv("HEV_DISPATCH_CONFIG"), []byte("[defaults.orchestrator]\nharness='codex'\nmodel='gpt-6.1-sol'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	text := strings.Replace(orchestratorExample, "harness: claude\n", "", 1)
	s, err := ApplyOrchestrator(text, false)
	if err != nil {
		t.Fatal(err)
	}
	st, err := orchestratorState(s.Name)
	if err != nil {
		t.Fatal(err)
	}
	if st.Harness != "codex" || st.Model != "gpt-6.1-sol" || st.HarnessSource != "node" || st.ModelSource != "node" {
		t.Fatalf("%+v", st)
	}
	calls := 0
	d := OrchestratorDriver{Now: time.Now, Gather: func(context.Context, OrchestratorSpec) ([]OrchestratorEvidence, error) { return nil, nil }, Session: func(_ context.Context, got OrchestratorSpec, dir string, p OrchestratorPacket) error {
		calls++
		if got.Harness != "codex" || got.Model != "gpt-6.1-sol" || p.Spec.Harness != got.Harness {
			t.Fatalf("%+v %+v", got, p.Spec)
		}
		return os.WriteFile(filepath.Join(dir, "checkpoint.md"), []byte("Pending next check.\n"), 0600)
	}}
	if err := RunOrchestratorRound(context.Background(), s, &st, d); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("%d calls", calls)
	}
	saved, err := orchestratorState(s.Name)
	if err != nil || saved.HarnessSource != "node" || saved.Model != "gpt-6.1-sol" {
		t.Fatalf("%+v %v", saved, err)
	}
}

func TestDefaultsRPCUsesTargetConfigAndExplicitPair(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	t.Setenv("HEV_DISPATCH_CONFIG", "")
	p := filepath.Join(t.TempDir(), "target.toml")
	if err := os.WriteFile(p, []byte("[defaults.validation]\nharness='codex'\nmodel='gpt-6.1-sol'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	r, err := Handle(Request{Action: "config-defaults", Meta: Meta{ConfigPath: p}, DefaultsRole: RoleValidation, DefaultsExplicit: HarnessModel{Harness: "claude"}})
	if err != nil {
		t.Fatal(err)
	}
	got := r.Defaults[RoleValidation]
	if got.Harness != "claude" || got.Model != "claude-opus-5-5" || got.HarnessSource != "explicit" || got.ModelSource != "builtin" {
		t.Fatalf("%+v", got)
	}
	if r.Defaults[RoleWorker].HarnessSource != "builtin" {
		t.Fatal(r.Defaults)
	}
	if _, err := Handle(Request{Action: "config-defaults", Meta: Meta{ConfigPath: p}, DefaultsRole: "bogus"}); err == nil {
		t.Fatal("unknown role accepted")
	}
}
