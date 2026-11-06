// Package businessflow provides an opt-in durable business admission ledger.
// It never schedules tasks or advances execution DAGs.
package businessflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const Version = "factory-businessflow-v1alpha1"

// WorkspaceVersion prevents older readers from silently ignoring quota authority.
const WorkspaceVersion = "factory-businessflow-v2-workspace"
const MaxRecords = 10000
const MaxBytes = 8 << 20

var ErrConflict = errors.New("businessflow resourceVersion or idempotency conflict")
var ErrHeld = errors.New("businessflow admission held")

type Queue string

const (
	Backlog    Queue = "Backlog"
	Acceptance Queue = "Acceptance"
)

type Policy struct {
	Execution, Validation, ReworkProtected, BacklogHigh, BacklogLow, BacklogHard, AcceptanceHigh, AcceptanceLow int
	WorkspaceQuotas                                                                                             []WorkspaceQuota
}
type Entry struct {
	Workspace                          WorkspaceIdentity
	EnteredAt, AcceptanceEnteredAt     time.Time
	Scopes                             []ScopeRevision
	CancelReason                       string
	Order, Obligation, Owner, Scope    string
	Sequence, Rank, AcceptanceSequence uint64
	Priority                           int
	Phase                              string
	Candidate                          string
	History                            []Candidate
}
type ScopeRevision struct {
	Scope, Reason, Evidence string
	Revision                uint64
}
type Candidate struct{ ID, Evidence, Validator, Result, Delivery string }
type Reservation struct {
	Workspace                                                                 WorkspaceIdentity
	ID, Order, Obligation, Attempt, Candidate, Executor, Intent, Kind, Status string
	Units                                                                     int
	PolicyRevision                                                            uint64
}
type Bypass struct{ Order, Reason, Evidence, Owner, Release string }
type Receipt struct {
	Revision                        uint64
	Command                         Command
	BacklogCount, AcceptanceCount   int
	CommitmentsPaused, StartsPaused bool
}
type Snapshot struct {
	Version                                   string
	ResourceVersion, Sequence, PolicyRevision uint64
	Policy                                    Policy
	CommitmentsPaused, StartsPaused           bool
	Entries                                   map[string]Entry
	Reservations                              map[string]Reservation
	Receipts                                  []Receipt
	Keys                                      map[string]string
}

// Gate must check current external authority, lease/fence, scope, enrollment,
// capability/capacity freshness and independence of every recorded bypass.
// It is called while holding the ledger lock. Unknown facts must return ErrHeld.
type Gate interface {
	Check(context.Context, Command, Snapshot) error
}
type GateFunc func(context.Context, Command, Snapshot) error

func (f GateFunc) Check(c context.Context, q Command, s Snapshot) error { return f(c, q, s) }

type Store struct {
	Dir  string
	Gate Gate
}
type Command struct {
	Workspace                                                                                                                WorkspaceIdentity
	ExpectedVersion                                                                                                          uint64
	Key, Actor, Authority, Action, Order, Obligation, Owner, Scope, Candidate, Evidence, Validator, Result, Delivery, Reason string
	Policy                                                                                                                   Policy
	Reservation                                                                                                              Reservation
	Bypasses                                                                                                                 []Bypass
	Priority                                                                                                                 int
	Rank                                                                                                                     uint64
}

func initial() Snapshot {
	return Snapshot{Version: Version, Entries: map[string]Entry{}, Reservations: map[string]Reservation{}, Keys: map[string]string{}}
}
func validPolicy(p Policy) bool {
	return validWorkspacePolicy(p) && p.Execution <= MaxRecords && p.Validation <= MaxRecords && p.ReworkProtected <= MaxRecords && p.BacklogHard <= MaxRecords && p.AcceptanceHigh <= MaxRecords && p.Execution > 0 && p.Validation > 0 && p.ReworkProtected >= 0 && p.ReworkProtected <= p.Execution && p.BacklogLow >= 0 && p.BacklogLow < p.BacklogHigh && p.BacklogHigh <= p.BacklogHard && p.AcceptanceLow >= 0 && p.AcceptanceLow < p.AcceptanceHigh
}
func counts(s Snapshot) (b, a int) {
	for _, e := range s.Entries {
		if e.Phase == "Backlog" {
			b++
		}
		if e.AcceptanceSequence > 0 && e.Phase != "Delivered" && e.Phase != "Cancelled" {
			a++
		}
	}
	return
}
func pressure(s *Snapshot) {
	b, a := counts(*s)
	if b >= s.Policy.BacklogHigh {
		s.CommitmentsPaused = true
	} else if b <= s.Policy.BacklogLow {
		s.CommitmentsPaused = false
	}
	if a >= s.Policy.AcceptanceHigh {
		s.StartsPaused = true
	} else if a <= s.Policy.AcceptanceLow {
		s.StartsPaused = false
	}
}
func ordered(s Snapshot, q Queue) []Entry {
	r := []Entry{}
	for _, e := range s.Entries {
		if q == Backlog && e.Phase == "Backlog" || q == Acceptance && e.AcceptanceSequence > 0 && e.Phase != "Delivered" && e.Phase != "Cancelled" {
			r = append(r, e)
		}
	}
	sort.Slice(r, func(i, j int) bool {
		a, b := r[i], r[j]
		if a.Priority != b.Priority {
			return a.Priority > b.Priority
		}
		if a.Rank != b.Rank {
			return a.Rank < b.Rank
		}
		if a.Sequence != b.Sequence {
			return a.Sequence < b.Sequence
		}
		if a.AcceptanceSequence != b.AcceptanceSequence {
			return a.AcceptanceSequence < b.AcceptanceSequence
		}
		return a.Obligation < b.Obligation
	})
	return r
}
func (s Store) List(ctx context.Context, q Queue, offset, limit int) ([]Entry, uint64, error) {
	if q != Backlog && q != Acceptance || offset < 0 || limit < 1 || limit > 100 {
		return nil, 0, errors.New("invalid bounded queue query")
	}
	st, e := s.Snapshot(ctx)
	if e != nil {
		return nil, 0, e
	}
	r := ordered(st, q)
	if offset > len(r) {
		offset = len(r)
	}
	end := offset + limit
	if end > len(r) {
		end = len(r)
	}
	return r[offset:end], st.ResourceVersion, nil
}

// Snapshot reads one atomic persisted image without creating any directory or lock.
func (s Store) Snapshot(ctx context.Context) (Snapshot, error) {
	if e := ctx.Err(); e != nil {
		return Snapshot{}, e
	}
	if !filepath.IsAbs(s.Dir) {
		return Snapshot{}, errors.New("absolute owned directory required")
	}
	f, e := openSnapshot(filepath.Join(s.Dir, "state.json"))
	if e != nil {
		return Snapshot{}, e
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil {
		return Snapshot{}, e
	}
	if !info.Mode().IsRegular() {
		return Snapshot{}, errors.New("regular snapshot required")
	}
	b, e := io.ReadAll(io.LimitReader(f, MaxBytes+1))
	if e != nil {
		return Snapshot{}, e
	}
	if len(b) > MaxBytes {
		return Snapshot{}, errors.New("snapshot too large")
	}
	var st Snapshot
	if e = json.Unmarshal(b, &st); e != nil {
		return Snapshot{}, e
	}
	if (st.Version != Version && st.Version != WorkspaceVersion) || st.Entries == nil || st.Reservations == nil || st.Keys == nil {
		return Snapshot{}, errors.New("invalid snapshot")
	}
	return st, nil
}
func (s Store) Apply(ctx context.Context, c Command) (Snapshot, error) {
	if err := mutationSupported(); err != nil {
		return Snapshot{}, err
	}
	if !filepath.IsAbs(s.Dir) || s.Gate == nil {
		return Snapshot{}, errors.New("absolute owned directory and authority gate required")
	}
	if c.Key == "" || c.Actor == "" || c.Authority == "" {
		return Snapshot{}, errors.New("intent key actor authority required")
	}
	if e := os.MkdirAll(s.Dir, 0700); e != nil {
		return Snapshot{}, e
	}
	for parent := filepath.Clean(s.Dir); ; parent = filepath.Dir(parent) {
		d, err := os.Open(parent)
		if err != nil {
			return Snapshot{}, err
		}
		err = d.Sync()
		d.Close()
		if err != nil {
			return Snapshot{}, err
		}
		if filepath.Dir(parent) == parent {
			break
		}
	}
	f, e := os.OpenFile(filepath.Join(s.Dir, "lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return Snapshot{}, e
	}
	defer f.Close()
	if e = lockFile(f); e != nil {
		return Snapshot{}, e
	}
	defer unlockFile(f)
	st, e := s.Snapshot(ctx)
	if errors.Is(e, os.ErrNotExist) {
		st = initial()
	} else if e != nil {
		return Snapshot{}, e
	}
	payload, _ := json.Marshal(c)
	h := sha256.Sum256(payload)
	digest := hex.EncodeToString(h[:])
	if old, ok := st.Keys[c.Key]; ok {
		if old != digest {
			return Snapshot{}, ErrConflict
		}
		return st, nil
	}
	if c.ExpectedVersion != st.ResourceVersion {
		return Snapshot{}, ErrConflict
	}
	if st.ResourceVersion == math.MaxUint64 || st.Sequence > math.MaxUint64-2 || st.PolicyRevision == math.MaxUint64 {
		return Snapshot{}, ErrHeld
	}
	if len(st.Receipts) >= MaxRecords {
		return Snapshot{}, errors.New("ledger bounded; explicit archival required")
	}
	if e = s.Gate.Check(ctx, c, st); e != nil {
		return Snapshot{}, e
	}
	if e = ctx.Err(); e != nil {
		return Snapshot{}, e
	}
	if e = transition(&st, c); e != nil {
		return Snapshot{}, e
	}
	st.ResourceVersion++
	pressure(&st)
	b, a := counts(st)
	st.Receipts = append(st.Receipts, Receipt{st.ResourceVersion, c, b, a, st.CommitmentsPaused, st.StartsPaused})
	st.Keys[c.Key] = digest
	data, e := json.Marshal(st)
	if e != nil {
		return Snapshot{}, e
	}
	if len(data) > MaxBytes {
		return Snapshot{}, errors.New("ledger byte bound exceeded")
	}
	tmp, e := os.CreateTemp(s.Dir, ".state-")
	if e != nil {
		return Snapshot{}, e
	}
	name := tmp.Name()
	defer os.Remove(name)
	if e = tmp.Chmod(0600); e == nil {
		_, e = tmp.Write(data)
	}
	if e == nil {
		e = tmp.Sync()
	}
	closeErr := tmp.Close()
	if e == nil {
		e = closeErr
	}
	if e != nil {
		return Snapshot{}, e
	}
	if e = ctx.Err(); e != nil {
		return Snapshot{}, e
	}
	if e = os.Rename(name, filepath.Join(s.Dir, "state.json")); e != nil {
		return Snapshot{}, e
	}
	d, e := os.Open(s.Dir)
	if e != nil {
		return Snapshot{}, e
	}
	e = d.Sync()
	d.Close()
	return st, e
}
