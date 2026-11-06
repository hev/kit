package businessflow

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
)

// ReservationCheck is an exact receiver-side binding, not a placement request.
type ReservationCheck struct {
	Workspace                                                      WorkspaceIdentity
	Order, Obligation, Candidate, Scope, Attempt, Intent, Executor string
	PolicyRevision                                                 uint64
}

// WithReservationGuard serializes a single external start with settlement and
// policy changes. The caller must already hold its chef/maintenance authority
// boundary. It does not invoke Gate or acquire any external authority lock.
// Starting is fsynced before callback; a crash or any error remains held, and
// retries never invoke callback again. Reconcile the original intent explicitly.
func (s Store) WithReservationGuard(ctx context.Context, id string, want ReservationCheck, effect func(context.Context) error) error {
	if err := mutationSupported(); err != nil {
		return err
	}
	if !filepath.IsAbs(s.Dir) || effect == nil {
		return errors.New("absolute directory and effect required")
	}
	f, err := openExistingLock(filepath.Join(s.Dir, "lock"))
	if err != nil {
		return err
	}
	defer f.Close()
	if err = lockFile(f); err != nil {
		return err
	}
	defer unlockFile(f)
	st, err := s.Snapshot(ctx)
	if err != nil {
		return err
	}
	r, ok := st.Reservations[id]
	e := st.Entries[want.Order]
	if r.Workspace != want.Workspace || e.Workspace != want.Workspace {
		return ErrHeld
	}
	if !ok || r.Status != "Reserved" || r.Order != want.Order || r.Obligation != want.Obligation || r.Candidate != want.Candidate || r.Attempt != want.Attempt || r.Intent != want.Intent || r.Executor != want.Executor || e.Obligation != want.Obligation || e.Scope != want.Scope || r.PolicyRevision != want.PolicyRevision || st.PolicyRevision != want.PolicyRevision {
		return ErrHeld
	}
	if r.Kind == "validation" && (e.Candidate != want.Candidate || e.Phase != "Validating") {
		return ErrHeld
	}
	if r.Kind == "execution" && (e.Phase != "Executing" && e.Phase != "Reworking") {
		return ErrHeld
	}
	if st.ResourceVersion > math.MaxUint64-2 || len(st.Receipts) >= MaxRecords-1 {
		return ErrHeld
	}
	record := func(status string) error {
		r.Status = status
		st.Reservations[id] = r
		st.ResourceVersion++
		b, a := counts(st)
		st.Receipts = append(st.Receipts, Receipt{Revision: st.ResourceVersion, Command: Command{Action: "receiver-" + status, Order: r.Order, Obligation: r.Obligation, Reservation: r}, BacklogCount: b, AcceptanceCount: a, CommitmentsPaused: st.CommitmentsPaused, StartsPaused: st.StartsPaused})
		return publish(ctx, s.Dir, st)
	}
	if err = record("Starting"); err != nil {
		return err
	}
	// Cancellation after the persisted start intent cannot erase the uncertain
	// boundary. A canceled context before callback leaves Starting for reconciliation.
	if err = ctx.Err(); err != nil {
		return err
	}
	effectErr := effect(ctx)
	// Record uncertainty even if callback canceled its context: the effect may exist.
	finalCtx := context.WithoutCancel(ctx)
	saved := ctx
	ctx = finalCtx
	status := "Started"
	if effectErr != nil || saved.Err() != nil {
		status = "Unknown"
	}
	persistErr := record(status)
	if persistErr != nil {
		return persistErr
	}
	if effectErr != nil {
		return effectErr
	}
	return saved.Err()
}
func publish(ctx context.Context, dir string, st Snapshot) error {
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	if len(data) > MaxBytes {
		return errors.New("ledger byte bound exceeded")
	}
	tmp, err := os.CreateTemp(dir, ".state-")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(data)
	}
	if err == nil {
		err = tmp.Sync()
	}
	closed := tmp.Close()
	if err == nil {
		err = closed
	}
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = os.Rename(name, filepath.Join(dir, "state.json")); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
