package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hev/kit/internal/platformops"
)

const (
	historyLimit = 20
	outputLimit  = 64 << 10
)

var errStorage = errors.New("could not access job state")

type State struct {
	LastStart           *time.Time    `json:"last_start,omitempty"`
	LastEnd             *time.Time    `json:"last_end,omitempty"`
	ExitCode            *int          `json:"exit_code,omitempty"`
	Duration            time.Duration `json:"duration_ns"`
	ConsecutiveFailures int           `json:"consecutive_failures"`
	Paused              bool          `json:"paused"`
	LastPlacementWinner string        `json:"last_placement_winner,omitempty"`
}

type Run struct {
	Start    time.Time     `json:"start"`
	End      time.Time     `json:"end"`
	Status   string        `json:"status"`
	ExitCode *int          `json:"exit_code,omitempty"`
	Duration time.Duration `json:"duration_ns"`
	Output   string        `json:"output,omitempty"`
	Message  string        `json:"message,omitempty"`
}

func readJSON(path string, dst any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	// Allow for JSON escapes of every output byte plus the record metadata.
	const recordLimit = 8 * outputLimit
	data, err := io.ReadAll(io.LimitReader(f, recordLimit+1))
	if err != nil || len(data) > recordLimit {
		return errStorage
	}
	return json.Unmarshal(data, dst)
}

func readState(dir string) (State, error) {
	var s State
	if err := readJSON(filepath.Join(dir, "state.json"), &s); err != nil && !os.IsNotExist(err) {
		return State{}, errStorage
	}
	return s, nil
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

func lockState(ctx context.Context, dir string) (func(), error) {
	if err := platformops.MkdirAll(dir, 0700); err != nil {
		return nil, errStorage
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		release, err := platformops.LockPath(filepath.Join(dir, "state.lock"), true)
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

func historyFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errStorage
	}
	var names []string
	for _, entry := range entries {
		name := entry.Name()
		if len(name) != 25 || !strings.HasSuffix(name, ".json") || !entry.Type().IsRegular() {
			continue
		}
		if _, err := strconv.ParseUint(name[:20], 10, 64); err == nil {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

// Called under the state lock. Evict before publishing so a crash cannot leave
// a twenty-first record. The sequence orders writes even if the clock changes.
func appendRun(dir string, r Run) error {
	dir = filepath.Join(dir, "runs")
	if err := platformops.MkdirAll(dir, 0700); err != nil {
		return errStorage
	}
	names, err := historyFiles(dir)
	if err != nil {
		return err
	}
	var seq uint64
	if n := len(names); n != 0 {
		seq, _ = strconv.ParseUint(names[n-1][:20], 10, 64)
	}
	for len(names) >= historyLimit {
		if err := os.Remove(filepath.Join(dir, names[0])); err != nil {
			return errStorage
		}
		names = names[1:]
	}
	return writeJSON(filepath.Join(dir, fmt.Sprintf("%020d.json", seq+1)), r)
}

func readRuns(dir string) ([]Run, error) {
	dir = filepath.Join(dir, "runs")
	names, err := historyFiles(dir)
	if err != nil {
		return nil, err
	}
	runs := make([]Run, 0, min(len(names), historyLimit))
	for i := len(names) - 1; i >= max(0, len(names)-historyLimit); i-- {
		var r Run
		if err := readJSON(filepath.Join(dir, names[i]), &r); err != nil {
			return nil, errStorage
		}
		runs = append(runs, r)
	}
	return runs, nil
}
