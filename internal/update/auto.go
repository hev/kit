package update

import (
	"context"
	"fmt"
	"time"
)

// maxBackoff bounds how far failed checks push the next one.
const maxBackoff = 24 * time.Hour

// maxStartAttempts is how many times a self-updated daemon may start without
// confirming before the next start rolls back.
const maxStartAttempts = 3

// AutoResult is what one scheduled check did.
type AutoResult struct {
	// Ran is false when it was not time to check (or auto is off).
	Ran bool
	// Restart asks the daemon to re-execute itself onto the new build.
	Restart bool
	Report  Report
	Err     error
}

// Auto is the daemon's scheduled check, called between its cycles, never
// during one. It does nothing unless the host's config turns automatic
// updates on with a channel chosen; otherwise, when the schedule says so, it
// updates by the same path as `hev update`, without restarting the daemon's
// own service: the caller re-executes. Failures back off exponentially.
func (u *Updater) Auto(ctx context.Context) AutoResult {
	if !u.Config.AutoEnabled() {
		return AutoResult{}
	}
	st, err := u.LoadState()
	if err != nil {
		return AutoResult{Ran: true, Err: err}
	}
	now := u.now()
	if !st.NextCheck.IsZero() && now.Before(st.NextCheck) {
		return AutoResult{}
	}
	u.By = "daemon"
	rep, err := u.Run(ctx, Request{Channel: u.Config.Channel, Self: true})
	st, lerr := u.LoadState()
	if lerr != nil {
		return AutoResult{Ran: true, Report: rep, Err: lerr}
	}
	st.LastCheck = now
	interval := u.Config.CheckInterval()
	if err != nil {
		st.Failures++
		st.LastError = err.Error()
		backoff := interval << min(st.Failures, 10)
		st.NextCheck = now.Add(min(backoff, maxBackoff))
		u.record(&st, Event{Action: "check", From: u.Current, Channel: u.Config.Channel, Result: "failed", Detail: err.Error()})
	} else {
		st.Failures, st.LastError = 0, ""
		st.NextCheck = now.Add(interval)
		if !rep.Changed {
			u.record(&st, Event{Action: "check", From: u.Current, To: rep.Latest, Channel: u.Config.Channel, Result: "current", Detail: rep.Message})
		}
	}
	if serr := u.saveState(st); serr != nil && err == nil {
		err = serr
	}
	return AutoResult{Ran: true, Restart: err == nil && rep.Changed, Report: rep, Err: err}
}

// StartCheck runs as the daemon starts. A daemon that switched itself and
// has since failed to start cleanly maxStartAttempts times switches back to
// the build it came from; rollback is true and the caller re-executes.
func (u *Updater) StartCheck(ctx context.Context) (rollback bool, err error) {
	if st, err := u.LoadState(); err != nil || st.Pending == nil {
		return false, err
	}
	unlock, err := u.lock()
	if err != nil {
		return false, err
	}
	st, err := u.LoadState()
	if err != nil || st.Pending == nil {
		unlock()
		return false, err
	}
	if st.Pending.To != st.Current {
		st.Pending = nil
		unlock()
		return false, u.saveState(st)
	}
	st.Pending.Attempts++
	if st.Pending.Attempts <= maxStartAttempts {
		unlock()
		return false, u.saveState(st)
	}
	from, to := st.Current, st.Pending.From
	u.record(&st, Event{Action: "rollback", From: from, To: to, Result: "started", Detail: fmt.Sprintf("the daemon did not start cleanly on %s after %d attempts", from, maxStartAttempts)})
	if err := u.saveState(st); err != nil {
		unlock()
		return false, err
	}
	unlock()
	u.By = "daemon"
	if _, err := u.Run(ctx, Request{Rollback: true, Self: true}); err != nil {
		return false, err
	}
	return true, nil
}

// Confirm records that a self-updated daemon completed a clean cycle.
func (u *Updater) Confirm() error {
	st, err := u.LoadState()
	if err != nil || st.Pending == nil {
		return err
	}
	unlock, err := u.lock()
	if err != nil {
		return err
	}
	defer unlock()
	st, err = u.LoadState()
	if err != nil || st.Pending == nil {
		return err
	}
	p := st.Pending
	st.Pending = nil
	u.record(&st, Event{Action: "confirm", From: p.From, To: p.To, Result: "ok", Detail: "the updated daemon completed a cycle"})
	return u.saveState(st)
}
