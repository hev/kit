//go:build !windows

package dispatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdmissionHookPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, script, want string
		list               []string
	}{
		{"allow", "echo '{\"decision\":\"continue\"}'", "", nil},
		{"deny", "echo '{\"decision\":\"block\",\"reason\":\"outside hours\"}'", "outside hours", nil},
		{"allowlist", "echo '{\"decision\":\"continue\"}'", "not admitted", []string{}},
		{"timeout", "exec /bin/sleep 60", "", nil},
		{"crash", "echo 'sensitive stderr' >&2; exit 4", "", nil},
		{"garbage", "echo nonsense", "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("PATH", home)
			t.Setenv("HEV_HOOKS", "")
			cfg := filepath.Join(home, "config.toml")
			t.Setenv("HEV_CONFIG", cfg)
			timeout := "10s"
			if tc.name == "timeout" {
				timeout = "300ms"
			}
			if err := os.WriteFile(cfg, []byte("[hooks]\ntimeout = \""+timeout+"\"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			script := "#!/bin/sh\n[ \"$1\" = admission-check ] || exit 0\n" + tc.script + "\n"
			if err := os.WriteFile(filepath.Join(home, "hev-hook-policy"), []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			err := admitWork(Config{Admission: Admission{Repos: tc.list}}, "", Meta{Order: Order{Name: "fixture", Repo: "kit"}}, "start")
			if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("admission = %v, want %q", err, tc.want)
			}
			raw, err := os.ReadFile(filepath.Join(home, ".hev", "hooks", "events.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			denied := 0
			if tc.want != "" {
				denied = 1
				if !strings.Contains(string(raw), tc.want) {
					t.Fatalf("denial reason absent: %s", raw)
				}
			}
			checks := 1
			if tc.list != nil {
				checks = 0
			}
			if strings.Count(string(raw), `"event":"admission-denied"`) != denied ||
				strings.Count(string(raw), `"event":"admission-check"`) != checks {
				t.Fatalf("unexpected admission events: %s", raw)
			}
		})
	}
}

func TestQueuedLaunchHookIsOnceAcrossHarnessFallbacks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", home)
	t.Setenv("HEV_CONFIG", filepath.Join(home, "config.toml"))
	t.Setenv("HEV_HOOKS", "")
	m := Meta{Order: Order{Name: "run", Repo: "kit", Harness: "codex"}}
	recordQueuedLaunch(home, m)
	m.Harness = "claude"
	recordQueuedLaunch(home, m)
	raw, err := os.ReadFile(filepath.Join(home, ".hev", "hooks", "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(raw), `"event":"order-started"`) != 1 ||
		!strings.Contains(string(raw), `"worker_harness":"codex"`) ||
		strings.Contains(string(raw), "claude") {
		t.Fatalf("unexpected launch events: %s", raw)
	}
}
