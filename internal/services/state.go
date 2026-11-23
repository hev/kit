package services

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/hev/kit/internal/platformops"
)

const (
	StatusRunning = "running"
	StatusStopped = "stopped"
	StatusBackoff = "backoff"
	StatusStandby = "standby"
	StatusFailed  = "failed"
)

var errStorage = errors.New("could not access service state")

type State struct {
	Status          string     `json:"status"`
	Reason          string     `json:"reason,omitempty"`
	PID             int        `json:"pid,omitempty"`
	StartTime       *time.Time `json:"start_time,omitempty"`
	LastStop        *time.Time `json:"last_stop,omitempty"`
	Restarts        int        `json:"restarts"`
	LastExit        *int       `json:"last_exit,omitempty"`
	PlacementWinner string     `json:"placement_winner,omitempty"`
	NextRestart     *time.Time `json:"next_restart,omitempty"`
	Stopped         bool       `json:"stopped,omitempty"`
	CrashLoop       bool       `json:"crash_loop,omitempty"`
	CrashEpisode    string     `json:"crash_episode,omitempty"`
}

type Summary struct {
	Name            string     `json:"name"`
	Status          string     `json:"status"`
	Reason          string     `json:"reason,omitempty"`
	PID             int        `json:"pid,omitempty"`
	StartTime       *time.Time `json:"start_time,omitempty"`
	Restarts        int        `json:"restarts"`
	LastExit        *int       `json:"last_exit,omitempty"`
	PlacementWinner string     `json:"placement_winner,omitempty"`
	NextRestart     *time.Time `json:"next_restart,omitempty"`
	Restart         string     `json:"restart"`
	Placement       []string   `json:"placement,omitempty"`
	Enabled         bool       `json:"enabled"`
	State           State      `json:"state"`
}

type Details struct {
	Summary
	StopTimeout string   `json:"stop_timeout"`
	EnvKeys     []string `json:"env_keys,omitempty"`
	Logs        string   `json:"logs,omitempty"`
}

type controlRequest struct {
	ID          string    `json:"id"`
	Action      string    `json:"action"`
	RequestedAt time.Time `json:"requested_at"`
}

type controlAck struct {
	ID     string `json:"id"`
	Status string `json:"status,omitempty"`
	Error  string `json:"error,omitempty"`
}

func servicesRoot(home string) string {
	return filepath.Join(home, ".hev", "services")
}

func supervisorLockPath(home string) string {
	return filepath.Join(servicesRoot(home), "supervisor.lock")
}

func supervisorRunning(home string) bool {
	lockPath := supervisorLockPath(home)
	if _, err := os.Stat(lockPath); err != nil {
		return false
	}
	release, err := platformops.LockPath(lockPath, true)
	if err == nil {
		release()
		return false
	}
	return platformops.IsLockBusy(err)
}

func readJSON(path string, dst any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	const recordLimit = 64 << 10
	data, err := io.ReadAll(io.LimitReader(f, recordLimit+1))
	if err != nil || len(data) > recordLimit {
		return errStorage
	}
	return json.Unmarshal(data, dst)
}

func writeJSON(path string, data any) error {
	b, err := json.Marshal(data)
	if err == nil {
		err = platformops.WriteFileAtomic(path, b, 0600)
	}
	if err != nil {
		return errStorage
	}
	return nil
}

func readState(dir string) (State, error) {
	var s State
	if err := readJSON(filepath.Join(dir, "state.json"), &s); err != nil && !os.IsNotExist(err) {
		return State{}, errStorage
	}
	return s, nil
}

func writeState(dir, home string, s State) error {
	s.Reason = scrubReason(s.Reason, home, dir)
	return writeJSON(filepath.Join(dir, "state.json"), s)
}

func lockPathWithTimeout(ctx context.Context, dir, filename string, timeout time.Duration) (func(), error) {
	if err := platformops.MkdirAll(dir, 0700); err != nil {
		return nil, errStorage
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	target := filepath.Join(dir, filename)
	for {
		release, err := platformops.LockPath(target, true)
		if err == nil {
			return release, nil
		}
		if !platformops.IsLockBusy(err) {
			return nil, errStorage
		}
		select {
		case <-ctx.Done():
			return nil, errStorage
		case <-ticker.C:
		}
	}
}

func lockState(ctx context.Context, dir string) (func(), error) {
	return lockPathWithTimeout(ctx, dir, "state.lock", 5*time.Second)
}
