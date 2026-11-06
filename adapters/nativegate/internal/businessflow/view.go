package businessflow

import (
	"context"
	"errors"
	"sort"
	"time"
)

// QueueView is a bounded projection of one immutable persisted ledger image.
// Positions are business ordering positions, never scheduling or readiness proof.
type QueueView struct {
	Version                         string
	ResourceVersion, PolicyRevision uint64
	Queue                           Queue
	Offset, Limit, Total            int
	NextOffset                      *int
	ObservedAt                      time.Time
	CommitmentsPaused, StartsPaused bool
	Rows                            []Row
	Summary                         QueueSummary
}
type Row struct {
	Position                                      int
	Entry                                         Entry
	EnteredAt                                     time.Time
	AgeSeconds                                    int64
	Eligibility, Reason, Evidence, Owner, Release string
	BypassRevision                                uint64
}

// View never calls Gate, initializes a store, or infers capacity from idle work.
// All potentially runnable rows remain Unknown without fresh external facts.
func (s Store) View(ctx context.Context, q Queue, offset, limit int) (QueueView, error) {
	if q != Backlog && q != Acceptance || offset < 0 || limit < 1 || limit > 100 {
		return QueueView{}, errors.New("invalid bounded queue query")
	}
	st, err := s.Snapshot(ctx)
	if err != nil {
		return QueueView{}, err
	}
	now := time.Now().UTC()
	entries := ordered(st, q)
	v := QueueView{Version: st.Version, ResourceVersion: st.ResourceVersion, PolicyRevision: st.PolicyRevision, Queue: q, Offset: offset, Limit: limit, Total: len(entries), ObservedAt: now, CommitmentsPaused: st.CommitmentsPaused, StartsPaused: st.StartsPaused, Rows: []Row{}}
	v.Summary, err = queueSummary(st)
	if err != nil {
		return QueueView{}, err
	}
	start := offset
	if start > len(entries) {
		start = len(entries)
	}
	end := start + limit
	if end > len(entries) {
		end = len(entries)
	}
	if end < len(entries) {
		n := end
		v.NextOffset = &n
	}
	for i := start; i < end; i++ {
		e := entries[i]
		entered := e.EnteredAt
		if q == Acceptance {
			entered = e.AcceptanceEnteredAt
		}
		row := Row{Position: i + 1, Entry: e, EnteredAt: entered, Eligibility: "Unknown", Reason: "fresh authority/capacity/dependency observations required", Owner: e.Owner, Release: "reconcile current admission and eligibility facts"}
		if !entered.IsZero() {
			row.AgeSeconds = int64(now.Sub(entered) / time.Second)
			if row.AgeSeconds < 0 {
				row.AgeSeconds = 0
			}
		}
		switch e.Phase {
		case "Reworking", "Validating", "Failed", "Passed", "CancellationHold":
			row.Eligibility = "Held"
			row.Reason = "current phase " + e.Phase + " is not ready for validation"
			row.Release = "settle current obligation phase with exact candidate evidence"
		}
		for j := len(st.Receipts) - 1; j >= 0; j-- {
			found := false
			for _, b := range st.Receipts[j].Command.Bypasses {
				if b.Order == e.Order {
					row.BypassRevision = st.Receipts[j].Revision
					row.Evidence = b.Evidence
					row.Owner = b.Owner
					row.Release = b.Release
					row.Reason = "historical recorded bypass: " + b.Reason
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		v.Rows = append(v.Rows, row)
	}
	return v, nil
}

// QueueSummary is charged authority accounting from the same revision as Rows.
// Unknown reservations remain charged; settled history does not consume capacity.
type QueueSummary struct {
	Policy                        Policy
	BacklogCount, AcceptanceCount int
	Execution, Validation         ReservationUsage
	Workspaces                    []WorkspaceUsage
}
type ReservationUsage struct {
	ChargedCount, ChargedUnits int
	States                     []ReservationStateUsage
}
type ReservationStateUsage struct {
	State                                    string
	Count, Units, ChargedCount, ChargedUnits int
}
type WorkspaceUsage struct {
	Workspace             WorkspaceIdentity
	Configured            bool
	Quota                 WorkspaceQuota
	Commitments           int
	Execution, Validation ReservationUsage
}

func (u *ReservationUsage) add(r Reservation) {
	state := r.Status
	switch state {
	case "Reserved", "Starting", "Started", "Unknown", "Settled":
	default:
		state = "Other"
	}
	if u.States == nil {
		u.States = []ReservationStateUsage{{State: "Reserved"}, {State: "Starting"}, {State: "Started"}, {State: "Unknown"}, {State: "Settled"}, {State: "Other"}}
	}
	for i := range u.States {
		v := &u.States[i]
		if v.State != state {
			continue
		}
		v.Count++
		v.Units += r.Units
		if r.Status != "Settled" {
			v.ChargedCount++
			v.ChargedUnits += r.Units
			u.ChargedCount++
			u.ChargedUnits += r.Units
		}
		return
	}
}
func queueSummary(st Snapshot) (QueueSummary, error) {
	b, a := counts(st)
	out := QueueSummary{Policy: st.Policy, BacklogCount: b, AcceptanceCount: a, Workspaces: []WorkspaceUsage{}}
	workspaces := map[WorkspaceIdentity]*WorkspaceUsage{}
	ensure := func(w WorkspaceIdentity) *WorkspaceUsage {
		v := workspaces[w]
		if v == nil {
			v = &WorkspaceUsage{Workspace: w}
			workspaces[w] = v
		}
		return v
	}
	for _, q := range st.Policy.WorkspaceQuotas {
		v := ensure(q.Workspace)
		v.Configured = true
		v.Quota = q
	}
	for _, e := range st.Entries {
		if outstanding(e) {
			ensure(e.Workspace).Commitments++
		}
	}
	for _, r := range st.Reservations {
		if r.Kind == "validation" {
			out.Validation.add(r)
		} else {
			out.Execution.add(r)
		}
		if r.Status != "Settled" {
			v := ensure(r.Workspace)
			if r.Kind == "validation" {
				v.Validation.add(r)
			} else {
				v.Execution.add(r)
			}
		}
	}
	if len(workspaces) > 256 {
		return QueueSummary{}, errors.New("workspace summary exceeds 256 identities")
	}
	for _, v := range workspaces {
		out.Workspaces = append(out.Workspaces, *v)
	}
	sort.Slice(out.Workspaces, func(i, j int) bool {
		a, b := out.Workspaces[i].Workspace, out.Workspaces[j].Workspace
		if a.UID != b.UID {
			return a.UID < b.UID
		}
		return a.Incarnation < b.Incarnation
	})
	return out, nil
}
