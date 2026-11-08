// Package chef implements opt-in single-writer coordination. It does not run a
// scheduler, discover hosts, advance DAG nodes, or adopt legacy orders.
package chef

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

const Version = "factory-chef-v1alpha1"
const PacketLimit = 32 * 1024
const MaxAttention = 100

var ErrFence = errors.New("chef lease expired or fence/owner changed")
var ErrHeld = errors.New("effect uncertain; inspection required")

// Lease is floor-wide, not per Order. Tokens are monotonically allocated even
// after release, restart and expiry. Owner is an immutable session identity.
type Lease struct {
	Owner   string    `json:"owner"`
	Fence   uint64    `json:"fence"`
	Expires time.Time `json:"expires"`
}
type Store struct{ Dir string }
type State struct {
	Version      string            `json:"version"`
	Lease        Lease             `json:"lease"`
	Cursor       uint64            `json:"handledCursor"`
	Sequence     uint64            `json:"sequence"`
	Handoff      string            `json:"handoff,omitempty"`
	Attention    []Attention       `json:"attention"`
	Observations map[string]string `json:"observations"`
}

func digest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func (s Store) locked(fn func(*State) error) error {
	if !filepath.IsAbs(s.Dir) {
		return errors.New("absolute owned chef journal directory required")
	}
	if err := os.MkdirAll(s.Dir, 0700); err != nil {
		return err
	}
	// Persist ancestry too: a synced journal cannot recover a lost parent entry.
	for p := filepath.Clean(s.Dir); ; p = filepath.Dir(p) {
		d, e := os.Open(p)
		if e != nil {
			return e
		}
		e = d.Sync()
		d.Close()
		if e != nil {
			return e
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	f, e := os.OpenFile(filepath.Join(s.Dir, "lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); e != nil {
		return e
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	st := State{Version: Version, Attention: []Attention{}, Observations: map[string]string{}}
	b, e := os.ReadFile(filepath.Join(s.Dir, "state.json"))
	if e == nil {
		if e = json.Unmarshal(b, &st); e != nil {
			return e
		}
		if st.Version != Version || st.Observations == nil {
			return errors.New("incompatible chef journal")
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		return e
	}
	return fn(&st)
}
func (s Store) commit(st *State) error {
	b, e := json.Marshal(st)
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(s.Dir, ".state-")
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
	if e = os.Rename(f.Name(), filepath.Join(s.Dir, "state.json")); e != nil {
		return e
	}
	d, e := os.Open(s.Dir)
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}
func check(st *State, l Lease) error {
	if l.Owner == "" || l.Fence == 0 || st.Lease.Owner != l.Owner || st.Lease.Fence != l.Fence || !time.Now().Before(st.Lease.Expires) {
		return ErrFence
	}
	return nil
}
func (s Store) Acquire(owner string, ttl time.Duration) (Lease, error) {
	var out Lease
	e := s.locked(func(st *State) error {
		if owner == "" || len(owner) > 128 || ttl <= 0 || ttl > 5*time.Minute {
			return errors.New("owner and lease duration (0,5m] required")
		}
		if st.Lease.Owner != "" && time.Now().Before(st.Lease.Expires) {
			return errors.New("chef already leased; renew with original token")
		}
		if st.Lease.Fence == math.MaxUint64 {
			return errors.New("fence exhausted")
		}
		out = Lease{owner, st.Lease.Fence + 1, time.Now().UTC().Add(ttl)}
		st.Lease = out
		return s.commit(st)
	})
	return out, e
}
func (s Store) Renew(l Lease, ttl time.Duration) (Lease, error) {
	var out Lease
	e := s.locked(func(st *State) error {
		if e := check(st, l); e != nil {
			return e
		}
		if ttl <= 0 || ttl > 5*time.Minute {
			return errors.New("invalid lease duration")
		}
		st.Lease.Expires = time.Now().UTC().Add(ttl)
		out = st.Lease
		return s.commit(st)
	})
	return out, e
}

// Handoff never erases attention, intents, uncertainty or the handled cursor.
func (s Store) Handoff(l Lease, checkpoint string) error {
	return s.locked(func(st *State) error {
		if e := check(st, l); e != nil {
			return e
		}
		if checkpoint == "" || len(checkpoint) > 8192 {
			return errors.New("bounded checkpoint required")
		}
		st.Handoff = checkpoint
		st.Lease.Owner = ""
		st.Lease.Expires = time.Time{}
		return s.commit(st)
	})
}
func (s Store) Status() (State, error) {
	var out State
	e := s.locked(func(st *State) error { out = *st; return nil })
	return out, e
}
func bounded(v any, n int) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	if len(b) > n {
		return fmt.Errorf("packet exceeds %d bytes", n)
	}
	return nil
}
