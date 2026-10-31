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
func writePID(dir string) error {
	t, e := created(os.Getpid())
	if e != nil {
		return e
	}
	return WriteJSON(filepath.Join(dir, "supervisor.json"), pidRecord{os.Getpid(), t})
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
func WindowsLaunch(exe, name string) []string { return []string{exe, "dispatch", "supervise", name} }
func launch(dir, name string) error {
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	a := WindowsLaunch(exe, name)
	c := exec.Command(a[0], a[1:]...)
	c.Dir = dir
	c.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS}
	if e = c.Start(); e != nil {
		return e
	}
	defer c.Process.Release()
	for i := 0; i < 100; i++ {
		var p pidRecord
		if readJSON(filepath.Join(dir, "supervisor.json"), &p) == nil {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("supervisor did not publish live pid")
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
	if e = c.Start(); e != nil {
		windows.CloseHandle(job)
		return nil, e
	}
	p, e := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(c.Process.Pid))
	if e == nil {
		e = windows.AssignProcessToJobObject(job, p)
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
