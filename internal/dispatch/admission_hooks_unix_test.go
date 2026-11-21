//go:build !windows

package dispatch

import (
	"encoding/json"
	"errors"
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
		{"deny-diagnostic", `echo '{"decision":"block","reason":"repo:/private/project=archive"}'`, "[redacted]", nil},
		{"deny-punctuation", `echo '{"decision":"block","reason":"workspace</private/project>"}'`, "[redacted]", nil},
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
			capture := filepath.Join(home, "denied.json")
			t.Setenv("DENIED_CAPTURE", capture)
			script := "#!/bin/sh\n" +
				"if [ \"$1\" = admission-denied ]; then /bin/cat > \"$DENIED_CAPTURE\"; exit; fi\n" +
				"[ \"$1\" = admission-check ] || exit 0\n" + tc.script + "\n"
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
				payload, err := os.ReadFile(capture)
				var event hooks.Event
				if err != nil || json.Unmarshal(payload, &event) != nil || !strings.Contains(event.Reason, tc.want) ||
					strings.Contains(string(payload), "/private/project") {
					t.Fatalf("unexpected denial payload: %s (%v)", payload, err)
				}
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
	for _, tc := range []struct {
		name, primaryScript, wantHarness string
	}{
		{"start-failure", "#!/missing-fixture-interpreter\n", "claude"},
		{"exit-failure", "#!/bin/sh\nexit 1\n", "codex"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			queueTestHome(t)
			home := os.Getenv("HOME")
			t.Setenv("HEV_HOOKS", "")
			t.Setenv("HEV_CONFIG", filepath.Join(home, "hooks.toml"))
			bin := t.TempDir()
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			for name, script := range map[string]string{
				"codex": tc.primaryScript, "claude": "#!/bin/sh\nexit 0\n",
				"gh": "#!/bin/sh\nexit 1\n",
			} {
				if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0700); err != nil {
					t.Fatal(err)
				}
			}
			source, repo := filepath.Join(home, "source"), filepath.Join(home, "repo")
			for _, args := range [][]string{
				{"init", "-b", "main", source},
				{"-C", source, "-c", "user.name=Test", "-c", "user.email=test@example.test", "commit", "--allow-empty", "-m", "initial"},
				{"clone", source, repo},
			} {
				if _, err := command("", append([]string{"git"}, args...)...); err != nil {
					t.Fatal(err)
				}
			}
			q := queueRequest("queued-launch")
			q.Order = strings.Replace(q.Order, "repo: kit", "repo: "+repo+"\nharness: codex\nvalidation: none\nfallbacks: [claude]", 1)
			var err error
			q.Meta.Order, err = Parse(q.Order)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := enqueue(q); err != nil {
				t.Fatal(err)
			}
			prior := runLauncher
			t.Cleanup(func() { runLauncher = prior })
			var launched string
			// Dequeue hands off to the supervisor under lock. Run its real
			// round after the handoff returns and releases that lock.
			runLauncher = func(_, name string) error {
				launched = name
				return nil
			}
			if err := SweepQueue(); err != nil || launched != q.Name {
				t.Fatalf("dequeue launched %q: %v", launched, err)
			}
			if err := superviseRound(launched); err != nil {
				t.Fatal(err)
			}
			var result Result
			if err := readJSON(filepath.Join(Root(), launched, "result.json"), &result); err != nil ||
				len(result.Attempts) != 2 || result.Attempts[0].Status != "failed" ||
				result.Attempts[1].Harness != "claude" || result.ExitCode != 0 {
				t.Fatalf("fallback result: %+v (%v)", result, err)
			}
			raw, err := os.ReadFile(filepath.Join(home, ".hev", "hooks", "events.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			for _, line := range strings.Split(string(raw), "\n") {
				var ev hooks.Event
				if json.Unmarshal([]byte(line), &ev) != nil || ev.Event != "order-started" {
					continue
				}
				count++
				if ev.Data["name"] != q.Name || ev.Data["worker_harness"] != tc.wantHarness {
					t.Fatalf("unexpected launch event: %s", line)
				}
			}
			if count != 1 || strings.Count(string(raw), `"event":"admission-check"`) != 1 {
				t.Fatalf("expected one admission check and one launch: %s", raw)
			}
		})
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
	if _, err := reserveQueueEntry(resident, resident.Meta.Order, ""); err != nil {
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

func TestQueueAdmissionClaimRefusesDuplicateStart(t *testing.T) {
	queueTestHome(t)
	home := os.Getenv("HOME")
	t.Setenv("HEV_HOOKS", "")
	t.Setenv("HEV_CONFIG", filepath.Join(home, "hooks.toml"))
	t.Setenv("PATH", home+string(os.PathListSeparator)+os.Getenv("PATH"))
	q := queueRequest("duplicate")
	release, err := claimQueueAdmission(q.Name)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	for _, start := range []func(Request) (Response, error){Handle, enqueue} {
		if _, err = start(q); !errors.Is(err, errRunExists) {
			t.Fatalf("duplicate crossed the pending queue claim: %v", err)
		}
	}
	if exists(filepath.Join(home, ".hev", "hooks", "events.jsonl")) {
		t.Fatal("a refused duplicate ran the policy hook")
	}
	release()
	if _, err = enqueue(q); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(home, ".hev", "hooks", "events.jsonl"))
	if err != nil || strings.Count(string(raw), `"event":"admission-check"`) != 1 {
		t.Fatalf("admission after claim release: %s (%v)", raw, err)
	}
}

func TestRestartAdmissionCanInspectQueueBeforeDenying(t *testing.T) {
	for _, action := range []string{"amend", "amend-check", "rework"} {
		t.Run(action, func(t *testing.T) {
			dir, m := amendFixture(t, false)
			home := os.Getenv("HOME")
			t.Setenv("HEV_HOOKS", "")
			t.Setenv("HEV_CONFIG", filepath.Join(home, "hooks.toml"))
			t.Setenv("HEV_DISPATCH_CONFIG", filepath.Join(home, "target.toml"))
			t.Setenv("PATH", home+string(os.PathListSeparator)+os.Getenv("PATH"))
			self, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv("QUEUE_TEST_BINARY", self)
			policy := filepath.Join(home, "hev-hook-inspect")
			for path, content := range map[string]string{
				os.Getenv("HEV_CONFIG"):          "[hooks]\ntimeout = \"10s\"\n",
				os.Getenv("HEV_DISPATCH_CONFIG"): "cap = 2\n",
				policy: `#!/bin/sh
[ "$1" = admission-check ] || exit 0
QUEUE_INSPECTION_HELPER=1 "$QUEUE_TEST_BINARY" -test.run '^TestQueuedAdmissionInspectionHelper$' || exit 2
echo '{"decision":"block","reason":"queue inspected"}'
`,
			} {
				if err = os.WriteFile(path, []byte(content), 0700); err != nil {
					t.Fatal(err)
				}
			}
			waiting := queueRequest("already-waiting")
			if _, err = reserveQueueEntry(waiting, waiting.Meta.Order, ""); err != nil {
				t.Fatal(err)
			}
			q := Request{Action: action, Name: m.Name, Order: good + "\nRevised.\n", Note: "Revised."}
			if strings.HasPrefix(action, "amend-") {
				q.Name = "imported"
				q.Order = strings.Replace(q.Order, "name: example", "name: imported", 1)
				o, err := Parse(q.Order)
				if err != nil {
					t.Fatal(err)
				}
				q.Meta = Meta{Order: o, Branch: "d/imported"}
			}
			if _, err = Handle(q); err == nil || !strings.Contains(err.Error(), "queue inspected") {
				t.Fatalf("policy could not inspect the queue: %v", err)
			}
			current, err := os.ReadFile(filepath.Join(dir, "order.md"))
			if err != nil || string(current) != good || exists(filepath.Join(Root(), "imported")) ||
				exists(filepath.Join(dir, "admission.json")) {
				t.Fatalf("denial modified the order or leaked custody: %s (%v)", current, err)
			}
			logPath := filepath.Join(home, ".hev", "hooks", "events.jsonl")
			raw, err := os.ReadFile(logPath)
			if err != nil || strings.Count(string(raw), `"event":"admission-check"`) != 1 ||
				strings.Count(string(raw), `"event":"admission-denied"`) != 1 {
				t.Fatalf("unexpected policy events: %s (%v)", raw, err)
			}
			// Simulate a receiver policy update while the external hook runs.
			r167Busy(t)
			target := os.Getenv("HEV_DISPATCH_CONFIG")
			script := `#!/bin/sh
[ "$1" = admission-check ] || exit 0
printf 'cap = 1\n' > "$HEV_DISPATCH_CONFIG"
echo '{"decision":"continue"}'
`
			if err = os.WriteFile(policy, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			if _, err = Handle(q); err == nil || !strings.Contains(err.Error(), "host full") {
				t.Fatalf("launch used stale capacity: %v", err)
			}
			current, err = os.ReadFile(filepath.Join(dir, "order.md"))
			if err != nil || string(current) != good || exists(filepath.Join(Root(), "imported")) ||
				exists(filepath.Join(dir, "admission.json")) {
				t.Fatalf("capacity refusal modified the order or leaked custody: %s (%v)", current, err)
			}
			if err = os.WriteFile(target, []byte("cap = 2\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if err = os.Remove(policy); err != nil {
				t.Fatal(err)
			}
			oldLaunch := runLauncher
			t.Cleanup(func() { runLauncher = oldLaunch })
			runLauncher = func(string, string) error { return nil }
			if _, err = Handle(q); err != nil {
				t.Fatalf("retry after denial: %v", err)
			}
		})
	}
}
