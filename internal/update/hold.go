package update

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Hold stops the daemon's automatic updates on this host until it expires or
// is released: an operator's one-host-at-a-time rollout sets one so the
// daemon does not move hosts ahead of it. It lives in
// ~/.hev/update/hold.json. A manual hev update ignores it, with a warning;
// hev dispatch hosts update refuses a held host unless told to ignore it.
type Hold struct {
	// Until is when the hold lapses; zero holds until released (and still no
	// longer than [update] max_defer).
	Until  time.Time `json:"until,omitzero"`
	Reason string    `json:"reason,omitempty"`
	By     string    `json:"by,omitempty"`
	At     time.Time `json:"at"`
}

// Active reports whether the hold applies at now.
func (h *Hold) Active(now time.Time) bool {
	return h != nil && (h.Until.IsZero() || now.Before(h.Until))
}

// String says what the hold is, for the CLI, the daemon log and a dispatch reply.
func (h Hold) String() string {
	s := "held"
	if !h.Until.IsZero() {
		s += " until " + h.Until.UTC().Format(time.RFC3339)
	} else {
		s += " until released"
	}
	if h.By != "" {
		s += " by " + h.By
	}
	if h.Reason != "" {
		s += " (" + h.Reason + ")"
	}
	return s
}

func (u *Updater) holdPath() string { return filepath.Join(u.dir(), "hold.json") }

// LoadHold returns the host's hold when one applies now, or nil.
func (u *Updater) LoadHold() (*Hold, error) {
	raw, err := os.ReadFile(u.holdPath())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var h Hold
	if err := json.Unmarshal(raw, &h); err != nil {
		return nil, fmt.Errorf("read %s: %w", u.holdPath(), err)
	}
	if !h.Active(u.now()) {
		return nil, nil
	}
	return &h, nil
}

// MaxHoldReason bounds a hold's reason, which travels in dispatch replies.
const MaxHoldReason = 200

// SetHold holds automatic updates for d (zero: until released), replacing any
// hold already set.
func (u *Updater) SetHold(d time.Duration, reason string) (Hold, error) {
	if d < 0 {
		return Hold{}, fmt.Errorf("hold duration %s is negative", d)
	}
	reason = strings.Join(strings.Fields(reason), " ")
	if len(reason) > MaxHoldReason {
		return Hold{}, fmt.Errorf("hold reason is longer than %d characters", MaxHoldReason)
	}
	now := u.now()
	h := Hold{Reason: reason, By: u.By, At: now}
	if d > 0 {
		h.Until = now.Add(d)
	}
	raw, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return h, err
	}
	if err := writeAtomic(u.holdPath(), append(raw, '\n'), 0o600); err != nil {
		return h, err
	}
	// An automatic update in its switch window either saw this hold or is
	// already switching; wait for it so the caller learns which.
	if AwaitSwitch(u.Home) {
		u.logf("note: an automatic update was switching builds as the hold was set; run hev update status to see whether it moved this host")
	}
	u.recordEvent(Event{Action: "hold", Result: "set", Detail: h.String()})
	return h, nil
}

// ReleaseHold removes the host's hold; false when there was none.
func (u *Updater) ReleaseHold() (bool, error) {
	h, _ := u.LoadHold()
	err := os.Remove(u.holdPath())
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if h != nil {
		u.recordEvent(Event{Action: "hold", Result: "released", Detail: h.String()})
	}
	return h != nil, nil
}

// recordEvent appends one event to the history under the update lock. A
// failure to record does not undo what was done.
func (u *Updater) recordEvent(e Event) {
	unlock, err := u.lock()
	if err != nil {
		return
	}
	defer unlock()
	st, err := u.LoadState()
	if err != nil {
		return
	}
	u.record(&st, e)
	_ = u.saveState(st)
}

// Deferral is why the daemon would not update this host now.
type Deferral struct {
	Hold *Hold `json:"hold,omitempty"`
	// Busy names the dispatch runs and orchestrator rounds running here.
	Busy []string `json:"busy,omitempty"`
	// BusyError is set when running work could not be listed; the daemon
	// defers then too, since it cannot tell the host is idle.
	BusyError string `json:"busy_error,omitempty"`
}

// Deferred reports whether anything defers an automatic update.
func (d Deferral) Deferred() bool { return d.Hold != nil || len(d.Busy) > 0 || d.BusyError != "" }

func (d Deferral) String() string {
	var parts []string
	if d.Hold != nil {
		parts = append(parts, "update "+d.Hold.String())
	}
	if len(d.Busy) > 0 {
		parts = append(parts, "running here: "+strings.Join(d.Busy, ", "))
	}
	if d.BusyError != "" {
		parts = append(parts, "could not list running work: "+d.BusyError)
	}
	return strings.Join(parts, "; ")
}

// Deferral is what would defer an automatic update on this host now.
func (u *Updater) Deferral() Deferral {
	var d Deferral
	if h, err := u.LoadHold(); err != nil {
		// An unreadable hold file is treated as a hold: someone set one.
		d.Hold = &Hold{Reason: "unreadable hold file: " + err.Error()}
	} else {
		d.Hold = h
	}
	if u.Busy != nil {
		busy, err := u.Busy()
		d.Busy = busy
		if err != nil {
			d.BusyError = err.Error()
		}
	}
	return d
}
