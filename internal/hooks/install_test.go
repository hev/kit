package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const foreignSettings = `{
  "model": "opus",
  "permissions": {"allow": ["Bash(ls:*)"]},
  "hooks": {
    "PreToolUse": [
      {"matcher": "Bash", "hooks": [{"type": "command", "command": "~/bin/guard.sh", "timeout": 5}]}
    ],
    "SessionEnd": [
      {"hooks": [{"type": "command", "command": "factory-feedback capture-hook --harness claude", "timeout": 3}]}
    ]
  },
  "statusLine": {"type": "command", "command": "echo ✓ ünïcode"}
}
`

func readSettings(t *testing.T, path string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("%s: %v", b, err)
	}
	return v
}

func commands(v map[string]any, event string) []string {
	var out []string
	groups, _ := v["hooks"].(map[string]any)[event].([]any)
	for _, g := range groups {
		for _, h := range g.(map[string]any)["hooks"].([]any) {
			out = append(out, h.(map[string]any)["command"].(string))
		}
	}
	return out
}

func TestInstallMergesIdempotentAndUninstallsOnlyKit(t *testing.T) {
	for _, harness := range Harnesses {
		t.Run(harness, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("CODEX_HOME", "")
			path := SettingsPath(home, harness)
			os.MkdirAll(filepath.Dir(path), 0700)
			if err := os.WriteFile(path, []byte(strings.Replace(foreignSettings, "PreToolUse", nativeName(harness, "pre-tool"), 1)), 0640); err != nil {
				t.Fatal(err)
			}
			exe := "/opt/kit bin/hev"
			changed, err := Install(home, harness, exe)
			if err != nil || !changed {
				t.Fatalf("install: %v %v", changed, err)
			}
			v := readSettings(t, path)
			if v["model"] != "opus" || v["statusLine"].(map[string]any)["command"] != "echo ✓ ünïcode" {
				t.Fatalf("foreign settings changed: %v", v)
			}
			raw, _ := os.ReadFile(path)
			if !strings.Contains(string(raw), "ünïcode") {
				t.Fatal("unicode escaped in the person's file")
			}
			pre := commands(v, nativeName(harness, "pre-tool"))
			if len(pre) != 2 || pre[0] != "~/bin/guard.sh" || pre[1] != `'/opt/kit bin/hev' hook pre-tool --harness `+harness+" "+Stamp {
				t.Fatalf("PreToolUse: %q", pre)
			}
			if got := commands(v, "SessionEnd"); len(got) != 1 || !strings.HasPrefix(got[0], "factory-feedback") {
				t.Fatalf("SessionEnd: %q", got)
			}
			for _, ev := range Events {
				if got := commands(v, nativeName(harness, ev)); len(got) == 0 || !strings.HasSuffix(got[len(got)-1], Stamp) {
					t.Fatalf("%s missing kit entry: %q", ev, got)
				}
			}
			if info, _ := os.Stat(path); info.Mode().Perm() != 0640 {
				t.Fatalf("mode changed to %v", info.Mode())
			}
			if _, err := os.Stat(path + ".before-hev-hooks"); err != nil {
				t.Fatal("no backup of the original")
			}
			st, err := Inspect(home, harness, exe)
			if err != nil || !st.Current() || st.Foreign != 1 || len(st.Kit) != len(Events) {
				t.Fatalf("inspect: %+v %v", st, err)
			}

			// Idempotent: a second install changes nothing.
			before, _ := os.ReadFile(path)
			if changed, err := Install(home, harness, exe); err != nil || changed {
				t.Fatalf("second install: %v %v", changed, err)
			}
			after, _ := os.ReadFile(path)
			if string(before) != string(after) {
				t.Fatal("second install rewrote the file")
			}

			// A new binary is stale until repaired in place, never duplicated.
			if st, _ := Inspect(home, harness, "/new/hev"); st.Current() || len(st.Stale) != len(Events) {
				t.Fatalf("stale not detected: %+v", st)
			}
			if _, err := Install(home, harness, "/new/hev"); err != nil {
				t.Fatal(err)
			}
			if pre := commands(readSettings(t, path), nativeName(harness, "pre-tool")); len(pre) != 2 || !strings.HasPrefix(pre[1], "/new/hev hook") {
				t.Fatalf("repair: %q", pre)
			}

			n, err := Uninstall(home, harness)
			if err != nil || n != len(Events) {
				t.Fatalf("uninstall removed %d: %v", n, err)
			}
			v = readSettings(t, path)
			if pre := commands(v, nativeName(harness, "pre-tool")); len(pre) != 1 || pre[0] != "~/bin/guard.sh" {
				t.Fatalf("uninstall touched foreign hooks: %q", pre)
			}
			if _, ok := v["hooks"].(map[string]any)[nativeName(harness, "post-tool")]; ok {
				t.Fatal("empty kit-only event left behind")
			}
			if got := commands(v, "SessionEnd"); len(got) != 1 {
				t.Fatalf("SessionEnd: %q", got)
			}
			if n, _ := Uninstall(home, harness); n != 0 {
				t.Fatal("second uninstall removed entries")
			}
		})
	}
}

func TestInstallFreshAndRefusesBrokenSettings(t *testing.T) {
	home := t.TempDir()
	if _, err := Install(home, "claude", "/bin/hev"); err != nil {
		t.Fatal(err)
	}
	if v := readSettings(t, SettingsPath(home, "claude")); len(v["hooks"].(map[string]any)) != len(Events) {
		t.Fatalf("%v", v)
	}
	if n, _ := Uninstall(home, "claude"); n != len(Events) {
		t.Fatal(n)
	}
	if v := readSettings(t, SettingsPath(home, "claude")); v["hooks"] != nil {
		t.Fatalf("hooks key left: %v", v)
	}
	bad := filepath.Join(home, ".claude", "settings.json")
	os.WriteFile(bad, []byte(`{"hooks": [1,2]`), 0600)
	if _, err := Install(home, "claude", "/bin/hev"); err == nil {
		t.Fatal("installed over unparseable settings")
	}
	if b, _ := os.ReadFile(bad); string(b) != `{"hooks": [1,2]` {
		t.Fatal("broken settings rewritten")
	}
}

func TestInvocationWiring(t *testing.T) {
	args := CodexInvocationArgs("/x/hev")
	if args[0] != "--dangerously-bypass-hook-trust" || len(args) != 1+2*len(Events) {
		t.Fatalf("%q", args)
	}
	if !strings.Contains(strings.Join(args, " "), `hooks.PreToolUse=[{ hooks = [{ type = "command", command = "/x/hev hook pre-tool --harness codex", timeout = 10 }] }]`) {
		t.Fatalf("%q", args)
	}
	if strings.Contains(strings.Join(args, " "), Stamp) {
		t.Fatal("invocation hooks must not carry the settings stamp")
	}
	h := InvocationHooks("/x/hev", "claude")
	if h.Len() != len(Events) {
		t.Fatal(h.Keys())
	}
}

func TestGeminiSettingsPreserveProjectHookGroups(t *testing.T) {
	project := t.TempDir()
	path := SettingsPath(project, "gemini")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	foreign := `{"security":{"folderTrust":{"enabled":true}},"hooks":{"BeforeTool":[{"matcher":"read_file","sequential":true,"hooks":[{"type":"command","name":"reviewed","command":"echo ok","timeout":3000}]}],"SessionEnd":[]}}`
	if err := os.WriteFile(path, []byte(foreign), 0600); err != nil {
		t.Fatal(err)
	}
	original := readSettings(t, path)
	if changed, err := Install(project, "gemini", "hev"); err != nil || !changed {
		t.Fatalf("install %v %v", changed, err)
	}
	groups := readSettings(t, path)["hooks"].(map[string]any)["BeforeTool"].([]any)
	if len(groups) != 2 {
		t.Fatalf("groups %v", groups)
	}
	installed := groups[1].(map[string]any)
	if installed["matcher"] != nil || installed["hooks"].([]any)[0].(map[string]any)["timeout"] != float64(10000) {
		t.Fatalf("kit group %v", installed)
	}
	if removed, err := Uninstall(project, "gemini"); err != nil || removed != len(Events) {
		t.Fatalf("removed %v %v", removed, err)
	}
	if result := readSettings(t, path); !reflect.DeepEqual(result, original) {
		t.Fatalf("settings %v, want %v", result, original)
	}
}
