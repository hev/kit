package harness

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLookup(t *testing.T) {
	want := map[string]string{
		"claude": Claude, "claude_code": Claude, "codex": Codex, "codex_cli": Codex,
		"pi": Pi, "opencode": OpenCode, "agy": Agy, "dot": Dot, "copilot": Copilot,
	}
	for _, d := range All() {
		for _, name := range append([]string{d.Name}, d.Aliases...) {
			if want[name] != d.Name {
				t.Errorf("unexpected registry name %q for %q", name, d.Name)
			}
		}
	}
	for name, canonical := range want {
		d, ok := Lookup(name)
		if !ok || d.Name != canonical || d.Binary != canonical {
			t.Errorf("Lookup(%q) = %+v, %v", name, d, ok)
		}
	}
	for _, name := range []string{"", "unknown", "Claude"} {
		if d, ok := Lookup(name); ok || d.Name != "" {
			t.Errorf("unknown name %q resolved to %q", name, d.Name)
		}
	}
	if _, ok := DispatchFor("claude_code"); ok || HooksFor("codex_cli") != nil {
		t.Fatal("trace aliases changed existing dispatch or hook validation")
	}
}

func TestCapabilities(t *testing.T) {
	t.Setenv("CODEX_HOME", "")
	t.Setenv("COPILOT_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HEV_PI_SESSIONS_DIR", "")
	t.Setenv("HEV_OPENCODE_DATA_DIR", "")
	t.Setenv("HEV_AGY_DATA_DIR", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, root, override, config, dispatchSkills string
		user, project, argv                          []string
		paste, inboxOnly                             bool
	}{
		{"claude", ".claude/projects", "", ".claude/settings.json", ".claude/skills",
			[]string{".claude/skills"}, []string{".claude/skills"},
			[]string{"-p", "--verbose", "--output-format", "stream-json", "--dangerously-skip-permissions", "--model", "sample", "--effort", "high", "prompt"}, true, false},
		{"codex", ".codex/sessions", "", ".codex/hooks.json", ".agents/skills",
			[]string{".codex/skills", ".agents/skills"}, []string{".agents/skills"},
			[]string{"exec", "--json", "--dangerously-bypass-approvals-and-sandbox", "--model", "sample", "-c", `model_reasoning_effort="high"`, "--skip-git-repo-check", "prompt"}, true, false},
		{"pi", ".pi/agent/sessions", "HEV_PI_SESSIONS_DIR", "", "",
			[]string{".pi/agent/skills", ".agents/skills"}, []string{".pi/skills", ".agents/skills"}, nil, false, true},
		{"opencode", ".local/share/opencode", "HEV_OPENCODE_DATA_DIR", "", "", nil, nil, nil, false, false},
		{"agy", ".gemini/antigravity-cli", "HEV_AGY_DATA_DIR", "", ".agents/skills", nil, nil,
			[]string{"--dangerously-skip-permissions", "--output-format", "stream-json", "--model", "sample", "--effort", "high", "--print=prompt"}, false, false},
		{"dot", ".dot/traces", "", "", "", nil, nil, nil, false, false},
		{"copilot", ".copilot/session-state", "COPILOT_HOME", ".copilot/hooks/hev.json", ".github/skills",
			[]string{".copilot/skills", ".agents/skills"}, []string{".github/skills", ".agents/skills", ".claude/skills"},
			[]string{"--allow-all-tools", "--no-ask-user", "--model", "sample", "--reasoning-effort", "high", "--prompt=prompt"}, false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d, ok := Lookup(tt.name)
			if !ok || d.Trace == nil {
				t.Fatal("missing trace capability")
			}
			if got := d.Trace.Root(); got != filepath.Join(home, filepath.FromSlash(tt.root)) {
				t.Errorf("trace root = %q", got)
			}
			if d.Trace.Override != tt.override {
				t.Errorf("trace override = %q", d.Trace.Override)
			}
			if tt.override != "" {
				t.Setenv(tt.override, "override")
				want := "override"
				if tt.name == Copilot {
					want = filepath.Join(want, "session-state")
				}
				if got := d.Trace.Root(); got != want {
					t.Errorf("overridden root = %q", got)
				}
			}
			var user, project, argv []string
			var config, dispatchSkills string
			if d.Skills != nil {
				user, project = d.Skills.User, d.Skills.Project
			}
			if d.Dispatch != nil {
				argv = d.Dispatch.Argv("sample", "high", "prompt", true)
				dispatchSkills = d.Dispatch.ProjectSkills
			}
			if d.Hooks != nil {
				config = d.Hooks.Config
				events := map[string]string{
					"session-start": "SessionStart", "prompt-submit": "UserPromptSubmit",
					"pre-tool": "PreToolUse", "post-tool": "PostToolUse", "stop": "Stop", "pre-compact": "PreCompact",
				}
				if tt.name == Copilot {
					events = map[string]string{"session-start": "sessionStart", "prompt-submit": "userPromptSubmitted",
						"pre-tool": "preToolUse", "post-tool": "postToolUse", "stop": "agentStop", "pre-compact": "preCompact"}
				}
				for event, want := range events {
					if got := d.Hooks.NativeEvent(event); got != want {
						t.Errorf("hook %s = %q, want %q", event, got, want)
					}
				}
			}
			if !reflect.DeepEqual(user, tt.user) || !reflect.DeepEqual(project, tt.project) || dispatchSkills != tt.dispatchSkills {
				t.Errorf("skill dirs user=%v project=%v dispatch=%q", user, project, dispatchSkills)
			}
			if !reflect.DeepEqual(argv, tt.argv) || config != tt.config {
				t.Errorf("argv=%q hook config=%q", argv, config)
			}
			if CanPaste(tt.name) != tt.paste || InboxOnly(tt.name) != tt.inboxOnly {
				t.Errorf("peer delivery changed for %s", tt.name)
			}
		})
	}
}
