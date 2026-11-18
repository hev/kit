package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

type daemonControl interface {
	Signal(bool) error
	Exited() (bool, error)
	Close()
}

// daemonControlResult converts a concrete process result to an interface without
// retaining a typed nil pointer when opening the process failed.
func daemonControlResult(p daemonControl, err error) (daemonControl, error) {
	if err != nil {
		return nil, err
	}
	return p, nil
}

// stopWindowsTask stops the daemon and confirms both scheduler and process
// exit, retaining process ownership through escalation. Missing publication
// never proves scheduler exit. action is "stop", which leaves the task
// enabled so its triggers start the daemon again (a stop that disabled it
// left win-oven's capture down until someone re-enabled the task by hand),
// "disable", which disables it first so no trigger restarts the daemon, or
// "uninstall", which disables it and then deletes it.
func stopWindowsTask(action string, query func() (bool, bool, error), run func(...string) (string, error), open func() (daemonControl, error), timeout time.Duration) error {
	registered, active, err := query()
	if err != nil {
		return err
	}
	if registered && action != "stop" {
		if _, err = run("/Change", "/TN", windowsTaskName, "/DISABLE"); err != nil {
			return err
		}
	}
	p, err := open()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if p != nil {
		defer p.Close()
		if err = p.Signal(false); err != nil {
			return err
		}
	}
	if registered && active && p == nil {
		if _, err = run("/End", "/TN", windowsTaskName); err != nil {
			return err
		}
	}
	deadline := time.Now().Add(timeout)
	forced := false
	for {
		exists, running, err := query()
		if err != nil {
			return err
		}
		gone := true
		if p != nil {
			gone, err = p.Exited()
			if err != nil {
				return err
			}
		}
		if !running && gone {
			if action == "uninstall" && exists {
				if _, err = run("/Delete", "/TN", windowsTaskName, "/F"); err != nil {
					return err
				}
				exists, _, err = query()
				if err != nil {
					return err
				}
				if exists {
					return fmt.Errorf("task remains registered after deletion")
				}
			}
			return nil
		}
		if time.Now().After(deadline) {
			if forced {
				return fmt.Errorf("daemon/task did not stop")
			}
			if p != nil {
				if err = p.Signal(true); err != nil {
					return err
				}
			}
			if exists {
				if _, err = run("/End", "/TN", windowsTaskName); err != nil {
					return err
				}
			}
			forced = true
			deadline = time.Now().Add(timeout)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func confirmTaskStartup(query func() (bool, bool, error), open func() (daemonControl, error), timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		exists, running, err := query()
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("task disappeared during startup")
		}
		if running {
			p, err := open()
			if err == nil {
				gone, waitErr := p.Exited()
				p.Close()
				if waitErr != nil {
					return waitErr
				}
				if !gone {
					return nil
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("scheduled daemon did not publish a live process before startup timeout")
}

// start enables the task (a stop --disable, or an older kit's stop, left it
// disabled) and runs it. It never re-registers the task, so a task whose
// action is an operator's own launcher keeps it.
func startWindowsTask(run func(...string) (string, error), confirm func() error) error {
	if _, err := run("/Change", "/TN", windowsTaskName, "/ENABLE"); err != nil {
		return err
	}
	if _, err := run("/Run", "/TN", windowsTaskName); err != nil {
		return err
	}
	return confirm()
}

// windowsStopMessage says what hev stop did to the scheduled task.
func windowsStopMessage(registered, disabled bool) string {
	switch {
	case !registered:
		return "Daemon: stopped"
	case disabled:
		return "Daemon: stopped; scheduled task " + windowsTaskName + " disabled, so nothing starts it again until hev d or hev daemon start"
	}
	return "Daemon: stopped; scheduled task " + windowsTaskName + " left enabled, so its triggers start the daemon again (within a minute for kit's task; --disable keeps it stopped)"
}

// taskAction is a scheduled task's first exec action.
type taskAction struct {
	Command   string
	Arguments string
}

func (a taskAction) String() string { return strings.TrimSpace(a.Command + " " + a.Arguments) }

// kitTaskAction reports whether a task's action is kit's own launcher, what
// hev daemon install registers: hev.exe run with "daemon". Anything else (a
// PowerShell wrapper that sets an operator's environment, say) is the
// operator's, and kit never re-registers over it.
func kitTaskAction(a taskAction) bool {
	cmd := strings.Trim(strings.TrimSpace(a.Command), `"`)
	if i := strings.LastIndexAny(cmd, `\/`); i >= 0 {
		cmd = cmd[i+1:]
	}
	if !strings.EqualFold(cmd, "hev.exe") && !strings.EqualFold(cmd, "hev") {
		return false
	}
	args := strings.Fields(a.Arguments)
	return len(args) > 0 && strings.EqualFold(args[0], "daemon")
}

// startExistingWindowsTask is hev d on a host with the task registered:
// enable and run the task as it is, never re-registering it, and say so when
// its action is not kit's own launcher.
func startExistingWindowsTask(query func() (bool, bool, error), action func() (taskAction, error), run func(...string) (string, error), confirm func() error) (started bool, note string, err error) {
	registered, _, err := query()
	if err != nil || !registered {
		return false, "", err
	}
	a, aerr := action()
	if err := startWindowsTask(run, confirm); err != nil {
		return false, "", err
	}
	switch {
	case aerr != nil:
		note = "Task Scheduler: " + windowsTaskName + " enabled and started (its action could not be read: " + aerr.Error() + ")"
	case kitTaskAction(a):
		note = "Task Scheduler: " + windowsTaskName + " enabled and started"
	default:
		note = "Task Scheduler: " + windowsTaskName + " enabled and started with its custom action, left as it is: " + a.String()
	}
	return true, note, nil
}

// refuseCustomTask stops hev daemon install from replacing a task whose
// action is not kit's own launcher, unless forced.
func refuseCustomTask(query func() (bool, bool, error), action func() (taskAction, error), force bool) error {
	if force {
		return nil
	}
	registered, _, err := query()
	if err != nil || !registered {
		return err
	}
	a, err := action()
	if err != nil {
		return fmt.Errorf("read the action of the registered %s task before replacing it: %w (--force replaces it anyway)", windowsTaskName, err)
	}
	if kitTaskAction(a) {
		return nil
	}
	return fmt.Errorf("the registered %s task runs a custom action (%s), not kit's launcher; not replacing it. Start it with hev daemon start, or pass --force to replace it with kit's", windowsTaskName, a)
}
