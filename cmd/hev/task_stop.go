package main

import (
	"errors"
	"fmt"
	"os"
	"time"
)

type daemonControl interface {
	Signal(bool) error
	Exited() (bool, error)
	Close()
}

// Disable first, retain process ownership through escalation, and confirm both
// scheduler and process exit. Missing publication never proves scheduler exit.
func stopWindowsTask(action string, query func() (bool, bool, error), run func(...string) (string, error), open func() (daemonControl, error), timeout time.Duration) error {
	registered, active, err := query()
	if err != nil {
		return err
	}
	if registered {
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
