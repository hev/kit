package update

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// State is what the updater remembers on a host, in ~/.hev/update/state.json.
type State struct {
	// Links are the paths this host runs hev from that the updater switches.
	Links []string `json:"links,omitempty"`
	// Current and Previous label builds under ~/.hev/versions.
	Current  string `json:"current,omitempty"`
	Previous string `json:"previous,omitempty"`
	// Pending is a daemon self-update not yet confirmed by the new daemon.
	Pending *Pending `json:"pending,omitempty"`
	// The daemon's schedule.
	LastCheck time.Time `json:"last_check,omitzero"`
	NextCheck time.Time `json:"next_check,omitzero"`
	Failures  int       `json:"failures,omitempty"`
	LastError string    `json:"last_error,omitempty"`
	// History is the newest events, oldest first.
	History []Event `json:"history,omitempty"`
}

// Pending marks a switch the daemon made to itself. The new daemon confirms
// it after a clean cycle; a daemon that keeps failing to start rolls back.
type Pending struct {
	From     string    `json:"from"`
	To       string    `json:"to"`
	At       time.Time `json:"at"`
	Attempts int       `json:"attempts"`
}

// Event is one thing the updater did.
type Event struct {
	At      time.Time `json:"at"`
	By      string    `json:"by"`
	Action  string    `json:"action"`
	From    string    `json:"from,omitempty"`
	To      string    `json:"to,omitempty"`
	Channel string    `json:"channel,omitempty"`
	Result  string    `json:"result"`
	Detail  string    `json:"detail,omitempty"`
}

const maxHistory = 50

func (u *Updater) dir() string         { return filepath.Join(u.Home, ".hev", "update") }
func (u *Updater) versionsDir() string { return filepath.Join(u.Home, ".hev", "versions") }
func (u *Updater) statePath() string   { return filepath.Join(u.dir(), "state.json") }

// LoadState reads the host's update state; a missing file is empty.
func (u *Updater) LoadState() (State, error) {
	var s State
	raw, err := os.ReadFile(u.statePath())
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	return s, json.Unmarshal(raw, &s)
}

func (u *Updater) saveState(s State) error {
	if len(s.History) > maxHistory {
		s.History = s.History[len(s.History)-maxHistory:]
	}
	if err := os.MkdirAll(u.dir(), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(u.statePath(), append(raw, '\n'), 0o600)
}

func (u *Updater) record(s *State, e Event) {
	e.At = u.now()
	if e.By == "" {
		e.By = u.By
	}
	s.History = append(s.History, e)
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
