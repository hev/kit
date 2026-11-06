package chef

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Archive releases bounded active attention capacity only through the handled
// cursor. Immutable settled receipts preserve dedupe and action/assessment
// lineage. This is explicit retention, not a janitor or retry scheduler.
func (s Store) Archive(l Lease, through uint64) error {
	return s.locked(func(st *State) error {
		if e := check(st, l); e != nil {
			return e
		}
		if through > st.Cursor {
			return errors.New("cannot archive unhandled attention")
		}
		kept := []Attention{}
		for _, a := range st.Attention {
			if a.Sequence > through {
				kept = append(kept, a)
				continue
			}
			if a.Status != "done" && a.Status != "stale" {
				return ErrHeld
			}
			path := filepath.Join(s.Dir, "receipt-"+a.ID+".json")
			b, e := json.Marshal(a)
			if e != nil {
				return e
			}
			old, e := os.ReadFile(path)
			if e == nil {
				if string(old) != string(b) {
					return errors.New("immutable archived receipt conflict")
				}
			} else if !errors.Is(e, os.ErrNotExist) {
				return e
			} else {
				// Atomic rename plus fsync before dropping the active record. Interrupted
				// archival repeats only a byte-identical receipt.
				if e = durableReceipt(s.Dir, path, b); e != nil {
					return e
				}
			}
		}
		st.Attention = kept
		// Notification dedupe survives in archived receipts. Keep only one floor
		// observation per Order; those are independently bounded on admission.
		for k := range st.Observations {
			if strings.Contains(k, "/notification/") {
				delete(st.Observations, k)
			}
		}
		return s.commit(st)
	})
}
func durableReceipt(dir, path string, b []byte) error {
	f, e := os.CreateTemp(dir, ".receipt-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if e = f.Chmod(0600); e != nil {
		return e
	}
	if _, e = f.Write(b); e != nil {
		return e
	}
	if e = f.Sync(); e != nil {
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	if e = os.Rename(f.Name(), path); e != nil {
		return e
	}
	d, e := os.Open(dir)
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}

// Receipt reads exactly one settled entry. No transcript scanning or path input.
func (s Store) Receipt(id string) (Attention, error) {
	var a Attention
	if len(id) != 64 || strings.Trim(id, "0123456789abcdef") != "" {
		return a, errors.New("invalid receipt identity")
	}
	e := s.locked(func(st *State) error {
		for _, v := range st.Attention {
			if v.ID == id {
				a = v
				return nil
			}
		}
		b, e := os.ReadFile(filepath.Join(s.Dir, "receipt-"+id+".json"))
		if e != nil {
			return e
		}
		if e = json.Unmarshal(b, &a); e != nil {
			return e
		}
		if a.ID != id {
			return errors.New("receipt identity mismatch")
		}
		return nil
	})
	return a, e
}
