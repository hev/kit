package chef

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

// StatusLimit bounds an existing journal read, independently of caller output
// pagination. Status is not model context; Next retains its 32 KiB packet limit.
const StatusLimit = 32 * 1024 * 1024

// ReadStatus reads an existing atomically published journal without changing
// filesystem contents or acquiring/creating a lock. A missing state returns an
// error matching os.ErrNotExist, never an empty initialized authority. The open
// file describes one complete published revision even during concurrent rename.
// It grants no lease or permission to execute. Expired owners remain historical
// facts; callers must compare Expires with their current time for display.
func (s Store) ReadStatus() (State, error) {
	var out State
	if !filepath.IsAbs(s.Dir) {
		return out, errors.New("absolute owned chef journal directory required")
	}
	f, err := os.OpenFile(filepath.Join(s.Dir, "state.json"), os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return out, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return out, err
	}
	if !info.Mode().IsRegular() {
		return out, errors.New("chef state must be a regular atomic journal file")
	}
	raw, err := io.ReadAll(io.LimitReader(f, StatusLimit+1))
	if err != nil {
		return out, err
	}
	if len(raw) > StatusLimit {
		return out, errors.New("chef status exceeds read limit")
	}
	if err = json.Unmarshal(raw, &out); err != nil {
		return State{}, err
	}
	if out.Version != Version || out.Observations == nil {
		return State{}, errors.New("incompatible chef journal")
	}
	return out, nil
}
