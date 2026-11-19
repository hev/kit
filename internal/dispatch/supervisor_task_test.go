package dispatch

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeTasks records Task Scheduler calls; run decides what a task does.
type fakeTasks struct {
	mu         sync.Mutex
	registered map[string]string
	ran        []string
	removed    []string
	run        func(name string) error
	status     func(name string) (taskStatus, error)
}

func (f *fakeTasks) Register(name, xml string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.registered == nil {
		f.registered = map[string]string{}
	}
	f.registered[name] = xml
	return nil
}
func (f *fakeTasks) Run(name string) error {
	f.mu.Lock()
	f.ran = append(f.ran, name)
	run := f.run
	f.mu.Unlock()
	if run != nil {
		return run(name)
	}
	return nil
}
func (f *fakeTasks) Remove(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removed = append(f.removed, name)
	return nil
}
func (f *fakeTasks) Status(name string) (taskStatus, error) {
	if f.status != nil {
		return f.status(name)
	}
	return taskStatus{Running: true}, nil
}
func (f *fakeTasks) wasRemoved(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, n := range f.removed {
		if n == name {
			return true
		}
	}
	return false
}

func useFakeTasks(t *testing.T, f *fakeTasks) {
	t.Helper()
	old := supervisorTasks
	supervisorTasks = f
	t.Cleanup(func() { supervisorTasks = old })
}

func TestWindowsSupervisorTaskDefinition(t *testing.T) {
	xml := renderSupervisorTaskXML(supervisorTask{Name: `hev\orch-ops`, Exe: `C:\Program Files\hev\hev.exe`, Args: []string{"orchestrate", "supervise", "ops"}, Dir: `C:\Users\me\dispatch\orch-ops`, UserSID: "S-1-5-21-1"})
	for _, want := range []string{
		`<URI>\hev\orch-ops</URI>`,
		`<UserId>S-1-5-21-1</UserId><LogonType>S4U</LogonType><RunLevel>LeastPrivilege</RunLevel>`,
		`<ExecutionTimeLimit>PT0S</ExecutionTimeLimit>`,
		`<MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>`,
		`<Command>C:\Program Files\hev\hev.exe</Command><Arguments>orchestrate supervise ops</Arguments><WorkingDirectory>C:\Users\me\dispatch\orch-ops</WorkingDirectory>`,
	} {
		if !strings.Contains(xml, want) {
			t.Fatalf("task XML missing %s:\n%s", want, xml)
		}
	}
	// On demand only: no trigger starts it, and the scheduler never restarts it.
	for _, unwanted := range []string{"<Triggers>", "RestartOnFailure"} {
		if strings.Contains(xml, unwanted) {
			t.Fatalf("task XML has %s:\n%s", unwanted, xml)
		}
	}
	if got := windowsCommandLineArg(`C:\a b\`); got != `"C:\a b\\"` {
		t.Fatalf("quoted %s", got)
	}
	if b := utf16TaskXML(xml); b[0] != 0xff || b[1] != 0xfe {
		t.Fatal("task XML needs a UTF-16LE byte order mark")
	}
}

func TestWindowsTaskStatusParse(t *testing.T) {
	for out, want := range map[string]taskStatus{
		"4 267009":      {Running: true},
		"3 267011":      {},
		"3 0":           {Ran: true},
		"3 1":           {Ran: true, Result: 1},
		"3 -2147024894": {Ran: true, Result: 0x80070002},
	} {
		got, err := parseTaskStatus(out)
		if err != nil || got != want {
			t.Fatalf("%q: %+v %v, want %+v", out, got, err, want)
		}
	}
	if _, err := parseTaskStatus("Ready"); err == nil {
		t.Fatal("accepted a display string")
	}
}

// A launch whose supervisor never records itself fails with the task, its
// exit code and the log, and removes the launch file and task it created.
func TestWindowsSupervisorLaunchUnconfirmedNamesReason(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "supervisor.log")
	if err := os.WriteFile(log, []byte("starting\nopen meta.json: access is denied\n"), 0600); err != nil {
		t.Fatal(err)
	}
	f := &fakeTasks{status: func(string) (taskStatus, error) { return taskStatus{Ran: true, Result: 1}, nil }}
	err := startSupervisorTask(f, supervisorTask{Name: `hev\run-x`, Exe: "hev.exe", Args: []string{"dispatch", "supervise", "--", "x"}, Dir: dir, Env: []string{"A=1"}, Log: log}, 5*time.Second)
	if !errors.Is(err, errLaunchUnconfirmed) {
		t.Fatalf("error %v", err)
	}
	for _, want := range []string{`hev\run-x`, "exited with code 0x1", log, "access is denied"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not name %q", err, want)
		}
	}
	if _, statErr := os.Stat(filepath.Join(dir, supervisorLaunchFile)); !os.IsNotExist(statErr) {
		t.Fatal("failed launch left its environment file behind")
	}
	if !f.wasRemoved(`hev\run-x`) {
		t.Fatal("failed launch left its task registered")
	}

	// A task that never starts the supervisor times out, saying so.
	f = &fakeTasks{}
	err = startSupervisorTask(f, supervisorTask{Name: `hev\run-y`, Dir: dir, Log: log}, 300*time.Millisecond)
	if !errors.Is(err, errLaunchUnconfirmed) || !strings.Contains(err.Error(), "did not start a supervisor within") || !strings.Contains(err.Error(), "task running") || !strings.Contains(err.Error(), log) {
		t.Fatalf("timeout error %v", err)
	}
}

// The supervisor adopts the launcher's environment, deletes the launch file
// at once, and removes its task when it is done.
func TestWindowsSupervisorAdoptsLaunch(t *testing.T) {
	saved := os.Environ()
	t.Cleanup(func() {
		os.Clearenv()
		for _, kv := range saved {
			if k, v, ok := strings.Cut(kv, "="); ok && k != "" {
				os.Setenv(k, v)
			}
		}
	})
	dir := t.TempDir()
	f := &fakeTasks{run: func(string) error { return writePID(dir) }}
	env := append(os.Environ(), "HEV_LAUNCH_TEST=from-launcher")
	if err := startSupervisorTask(f, supervisorTask{Name: `hev\orch-z`, Dir: dir, Env: env}, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(filepath.Join(dir, "supervisor.json"))
	f.removed = nil // The launch clears any earlier registration first.
	done, err := adoptSupervisorLaunch(f, dir)
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("HEV_LAUNCH_TEST") != "from-launcher" {
		t.Fatal("launch environment not adopted")
	}
	if _, err = os.Stat(filepath.Join(dir, supervisorLaunchFile)); !os.IsNotExist(err) {
		t.Fatal("launch file kept after adoption")
	}
	if f.wasRemoved(`hev\orch-z`) {
		t.Fatal("task removed while its supervisor runs")
	}
	done()
	if !f.wasRemoved(`hev\orch-z`) {
		t.Fatal("exiting supervisor left its task registered")
	}
	// Supervisors started any other way have nothing to adopt.
	if done, err = adoptSupervisorLaunch(f, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	done()
}
