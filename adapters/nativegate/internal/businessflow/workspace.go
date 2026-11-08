package businessflow

// WorkspaceIdentity is enrolled client identity, not an access grant. Gate must
// verify executor-side access and current incarnation against original owners.
type WorkspaceIdentity struct{ UID, Incarnation string }

// WorkspaceQuota bounds outstanding obligations and charged reservation units.
// It neither selects work nor promises equal service; FIFO and visible bypasses
// remain the only selection contract. Unknown effects retain their charges.
type WorkspaceQuota struct {
	Workspace                          WorkspaceIdentity
	Commitments, Execution, Validation int
}

func validWorkspacePolicy(p Policy) bool {
	if len(p.WorkspaceQuotas) > 256 {
		return false
	}
	seen := map[string]bool{}
	for _, q := range p.WorkspaceQuotas {
		if !validWorkspace(q.Workspace) || seen[q.Workspace.UID] || q.Commitments < 1 || q.Commitments > MaxRecords || q.Execution < 1 || q.Execution > p.Execution || q.Validation < 1 || q.Validation > p.Validation {
			return false
		}
		seen[q.Workspace.UID] = true
	}
	return true
}
func validWorkspace(w WorkspaceIdentity) bool {
	return w.UID != "" && len(w.UID) <= 128 && w.Incarnation != "" && len(w.Incarnation) <= 128
}
func workspaceQuota(p Policy, w WorkspaceIdentity) (WorkspaceQuota, bool) {
	for _, q := range p.WorkspaceQuotas {
		if q.Workspace == w {
			return q, true
		}
	}
	return WorkspaceQuota{}, false
}
func outstanding(e Entry) bool { return e.Phase != "Delivered" && e.Phase != "Cancelled" }
func workspaceUsage(s Snapshot, w WorkspaceIdentity) (commitments, execution, validation int) {
	for _, e := range s.Entries {
		if e.Workspace == w && outstanding(e) {
			commitments++
		}
	}
	for _, r := range s.Reservations {
		if r.Workspace == w && r.Status != "Settled" {
			if r.Kind == "validation" {
				validation += r.Units
			} else {
				execution += r.Units
			}
		}
	}
	return
}
func checkWorkspacePolicy(s Snapshot, p Policy) error {
	// Never silently move an outstanding obligation into another incarnation or
	// remove its quota. An explicit settlement/delivery precedes retirement.
	for _, e := range s.Entries {
		if outstanding(e) {
			if e.Workspace == (WorkspaceIdentity{}) {
				if len(p.WorkspaceQuotas) > 0 {
					return ErrHeld
				}
				continue
			}
			q, ok := workspaceQuota(p, e.Workspace)
			if !ok {
				return ErrHeld
			}
			c, x, v := workspaceUsage(s, e.Workspace)
			if c > q.Commitments || x > q.Execution || v > q.Validation {
				return ErrHeld
			}
		}
	}
	return nil
}
func checkWorkspaceCommit(s Snapshot, w WorkspaceIdentity) error {
	if w == (WorkspaceIdentity{}) {
		if len(s.Policy.WorkspaceQuotas) == 0 {
			return nil
		}
		return ErrHeld
	}
	q, ok := workspaceQuota(s.Policy, w)
	if !ok {
		return ErrHeld
	}
	c, _, _ := workspaceUsage(s, w)
	if c >= q.Commitments {
		return ErrHeld
	}
	return nil
}
func checkWorkspaceReservation(s Snapshot, e Entry, r Reservation, kind string) error {
	if r.Workspace != e.Workspace {
		return ErrHeld
	}
	if e.Workspace == (WorkspaceIdentity{}) {
		if len(s.Policy.WorkspaceQuotas) == 0 {
			return nil
		}
		return ErrHeld
	}
	q, ok := workspaceQuota(s.Policy, e.Workspace)
	if !ok {
		return ErrHeld
	}
	_, x, v := workspaceUsage(s, e.Workspace)
	if kind == "validation" {
		if v+r.Units > q.Validation {
			return ErrHeld
		}
	} else if x+r.Units > q.Execution {
		return ErrHeld
	}
	return nil
}
