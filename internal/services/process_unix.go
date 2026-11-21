//go:build !windows

package services

import (
	"os/exec"
	"syscall"
	"time"
)

func startProcessTree(cmd *exec.Cmd) (int, func(time.Duration, <-chan struct{}), func(), error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return 0, nil, nil, err
	}
	pgid := cmd.Process.Pid
	killGroup := func() {
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	}
	gracefulStop := func(timeout time.Duration, waitDone <-chan struct{}) {
		_ = syscall.Kill(-pgid, syscall.SIGTERM)
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case <-waitDone:
		case <-timer.C:
			killGroup()
			<-waitDone
			return
		}
		// Ensure any child processes remaining in the group also exit within
		// the remaining stop_timeout, otherwise escalate to SIGKILL.
		if syscall.Kill(-pgid, 0) == syscall.ESRCH {
			return
		}
		tick := time.NewTicker(20 * time.Millisecond)
		defer tick.Stop()
		for {
			if syscall.Kill(-pgid, 0) == syscall.ESRCH {
				return
			}
			select {
			case <-timer.C:
				killGroup()
				for range 25 {
					if syscall.Kill(-pgid, 0) == syscall.ESRCH {
						return
					}
					time.Sleep(10 * time.Millisecond)
				}
				return
			case <-tick.C:
			}
		}
	}
	return pgid, gracefulStop, killGroup, nil
}
