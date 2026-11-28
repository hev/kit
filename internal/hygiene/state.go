package hygiene

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/hev/kit/internal/platformops"
)

// State is the persisted summary stored in ~/.hev/hygiene/state.json.
// It contains no secrets and no local filesystem paths.
type State struct {
	LastRun         *time.Time `json:"last_run,omitempty"`
	LastDurationMS  int64      `json:"last_duration_ms,omitempty"`
	FreeBytesBefore uint64     `json:"free_bytes_before"`
	FreeBytesAfter  uint64     `json:"free_bytes_after"`
	BytesFreed      int64      `json:"bytes_freed"`
	CompactedRuns   int        `json:"compacted_runs"`
	PrunedTmpDirs   int        `json:"pruned_tmp_dirs"`
	EvictedSlots    int        `json:"evicted_slots"`
	APFSThinned     bool       `json:"apfs_thinned"`
	UnderPressure   bool       `json:"under_pressure"`
	LastError       string     `json:"last_error,omitempty"`
}

func stateDir(home string) string {
	return filepath.Join(home, ".hev", "hygiene")
}

func StatePath(home string) string {
	return filepath.Join(stateDir(home), "state.json")
}

func readState(home string) (State, error) {
	var s State
	b, err := os.ReadFile(StatePath(home))
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, errors.New("could not read hygiene state")
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return s, errors.New("hygiene state is corrupted")
	}
	return s, nil
}

func writeState(home string, s State) error {
	dir := stateDir(home)
	if err := platformops.MkdirAll(dir, 0o700); err != nil {
		return errors.New("could not create hygiene state directory")
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return errors.New("could not encode hygiene state")
	}
	if err := platformops.WriteFileAtomic(StatePath(home), append(b, '\n'), 0o600); err != nil {
		return errors.New("could not write hygiene state")
	}
	return nil
}
