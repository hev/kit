package dispatch

import (
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

func setupChild(c *exec.Cmd) {}
func killTree(c *exec.Cmd) {
	if c.Process != nil {
		_ = exec.Command("taskkill", "/PID", strconv.Itoa(c.Process.Pid), "/T", "/F").Run()
	}
}

type pidRecord struct {
	PID     int    `json:"pid"`
	Created uint64 `json:"created"`
}

func created(pid int) (uint64, error) {
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
	return exec.Command("taskkill", "/PID", strconv.Itoa(p.PID), "/T", "/F").Run()
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
func launch(dir, name string) error {
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	a := WindowsLaunch(exe, name)
	c := exec.Command(a[0], a[1:]...)
	c.Dir = dir
	c.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windowsSupervisorFlags()}
	if e = c.Start(); e != nil {
		// Task Scheduler starts outside the SSH service's restrictive job.
		task := "hev-dispatch-" + name
		line := syscall.EscapeArg(exe) + " dispatch supervise -- " + name
		if _, err := command("", "schtasks", "/Create", "/TN", task, "/TR", line, "/SC", "ONCE", "/ST", "00:00", "/F"); err != nil {
			return fmt.Errorf("breakaway: %v; scheduled task: %w", e, err)
		}
		if _, err := command("", "schtasks", "/Run", "/TN", task); err != nil {
			return err
		}
		defer command("", "schtasks", "/Delete", "/TN", task, "/F")
	} else {
		defer c.Process.Release()
	}
	for i := 0; i < 100; i++ {
		var p pidRecord
		if readJSON(filepath.Join(dir, "supervisor.json"), &p) == nil {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return errLaunchUnconfirmed
}
func lockRoot() (func(), error) {
	p := filepath.Join(Root(), ".lockdir")
	for i := 0; i < 300; i++ {
		if e := os.Mkdir(p, 0700); e == nil {
			return func() { _ = os.Remove(p) }, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, fmt.Errorf("admission lock busy; inspect %s", p)
}

// A job retains the complete descendant tree even after its root exits.
func startChild(c *exec.Cmd) (func(), error) {
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

func windowsSupervisorFlags() uint32 {
	return windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS | windows.CREATE_BREAKAWAY_FROM_JOB
}
func windowsHarnessFlags() uint32 { return windows.CREATE_SUSPENDED }

func configureHarnessCommandLine(c *exec.Cmd) {
	if c.SysProcAttr == nil {
		c.SysProcAttr = &syscall.SysProcAttr{}
	}
	c.SysProcAttr.CmdLine = windows.ComposeCommandLine(c.Args)
}
