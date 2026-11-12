package dispatch

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

// The CI runner's job forbids breakaway, so the direct start runs without it;
// only the environment the supervisor receives matters here.
func TestWindowsNestedHoldDirectSupervisorKeepsHeldValues(t *testing.T) {
	oldStart, oldTasks := startDetached, schtasks
	t.Cleanup(func() { startDetached, schtasks = oldStart, oldTasks })
	startDetached = func(c *exec.Cmd) error {
		c.SysProcAttr.CreationFlags &^= windows.CREATE_BREAKAWAY_FROM_JOB
		return c.Start()
	}
	schtasks = func(context.Context, ...string) error {
		t.Error("direct start fell back to Task Scheduler")
		return errors.New("unexpected schtasks")
	}
	testNestedHoldDirectSupervisor(t)
}

// schtasks is a helper child: it never receives a plugin-only name, even from
// a launcher that has not held them.
func TestWindowsSchtasksHelperOmitsPluginOnlyEnv(t *testing.T) {
	_, dump := pluginEnvFixture(t)
	bin := t.TempDir()
	installEnvDump(t, bin, "schtasks")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if e := runSchtasks(context.Background(), "/Query", "/TN", "hev-orchestrator-env"); e != nil {
		t.Fatal(e)
	}
	pluginOnlyAbsent(t, dump, "schtasks", "schtasks helper")
}

// Before any hold, a forced fallback whose profile has the caller's
// plugin-only values goes ahead. Every schtasks child it runs omits them, and
// the task definition names neither them nor their values.
func TestWindowsOrchestratorFallbackSchedulerOmitsPluginOnlyEnv(t *testing.T) {
	s, dump := pluginEnvFixture(t)
	dir := OrchestratorDir(s.Name)
	realProbes(t)
	t.Cleanup(func() { os.Remove(filepath.Join(dir, "supervisor.json")) })
	bin := t.TempDir()
	installEnvDump(t, bin, "schtasks")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	f := &fakeScheduler{}
	fakeFallback(t, f)
	var calls []string
	schtasks = func(ctx context.Context, args ...string) error {
		if args[0] == "/Create" {
			definition, e := readTaskDefinition(args[4])
			if e != nil {
				t.Fatal(e)
			}
			for _, secret := range []string{pluginSecret, otherSecret, "secret-value"} {
				if strings.Contains(definition, secret) {
					t.Fatalf("task definition carries %s", secret)
				}
			}
		}
		e := runSchtasks(ctx, args...)
		calls = append(calls, args[0])
		pluginOnlyAbsent(t, dump, "schtasks", "schtasks "+args[0])
		if args[0] == "/Run" && e == nil {
			// The task's supervisor would record itself; this process stands in.
			e = writePID(dir)
		}
		return e
	}
	if e := LaunchOrchestrator(s.Name); e != nil {
		t.Fatal(e)
	}
	if got := strings.Join(calls, " "); got != "/Create /Run /Delete" {
		t.Fatalf("schtasks calls %q", got)
	}
}

// After the hold, a held GH_TOKEN the profile lacks is refused by name before
// a task definition is written or schtasks runs: the task could neither carry
// the secret nor start the supervisor without it.
func TestWindowsOrchestratorFallbackRefusesHeldPluginOnlyEnv(t *testing.T) {
	s := fallbackHome(t)
	dir := OrchestratorDir(s.Name)
	other := strings.Replace(strings.Replace(orchestratorExample, "name: release-train", "name: other", 1), "source: none", "source:\n  plugin: other-source\n  env: [GH_TOKEN]", 1)
	if _, e := ApplyOrchestrator(other, false); e != nil {
		t.Fatal(e)
	}
	t.Setenv("GH_TOKEN", "synthetic-token")
	holdForTest(t)
	f := &fakeScheduler{}
	fakeFallback(t, f)
	e := LaunchOrchestrator(s.Name)
	if len(f.calls) != 0 || f.definition != "" {
		t.Fatalf("launch ran schtasks %q without the held GH_TOKEN; definition:\n%s", f.calls, f.definition)
	}
	if e == nil || !strings.Contains(e.Error(), "cannot reproduce GH_TOKEN (plugin-only)") || strings.Contains(e.Error(), "synthetic-token") {
		t.Fatalf("launch error %v", e)
	}
	if exists(filepath.Join(dir, "supervisor.json")) || len(launchTasks(t, dir)) != 0 {
		t.Fatal("refused launch started a supervisor")
	}
	if xml, _ := filepath.Glob(filepath.Join(dir, "*.xml")); len(xml) != 0 {
		t.Fatalf("refused launch wrote %q", xml)
	}
}
