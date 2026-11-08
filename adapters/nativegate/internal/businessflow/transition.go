package businessflow

import (
	"fmt"
	"time"
)

func transition(s *Snapshot, c Command) error {
	e, exists := s.Entries[c.Order]
	if c.Action != "policy" && s.PolicyRevision == 0 {
		return ErrHeld
	}
	switch c.Action {
	case "policy":
		if !validPolicy(c.Policy) || c.Reason == "" {
			return fmt.Errorf("valid policy and reason required")
		}
		if err := checkWorkspacePolicy(*s, c.Policy); err != nil {
			return err
		}
		if len(c.Policy.WorkspaceQuotas) > 0 {
			s.Version = WorkspaceVersion
		}
		s.Policy = c.Policy
		s.PolicyRevision++
	case "commit":
		b, _ := counts(*s)
		if exists || s.CommitmentsPaused || b >= s.Policy.BacklogHard {
			return ErrHeld
		}
		if c.Order == "" || c.Obligation == "" || c.Scope == "" || c.Owner == "" {
			return fmt.Errorf("canonical order obligation scope owner required")
		}
		for _, v := range s.Entries {
			if v.Obligation == c.Obligation {
				return ErrConflict
			}
		}
		if err := checkWorkspaceCommit(*s, c.Workspace); err != nil {
			return err
		}
		s.Sequence++
		e = Entry{Workspace: c.Workspace, EnteredAt: time.Now().UTC(), Order: c.Order, Obligation: c.Obligation, Scope: c.Scope, Owner: c.Owner, Sequence: s.Sequence, Rank: s.Sequence, Priority: 50, Phase: "Backlog"}
	case "start", "rework", "validate":
		if !exists || c.Obligation != e.Obligation {
			return ErrConflict
		}
		q := Backlog
		if c.Action == "start" {
			if e.Phase != "Backlog" || s.StartsPaused {
				return ErrHeld
			}
		} else {
			q = Acceptance
			if e.AcceptanceSequence == 0 || e.Phase == "Delivered" {
				return ErrHeld
			}
			if c.Action == "validate" && (e.Phase != "Acceptance" || c.Candidate != e.Candidate) {
				return ErrHeld
			}
			if c.Action == "rework" && e.Phase != "Failed" {
				return ErrHeld
			}
		}
		if err := selection(*s, q, c); err != nil {
			return err
		}
		r := c.Reservation
		if r.ID == "" || r.Attempt == "" || r.Executor == "" || r.Intent == "" || r.Units < 1 || r.Units > MaxRecords {
			return fmt.Errorf("complete reservation required")
		}
		if _, ok := s.Reservations[r.ID]; ok {
			return ErrConflict
		}
		kind := "execution"
		bound := s.Policy.Execution
		if c.Action == "validate" {
			kind = "validation"
			bound = s.Policy.Validation
		}
		used := 0
		for _, v := range s.Reservations {
			if v.Order == c.Order && v.Status != "Settled" {
				return ErrHeld
			}
			if v.Kind == kind && v.Status != "Settled" {
				used += v.Units
			}
		}
		_, a := counts(*s)
		if c.Action == "start" && a > 0 {
			bound -= s.Policy.ReworkProtected
		}
		if used+r.Units > bound {
			return ErrHeld
		}
		if err := checkWorkspaceReservation(*s, e, r, kind); err != nil {
			return err
		}
		r.Order = e.Order
		r.Obligation = e.Obligation
		r.Candidate = c.Candidate
		r.Kind = kind
		r.Status = "Reserved"
		r.PolicyRevision = s.PolicyRevision
		s.Reservations[r.ID] = r
		if c.Action == "validate" {
			e.Phase = "Validating"
		} else if c.Action == "rework" {
			e.Phase = "Reworking"
		} else {
			e.Phase = "Executing"
		}
	case "cancel":
		if !exists || c.Obligation != e.Obligation || c.Reason == "" || c.Evidence == "" || e.Phase == "Delivered" || e.Phase == "Cancelled" {
			return ErrHeld
		}
		e.CancelReason = c.Reason
		if active(*s, c.Order) {
			e.Phase = "CancellationHold"
		} else {
			e.Phase = "Cancelled"
		}
	case "revise-scope":
		if !exists || c.Obligation != e.Obligation || c.Scope == "" || c.Scope == e.Scope || c.Reason == "" || c.Evidence == "" || e.Phase == "Delivered" || e.Phase == "Cancelled" || e.Phase == "CancellationHold" || active(*s, c.Order) {
			return ErrHeld
		}
		e.Scopes = append(e.Scopes, ScopeRevision{Scope: e.Scope, Reason: c.Reason, Evidence: c.Evidence, Revision: s.ResourceVersion + 1})
		e.Scope = c.Scope
		if e.AcceptanceSequence > 0 {
			e.Candidate = ""
			e.Phase = "Failed"
		}
	case "unknown", "settle":
		r, ok := s.Reservations[c.Reservation.ID]
		if !ok || r.Order != c.Order || r.Obligation != c.Obligation || r.Status == "Settled" {
			return ErrConflict
		}
		if c.Reason == "" || c.Evidence == "" {
			return fmt.Errorf("settlement evidence required")
		}
		if c.Action == "unknown" {
			r.Status = "Unknown"
		} else {
			r.Status = "Settled"
		}
		s.Reservations[r.ID] = r
	case "candidate":
		if !exists || c.Obligation != e.Obligation || c.Candidate == "" || c.Evidence == "" || e.Phase != "Executing" && e.Phase != "Reworking" {
			return ErrHeld
		}
		if active(*s, c.Order) {
			return ErrHeld
		}
		for _, v := range e.History {
			if v.ID == c.Candidate {
				return ErrConflict
			}
		}
		if e.AcceptanceSequence == 0 {
			s.Sequence++
			e.AcceptanceSequence = s.Sequence
			e.AcceptanceEnteredAt = time.Now().UTC()
		}
		e.Candidate = c.Candidate
		e.History = append(e.History, Candidate{ID: c.Candidate, Evidence: c.Evidence})
		e.Phase = "Acceptance"
	case "result":
		if !exists || e.Phase != "Validating" || e.Candidate != c.Candidate || e.Obligation != c.Obligation || c.Validator == "" || c.Validator == e.Owner || c.Evidence == "" || c.Result != "Passed" && c.Result != "Failed" || active(*s, c.Order) {
			return ErrHeld
		}
		i := len(e.History) - 1
		e.History[i].Result = c.Result
		e.History[i].Validator = c.Validator
		e.History[i].Evidence += "\n" + c.Evidence
		e.Phase = c.Result
	case "deliver":
		if !exists || e.Phase != "Passed" || e.Candidate != c.Candidate || e.Obligation != c.Obligation || c.Delivery == "" || c.Evidence == "" || active(*s, c.Order) {
			return ErrHeld
		}
		e.History[len(e.History)-1].Delivery = c.Delivery
		e.Phase = "Delivered"
	case "reorder":
		if !exists || c.Reason == "" || c.Priority < 0 || c.Priority > 100 || c.Rank == 0 || (e.Phase != "Backlog" && e.AcceptanceSequence == 0) || e.Phase == "Cancelled" || e.Phase == "Delivered" {
			return ErrHeld
		}
		e.Priority = c.Priority
		e.Rank = c.Rank
	default:
		return fmt.Errorf("unsupported business transition %q", c.Action)
	}
	if c.Action != "policy" {
		s.Entries[c.Order] = e
	}
	return nil
}
func active(s Snapshot, order string) bool {
	for _, r := range s.Reservations {
		if r.Order == order && r.Status != "Settled" {
			return true
		}
	}
	return false
}

// selection refuses undocumented skipping; Gate must substantiate every bypass's
// limiting fact and later-work independence against authoritative sources.
func selection(s Snapshot, q Queue, c Command) error {
	rows := ordered(s, q)
	i := 0
	for _, e := range rows {
		if e.Order == c.Order {
			if i != len(c.Bypasses) {
				return ErrConflict
			}
			return nil
		}
		if i >= len(c.Bypasses) {
			return ErrHeld
		}
		b := c.Bypasses[i]
		if b.Order != e.Order || b.Reason == "" || b.Evidence == "" || b.Owner == "" || b.Release == "" {
			return ErrHeld
		}
		i++
	}
	return ErrHeld
}
