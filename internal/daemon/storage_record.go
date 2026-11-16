package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hev/kit/internal/platformops"
	"github.com/hev/kit/internal/storagemode"
)

// StorageRecord is what the running capture daemon says about where it keeps
// traces. hevd writes it once it holds daemon.lock and removes it on exit, so
// another process can ask the daemon (not its own environment) which mode it
// is in. It names where a key came from, never the key.
type StorageRecord struct {
	PID       int              `json:"pid"`
	Started   time.Time        `json:"started"`
	Mode      storagemode.Mode `json:"mode"`
	Namespace string           `json:"namespace"`
	Endpoint  string           `json:"endpoint,omitempty"`
	KeySource string           `json:"key_source,omitempty"`
	Archive   string           `json:"archive,omitempty"`
}

// hevDir is ~/.hev, where the daemon keeps its lock, status and record.
func hevDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".hev"), nil
}

// StorageRecordPath is ~/.hev/daemon-storage.json.
func StorageRecordPath() string {
	dir, err := hevDir()
	if err != nil {
		return ".hev-daemon-storage.json"
	}
	return filepath.Join(dir, "daemon-storage.json")
}

// DaemonLockPath is the lock hevd holds for as long as it runs.
func DaemonLockPath() string {
	dir, err := hevDir()
	if err != nil {
		return ".hev-daemon.lock"
	}
	return filepath.Join(dir, "daemon.lock")
}

// recordFor describes target as the daemon opened it.
func recordFor(target Target) StorageRecord {
	r := StorageRecord{PID: os.Getpid(), Started: time.Now().UTC(), Mode: target.Mode, KeySource: target.KeySource}
	switch {
	case target.Files != nil:
		r.Namespace, r.Archive = target.Files.Namespace(), target.Files.Dir()
	case target.Layer != nil:
		r.Namespace, r.Endpoint = target.Layer.Namespace, target.Layer.Endpoint
	}
	return r
}

func writeStorageRecord(r StorageRecord) error {
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return platformops.WriteFileAtomic(StorageRecordPath(), raw, 0o600)
}

func removeStorageRecord() { _ = os.Remove(StorageRecordPath()) }

// DaemonState is the answer to "is a capture daemon running here, and where
// does it keep traces?".
type DaemonState struct {
	// Running is true while some process holds daemon.lock.
	Running bool
	// Record is the running daemon's own account of its storage; nil when no
	// daemon runs or the one that runs does not write a record (an older
	// build).
	Record *StorageRecord
}

// RunningStorage asks the capture daemon on this machine where it keeps
// traces. A record counts only while daemon.lock is held: a daemon that
// died leaves a record no one holds the lock for, and that record is
// ignored. It creates nothing: with no lock file there is no daemon.
func RunningStorage() (DaemonState, error) {
	lock := DaemonLockPath()
	if _, err := os.Stat(lock); errors.Is(err, os.ErrNotExist) {
		return DaemonState{}, nil
	} else if err != nil {
		return DaemonState{}, err
	}
	release, err := platformops.LockPath(lock, true)
	if err == nil {
		// Nobody holds it: no daemon is running.
		release()
		return DaemonState{}, nil
	}
	if !platformops.IsLockBusy(err) {
		return DaemonState{}, fmt.Errorf("check %s: %w", lock, err)
	}
	st := DaemonState{Running: true}
	raw, err := os.ReadFile(StorageRecordPath())
	if errors.Is(err, os.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	var r StorageRecord
	if err := json.Unmarshal(raw, &r); err != nil {
		return st, fmt.Errorf("read %s: %w", StorageRecordPath(), err)
	}
	st.Record = &r
	return st, nil
}

// daemonIntent is the running daemon's claim on the machine: a daemon that
// writes to Layer means this machine is set up for Layer, whatever this
// process's environment holds.
func daemonIntent() []string {
	st, err := RunningStorage()
	if err != nil || !st.Running || st.Record == nil || st.Record.Mode != storagemode.Layer || st.Record.PID == os.Getpid() {
		return nil
	}
	return []string{fmt.Sprintf("the capture daemon on this machine (pid %d) is writing to hev layer namespace %s", st.Record.PID, st.Record.Namespace)}
}
