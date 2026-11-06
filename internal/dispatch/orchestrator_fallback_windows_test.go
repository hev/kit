package dispatch

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// launchContextKeys choose the spec a supervisor resolves. The fallback tests
// give each one a value the user's profile does not have.
var launchContextKeys = []string{"HOME", "USERPROFILE", "HEV_DISPATCH_CONFIG", "PEERS_SELF"}

func launchContext() string {
	s := ""
	for _, k := range launchContextKeys {
		s += k + "=" + os.Getenv(k) + "\n"
	}
	return s
}

// fallbackHome is an orchestrator home with a custom profile and config.
func fallbackHome(t *testing.T) OrchestratorSpec {
	t.Helper()
	orchHome(t)
	config := filepath.Join(t.TempDir(), "dispatch.toml")
	if e := os.WriteFile(config, nil, 0600); e != nil {
		t.Fatal(e)
	}
	t.Setenv("HEV_DISPATCH_CONFIG", config)
	s := applyOrch(t)
	realProbes(t)
	stopSupervisorOnCleanup(t, OrchestratorDir(s.Name))
	return s
}

// startDeniedLaunch runs LaunchOrchestrator in a helper inside a kill-on-close
// job that forbids breakaway, so the launch must take the Task Scheduler
// branch. The helper has only the user's profile environment plus this test's
// dispatch context, which is all a scheduled task can reproduce.
func startDeniedLaunch(t *testing.T) (*exec.Cmd, *strings.Builder) {
	t.Helper()
	env, e := userProfileEnvironment()
	if e != nil {
		t.Fatal(e)
	}
	args := windowsHelperArgs("launch")
	for _, k := range launchContextKeys {
		env = withEnv(env, k, os.Getenv(k))
		args = append(args, "--launch-env", k+"="+os.Getenv(k))
	}
	job, e := windows.CreateJobObject(nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { windows.CloseHandle(job) })
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, e = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); e != nil {
		t.Fatal(e)
	}
	c := exec.Command(args[0], args[1:]...)
	c.Env = env
	c.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_SUSPENDED}
	out := &strings.Builder{}
	c.Stdout, c.Stderr = out, out
	if e = c.Start(); e != nil {
		t.Fatal(e)
	}
	p, e := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(c.Process.Pid))
	if e == nil {
		e = windows.AssignProcessToJobObject(job, p)
		windows.CloseHandle(p)
	}
	if e == nil {
		e = resumePrimaryThread(uint32(c.Process.Pid))
	}
	if e != nil {
		_ = c.Process.Kill()
		t.Fatal(e)
	}
	return c, out
}

func withEnv(env []string, key, value string) []string {
	out := []string{}
	for _, kv := range env {
		if name, _, _ := strings.Cut(kv, "="); !strings.EqualFold(name, key) {
			out = append(out, kv)
		}
	}
	return append(out, key+"="+value)
}

// launchTasks lists the scheduled tasks launches recorded in supervisor.log.
func launchTasks(t *testing.T, dir string) []string {
	t.Helper()
	b, _ := os.ReadFile(filepath.Join(dir, "supervisor.log"))
	var tasks []string
	for _, line := range strings.Split(string(b), "\n") {
		if _, task, ok := strings.Cut(strings.TrimSpace(line), "; scheduled task "); ok {
			tasks = append(tasks, task)
		}
	}
	return tasks
}

func taskRegistered(task string) bool {
	return exec.Command("schtasks", "/Query", "/TN", task).Run() == nil
}

func waitLaunch(t *testing.T, c *exec.Cmd, out *strings.Builder, dir string) {
	t.Helper()
	if e := c.Wait(); e != nil {
		b, _ := os.ReadFile(filepath.Join(dir, "supervisor.log"))
		t.Fatalf("launcher: %v\n%s\nsupervisor.log:\n%s", e, out.String(), b)
	}
}

// Rework 1(a): with breakaway denied, the scheduled task's supervisor resolves
// the caller's custom profile and config, the launcher confirms its record,
// and the task is gone while the supervisor runs on.
func TestWindowsOrchestratorFallbackCarriesLaunchContext(t *testing.T) {
	s := fallbackHome(t)
	dir := OrchestratorDir(s.Name)
	c, out := startDeniedLaunch(t)
	waitLaunch(t, c, out, dir)
	if !processRecordLive(filepath.Join(dir, "supervisor.json")) {
		t.Fatal("launcher returned before supervisor.json named a live process")
	}
	pid := supervisorPID(t, dir)
	tasks := launchTasks(t, dir)
	if len(tasks) != 1 {
		t.Fatalf("launch did not take the Task Scheduler branch: %q\n%s", tasks, out.String())
	}
	if b, e := os.ReadFile(filepath.Join(dir, "launch-context")); e != nil || string(b) != launchContext() {
		t.Fatalf("scheduled supervisor resolved %q, want %q: %v", b, launchContext(), e)
	}
	if taskRegistered(tasks[0]) {
		t.Fatalf("scheduled task %s left behind", tasks[0])
	}
	time.Sleep(500 * time.Millisecond)
	if _, e := processCreated(pid); e != nil {
		t.Fatalf("supervisor %d ended with its task: %v", pid, e)
	}
	if e := OrchestratorControl(s.Name, "stop", ""); e != nil {
		t.Fatal(e)
	}
	waitWindows(t, func() bool { _, e := processCreated(pid); return e != nil })
}

// Rework 2: concurrent fallback attempts each own a task, neither overwrites
// the other, and the root lock still admits one supervisor. Holding the root
// lock keeps both attempts ahead of any supervisor record.
func TestWindowsOrchestratorFallbackAttemptsOwnTheirTasks(t *testing.T) {
	s := fallbackHome(t)
	dir := OrchestratorDir(s.Name)
	unlock, e := lockRoot()
	if e != nil {
		t.Fatal(e)
	}
	a, outA := startDeniedLaunch(t)
	b, outB := startDeniedLaunch(t)
	waitWindows(t, func() bool { return len(launchTasks(t, dir)) == 2 })
	time.Sleep(time.Second) // Both tasks reach /Run and start a supervisor.
	unlock()
	waitLaunch(t, a, outA, dir)
	waitLaunch(t, b, outB, dir)
	tasks := launchTasks(t, dir)
	if len(tasks) != 2 || tasks[0] == tasks[1] {
		t.Fatalf("attempts shared a scheduled task: %q", tasks)
	}
	for _, task := range tasks {
		if taskRegistered(task) {
			t.Fatalf("scheduled task %s left behind", task)
		}
	}
	pid := supervisorPID(t, dir)
	if st, e := orchestratorState(s.Name); e != nil || st.Error != "" {
		t.Fatalf("state %+v: %v", st, e)
	}
	if e := OrchestratorControl(s.Name, "stop", ""); e != nil {
		t.Fatal(e)
	}
	waitWindows(t, func() bool { _, e := processCreated(pid); return e != nil })
}

type fakeScheduler struct {
	mu         sync.Mutex
	calls      []string
	tasks      []string
	definition string
	fail       map[string]error
	registered bool
	run        func()
}

func (f *fakeScheduler) schtasks(_ context.Context, args ...string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, args[0])
	switch args[0] {
	case "/Create":
		f.tasks = append(f.tasks, args[2])
		if args[len(args)-1] == "/F" {
			return errors.New("/Create would overwrite another attempt's task")
		}
		definition, e := readTaskDefinition(args[4])
		if e != nil {
			return e
		}
		f.definition = definition
	case "/Run":
		if f.run != nil && f.fail["/Run"] == nil {
			f.run()
		}
	case "/Query":
		if !f.registered {
			return errors.New("task not found")
		}
		return nil
	}
	return f.fail[args[0]]
}

// readTaskDefinition decodes a task definition file the launcher wrote.
func readTaskDefinition(file string) (string, error) {
	b, e := os.ReadFile(file)
	if e != nil {
		return "", e
	}
	units := make([]uint16, (len(b)-2)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(b[2+2*i:])
	}
	return string(utf16.Decode(units)), nil
}

// fakeFallback denies breakaway and replaces Task Scheduler. The profile
// differs from the caller only in HOME, which the launch must carry.
func fakeFallback(t *testing.T, f *fakeScheduler) {
	t.Helper()
	oldStart, oldTasks, oldProfile := startDetached, schtasks, profileEnvironment
	t.Cleanup(func() { startDetached, schtasks, profileEnvironment = oldStart, oldTasks, oldProfile })
	startDetached = func(*exec.Cmd) error { return windows.ERROR_ACCESS_DENIED }
	schtasks = f.schtasks
	profile := withEnv(orchestratorEnvironment(""), "HOME", `C:\profile`)
	profileEnvironment = func() ([]string, error) { return profile, nil }
}

// Rework 1(b): a caller environment a task cannot reproduce is refused before
// anything is created, so no supervisor can start from the profile's spec.
func TestWindowsOrchestratorFallbackRefusesUnreproducibleEnvironment(t *testing.T) {
	s := fallbackHome(t)
	dir := OrchestratorDir(s.Name)
	f := &fakeScheduler{}
	fakeFallback(t, f)
	t.Setenv("GH_TOKEN", "synthetic-token")
	t.Setenv("KIT_TEST_SESSION_SETTING", "1")
	e := LaunchOrchestrator(s.Name)
	if e == nil || !strings.Contains(e.Error(), "cannot reproduce GH_TOKEN, KIT_TEST_SESSION_SETTING") {
		t.Fatalf("launch error %v", e)
	}
	if strings.Contains(e.Error(), "synthetic-token") {
		t.Fatal("refusal printed a secret value")
	}
	if len(f.calls) != 0 {
		t.Fatalf("refused launch ran schtasks %q", f.calls)
	}
	if exists(filepath.Join(dir, "supervisor.json")) || len(launchTasks(t, dir)) != 0 {
		t.Fatal("refused launch started a supervisor")
	}
	if xml, _ := filepath.Glob(filepath.Join(dir, "*.xml")); len(xml) != 0 {
		t.Fatalf("refused launch wrote %q", xml)
	}
}

// Rework r2: the task starts in the orchestrator directory, so a relative
// config would select that directory's dispatch.toml rather than the caller's.
// The launch refuses it by name before anything is created.
func TestWindowsOrchestratorFallbackRefusesRelativeConfig(t *testing.T) {
	s := fallbackHome(t)
	dir := OrchestratorDir(s.Name)
	caller := t.TempDir()
	for d, body := range map[string]string{caller: "cap = 3\n", dir: "cap = 9\n"} {
		if e := os.WriteFile(filepath.Join(d, "dispatch.toml"), []byte(body), 0600); e != nil {
			t.Fatal(e)
		}
	}
	f := &fakeScheduler{}
	fakeFallback(t, f)
	t.Chdir(caller)
	t.Setenv("HEV_DISPATCH_CONFIG", "dispatch.toml")
	e := LaunchOrchestrator(s.Name)
	if f.definition != "" {
		// Resolve the carried config as the task would.
		wd := taskWorkingDirectory(f.definition)
		_, config, _ := strings.Cut(f.definition, "--launch-env HEV_DISPATCH_CONFIG=")
		config, _, _ = strings.Cut(config, "<")
		config, _, _ = strings.Cut(config, " ")
		t.Chdir(wd)
		t.Setenv("HEV_DISPATCH_CONFIG", config)
		got, le := Load(ConfigPath(""))
		t.Fatalf("scheduled task would load cap %d (%v) from %s, the caller's config has cap 3; schtasks %q", got.Cap, le, wd, f.calls)
	}
	if e == nil || !strings.Contains(e.Error(), "cannot reproduce HEV_DISPATCH_CONFIG (relative path)") || strings.Contains(e.Error(), "dispatch.toml") {
		t.Fatalf("launch error %v", e)
	}
	if len(f.calls) != 0 || exists(filepath.Join(dir, "supervisor.json")) || len(launchTasks(t, dir)) != 0 {
		t.Fatalf("refused launch ran schtasks %q or started a supervisor", f.calls)
	}
	if xml, _ := filepath.Glob(filepath.Join(dir, "*.xml")); len(xml) != 0 {
		t.Fatalf("refused launch wrote %q", xml)
	}
}

func taskWorkingDirectory(definition string) string {
	wd, _, _ := strings.Cut(definition[strings.Index(definition, "<WorkingDirectory>")+len("<WorkingDirectory>"):], "<")
	return wd
}

// Rework r3: a plugin may run SHELL and hev peers runs HEV_PEERS_BACKEND. The
// task starts in the orchestrator directory, so a relative program would be
// that directory's copy; the launch refuses it by name before anything is
// created. A bare backend name is carried, as os/exec resolves it on PATH.
func TestWindowsOrchestratorFallbackRefusesRelativeExecutables(t *testing.T) {
	for _, tc := range []struct{ name, value, refused string }{
		{"SHELL", `tools\shell.exe`, "SHELL (relative path)"},
		{"SHELL", "shell.exe", "SHELL (relative path)"},
		{"HEV_PEERS_BACKEND", `tools\peers.exe`, "HEV_PEERS_BACKEND (relative path)"},
		{"HEV_PEERS_BACKEND", "peers.exe", ""},
	} {
		t.Run(tc.name+"="+tc.value, func(t *testing.T) {
			s := fallbackHome(t)
			dir := OrchestratorDir(s.Name)
			caller := t.TempDir()
			for _, d := range []string{filepath.Join(caller, "tools"), filepath.Join(dir, "tools")} {
				if e := os.MkdirAll(d, 0755); e != nil {
					t.Fatal(e)
				}
				if e := os.WriteFile(filepath.Join(d, filepath.Base(tc.value)), nil, 0755); e != nil {
					t.Fatal(e)
				}
			}
			// /Run fails so a launch that gets that far returns at once.
			f := &fakeScheduler{fail: map[string]error{"/Run": errors.New("/Run failed")}}
			fakeFallback(t, f)
			t.Chdir(caller)
			t.Setenv(tc.name, tc.value)
			want, _ := filepath.Abs(tc.value)
			e := LaunchOrchestrator(s.Name)
			if tc.refused == "" {
				if e == nil || !strings.Contains(e.Error(), "/Run failed") || !strings.Contains(f.definition, "--launch-env "+tc.name+"="+tc.value) {
					t.Fatalf("launch error %v; task definition does not carry %s:\n%s", e, tc.name, f.definition)
				}
				return
			}
			if f.definition != "" {
				// Resolve the carried program as the task would.
				t.Chdir(taskWorkingDirectory(f.definition))
				got, le := filepath.Abs(tc.value)
				t.Fatalf("scheduled task would run %s (%v) for %s, the caller's is %s; schtasks %q", got, le, tc.name, want, f.calls)
			}
			if e == nil || !strings.Contains(e.Error(), "cannot reproduce "+tc.refused) || strings.Contains(e.Error(), tc.value) {
				t.Fatalf("launch error %v", e)
			}
			if len(f.calls) != 0 || exists(filepath.Join(dir, "supervisor.json")) || len(launchTasks(t, dir)) != 0 {
				t.Fatalf("refused launch ran schtasks %q or started a supervisor", f.calls)
			}
			if xml, _ := filepath.Glob(filepath.Join(dir, "*.xml")); len(xml) != 0 {
				t.Fatalf("refused launch wrote %q", xml)
			}
		})
	}
}

// The launcher classifies values with its own Windows predicate on every OS;
// on Windows it must agree with filepath.IsAbs.
func TestWindowsLaunchPathPredicateMatchesIsAbs(t *testing.T) {
	for _, p := range []string{`C:\x`, `C:/x`, `c:\`, `C:x`, `C:`, `\x`, `/x`, `/custom`, `x`, `.\x`, `..\x`, `~/x`, `\\server\share\x`, `//server/share/x`, `\\?\C:\x`, `\\.\pipe\x`, `\\?\UNC\server\share`} {
		if got, want := windowsAbsPath(p), filepath.IsAbs(p); got != want {
			t.Errorf("windowsAbsPath(%q) = %v, filepath.IsAbs = %v", p, got, want)
		}
	}
}

// Rework 2 and 4: every attempt gets its own task without /F, and cleanup runs
// whatever /Create, /Run or /Delete report, surfacing a task it cannot remove.
func TestWindowsOrchestratorFallbackTaskFailures(t *testing.T) {
	for _, tc := range []struct {
		name       string
		fail       []string
		registered bool
		start      bool
		calls      string
		want, not  string
	}{
		{name: "create fails before registering", fail: []string{"/Create", "/Delete"}, calls: "/Create /Delete /Query", want: "scheduled task: /Create failed", not: "remove scheduled task"},
		{name: "create times out after registering", fail: []string{"/Create"}, registered: true, calls: "/Create /Delete", want: "scheduled task: /Create failed", not: "remove scheduled task"},
		{name: "create fails and the registered task cannot be deleted", fail: []string{"/Create", "/Delete"}, registered: true, calls: "/Create /Delete /Query", want: "remove scheduled task"},
		{name: "run fails", fail: []string{"/Run"}, calls: "/Create /Run /Delete", want: "/Run failed", not: "remove scheduled task"},
		{name: "run and delete fail", fail: []string{"/Run", "/Delete"}, calls: "/Create /Run /Delete", want: "remove scheduled task"},
		{name: "delete fails after the supervisor starts", fail: []string{"/Delete"}, start: true, calls: "/Create /Run /Delete", want: "orchestrator supervisor started, but remove scheduled task"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := fallbackHome(t)
			dir := OrchestratorDir(s.Name)
			// The fake supervisor is this test process; never force-stop it.
			t.Cleanup(func() { os.Remove(filepath.Join(dir, "supervisor.json")) })
			f := &fakeScheduler{fail: map[string]error{}, registered: tc.registered}
			for _, step := range tc.fail {
				f.fail[step] = errors.New(step + " failed")
			}
			if tc.start {
				f.run = func() {
					if e := writePID(dir); e != nil {
						t.Error(e)
					}
				}
			}
			fakeFallback(t, f)
			e := LaunchOrchestrator(s.Name)
			if e == nil || !strings.Contains(e.Error(), tc.want) || tc.not != "" && strings.Contains(e.Error(), tc.not) {
				t.Fatalf("launch error %v, want %q", e, tc.want)
			}
			if got := strings.Join(f.calls, " "); got != tc.calls {
				t.Fatalf("schtasks calls %q, want %q", got, tc.calls)
			}
			if !strings.Contains(f.definition, "--launch-env HOME=") || strings.Contains(f.definition, "Trigger") {
				t.Fatalf("task definition does not carry HOME or has a trigger:\n%s", f.definition)
			}
			if xml, _ := filepath.Glob(filepath.Join(dir, "*.xml")); len(xml) != 0 {
				t.Fatalf("launch left %q", xml)
			}
			// A second attempt owns a different task.
			os.Remove(filepath.Join(dir, "supervisor.json"))
			_ = LaunchOrchestrator(s.Name)
			if len(f.tasks) != 2 || f.tasks[0] == f.tasks[1] || !strings.HasPrefix(f.tasks[0], "hev-orchestrator-"+s.Name+"-") {
				t.Fatalf("attempts shared a task name: %q", f.tasks)
			}
		})
	}
}
