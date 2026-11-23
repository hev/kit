package services

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type procResult struct {
	exitCode *int
	err      error
	reason   string
}

type runningProcess struct {
	pid      int
	started  time.Time
	done     <-chan procResult
	stopOnce sync.Once
	stopFn   func(timeout time.Duration)
	flushFn  func()
}

func (p *runningProcess) stop(timeout time.Duration) {
	p.stopOnce.Do(func() {
		p.stopFn(timeout)
	})
}

func (p *runningProcess) flushLog() {
	if p.flushFn != nil {
		p.flushFn()
	}
}

func resolveWorkdir(workdir, home string) string {
	dir := workdir
	if dir == "" || dir == "~" {
		return home
	}
	if strings.HasPrefix(dir, "~/") || strings.HasPrefix(dir, `~\`) {
		return filepath.Join(home, dir[2:])
	}
	if !filepath.IsAbs(dir) {
		return filepath.Join(home, dir)
	}
	return dir
}

func startServiceProcess(s Service, home, dir string, values map[string]string) (*runningProcess, error) {
	procCtx, procCancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(procCtx, s.Cmd[0], s.Cmd[1:]...)
	cmd.Dir = resolveWorkdir(s.Workdir, home)
	cmd.Env = cmd.Environ()
	for name, value := range values {
		cmd.Env = append(cmd.Env, name+"="+value)
	}

	paths := append([]string{home, cmd.Dir, cmd.Path}, s.Cmd...)
	scrub, multiline, maxMultiLen := newTextScrubber(values, paths...)
	ring := newRingLog(filepath.Join(dir, "service.log"), multiline, maxMultiLen)
	stdout := &streamWriter{ring: ring, scrub: scrub}
	stderr := &streamWriter{ring: ring, scrub: scrub}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	// Allow in-process redaction writers enough time after fast child exit to
	// drain the OS pipe buffer under heavy CPU/race load before closing pipes.
	cmd.WaitDelay = 15 * time.Second
	// Process group / Job Object termination is managed explicitly below so
	// stop_timeout can deliver graceful termination before force kill.
	cmd.Cancel = nil

	// Keep monotonic reading on started for accurate uptime measurement.
	started := time.Now()
	pid, gracefulStop, cleanupGroup, err := startProcessTree(cmd)
	if err != nil {
		procCancel()
		return nil, errors.New("could not start service process")
	}

	waitDone := make(chan struct{})
	resCh := make(chan procResult, 1)
	var stopping atomic.Bool

	go func() {
		waitErr := cmd.Wait()
		if !stopping.Load() {
			cleanupGroup()
		}
		procCancel()
		stdout.Flush()
		stderr.Flush()
		close(waitDone)

		var exit *int
		if cmd.ProcessState != nil {
			code := cmd.ProcessState.ExitCode()
			exit = &code
		}
		reason := ""
		if waitErr != nil {
			var exitErr *exec.ExitError
			if !errors.As(waitErr, &exitErr) {
				reason = "could not collect service process"
			}
		}
		resCh <- procResult{exitCode: exit, err: waitErr, reason: reason}
	}()

	var flushOnce sync.Once
	flush := func() {
		flushOnce.Do(func() {
			stdout.Flush()
			stderr.Flush()
		})
	}

	return &runningProcess{
		pid:     pid,
		started: started,
		done:    resCh,
		stopFn: func(timeout time.Duration) {
			stopping.Store(true)
			gracefulStop(timeout, waitDone)
		},
		flushFn: flush,
	}, nil
}
