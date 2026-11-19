package dispatch

import (
	"errors"
	"html"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeTasks records Task Scheduler calls; run decides what a task does.
// Like Task Scheduler, it reports a task as missing once it is removed.
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
	if _, ok := f.registered[name]; !ok {
		return errors.New("no such task")
	}
	delete(f.registered, name)
	return nil
}
func (f *fakeTasks) Status(name string) (taskStatus, error) {
	f.mu.Lock()
	_, ok := f.registered[name]
	status := f.status
	f.mu.Unlock()
	if !ok {
		return taskStatus{missing: true}, nil
	}
	if status != nil {
		return status(name)
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
func (f *fakeTasks) isRegistered(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.registered[name]
	return ok
}

// taskDir is the working directory a registered task starts in.
func (f *fakeTasks) taskDir(name string) string {
	f.mu.Lock()
	xml := f.registered[name]
	f.mu.Unlock()
	_, rest, _ := strings.Cut(xml, "<WorkingDirectory>")
	dir, _, _ := strings.Cut(rest, "</WorkingDirectory>")
	return html.UnescapeString(dir)
}

// supervisorExits is a task whose supervisor adopts its launch, runs, and
// exits with code (and message) without ever being seen live: it consumes
// the launch file, leaves its exit record and removes its task, in the order
// a real one does.
func (f *fakeTasks) supervisorExits(code int, message string) func(string) error {
	return func(name string) error {
		dir := f.taskDir(name)
		if err := os.Remove(filepath.Join(dir, supervisorLaunchFile)); err != nil {
			return nil // nothing to adopt: a real supervisor would refuse
		}
		_ = WriteJSON(filepath.Join(dir, supervisorExitFile), supervisorExit{Task: name, PID: 1, Code: code, Error: message})
		_ = f.Remove(name)
		return nil
	}
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
	err := startSupervisorTask(f, supervisorTask{Name: `hev\run-x`, Exe: "hev.exe", Args: []string{"dispatch", "supervise", scheduledFlag, "--", "x"}, Dir: dir, Env: []string{"A=1"}, Log: log}, 5*time.Second)
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
	if f.isRegistered(`hev\run-x`) {
		t.Fatal("failed launch left its task registered")
	}
}

// Regression (review finding 1): a launch that times out before its
// supervisor adopts the launch withdraws it: the environment file and the
// task are gone when the command returns, and a supervisor the task starts
// late refuses to run without them.
func TestSupervisorLaunchTimeoutWithdrawsLaunch(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "supervisor.log")
	f := &fakeTasks{} // the task "runs" and never records a supervisor
	task := `hev\orch-release-train`
	err := startSupervisorTask(f, supervisorTask{Name: task, Dir: dir, Env: []string{"HEVLAYER_API_KEY=secret"}, Log: log}, 300*time.Millisecond)
	for _, want := range []string{"did not start a supervisor within", "task running", "withdrawn", log} {
		if !errors.Is(err, errLaunchUnconfirmed) || !strings.Contains(err.Error(), want) {
			t.Fatalf("timeout error %v does not name %q", err, want)
		}
	}
	if _, statErr := os.Stat(filepath.Join(dir, supervisorLaunchFile)); !os.IsNotExist(statErr) {
		t.Fatal("timed-out launch left its environment file behind")
	}
	if f.isRegistered(task) {
		t.Fatal("timed-out launch left its task registered")
	}
	// The task's supervisor, started late anyway, refuses to run.
	t.Setenv("HEV_LATE_START", "kept")
	if _, err = adoptSupervisorLaunch(f, dir, task); err == nil || !strings.Contains(err.Error(), "withdrawn") {
		t.Fatalf("late supervisor adopted a withdrawn launch: %v", err)
	}
	if os.Getenv("HEV_LATE_START") != "kept" {
		t.Fatal("late supervisor changed its environment")
	}

	// A supervisor that adopted its launch but has not recorded itself yet
	// is left to finish: the error says so, and its task stays.
	f = &fakeTasks{}
	f.run = func(name string) error { return os.Remove(filepath.Join(dir, supervisorLaunchFile)) }
	err = startSupervisorTask(f, supervisorTask{Name: task, Dir: dir, Log: log}, 300*time.Millisecond)
	if !errors.Is(err, errLaunchUnconfirmed) || !strings.Contains(err.Error(), "adopted its launch") {
		t.Fatalf("adopted timeout error %v", err)
	}
	if !f.isRegistered(task) {
		t.Fatal("removed the task of a supervisor that adopted its launch")
	}
}

// Regression (review finding 1): whatever a lapsed launch left (launch file,
// exit record, task) is cleared once no supervisor may own the directory,
// and never while one may.
func TestClearSupervisorLaunch(t *testing.T) {
	dir := t.TempDir()
	task := `hev\run-example-run`
	for _, file := range []string{supervisorLaunchFile, supervisorExitFile} {
		if err := os.WriteFile(filepath.Join(dir, file), []byte(`{"env":["HEVLAYER_API_KEY=secret"]}`), 0600); err != nil {
			t.Fatal(err)
		}
	}
	f := &fakeTasks{}
	_ = f.Register(task, "")
	if err := writePID(dir); err != nil {
		t.Fatal(err)
	}
	clearSupervisorLaunch(f, dir, task)
	if !f.isRegistered(task) {
		t.Fatal("cleared the launch of a live supervisor")
	}
	if err := os.Remove(filepath.Join(dir, "supervisor.json")); err != nil {
		t.Fatal(err)
	}
	clearSupervisorLaunch(f, dir, task)
	for _, file := range []string{supervisorLaunchFile, supervisorExitFile} {
		if _, err := os.Stat(filepath.Join(dir, file)); !os.IsNotExist(err) {
			t.Fatalf("%s left behind", file)
		}
	}
	if f.isRegistered(task) {
		t.Fatal("task left registered")
	}
}

// Regression (review finding 1): a run whose launch lapsed has its launch
// file withdrawn when the launch is reconciled as failed.
func TestReconcileLaunchWithdrawsLapsedLaunch(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "example-run")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	launchPath := filepath.Join(dir, supervisorLaunchFile)
	if err := os.WriteFile(launchPath, []byte(`{"env":["HEVLAYER_API_KEY=secret"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	r := Result{LaunchUnconfirmed: true, Start: time.Now().Add(-2 * launchGrace), Error: errLaunchUnconfirmed.Error() + ": scheduled task hev\\run-example-run did not start"}
	reconcileLaunch(dir, &r)
	if r.LaunchUnconfirmed || !strings.Contains(r.Error, "did not start") {
		t.Fatalf("result %+v", r)
	}
	if _, err := os.Stat(launchPath); !os.IsNotExist(err) {
		t.Fatal("lapsed launch left its environment file behind")
	}
}

// Regression (review finding 2): a supervisor that adopts its launch and
// then fails removes its task, as a real one does; the launch still reports
// its exit code and error at once, and a clean exit confirms.
func TestSupervisorLaunchReportsExitAfterAdopt(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "supervisor.log")
	task := `hev\run-example-run`
	f := &fakeTasks{}
	f.run = func(name string) error {
		// The real adopt-and-exit sequence, with the supervisor failing.
		done, err := adoptSupervisorLaunch(f, dir, name)
		if err != nil {
			return err
		}
		done(errors.New("read meta.json: access is denied"))
		return nil
	}
	start := time.Now()
	err := startSupervisorTask(f, supervisorTask{Name: task, Dir: dir, Log: log}, 10*time.Second)
	if !errors.Is(err, errLaunchUnconfirmed) {
		t.Fatalf("error %v", err)
	}
	for _, want := range []string{task, "exited with code 1", "access is denied", log} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not name %q", err, want)
		}
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("exit reported only after %s", time.Since(start))
	}
	if !f.wasRemoved(task) || f.isRegistered(task) {
		t.Fatal("fake supervisor did not remove its task")
	}
	for _, file := range []string{supervisorLaunchFile, supervisorExitFile} {
		if _, statErr := os.Stat(filepath.Join(dir, file)); !os.IsNotExist(statErr) {
			t.Fatalf("%s left behind", file)
		}
	}

	// A supervisor that exits cleanly before it is seen live confirms.
	f = &fakeTasks{}
	f.run = f.supervisorExits(0, "")
	if err = startSupervisorTask(f, supervisorTask{Name: task, Dir: dir, Log: log}, 10*time.Second); err != nil {
		t.Fatalf("clean exit: %v", err)
	}
	// A task removed with no exit record (someone else deleted it) says so.
	f = &fakeTasks{}
	f.run = func(name string) error { return f.Remove(name) }
	err = startSupervisorTask(f, supervisorTask{Name: task, Dir: dir, Log: log}, 10*time.Second)
	if !errors.Is(err, errLaunchUnconfirmed) || !strings.Contains(err.Error(), "was removed before its supervisor started") {
		t.Fatalf("removed task: %v", err)
	}
}

// The supervisor adopts the launcher's environment, deletes the launch file
// at once, and records its exit and removes its task when it is done.
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
	done, err := adoptSupervisorLaunch(f, dir, `hev\orch-z`)
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
	done(nil)
	if !f.wasRemoved(`hev\orch-z`) {
		t.Fatal("exiting supervisor left its task registered")
	}
	var x supervisorExit
	if err = readJSON(filepath.Join(dir, supervisorExitFile), &x); err != nil || x.Task != `hev\orch-z` || x.Code != 0 {
		t.Fatalf("exit record %+v %v", x, err)
	}
	// Supervisors started any other way never adopt a launch file, even one
	// that is present.
	other := t.TempDir()
	if err = os.WriteFile(filepath.Join(other, supervisorLaunchFile), []byte(`{"task":"x","env":["HEV_LAUNCH_TEST=stray"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if done, err = adoptSupervisorLaunch(f, other, ""); err != nil {
		t.Fatal(err)
	}
	done(nil)
	if os.Getenv("HEV_LAUNCH_TEST") != "from-launcher" {
		t.Fatal("unscheduled supervisor adopted a launch file")
	}
}
