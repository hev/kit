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

// deferRetry is how soon a deferred check is tried again (or the interval,
// when shorter): an update waits for running work, not for the next hour.
const deferRetry = 5 * time.Minute

// AutoResult is what one scheduled check did.
type AutoResult struct {
	// Ran is false when it was not time to check (or auto is off), and when
	// the check was deferred.
	Ran bool
	// Deferred says why a due check did not update, when it did not.
	Deferred string
	// Forced says why a check went ahead although something still deferred
	// it: it had been deferred for [update] max_defer.
	Forced string
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
	interval := u.Config.CheckInterval()
	forced := ""
	if d := u.Deferral(); d.Deferred() {
		reason := d.String()
		since := st.DeferredSince
		if since.IsZero() {
			since = now
		}
		if now.Sub(since) < u.Config.MaxDeferral() {
			return u.deferAuto(st, now, reason)
		}
		forced = fmt.Sprintf("deferred since %s, longer than [update] max_defer %s; updating anyway (%s)", since.UTC().Format(time.RFC3339), u.Config.MaxDeferral(), reason)
		u.record(&st, Event{Action: "check", From: u.Current, Channel: u.Config.Channel, Result: "max deferral reached", Detail: forced})
		if err := u.saveState(st); err != nil {
			return AutoResult{Ran: true, Forced: forced, Err: err}
		}
	}
	q := Request{Channel: u.Config.Channel, Self: true}
	if forced == "" && (st.DeferredSince.IsZero() || now.Sub(st.DeferredSince) < u.Config.MaxDeferral()) {
		// Work or a hold can arrive while the build downloads: check again
		// in the switch window, where admission and hold writers wait.
		u.guard = func() string {
			if d := u.Deferral(); d.Deferred() {
				return d.String()
			}
			return ""
		}
	}
	rep, err := u.Run(ctx, q)
	u.guard = nil
	if reason := DeferredReason(err); reason != "" {
		st, lerr := u.LoadState()
		if lerr != nil {
			return AutoResult{Ran: true, Deferred: reason, Report: rep, Err: lerr}
		}
		res := u.deferAuto(st, now, reason)
		res.Report = rep
		return res
	}
	if forced != "" {
		rep.Steps = append([]string{forced}, rep.Steps...)
	}
	st, lerr := u.LoadState()
	if lerr != nil {
		return AutoResult{Ran: true, Forced: forced, Report: rep, Err: lerr}
	}
	st.LastCheck = now
	st.DeferredSince, st.DeferReason = time.Time{}, ""
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
	return AutoResult{Ran: true, Forced: forced, Restart: err == nil && rep.Changed, Report: rep, Err: err}
}

// deferAuto records a deferred check and schedules the retry. It records the
// first deferral and each change of reason, not every retry.
func (u *Updater) deferAuto(st State, now time.Time, reason string) AutoResult {
	since := st.DeferredSince
	if since.IsZero() {
		since = now
	}
	if st.DeferredSince.IsZero() || st.DeferReason != reason {
		u.record(&st, Event{Action: "check", From: u.Current, Channel: u.Config.Channel, Result: "deferred", Detail: reason})
	}
	st.DeferredSince, st.DeferReason = since, reason
	st.NextCheck = now.Add(min(u.Config.CheckInterval(), deferRetry))
	if err := u.saveState(st); err != nil {
		return AutoResult{Ran: true, Deferred: reason, Err: err}
	}
	return AutoResult{Deferred: reason}
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
