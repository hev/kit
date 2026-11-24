package dispatch

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

func setupChild(c *exec.Cmd) {}
func killTree(c *exec.Cmd) {
	if c.Process != nil {
		_ = helperExec("taskkill", "/PID", strconv.Itoa(c.Process.Pid), "/T", "/F").Run()
	}
}

type pidRecord struct {
	PID     int    `json:"pid"`
	Created uint64 `json:"created"`
}

// Process probes and detached launches are replaced in tests.
var created = processCreated
var launch = launchSupervisor

func processCreated(pid int) (uint64, error) {
	h, e := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if e != nil {
		return 0, e
	}
	defer windows.CloseHandle(h)
	var exit uint32
	if e = windows.GetExitCodeProcess(h, &exit); e != nil {
		return 0, e
	}
	if exit != 259 {
		return 0, fmt.Errorf("process exited")
	}
	var a, b, c, d windows.Filetime
	e = windows.GetProcessTimes(h, &a, &b, &c, &d)
	if e != nil {
		return 0, e
	}
	return uint64(a.HighDateTime)<<32 | uint64(a.LowDateTime), nil
}
func writeProcessRecord(path string, pid int) error {
	t, e := created(pid)
	if e != nil {
		return e
	}
	return WriteJSON(path, pidRecord{pid, t})
}
func writePID(dir string) error {
	return writeProcessRecord(filepath.Join(dir, "supervisor.json"), os.Getpid())
}
func processRecordLive(path string) bool {
	var p pidRecord
	if readJSON(path, &p) != nil {
		return false
	}
	t, e := created(p.PID)
	return e == nil && t == p.Created
}
func terminationSignals() []os.Signal { return []os.Signal{os.Interrupt, syscall.SIGTERM} }
func recordHarness(dir string, c *exec.Cmd) error {
	return writeProcessRecord(filepath.Join(dir, "harness.json"), c.Process.Pid)
}
func stopHarness(dir string) error {
	path := filepath.Join(dir, "harness.json")
	if !processRecordLive(path) {
		return nil
	}
	var p pidRecord
	_ = readJSON(path, &p)
	return helperExec("taskkill", "/PID", strconv.Itoa(p.PID), "/T", "/F").Run()
}
func liveNames() (map[string]bool, error) {
	m := map[string]bool{}
	dirs, e := os.ReadDir(Root())
	if os.IsNotExist(e) {
		return m, nil
	}
	if e != nil {
		return nil, e
	}
	for _, d := range dirs {
		var p pidRecord
		if orchestratorStateDir(d.Name()) {
			continue
		}
		if readJSON(filepath.Join(Root(), d.Name(), "supervisor.json"), &p) == nil {
			if t, e := created(p.PID); e == nil && t == p.Created {
				m[d.Name()] = true
			}
		}
	}
	return m, nil
}
func WindowsLaunch(exe, name string) []string {
	return []string{exe, "dispatch", "supervise", "--", name}
}

// launchSupervisor starts a run's supervisor through its own scheduled task,
// outside the launching console and job: an SSH session's, or the job an
// orchestrator round's harness runs in.
func launchSupervisor(dir, name string) error {
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	// Started by its task, the supervisor must adopt the launch file.
	args := []string{"dispatch", "supervise", scheduledFlag, "--", name}
	t, e := newSupervisorTask(runTaskName(name), exe, args, dir, childEnvironment(""))
	if e != nil {
		return e
	}
	t.Log = filepath.Join(dir, "supervisor.log")
	return startSupervisorTask(supervisorTasks, t, supervisorConfirmWait)
}
func lockRoot() (func(), error) { return lockDirectory(Root()) }

func tryLockRoot() (func(), error) { return tryLockDirectory(Root()) }

func tryLockDirectory(dir string) (func(), error) {
	p := filepath.Join(dir, ".lockdir")
	if err := os.Mkdir(p, 0700); err != nil {
		return nil, err
	}
	return func() { _ = os.Remove(p) }, nil
}

func lockDirectory(dir string) (func(), error) {
	for i := 0; i < 300; i++ {
		if unlock, e := tryLockDirectory(dir); e == nil {
			return unlock, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, fmt.Errorf("admission lock busy; inspect %s", filepath.Join(dir, ".lockdir"))
}

// A job retains the complete descendant tree even after its root exits.
func startChild(c *exec.Cmd) (func(), error) { return startJobChild(c, "") }

// startSourceChild runs a plugin inside a kill-on-close job. Cancellation
// closes the job, which terminates grandchildren that inherited its pipes.
func startSourceChild(c *exec.Cmd, cmdline string) (func(), error) {
	var mu sync.Mutex
	var closeJob func()
	c.Cancel = func() error {
		mu.Lock()
		f := closeJob
		mu.Unlock()
		if f != nil {
			f()
		}
		return c.Process.Kill()
	}
	cleanup, e := startJobChild(c, cmdline)
	if e != nil {
		return nil, e
	}
	mu.Lock()
	closeJob = cleanup
	mu.Unlock()
	return cleanup, nil
}

func startJobChild(c *exec.Cmd, cmdline string) (func(), error) {
	job, e := windows.CreateJobObject(nil, nil)
	if e != nil {
		return nil, e
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, e = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); e != nil {
		windows.CloseHandle(job)
		return nil, e
	}
	c.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windowsHarnessFlags()}
	configureHarnessCommandLine(c)
	if cmdline != "" {
		c.SysProcAttr.CmdLine = cmdline
	}
	if e = c.Start(); e != nil {
		windows.CloseHandle(job)
		return nil, e
	}
	p, e := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(c.Process.Pid))
	if e == nil {
		e = windows.AssignProcessToJobObject(job, p)
		if e == nil {
			e = resumePrimaryThread(uint32(c.Process.Pid))
		}
		windows.CloseHandle(p)
	}
	if e != nil {
		killTree(c)
		_ = c.Wait()
		windows.CloseHandle(job)
		return nil, e
	}
	var once sync.Once
	return func() { once.Do(func() { _ = windows.CloseHandle(job) }) }, nil
}

// The primary thread is the only thread in a newly created suspended process.
func resumePrimaryThread(pid uint32) error {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	for err = windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID != pid {
			continue
		}
		thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if err != nil {
			return err
		}
		defer windows.CloseHandle(thread)
		_, err = windows.ResumeThread(thread)
		return err
	}
	return fmt.Errorf("suspended primary thread not found: %w", err)
}

func windowsHarnessFlags() uint32 { return windows.CREATE_SUSPENDED }

func configureHarnessCommandLine(c *exec.Cmd) {
	if c.SysProcAttr == nil {
		c.SysProcAttr = &syscall.SysProcAttr{}
	}
	c.SysProcAttr.CmdLine = windows.ComposeCommandLine(c.Args)
}

func notificationSessionExists(name string) bool { return false }

// processRecordMayBeLive is processRecordLive, except a hev process Windows
// will not let this token query (another logon's, or a higher integrity
// level's) also counts: unknown is not gone. A recorded process that has
// exited is gone, and so is one whose PID now names a different process:
// a different creation time, or, when that cannot be read, a different
// executable in the process snapshot (which needs no access to the process),
// as when Windows has reused the PID for a service.
func processRecordMayBeLive(path string) bool {
	var p pidRecord
	if readJSON(path, &p) != nil {
		return false
	}
	t, e := created(p.PID)
	if e == nil {
		return t == p.Created
	}
	if !errors.Is(e, windows.ERROR_ACCESS_DENIED) {
		return false
	}
	image, e := processImage(p.PID)
	if errors.Is(e, os.ErrNotExist) {
		return false
	}
	return e != nil || hevImage(image)
}

// processImage is replaced in tests, which must not read the process table.
var processImage = snapshotProcessImage

// snapshotProcessImage names pid's executable from a process snapshot.
func snapshotProcessImage(pid int) (string, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(snap)
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	for err = windows.Process32First(snap, &entry); err == nil; err = windows.Process32Next(snap, &entry) {
		if int(entry.ProcessID) == pid {
			return windows.UTF16ToString(entry.ExeFile[:]), nil
		}
	}
	if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return "", os.ErrNotExist
	}
	return "", err
}

// hevImage is true for hev's own executable name, installed or as running.
func hevImage(image string) bool {
	if strings.EqualFold(image, "hev.exe") {
		return true
	}
	self, err := os.Executable()
	return err == nil && strings.EqualFold(image, filepath.Base(self))
}
