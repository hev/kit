//go:build !windows

package dispatch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/hooks"
)

func TestAdmissionHookPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, script, want string
		list               []string
	}{
		{"allow", "echo '{\"decision\":\"continue\"}'", "", nil},
		{"deny", "echo '{\"decision\":\"block\",\"reason\":\"outside hours\"}'", "outside hours", nil},
		{"deny-path", `echo '{"decision":"block","reason":"avoid /private/project=archive"}'`, "[redacted]", nil},
		{"deny-assignment", `echo '{"decision":"block","reason":"avoid path=/private/project"}'`, "[redacted]", nil},
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
			if strings.Contains(string(raw), "/private/project") {
				t.Fatalf("path escaped hook event redaction: %s", raw)
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

// Run via the actual hook child, so it must take the same receiver lock as
// dispatch show while the admission caller waits for its policy decision.
func TestQueuedAdmissionInspectionHelper(t *testing.T) {
	if os.Getenv("QUEUE_INSPECTION_HELPER") != "1" {
		return
	}
	_, found, err := controlQueued(Request{Action: "show", Name: "already-waiting"})
	if err != nil || !found {
		os.Exit(2)
	}
	os.Exit(0)
}

func TestQueuedAdmissionCanInspectQueueBeforeDenying(t *testing.T) {
	queueTestHome(t)
	resident := queueRequest("already-waiting")
	if _, err := reserveQueueEntry(resident, resident.Meta.Order); err != nil {
		t.Fatal(err)
	}
	home := os.Getenv("HOME")
	cfg := filepath.Join(home, "hooks.toml")
	t.Setenv("HEV_CONFIG", cfg)
	t.Setenv("HEV_HOOKS", "")
	if err := os.WriteFile(cfg, []byte("[hooks]\ntimeout = \"10s\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("QUEUE_TEST_BINARY", self)
	t.Setenv("PATH", home+string(os.PathListSeparator)+os.Getenv("PATH"))
	script := `#!/bin/sh
[ "$1" = admission-check ] || exit 0
QUEUE_INSPECTION_HELPER=1 "$QUEUE_TEST_BINARY" -test.run '^TestQueuedAdmissionInspectionHelper$' || exit 2
echo '{"decision":"block","reason":"queue inspected"}'
`
	policy := filepath.Join(home, "hev-hook-inspect")
	if err := os.WriteFile(policy, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	q := queueRequest("new-order")
	if _, err := Handle(q); err == nil || !strings.Contains(err.Error(), "queue inspected") {
		t.Fatalf("policy could not inspect the queue: %v", err)
	}
	if err := os.Remove(policy); err != nil {
		t.Fatal(err)
	}
	if _, err := Handle(q); err != nil {
		t.Fatal(err)
	}
	if _, err := Handle(q); err == nil {
		t.Fatal("duplicate order was accepted")
	}
	raw, err := os.ReadFile(filepath.Join(home, ".hev", "hooks", "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	events := map[string]int{}
	for _, line := range strings.Split(string(raw), "\n") {
		var ev hooks.Event
		if json.Unmarshal([]byte(line), &ev) != nil {
			continue
		}
		events[ev.Event]++
		if ev.Data["name"] != q.Name || ev.Data["repo"] != q.Meta.Repo {
			t.Fatalf("incorrect queue metadata: %s", line)
		}
		if ev.Event == "order-queued" && ev.Data["worker_harness"] != q.Meta.Harness {
			t.Fatalf("incorrect worker harness: %s", line)
		}
	}
	if events["admission-denied"] != 1 || events["admission-check"] != 2 || events["order-queued"] != 1 {
		t.Fatalf("unexpected events: %s", raw)
	}
}
