//go:build !windows

package dispatch

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

func setupChild(c *exec.Cmd) { c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }
func killTree(c *exec.Cmd) {
	if c.Process != nil {
		_ = syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
	}
}

type pidRecord struct {
	PID     int    `json:"pid"`
	Created string `json:"created"`
}

// Include the kernel start identity: a recycled PID never grants kill authority.
func processIdentity(pid int) (string, error) {
	if runtime.GOOS == "linux" {
		data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
		if err != nil {
			return "", err
		}
		fields := strings.Fields(string(data)[strings.LastIndex(string(data), ")")+1:])
		if len(fields) < 20 {
			return "", fmt.Errorf("invalid process stat")
		}
		return fields[19], nil
	}
	out, err := helperExec("ps", "-p", strconv.Itoa(pid), "-o", "lstart=").Output()
	if err != nil {
		return "", err
	}
	identity := strings.TrimSpace(string(out))
	if identity == "" {
		return "", fmt.Errorf("process exited")
	}
	return identity, nil
}
func writeProcessRecord(path string, pid int) error {
	identity, err := processIdentity(pid)
	if err != nil {
		return err
	}
	return WriteJSON(path, pidRecord{PID: pid, Created: identity})
}
func processRecordLive(path string) bool {
	var p pidRecord
	if readJSON(path, &p) != nil || p.PID <= 0 || p.Created == "" {
		return false
	}
	identity, err := processIdentity(p.PID)
	return err == nil && identity == p.Created && syscall.Kill(p.PID, 0) == nil
}
func writePID(dir string) error {
	return writeProcessRecord(filepath.Join(dir, "supervisor.json"), os.Getpid())
}
func terminationSignals() []os.Signal {
	return []os.Signal{syscall.SIGINT, syscall.SIGHUP, syscall.SIGTERM}
}
func recordHarness(dir string, c *exec.Cmd) error {
	return writeProcessRecord(filepath.Join(dir, "harness.json"), c.Process.Pid)
}
func stopHarness(dir string) error {
	path := filepath.Join(dir, "harness.json")
	var p pidRecord
	if !processRecordLive(path) || readJSON(path, &p) != nil {
		return nil
	}
	err := syscall.Kill(-p.PID, syscall.SIGKILL)
	if err == syscall.ESRCH {
		return nil
	}
	return err
}

// tmuxCommand is replaced by the test suite to require a private socket.
var tmuxCommand = newTmuxCommand

func newTmuxCommand(args ...string) *exec.Cmd {
	return helperCommand(context.Background(), "tmux", args...)
}

func liveNames() (map[string]bool, error) {
	out, e := tmuxCommand("list-sessions", "-F", "#S").CombinedOutput()
	if e != nil {
		// tmux exits when its last session closes (exit-empty). A query that
		// connected just before that loses the server mid-reply and reports
		// "server exited unexpectedly" instead of "no server running". Either
		// way the server, and every session it held, is gone.
		if strings.Contains(string(out), "no server running") || strings.Contains(string(out), "server exited unexpectedly") || strings.Contains(string(out), "no sessions") || strings.Contains(string(out), "No such file") {
			return map[string]bool{}, nil
		}
		return nil, fmt.Errorf("tmux: %w: %s", e, out)
	}
	m := map[string]bool{}
	for _, s := range strings.Fields(string(out)) {
		if strings.HasPrefix(s, "d-") {
			m[strings.TrimPrefix(s, "d-")] = true
		}
	}
	return m, nil
}
func launch(dir, name string) error {
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	// A pre-existing tmux server may carry plugin-only names in its environment.
	unset := "unset HEV_ORCHESTRATOR"
	for _, n := range pluginOnlyEnvNames() {
		unset += " " + Quote(n)
	}
	cmd := unset + "; exec " + Quote(exe) + " dispatch supervise -- " + Quote(name)
	args := []string{"new-session", "-d", "-s", "d-" + name, "-c", filepath.Join(dir, "wt")}
	env, err := launchEnvironment(dir)
	if err != nil {
		return err
	}
	for _, value := range env {
		args = append(args, "-e", value)
	}
	args = append(args, cmd)
	c := tmuxCommand(args...)
	c.Env = childEnvironment("")
	if e = c.Run(); e != nil {
		return fmt.Errorf("tmux launch: %w", e)
	}
	return e
}
func lockRoot() (func(), error) { return lockDirectory(Root()) }

func lockDirectory(dir string) (func(), error) {
	f, e := os.OpenFile(filepath.Join(dir, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); e != nil {
		f.Close()
		return nil, e
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}
func WindowsLaunch(exe, name string) []string {
	return []string{exe, "dispatch", "supervise", "--", name}
}

func startChild(c *exec.Cmd) (func(), error) {
	if e := c.Start(); e != nil {
		return nil, e
	}
	return func() { killTree(c) }, nil
}

func configureHarnessCommandLine(c *exec.Cmd) {}

func notificationSessionExists(name string) bool {
	out, err := tmuxCommand("list-panes", "-t", "=d-"+name+":", "-F", "#{session_name}").Output()
	if err != nil {
		return false
	}
	for _, session := range strings.Fields(string(out)) {
		if session == "d-"+name {
			return true
		}
	}
	return false
}

// signalProcessGroup sends SIGKILL to a process group. Tests replace it to
// check that the group is still owned when signalled.
var signalProcessGroup = func(pgid int) error { return syscall.Kill(-pgid, syscall.SIGKILL) }

// startSourceChild runs a plugin in a process group so cancellation and
// cleanup reach grandchildren that inherited its pipes. Wait reaps the plugin,
// after which its PID may be reused, so the plugin cannot name the group.
// Instead an owned holder process leads it and is reaped only after the final
// group kill: every signal targets a group number that is still reserved.
func startSourceChild(c *exec.Cmd, cmdline string) (func(), error) {
	if cmdline != "" {
		return nil, fmt.Errorf("batch plugins require Windows")
	}
	h, e := startGroupHolder()
	if e != nil {
		return nil, e
	}
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pgid: h.cmd.Process.Pid}
	c.Cancel = func() error { h.kill(); return nil }
	if e := c.Start(); e != nil {
		h.release()
		return nil, e
	}
	return h.release, nil
}

// groupHolder leads a process group and keeps its number reserved until
// release. Its stdin is a pipe only this process writes, so the holder exits
// on EOF if this process dies, and never holds the plugin's output pipes.
type groupHolder struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	keep   *os.File
	reaped bool
}

func startGroupHolder() (*groupHolder, error) {
	r, w, e := os.Pipe()
	if e != nil {
		return nil, e
	}
	defer r.Close()
	c := exec.Command("/bin/sh", "-c", "read _")
	c.Stdin = r
	c.Env = []string{}
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if e = c.Start(); e != nil {
		w.Close()
		return nil, e
	}
	return &groupHolder{cmd: c, keep: w}, nil
}

// kill signals the group unless release has already reaped the holder.
func (h *groupHolder) kill() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.reaped {
		_ = signalProcessGroup(h.cmd.Process.Pid)
	}
}

// release makes the final group kill, then reaps the holder.
func (h *groupHolder) release() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.reaped {
		return
	}
	_ = signalProcessGroup(h.cmd.Process.Pid)
	_ = h.keep.Close()
	_ = h.cmd.Wait()
	h.reaped = true
}
