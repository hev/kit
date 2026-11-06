package maintenance

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"
)

// Resources are declared physical allocations, not business WIP or quotas.
// CPUMilli uses thousandths of one CPU; MemoryBytes uses bytes. Both are bounded.
type Resources struct{ CPUMilli, MemoryBytes uint64 }

// ParentEnrollment is immutable guest enrollment, never an instruction to create
// a VM/container/workspace or to move a process. WorkspaceRef is an opaque owned
// reference, not a path this package opens.
type ParentEnrollment struct {
	UID, Incarnation, WorkspaceRef string
	Allocation                     Resources
}

// Capacity is a trusted current-incarnation observation of the parent's total
// allocatable guest envelope, not free process utilization. Every enrolled direct
// child retains its allocation while cordoned, running, unknown, or idle.
type Capacity struct {
	Incarnation, Evidence string
	VerifiedAt            time.Time
	Known                 bool
	Allocatable           Resources
}

func validResources(r Resources) bool { return r.CPUMilli > 0 && r.MemoryBytes > 0 }
func freshCapacity(n Node) bool {
	c := n.Capacity
	return c != nil && c.Known && c.Incarnation == n.Incarnation && c.Evidence != "" && !c.VerifiedAt.IsZero() && !c.VerifiedAt.After(time.Now()) && time.Since(c.VerifiedAt) <= time.Minute && validResources(c.Allocatable)
}
func ancestry(v Snapshot, id string) ([]Node, error) {
	result := []Node{}
	seen := map[string]bool{}
	for {
		if seen[id] {
			return nil, fmt.Errorf("%w: topology cycle", ErrConflict)
		}
		seen[id] = true
		n, ok := v.Nodes[id]
		if !ok {
			return nil, fmt.Errorf("%w: parent not enrolled", ErrHeld)
		}
		result = append(result, n)
		if n.Parent == nil {
			return result, nil
		}
		p, ok := v.Nodes[n.Parent.UID]
		if !ok || p.Incarnation != n.Parent.Incarnation {
			return nil, fmt.Errorf("%w: parent enrollment/incarnation changed", ErrHeld)
		}
		id = p.UID
	}
}
func inSubtree(v Snapshot, id, root string) (bool, error) {
	seen := map[string]bool{}
	for {
		if id == root {
			return true, nil
		}
		if seen[id] {
			return false, fmt.Errorf("%w: topology cycle", ErrConflict)
		}
		seen[id] = true
		n, ok := v.Nodes[id]
		if !ok || n.Parent == nil {
			return false, nil
		}
		id = n.Parent.UID
	}
}

func allocationsFit(v Snapshot, parent Node, extra *Node) error {
	if !freshCapacity(parent) {
		return fmt.Errorf("%w: parent capacity Unknown or stale", ErrHeld)
	}
	remaining := parent.Capacity.Allocatable
	if parent.Parent != nil && (remaining.CPUMilli > parent.Parent.Allocation.CPUMilli || remaining.MemoryBytes > parent.Parent.Allocation.MemoryBytes) {
		return fmt.Errorf("%w: guest capacity exceeds parent allocation", ErrHeld)
	}
	take := func(n Node) error {
		if n.Parent == nil || n.Parent.UID != parent.UID {
			return nil
		}
		if n.Parent.Incarnation != parent.Incarnation || !validResources(n.Parent.Allocation) {
			return fmt.Errorf("%w: invalid child allocation/enrollment", ErrHeld)
		}
		a := n.Parent.Allocation
		if a.CPUMilli > remaining.CPUMilli || a.MemoryBytes > remaining.MemoryBytes {
			return fmt.Errorf("%w: parent capacity overallocated", ErrHeld)
		}
		remaining.CPUMilli -= a.CPUMilli
		remaining.MemoryBytes -= a.MemoryBytes
		return nil
	}
	for _, n := range v.Nodes {
		if e := take(n); e != nil {
			return e
		}
	}
	if extra != nil {
		return take(*extra)
	}
	return nil
}
func validateEnrollment(v *Snapshot, n Node) error {
	if n.Capacity != nil && n.Capacity.Incarnation != n.Incarnation {
		return ErrConflict
	}
	if n.Parent == nil {
		return nil
	}
	p := n.Parent
	if p.UID == "" || p.UID == n.UID || p.Incarnation == "" || p.WorkspaceRef == "" || !validResources(p.Allocation) {
		return ErrConflict
	}
	parent, ok := v.Nodes[p.UID]
	if !ok || parent.Incarnation != p.Incarnation {
		return fmt.Errorf("%w: missing/current parent enrollment required", ErrHeld)
	}
	if _, e := ancestry(*v, p.UID); e != nil {
		return e
	}
	for _, old := range v.Nodes {
		if old.Parent != nil && old.Parent.UID == p.UID && old.Parent.WorkspaceRef == p.WorkspaceRef {
			return fmt.Errorf("%w: workspace already enrolled", ErrConflict)
		}
	}
	return allocationsFit(*v, parent, &n)
}

// topologyEligible checks every ancestor at the final reservation/start boundary.
// Bare legacy nodes have no new topology capacity prerequisite.
func topologyEligible(v Snapshot, n Node, requireSelfActive bool) error {
	chain, e := ancestry(v, n.UID)
	if e != nil {
		return e
	}
	if requireSelfActive && (n.Cordoned || n.Phase != "Active") {
		return ErrHeld
	}
	for i := 1; i < len(chain); i++ {
		parent := chain[i]
		if parent.Cordoned || parent.Phase != "Active" || parent.AdmissionGeneration == math.MaxUint64 {
			return fmt.Errorf("%w: ancestor admission closed", ErrHeld)
		}
		if e := allocationsFit(v, parent, nil); e != nil {
			return e
		}
	}
	return nil
}
func affected(v Snapshot, root string) ([]string, error) {
	ids := []string{}
	pending := []string{root}
	seen := map[string]bool{}
	for len(pending) > 0 {
		id := pending[0]
		pending = pending[1:]
		if seen[id] {
			return nil, fmt.Errorf("%w: topology cycle", ErrConflict)
		}
		seen[id] = true
		ids = append(ids, id)
		for child, n := range v.Nodes {
			if n.Parent != nil && n.Parent.UID == id {
				pending = append(pending, child)
			}
		}
	}
	sort.Strings(ids)
	return ids, nil
}

func topologyBarrier(v *Snapshot, n *Node, drain bool, deadline time.Time) error {
	ids, e := affected(*v, n.UID)
	if e != nil {
		return e
	}
	// Preflight the entire subtree before changing any generation/effect state.
	for _, id := range ids {
		d := v.Nodes[id]
		if !d.Cordoned && d.AdmissionGeneration == math.MaxUint64 {
			return errors.New("maintenance admission generation exhausted")
		}
	}
	topology := len(ids) > 1 || n.Parent != nil
	for _, id := range ids {
		d := v.Nodes[id]
		if !d.Cordoned {
			d.AdmissionGeneration++
		}
		d.Cordoned = true
		d.Phase = "Cordoned"
		if topology {
			d.TopologyHealthAfter = time.Now()
		}
		if drain {
			d.Deadline = deadline
			d.Phase = "Draining"
		}
		v.Nodes[id] = d
	}
	for _, id := range ids {
		d := v.Nodes[id]
		if drain {
			reconcile(v, &d)
		} else if topology {
			d.Holds = topologyHolds(*v, d)
		}
		v.Nodes[id] = d
	}
	*n = v.Nodes[n.UID]
	return nil
}
func topologyHolds(v Snapshot, n Node) []Hold {
	holds := []Hold{}
	for id, r := range v.Reservations {
		yes, e := inSubtree(v, r.NodeUID, n.UID)
		if e != nil {
			holds = append(holds, Hold{"invalid topology", v.Owner.ID, "repair enrollment through reviewed protocol", "topology"})
			continue
		}
		if !yes || r.State == "Settled" || r.State == "Revoked" {
			continue
		}
		reason := "running descendant effect must settle"
		if r.State == "Unknown" || r.State == "Starting" {
			reason = "descendant effect outcome unknown"
		} else if r.State == "Reserved" {
			reason = "unstarted reservation fenced by topology barrier"
		} else if !r.Movable {
			reason = "nonmovable descendant must finish locally"
		}
		holds = append(holds, Hold{reason, v.Owner.ID, "authoritative settlement of " + id, r.Evidence})
	}
	if !n.Deadline.IsZero() && !time.Now().Before(n.Deadline) {
		holds = append(holds, Hold{"drain deadline expired", v.Owner.ID, "explicit reconciliation; timeout never proves settlement", "deadline:" + n.Deadline.UTC().Format(time.RFC3339Nano)})
	}
	sort.Slice(holds, func(i, j int) bool { return holds[i].ReleaseCondition < holds[j].ReleaseCondition })
	return holds
}
func refreshAncestors(v *Snapshot, id string) error {
	chain, e := ancestry(*v, id)
	if e != nil {
		return e
	}
	for _, n := range chain[1:] {
		if !n.Cordoned {
			continue
		}
		n.Holds = topologyHolds(*v, n)
		if n.Phase != "Cordoned" {
			if len(n.Holds) > 0 {
				n.Phase = "Held"
			} else {
				n.Phase = "Evacuated"
			}
		}
		v.Nodes[n.UID] = n
	}
	return nil
}

// ObserveCapacity records explicit trusted evidence; Unknown is valid observed
// state and closes affected guest starts. It never shrinks/reclaims allocations,
// frees uncertain effects, or changes any businessflow quota/WIP accounting.
func (s Store) ObserveCapacity(ctx context.Context, r Request, c Capacity) (Snapshot, error) {
	return s.mutate(ctx, "capacity", r, c, func(v *Snapshot, n *Node) error {
		if c.Incarnation != n.Incarnation {
			return ErrConflict
		}
		if c.Known && (c.Evidence == "" || !validResources(c.Allocatable) || c.VerifiedAt.IsZero() || c.VerifiedAt.After(time.Now())) {
			return ErrConflict
		}
		n.Capacity = &c
		return nil
	})
}

// CheckAdmission is a side-effect-free eligibility projection. Its result is not
// a detached start permit; Reserve and WithStartGuard repeat the same predicate.
func (s Store) CheckAdmission(ctx context.Context, uid, incarnation string) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	v, e := s.ReadSnapshot()
	if e != nil {
		return e
	}
	n, ok := v.Nodes[uid]
	if !ok || n.Incarnation != incarnation {
		return ErrConflict
	}
	return topologyEligible(v, n, true)
}
