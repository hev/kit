package dispatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

// The orchestrator supervisor starts through its own scheduled task, never as
// a child of the caller's console or job: Task Scheduler is the only route,
// and the task runs as this user (S4U) with the launcher's environment.
func TestWindowsOrchestratorSupervisorLaunchesThroughScheduledTask(t *testing.T) {
	orchHome(t)
	s := applyOrch(t)
	dir := OrchestratorDir(s.Name)
	t.Setenv("HEV_LAUNCH_TEST", "from-ssh-session")
	var launchEnv []string
	f := &fakeTasks{}
	f.run = func(string) error {
		var l supervisorLaunch
		if err := readJSON(filepath.Join(dir, supervisorLaunchFile), &l); err != nil {
			return err
		}
		launchEnv = l.Env
		// The scheduled supervisor records itself.
		return writePID(dir)
	}
	useFakeTasks(t, f)
	if err := LaunchOrchestrator(s.Name); err != nil {
		t.Fatal(err)
	}
	task := orchestratorTaskName(s.Name)
	xml := f.registered[task]
	if len(f.ran) != 1 || f.ran[0] != task {
		t.Fatalf("ran %v, want %s", f.ran, task)
	}
	exe, _ := os.Executable()
	for _, want := range []string{"<LogonType>S4U</LogonType>", "<Command>" + exe + "</Command>", "<Arguments>orchestrate supervise " + s.Name + "</Arguments>", "<ExecutionTimeLimit>PT0S</ExecutionTimeLimit>"} {
		if !strings.Contains(xml, want) {
			t.Fatalf("task XML missing %s:\n%s", want, xml)
		}
	}
	if !hasEnvName(launchEnv, "HEV_LAUNCH_TEST") || hasEnvName(launchEnv, "HEV_ORCHESTRATOR") {
		t.Fatalf("launch environment %v", launchEnv)
	}
	// A live supervisor is not launched twice.
	if err := LaunchOrchestrator(s.Name); err != nil || len(f.ran) != 1 {
		t.Fatalf("second launch: %v, ran %v", err, f.ran)
	}
	// Stop removes the task once no supervisor is left to remove its own.
	if err := os.Remove(filepath.Join(dir, "supervisor.json")); err != nil {
		t.Fatal(err)
	}
	if err := OrchestratorControl(s.Name, "stop", ""); err != nil {
		t.Fatal(err)
	}
	if !f.wasRemoved(task) {
		t.Fatalf("stop left %s registered", task)
	}
}

// A run supervisor launched from inside a round (the harness's kill-on-close
// job) or an SSH session takes the same route, under hev\run-NAME.
func TestWindowsRunSupervisorLaunchesThroughScheduledTask(t *testing.T) {
	dir := t.TempDir()
	f := &fakeTasks{run: func(string) error { return writePID(dir) }}
	useFakeTasks(t, f)
	if err := launchSupervisor(dir, "story-agent"); err != nil {
		t.Fatal(err)
	}
	xml := f.registered[runTaskName("story-agent")]
	if !strings.Contains(xml, "<Arguments>dispatch supervise -- story-agent</Arguments>") || !strings.Contains(xml, "<LogonType>S4U</LogonType>") {
		t.Fatalf("run task XML:\n%s", xml)
	}
}

// Apply treats a supervisor Windows will not let it query as live.
func TestWindowsApplyRefusesUnqueryableSupervisor(t *testing.T) {
	orchHome(t)
	s := applyOrch(t)
	dir := OrchestratorDir(s.Name)
	if err := WriteJSON(filepath.Join(dir, "supervisor.json"), pidRecord{PID: 4242, Created: 7}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(dir, "state.json"))
	previous := created
	created = func(pid int) (uint64, error) {
		if pid == 4242 {
			return 0, windows.ERROR_ACCESS_DENIED
		}
		return previous(pid)
	}
	t.Cleanup(func() { created = previous })
	if _, err := ApplyOrchestrator(orchestratorExample, false); err == nil || !strings.Contains(err.Error(), "stop orchestrator before applying changes") {
		t.Fatalf("apply error %v", err)
	}
	if after, _ := os.ReadFile(filepath.Join(dir, "state.json")); string(after) != string(before) {
		t.Fatal("refused apply changed state.json")
	}
}
