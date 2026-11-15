package dispatch

import (
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// superviseProcess is shared by agent rounds and finite command runs. The
// supervisor, rather than hevd, owns timeout, stop, process-tree cleanup and
// birth records, so restarting the scheduler leaves admitted work alone.
func superviseProcess(dir string, child *exec.Cmd, r Result, limit time.Duration, signals <-chan os.Signal, startErr error, tick func(*time.Timer, *Result)) Result {
	setupChild(child)
	_ = WriteJSON(filepath.Join(dir, "result.json"), r)
	kill := func() {}
	if startErr == nil {
		kill, startErr = startChild(child)
	}
	if startErr == nil {
		if err := recordHarness(dir, child); err != nil {
			kill()
			_ = child.Wait()
			startErr = err
		}
	}
	if startErr != nil {
		r.Error = startErr.Error()
		return r
	}
	done := make(chan error, 1)
	go func() { done <- child.Wait() }()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(limit)
	defer deadline.Stop()
	var err error
	finished := false
	for !finished {
		select {
		case err = <-done:
			finished = true
		case <-ticker.C:
			if tick != nil {
				tick(deadline, &r)
			}
			if _, e := os.Stat(filepath.Join(dir, "stop")); e == nil {
				r.Stopped = true
				kill()
				err = <-done
				finished = true
			}
		case sig := <-signals:
			r.Stopped = true
			r.Signal = sig.String()
			kill()
			err = <-done
			finished = true
		case <-deadline.C:
			r.Timeout = true
			kill()
			err = <-done
			finished = true
		}
	}
	kill()
	r.ExitCode = 0
	if err != nil {
		r.ExitCode = 1
		if ee, ok := err.(*exec.ExitError); ok {
			r.ExitCode = ee.ExitCode()
			if r.ExitCode < 0 {
				r.ExitCode = 137
			}
		}
	}
	if r.Stopped {
		r.ExitCode = 143
	}
	return r
}
