// Package maintenance implements opt-in durable node admission barriers. It does
// not schedule work, move processes, or activate a shared cluster.
package maintenance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"
)

var (
	ErrConflict = errors.New("maintenance revision or identity conflict")
	ErrFenced   = errors.New("maintenance owner fenced or expired")
	ErrHeld     = errors.New("maintenance admission held")
	ErrNotFound = errors.New("maintenance state not found")
)

var unchanged = errors.New("unchanged")

const Version = "maintenance/v1"
const TopologyVersion = "maintenance/v2-topology"
const MaxSnapshotBytes = 8 << 20

type Owner struct {
	ID        string
	Fence     uint64
	ExpiresAt time.Time
}
type Hold struct{ Reason, Owner, ReleaseCondition, Evidence string }
type Node struct {
	UID, Incarnation, Name string
	AdmissionGeneration    uint64
	Cordoned               bool
	Phase                  string
	Deadline               time.Time
	Holds                  []Hold
	// Parent binds an optional enrolled guest to one immutable parent incarnation.
	Parent              *ParentEnrollment
	Capacity            *Capacity
	TopologyHealthAfter time.Time
}
type Reservation struct {
	ID, NodeUID, Incarnation, OrderUID, ObligationID, AttemptID, Candidate, Scope string
	AdmissionGeneration                                                           uint64
	State                                                                         string // Reserved, Starting, Running, Unknown, Settled, Revoked
	Movable                                                                       bool
	Evidence, CheckpointDigest                                                    string
}
type Operation struct {
	AffectedNodes     []string
	Key, Kind, Digest string
	Revision          uint64
}
type Snapshot struct {
	Version      string
	Revision     uint64
	Owner        Owner
	Nodes        map[string]Node
	Reservations map[string]Reservation
	Operations   map[string]Operation
}
type Store struct{ Dir string }
type Request struct {
	Key                          string
	ExpectedRevision             uint64
	Owner                        Owner
	NodeUID, Incarnation, Reason string
	Deadline                     time.Time
	Health                       *Health
	Observations                 []Observation
}
type Health struct {
	Incarnation, Evidence string
	VerifiedAt            time.Time
	Healthy               bool
}
type Observation struct{ ReservationID, State, Evidence, CheckpointDigest string }
type ReservationRequest struct {
	Request
	Reservation Reservation
}
type StartRequest struct {
	Request
	ReservationID string
}

func (s Store) path() (string, error) {
	if !filepath.IsAbs(s.Dir) || filepath.Clean(s.Dir) != s.Dir {
		return "", errors.New("explicit canonical absolute maintenance store required")
	}
	fi, e := os.Lstat(s.Dir)
	if e == nil && (!fi.IsDir() || fi.Mode()&os.ModeSymlink != 0) {
		return "", errors.New("maintenance store must be regular directory")
	}
	if e != nil && !os.IsNotExist(e) {
		return "", e
	}
	// Reject aliases in existing parents before creating a missing owned store.
	ancestor := s.Dir
	for {
		_, err := os.Lstat(ancestor)
		if err == nil {
			break
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return "", err
		}
		ancestor = parent
	}
	resolved, err := filepath.EvalSymlinks(ancestor)
	if err != nil {
		return "", err
	}
	if resolved != ancestor {
		return "", errors.New("maintenance directory aliases forbidden")
	}

	return filepath.Join(s.Dir, "state.json"), nil
}
func (s Store) ReadSnapshot() (Snapshot, error) {
	p, e := s.path()
	if e != nil {
		return Snapshot{}, e
	}
	info, e := os.Lstat(p)
	if os.IsNotExist(e) {
		return Snapshot{}, ErrNotFound
	}
	if e != nil {
		return Snapshot{}, e
	}
	if !info.Mode().IsRegular() {
		return Snapshot{}, errors.New("maintenance snapshot must be regular file")
	}
	f, e := openRegularSnapshot(p)
	if os.IsNotExist(e) {
		return Snapshot{}, ErrNotFound
	}
	if e != nil {
		return Snapshot{}, e
	}
	defer f.Close()
	fi, e := f.Stat()
	if e != nil {
		return Snapshot{}, e
	}
	if !fi.Mode().IsRegular() || fi.Size() > MaxSnapshotBytes {
		return Snapshot{}, errors.New("invalid maintenance snapshot size/type")
	}
	b, e := io.ReadAll(io.LimitReader(f, MaxSnapshotBytes+1))
	if e != nil {
		return Snapshot{}, e
	}
	if len(b) > MaxSnapshotBytes {
		return Snapshot{}, errors.New("maintenance snapshot limit")
	}
	var v Snapshot
	d := json.NewDecoder(bytes.NewReader(b))
	if e = d.Decode(&v); e != nil {
		return v, e
	}
	var extra any
	if e = d.Decode(&extra); e != io.EOF {
		return v, errors.New("trailing maintenance snapshot data")
	}
	if (v.Version != Version && v.Version != TopologyVersion) || v.Nodes == nil || v.Reservations == nil || v.Operations == nil {
		return v, errors.New("invalid maintenance snapshot")
	}
	if v.Version == Version {
		for _, n := range v.Nodes {
			if n.Parent != nil {
				return v, errors.New("topology requires maintenance/v2-topology snapshot")
			}
		}
	}
	return v, nil
}
func (s Store) write(v Snapshot) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	if len(b) > MaxSnapshotBytes {
		return errors.New("maintenance snapshot limit")
	}
	f, e := os.CreateTemp(s.Dir, ".snapshot-")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	if e = os.Rename(name, filepath.Join(s.Dir, "state.json")); e != nil {
		return e
	}
	dir, e := os.Open(s.Dir)
	if e != nil {
		return e
	}
	defer dir.Close()
	return dir.Sync()
}
func (s Store) update(ctx context.Context, fn func(*Snapshot) error) (Snapshot, error) {
	if _, e := s.path(); e != nil {
		return Snapshot{}, e
	}
	if e := ctx.Err(); e != nil {
		return Snapshot{}, e
	}
	if e := os.MkdirAll(s.Dir, 0700); e != nil {
		return Snapshot{}, e
	}
	// Persist a freshly initialized authority's directory ancestry too.
	for parent := s.Dir; ; parent = filepath.Dir(parent) {
		d, e := os.Open(parent)
		if e != nil {
			return Snapshot{}, e
		}
		e = d.Sync()
		d.Close()
		if e != nil {
			return Snapshot{}, e
		}
		if filepath.Dir(parent) == parent {
			break
		}
	}

	unlock, e := lockStore(ctx, filepath.Join(s.Dir, "lock"))
	if e != nil {
		return Snapshot{}, e
	}
	defer unlock()
	v, e := s.ReadSnapshot()
	if errors.Is(e, ErrNotFound) {
		v = Snapshot{Version: Version, Nodes: map[string]Node{}, Reservations: map[string]Reservation{}, Operations: map[string]Operation{}}
	} else if e != nil {
		return v, e
	}
	if v.Revision == math.MaxUint64 {
		return v, errors.New("maintenance revision exhausted")
	}
	if e = fn(&v); e != nil {
		if errors.Is(e, unchanged) {
			return v, nil
		}
		return v, e
	}
	v.Revision++
	if e = s.write(v); e != nil {
		return v, e
	}
	return v, nil
}
func check(v *Snapshot, r Request) error {
	if r.ExpectedRevision != v.Revision {
		return ErrConflict
	}
	if r.Owner.ID == "" || r.Owner.ID != v.Owner.ID || r.Owner.Fence != v.Owner.Fence || !time.Now().Before(v.Owner.ExpiresAt) {
		return ErrFenced
	}
	return nil
}
func (s Store) Claim(ctx context.Context, rev uint64, o Owner) (Snapshot, error) {
	return s.update(ctx, func(v *Snapshot) error {
		if rev != v.Revision {
			return ErrConflict
		}
		if o.ID == "" || !time.Now().Before(o.ExpiresAt) {
			return ErrFenced
		}
		if o.Fence <= v.Owner.Fence {
			return ErrFenced
		}
		if time.Now().Before(v.Owner.ExpiresAt) {
			return ErrHeld
		}
		v.Owner = o
		return nil
	})
}
func (s Store) Enroll(ctx context.Context, r Request, n Node) (Snapshot, error) {
	return s.update(ctx, func(v *Snapshot) error {
		if e := check(v, r); e != nil {
			return e
		}
		if n.UID == "" || n.Incarnation == "" {
			return ErrConflict
		}
		if old, ok := v.Nodes[n.UID]; ok {
			if old.Incarnation != n.Incarnation {
				return fmt.Errorf("%w: incarnation replacement requires settlement protocol", ErrHeld)
			}
			return ErrConflict
		}
		if e := validateEnrollment(v, n); e != nil {
			return e
		}
		n.Cordoned = true
		n.Phase = "Cordoned"
		n.AdmissionGeneration = 1
		if n.Parent != nil {
			if r.NodeUID != n.UID || r.Incarnation != n.Incarnation {
				return ErrConflict
			}
			v.Version = TopologyVersion
			n.TopologyHealthAfter = time.Now()
		}
		v.Nodes[n.UID] = n
		return nil
	})
}

// mutate deduplicates immutable operation payloads before CAS. A retry must use
// the original key and payload; it never invents a second operation.
func (s Store) mutate(ctx context.Context, kind string, r Request, payload any, fn func(*Snapshot, *Node) error) (Snapshot, error) {
	b, e := json.Marshal(struct {
		Kind, Node, Incarnation, Reason string
		Deadline                        time.Time
		Payload                         any
	}{kind, r.NodeUID, r.Incarnation, r.Reason, r.Deadline, payload})
	if e != nil {
		return Snapshot{}, e
	}
	digest := string(b)
	return s.update(ctx, func(v *Snapshot) error {
		if r.Key == "" {
			return ErrConflict
		}
		if op, ok := v.Operations[r.Key]; ok {
			if op.Kind != kind || op.Digest != digest {
				return ErrConflict
			}
			return unchanged
		}
		if e := check(v, r); e != nil {
			return e
		}
		n, ok := v.Nodes[r.NodeUID]
		if !ok || n.Incarnation != r.Incarnation {
			return ErrConflict
		}
		before := map[string]string{}
		for id, node := range v.Nodes {
			image, _ := json.Marshal(node)
			before[id] = string(image)
		}
		if e := fn(v, &n); e != nil {
			return e
		}
		v.Nodes[n.UID] = n
		if e := refreshAncestors(v, n.UID); e != nil {
			return e
		}
		affected := []string{}
		for id, node := range v.Nodes {
			image, _ := json.Marshal(node)
			if before[id] != string(image) {
				affected = append(affected, id)
			}
		}
		sort.Strings(affected)
		v.Operations[r.Key] = Operation{Key: r.Key, Kind: kind, Digest: digest, Revision: v.Revision + 1, AffectedNodes: affected}
		return nil
	})
}
func (s Store) Cordon(ctx context.Context, r Request) (Snapshot, error) {
	return s.mutate(ctx, "cordon", r, nil, func(v *Snapshot, n *Node) error {
		return topologyBarrier(v, n, false, time.Time{})
	})
}
func (s Store) Drain(ctx context.Context, r Request) (Snapshot, error) {
	return s.mutate(ctx, "drain", r, nil, func(v *Snapshot, n *Node) error {
		if r.Deadline.IsZero() {
			return errors.New("bounded drain deadline required")
		}

		return topologyBarrier(v, n, true, r.Deadline)
	})
}
func reconcile(v *Snapshot, n *Node) {
	n.Holds = nil
	for id, res := range v.Reservations {
		descendant, e := inSubtree(*v, res.NodeUID, n.UID)
		if e != nil {
			n.Holds = append(n.Holds, Hold{"invalid topology", v.Owner.ID, "repair enrollment through reviewed protocol", "topology"})
			continue
		}
		if !descendant {
			continue
		}
		switch res.State {
		case "Reserved":
			res.State = "Revoked"
			res.Evidence = "admission barrier: unstarted reservation revoked"
			v.Reservations[id] = res
		case "Settled", "Revoked":
		default:
			reason := "running effect must settle"
			if res.State == "Unknown" || res.State == "Starting" {
				reason = "effect outcome unknown"
			} else if !res.Movable {
				reason = "nonmovable effect must finish locally"
			}
			n.Holds = append(n.Holds, Hold{reason, v.Owner.ID, "authoritative settlement of " + id, res.Evidence})
		}
	}
	if !n.Deadline.IsZero() && !time.Now().Before(n.Deadline) {
		n.Holds = append(n.Holds, Hold{"drain deadline expired", v.Owner.ID, "explicit reconciliation; timeout never proves settlement", "deadline:" + n.Deadline.UTC().Format(time.RFC3339Nano)})
	}
	sort.Slice(n.Holds, func(i, j int) bool { return n.Holds[i].ReleaseCondition < n.Holds[j].ReleaseCondition })
	if len(n.Holds) > 0 {
		n.Phase = "Held"
	} else {
		n.Phase = "Evacuated"
	}
}

// Reconcile accepts authoritative effect observations. Transport failure must be
// reported as Unknown; it must never be translated into Settled. Evidence is a
// reference to an owned receiver receipt, not proof inferred by this package.
func (s Store) Reconcile(ctx context.Context, r Request) (Snapshot, error) {
	return s.mutate(ctx, "reconcile", r, r.Observations, func(v *Snapshot, n *Node) error {
		if !n.Cordoned && len(r.Observations) == 0 {
			return ErrHeld
		}
		for _, o := range r.Observations {
			res, ok := v.Reservations[o.ReservationID]
			if !ok || res.NodeUID != n.UID || res.Incarnation != n.Incarnation || o.Evidence == "" {
				return ErrConflict
			}
			if res.State == "Settled" || res.State == "Revoked" {
				if o.State != res.State {
					return ErrConflict
				}
				continue
			}
			if o.State != "Unknown" && o.State != "Running" && o.State != "Settled" {
				return ErrConflict
			}
			if res.State == "Reserved" && o.State != "Settled" {
				return ErrConflict
			}
			res.State = o.State
			res.Evidence = o.Evidence
			res.CheckpointDigest = o.CheckpointDigest
			v.Reservations[res.ID] = res
		}
		if n.Cordoned {
			reconcile(v, n)
		}
		return nil
	})
}
func (s Store) Uncordon(ctx context.Context, r Request) (Snapshot, error) {
	return s.mutate(ctx, "uncordon", r, r.Health, func(v *Snapshot, n *Node) error {
		if e := topologyEligible(*v, *n, false); e != nil {
			return e
		}
		h := r.Health
		if h == nil || !h.Healthy || h.Incarnation != n.Incarnation || h.Evidence == "" || h.VerifiedAt.IsZero() || h.VerifiedAt.After(time.Now()) || time.Since(h.VerifiedAt) > time.Minute || h.VerifiedAt.Before(n.TopologyHealthAfter) {
			return ErrHeld
		}
		for _, res := range v.Reservations {
			if res.NodeUID == n.UID && res.State != "Settled" && res.State != "Revoked" {
				return ErrHeld
			}
		}
		if len(n.Holds) > 0 {
			return ErrHeld
		}
		if n.AdmissionGeneration == math.MaxUint64 {
			return errors.New("maintenance admission generation exhausted")
		}
		n.AdmissionGeneration++
		n.Cordoned = false
		n.Phase = "Active"
		n.Deadline = time.Time{}
		return nil
	})
}
func (s Store) Reserve(ctx context.Context, r ReservationRequest) (Snapshot, error) {
	return s.mutate(ctx, "reserve", r.Request, r.Reservation, func(v *Snapshot, n *Node) error {
		if e := topologyEligible(*v, *n, true); e != nil {
			return e
		}
		x := r.Reservation
		if n.AdmissionGeneration == math.MaxUint64 {
			return ErrHeld
		}
		if n.Cordoned || n.Phase != "Active" {
			return ErrHeld
		}
		if x.ID == "" || x.NodeUID != n.UID || x.Incarnation != n.Incarnation || x.OrderUID == "" || x.ObligationID == "" || x.AttemptID == "" || x.Candidate == "" || x.Scope == "" || x.AdmissionGeneration != n.AdmissionGeneration {
			return ErrConflict
		}
		if _, ok := v.Reservations[x.ID]; ok {
			return ErrConflict
		}
		x.State = "Reserved"
		v.Reservations[x.ID] = x
		return nil
	})
}

// WithStartGuard is the final opt-in receiver boundary. The callback must perform
// only the bound native submission, with its own immutable attempt deduplication;
// it must not call back into this store. Starting is fsynced before invocation.
// A crash or callback error holds the same reservation as uncertain. Cordon and
// starts serialize across processes. No scheduler or automatic retry is provided.
func (s Store) WithStartGuard(ctx context.Context, r StartRequest, start func(context.Context) error) (Snapshot, error) {
	var effectErr error
	v, e := s.mutate(ctx, "start", r.Request, r.ReservationID, func(v *Snapshot, n *Node) error {
		if e := topologyEligible(*v, *n, true); e != nil {
			return e
		}
		x, ok := v.Reservations[r.ReservationID]
		if !ok || x.NodeUID != n.UID || x.Incarnation != n.Incarnation || x.State != "Reserved" || x.AdmissionGeneration != n.AdmissionGeneration || n.Cordoned || n.AdmissionGeneration == math.MaxUint64 {
			return ErrHeld
		}
		if start == nil {
			return errors.New("start callback required")
		}
		x.State = "Starting"
		v.Reservations[x.ID] = x
		// This intermediate publication intentionally retains the revision: the writer
		// lock excludes mutations; a restart sees Starting and cannot replay the effect.
		if e := s.write(*v); e != nil {
			return e
		}
		if e := check(v, r.Request); e != nil {
			return e
		}
		if e := ctx.Err(); e != nil {
			return e
		}
		effectErr = start(ctx)
		// A slow callback may cross cancellation/lease expiry. Preserve its
		// possible effect, but never report that boundary as a known success.
		if effectErr == nil {
			effectErr = ctx.Err()
		}
		if effectErr == nil {
			effectErr = check(v, r.Request)
		}
		if effectErr != nil {
			x.State = "Unknown"
			x.Evidence = "start callback returned an ambiguous error"
		} else {
			x.State = "Running"
		}
		v.Reservations[x.ID] = x
		return nil
	})
	if e != nil {
		return v, e
	}
	return v, effectErr
}

// Renew retains the monotonic fence, extending only the current live owner's
// lease. An expired owner must obtain a new Claim and fence instead.
func (s Store) Renew(ctx context.Context, rev uint64, o Owner) (Snapshot, error) {
	return s.update(ctx, func(v *Snapshot) error {
		if e := check(v, Request{ExpectedRevision: rev, Owner: o}); e != nil {
			return e
		}
		if !o.ExpiresAt.After(v.Owner.ExpiresAt) {
			return ErrConflict
		}
		v.Owner.ExpiresAt = o.ExpiresAt
		return nil
	})
}
