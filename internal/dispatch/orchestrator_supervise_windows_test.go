package dispatch

import (
	"context"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// TestWindowsProcessHelper is the body of every helper process these tests
// start. It does nothing in the parent test run.
func TestWindowsProcessHelper(t *testing.T) {
	mode, rest := os.Getenv("WINDOWS_HELPER_MODE"), []string(nil)
	if a := flag.Args(); len(a) > 1 && a[0] == "windows-helper" {
		// A scheduled task cannot pass this process's environment, so the mode
		// and the dispatch context arrive as arguments. Without
		// ORCH_PROCESS_HELPER, TestMain isolated this process; undo that.
		mode, rest = a[1], a[2:]
		created = processCreated
	} else if os.Getenv("ORCH_PROCESS_HELPER") != "1" {
		return
	}
	var carried []string
	for i := 0; i+1 < len(rest); i += 2 {
		if rest[i] == "--launch-env" {
			carried = append(carried, rest[i+1])
		}
	}
	if e := ApplyLaunchEnvironment(carried); e != nil {
		t.Fatal(e)
	}
	out := os.Getenv("WINDOWS_HELPER_OUT")
	switch mode {
	case "lock":
		if _, e := lockDirectory(os.Getenv("WINDOWS_HELPER_DIR")); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(out, []byte("locked"), 0600); e != nil {
			t.Fatal(e)
		}
		time.Sleep(time.Minute)
	case "identity":
		if e := os.WriteFile(out, []byte("identity="+roundProcessIdentity()), 0600); e != nil {
			t.Fatal(e)
		}
	case "launch":
		orchestratorSuperviseArgs = func(string, string) []string { return windowsHelperArgs("supervise") }
		if e := LaunchOrchestrator("release-train"); e != nil {
			t.Fatal(e)
		}
	case "supervise":
		// The fallback tests check which context the supervisor resolved.
		if e := os.WriteFile(filepath.Join(OrchestratorDir("release-train"), "launch-context"), []byte(launchContext()), 0600); e != nil {
			t.Fatal(e)
		}
		d := DefaultOrchestratorDriver()
		d.Gather = func(context.Context, OrchestratorSpec) ([]OrchestratorEvidence, error) {
			return []OrchestratorEvidence{}, nil
		}
		d.Session = func(_ context.Context, _ OrchestratorSpec, dir string, _ OrchestratorPacket) error {
			return os.WriteFile(filepath.Join(dir, "checkpoint.md"), []byte("Next: wait\n"), 0600)
		}
		d.Notify = func(context.Context, OrchestratorNotice) error { return nil }
		if e := SuperviseOrchestrator(context.Background(), "release-train", d); e != nil {
			t.Fatal(e)
		}
	default:
		t.Fatal("unknown helper mode")
	}
}

// No caret: cmd.exe treats it as an escape character. A mode in the arguments
// takes precedence over WINDOWS_HELPER_MODE.
func windowsHelperArgs(mode ...string) []string {
	args := []string{os.Args[0], "-test.run=TestWindowsProcessHelper$", "-test.count=1"}
	if len(mode) > 0 {
		args = append(append(args, "--", "windows-helper"), mode...)
	}
	return args
}

func windowsHelperEnv(mode, out string, extra ...string) []string {
	return append(append(os.Environ(), "ORCH_PROCESS_HELPER=1", "WINDOWS_HELPER_MODE="+mode, "WINDOWS_HELPER_OUT="+out), extra...)
}

// realProbes lets this test inspect the processes it launches. Every record it
// reads lives under the test's own HOME.
func realProbes(t *testing.T) {
	previous := created
	created = processCreated
	t.Cleanup(func() { created = previous })
}

// stopSupervisorOnCleanup force-stops a supervisor the test failed to stop.
func stopSupervisorOnCleanup(t *testing.T, dir string) {
	t.Cleanup(func() {
		var p pidRecord
		if readJSON(filepath.Join(dir, "supervisor.json"), &p) == nil && processRecordLive(filepath.Join(dir, "supervisor.json")) {
			_ = exec.Command("taskkill", "/PID", strconv.Itoa(p.PID), "/T", "/F").Run()
		}
	})
}

func supervisorPID(t *testing.T, dir string) int {
	t.Helper()
	var p pidRecord
	if e := readJSON(filepath.Join(dir, "supervisor.json"), &p); e != nil {
		t.Fatal(e)
	}
	return p.PID
}

// W1: the launcher returns only after supervisor.json names a live process,
// and that process breaks away from the caller's job. Closing a kill-on-close
// job models the end of an OpenSSH session.
func TestWindowsOrchestratorLaunchOutlivesSessionJob(t *testing.T) {
	orchHome(t)
	s := applyOrch(t)
	realProbes(t)
	dir := OrchestratorDir(s.Name)
	stopSupervisorOnCleanup(t, dir)
	closeJob := launchInSessionJob(t)
	if !processRecordLive(filepath.Join(dir, "supervisor.json")) {
		t.Fatal("launcher returned before supervisor.json named a live process")
	}
	pid := supervisorPID(t, dir)
	closeJob()
	time.Sleep(500 * time.Millisecond)
	if _, e := processCreated(pid); e != nil {
		t.Fatalf("supervisor %d died with the launching job: %v", pid, e)
	}
	if e := OrchestratorControl(s.Name, "stop", ""); e != nil {
		t.Fatal(e)
	}
	waitWindows(t, func() bool { _, e := processCreated(pid); return e != nil })
}

// launchInSessionJob runs LaunchOrchestrator in a helper inside a kill-on-close
// job that permits breakaway, as an OpenSSH session does. The returned function
// closes the job. The CI runner's own job forbids breakaway, so a launch from
// the test process itself would take the Task Scheduler branch, which does not
// inherit the test's HOME.
func launchInSessionJob(t *testing.T) func() {
	t.Helper()
	job, e := windows.CreateJobObject(nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	closed := false
	closeJob := func() {
		if !closed {
			closed = true
			windows.CloseHandle(job)
		}
	}
	t.Cleanup(closeJob)
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE | windows.JOB_OBJECT_LIMIT_BREAKAWAY_OK
	if _, e = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); e != nil {
		t.Fatal(e)
	}
	args := windowsHelperArgs()
	c := exec.Command(args[0], args[1:]...)
	c.Env = windowsHelperEnv("launch", "")
	c.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_SUSPENDED}
	var out strings.Builder
	c.Stdout, c.Stderr = &out, &out
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
	if e = c.Wait(); e != nil {
		b, _ := os.ReadFile(filepath.Join(OrchestratorDir("release-train"), "supervisor.log"))
		t.Fatalf("launcher: %v\n%s\nsupervisor.log:\n%s", e, out.String(), b)
	}
	return closeJob
}

// Supervise single-instance, wake and stop against a real detached supervisor.
func TestWindowsOrchestratorSupervisorSingleInstanceWakeStop(t *testing.T) {
	orchHome(t)
	s := applyOrch(t)
	realProbes(t)
	dir := OrchestratorDir(s.Name)
	stopSupervisorOnCleanup(t, dir)
	launchInSessionJob(t)()
	pid := supervisorPID(t, dir)
	launchInSessionJob(t)()
	if again := supervisorPID(t, dir); again != pid {
		t.Fatalf("second launch replaced supervisor %d with %d", pid, again)
	}
	if e := SuperviseOrchestrator(context.Background(), s.Name, OrchestratorDriver{}); e == nil || !strings.Contains(e.Error(), "already running") {
		t.Fatalf("overlapping supervisor admitted: %v", e)
	}
	waitWindows(t, func() bool {
		st, e := orchestratorState(s.Name)
		return e == nil && st.Rounds == 1 && st.State == "waiting" && !st.Next.IsZero()
	})
	if e := OrchestratorControl(s.Name, "wake", ""); e != nil {
		t.Fatal(e)
	}
	// The next tick is 20 minutes out. With unchanged evidence and idle: skip,
	// only a wake evaluates a round before then, and it records idle.
	waitWindows(t, func() bool {
		st, e := orchestratorState(s.Name)
		return e == nil && st.Rounds == 1 && st.State == "idle"
	})
	if e := OrchestratorControl(s.Name, "stop", ""); e != nil {
		t.Fatal(e)
	}
	waitWindows(t, func() bool { _, e := processCreated(pid); return e != nil })
	st, e := orchestratorState(s.Name)
	if e != nil || st.State != "stopped" {
		t.Fatalf("state %+v: %v", st, e)
	}
	if exists(filepath.Join(dir, "supervisor.json")) {
		t.Fatal("stopped supervisor left its record")
	}
}

func waitWindows(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			b, _ := os.ReadFile(filepath.Join(OrchestratorDir("release-train"), "supervisor.log"))
			t.Fatalf("orchestrator boundary timed out; supervisor.log:\n%s", b)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// W2: the OS releases the root lock when its holder is killed.
func TestWindowsLockReleasedWhenHolderKilled(t *testing.T) {
	dir := t.TempDir()
	ready := filepath.Join(t.TempDir(), "ready")
	args := windowsHelperArgs()
	c := exec.Command(args[0], args[1:]...)
	c.Env = windowsHelperEnv("lock", ready, "WINDOWS_HELPER_DIR="+dir)
	if e := c.Start(); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = c.Process.Kill(); _ = c.Wait() })
	waitWindows(t, func() bool { return exists(ready) })
	if e := c.Process.Kill(); e != nil {
		t.Fatal(e)
	}
	_ = c.Wait()
	done := make(chan error, 1)
	go func() {
		unlock, e := lockDirectory(dir)
		if e == nil {
			unlock()
		}
		done <- e
	}()
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("lock held by a killed process blocked the next lock")
	}
}

// W3: a reader holding state.json without delete sharing during persist does
// not end supervise.
func TestWindowsSupervisePersistSurvivesHeldReader(t *testing.T) {
	orchHome(t)
	s := applyOrch(t)
	state := filepath.Join(OrchestratorDir(s.Name), "state.json")
	waits := 0
	released := make(chan struct{})
	d := OrchestratorDriver{Now: time.Now, Gather: func(context.Context, OrchestratorSpec) ([]OrchestratorEvidence, error) { return nil, nil }, Session: func(_ context.Context, _ OrchestratorSpec, dir string, _ OrchestratorPacket) error {
		p, e := windows.UTF16PtrFromString(state)
		if e != nil {
			return e
		}
		// os.Open's sharing mode is a runtime detail; request exactly no FILE_SHARE_DELETE.
		h, e := windows.CreateFile(p, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
		if e != nil {
			return e
		}
		go func() {
			time.Sleep(300 * time.Millisecond)
			windows.CloseHandle(h)
			close(released)
		}()
		return os.WriteFile(filepath.Join(dir, "checkpoint.md"), []byte("Next: wait\n"), 0600)
	}, Wait: func(context.Context) error {
		waits++
		if waits > 1 {
			t.Fatal("supervisor did not stop")
		}
		return OrchestratorControl(s.Name, "stop", "")
	}}
	e := SuperviseOrchestrator(context.Background(), s.Name, d)
	<-released
	if e != nil {
		t.Fatalf("persist against a held reader ended supervise: %v", e)
	}
	st, e := orchestratorState(s.Name)
	if e != nil || st.State != "stopped" || st.Rounds != 1 || st.Error != "" {
		t.Fatalf("state %+v: %v", st, e)
	}
}

// W4: a descendant of the recorded harness, two levels down through cmd.exe,
// finds its orchestrator through the Toolhelp parent walk.
func TestWindowsRoundIdentityWalksParents(t *testing.T) {
	orchHome(t)
	s := applyOrch(t)
	stamp, e := processCreated(os.Getpid())
	if e != nil {
		t.Fatal(e)
	}
	if e = WriteJSON(filepath.Join(OrchestratorDir(s.Name), "harness.json"), pidRecord{os.Getpid(), stamp}); e != nil {
		t.Fatal(e)
	}
	out := filepath.Join(t.TempDir(), "identity")
	c := exec.Command("cmd.exe", append([]string{"/c"}, windowsHelperArgs()...)...)
	c.Env = windowsHelperEnv("identity", out)
	if b, e := c.CombinedOutput(); e != nil {
		t.Fatalf("%v\n%s", e, b)
	}
	b, e := os.ReadFile(out)
	if e != nil || string(b) != "identity="+s.Name {
		t.Fatalf("identity %q: %v", b, e)
	}
}
