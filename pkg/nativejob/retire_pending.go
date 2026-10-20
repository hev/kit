package nativejob

import (
	"errors"
	"strings"
)

// RetirePending terminalizes only an original pristine, never-claimed intent.
// It cannot execute, inspect processes, reconcile uncertainty, issue a fence or
// change the original request. Callers authenticate the retirement evidence and
// local enrollment before calling this SDK operation.
func (s *Store) RetirePending(id string, current Identity, in PendingRetirement) (Job, error) {
	if id == "" || strings.TrimSpace(in.ID) == "" || strings.TrimSpace(in.Evidence) == "" || strings.TrimSpace(in.Inspector) == "" {
		return Job{}, errors.New("pending retirement identity and evidence required")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback()
	// Acquire the same writer exclusion used by Execute, submission and issuance.
	if _, err = tx.Exec(`UPDATE metadata SET value=value WHERE key='identity'`); err != nil {
		return Job{}, err
	}
	j, err := load(tx, id)
	if err != nil {
		return Job{}, err
	}
	original := j.Request.Identity
	if current.ChefID != original.ChefID || current.Fence <= original.Fence {
		return Job{}, ErrFence
	}
	original.Fence = current.Fence
	if encode(original) != encode(current) || j.Request.validate(s.enrollment) != nil || j.ID != "job-"+Digest(j.Request.IntentKey)[7:] {
		return Job{}, ErrConflict
	}
	if err = s.check(tx, current); err != nil {
		return Job{}, err
	}
	// Any claim/start/inspection history is outside this operation, even if the
	// referenced process has exited. Reconcile's rules remain unchanged.
	if j.Owner != nil || j.Birth != nil || j.StartReceipt != nil || len(j.Inspections) != 0 {
		return Job{}, ErrConflict
	}
	result := Result{ExitCode: 125, Artifacts: []Artifact{}, Evidence: in.Evidence}
	if j.PendingRetirement != nil {
		if encode(*j.PendingRetirement) != encode(in) || j.State != "Completed" || j.Result == nil || encode(*j.Result) != encode(result) || j.ResultDigest != Digest(result) {
			return Job{}, ErrConflict
		}
		expected := "result-" + Digest(struct{ ID, Digest string }{j.ID, j.ResultDigest})[7:]
		if j.EventID != expected {
			return Job{}, ErrConflict
		}
		var body string
		var ack int
		if err = tx.QueryRow(`SELECT body,ack FROM outbox WHERE event_id=? AND job_id=?`, j.EventID, j.ID).Scan(&body, &ack); err != nil {
			return Job{}, err
		}
		event := Event{Version: Version, EventID: j.EventID, JobID: j.ID, Identity: j.Request.Identity, Result: result, ResultDigest: j.ResultDigest}
		if body != encode(event) || (ack != 0 && ack != 1) || (ack == 1) != j.Acknowledged {
			return Job{}, ErrConflict
		}
		if err = s.check(tx, current); err != nil {
			return Job{}, err
		}
		if err = tx.Commit(); err != nil {
			return Job{}, err
		}
		return j, nil
	}
	if j.State != "Pending" || j.Result != nil || j.ResultDigest != "" || j.EventID != "" || j.Acknowledged {
		return Job{}, ErrConflict
	}
	var events int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM outbox WHERE job_id=?`, j.ID).Scan(&events); err != nil {
		return Job{}, err
	}
	if events != 0 {
		return Job{}, ErrConflict
	}
	j.PendingRetirement = &in
	if err = finish(tx, &j, result); err != nil {
		return Job{}, err
	}
	if err = s.hit("pending-retirement-staged"); err != nil {
		return Job{}, err
	}
	if err = s.check(tx, current); err != nil {
		return Job{}, err
	}
	if err = tx.Commit(); err != nil {
		return Job{}, err
	}
	if err = s.hit("pending-retirement-committed"); err != nil {
		return Job{}, err
	}
	return j, nil
}
