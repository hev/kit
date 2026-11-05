package dispatch

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hev/kit/internal/version"
)

// occupiedSlot is a run that holds a class slot on this host before its run
// directory exists.
type occupiedSlot struct{ Run, Class string }

// occupiedReceipts lists the receipt kinds that hold a class slot. A later
// receipt kind joins by appending its reader here.
var occupiedReceipts = []func() ([]occupiedSlot, error){classReservationSlots}

// admitClass is called under the root lock for every launch: start, retry,
// rework, amend restarts and source class reservations, from any caller. It
// counts this host's running, reserved and launch-unconfirmed runs of the
// class, plus occupied receipts whose run has not appeared yet. self is the
// run being admitted.
func admitClass(classes map[string]int, class, self string, live map[string]bool) error {
	limit, ok := classes[class]
	if class == "" || !ok {
		return nil
	}
	n, e := classOccupancy(class, self, live)
	if e != nil {
		return e
	}
	if n >= limit {
		return fmt.Errorf("class %s full (%d/%d)", class, n, limit)
	}
	return nil
}

// classOccupancy counts the slots of class held by runs other than self.
func classOccupancy(class, self string, live map[string]bool) (int, error) {
	n := 0
	seen := map[string]bool{self: true}
	dirs, e := os.ReadDir(Root())
	if e != nil && !os.IsNotExist(e) {
		return 0, e
	}
	for _, d := range dirs {
		name := d.Name()
		var m Meta
		if !d.IsDir() || seen[name] || readJSON(filepath.Join(Root(), name, "meta.json"), &m) != nil {
			continue
		}
		// A run directory replaces any receipt for it: its state decides.
		seen[name] = true
		if m.Forward != nil || m.Class != class {
			continue
		}
		if runOccupied(filepath.Join(Root(), name), live[name]) {
			n++
		}
	}
	for _, read := range occupiedReceipts {
		slots, e := read()
		if e != nil {
			return 0, fmt.Errorf("class %s: %w", class, e)
		}
		for _, s := range slots {
			if s.Class == class && !seen[s.Run] {
				seen[s.Run] = true
				n++
			}
		}
	}
	return n, nil
}

// runOccupied is true for running and reserved runs, and for launches whose
// supervisor has not yet confirmed within the launch grace.
func runOccupied(dir string, live bool) bool {
	if live {
		return true
	}
	var r Result
	if readJSON(filepath.Join(dir, "result.json"), &r) != nil {
		return false
	}
	reconcileLaunch(dir, &r)
	return r.LaunchUnconfirmed
}

// classReservation is the target's half of a source attempting receipt. The
// owner reserves before it writes the receipt, through the same admission
// RPC on any host, so the slot is admitted and held in one step under this
// root's lock. The run's start consumes it; a definitive start rejection
// releases it; otherwise it lapses with the attempt grace, when the owner's
// receipt stops being an attempt.
type classReservation struct {
	Run          string    `json:"run"`
	Class        string    `json:"class"`
	Orchestrator string    `json:"orchestrator,omitempty"`
	At           time.Time `json:"at"`
}

func classReservationDir() string { return filepath.Join(Root(), ".class-reservations") }

func classReservationPath(run string) string {
	return filepath.Join(classReservationDir(), run+".json")
}

// reserveClassSlot admits a source start's order into its class and records
// the reservation, under the root lock. Repeating it for the same run is
// idempotent: the run never counts against itself.
func reserveClassSlot(q Request) (Response, error) {
	r := Response{Version: version.Version}
	o, e := Parse(q.Order)
	if e != nil {
		return r, e
	}
	if o.Name != q.Name || !ValidName(q.Name) {
		return r, fmt.Errorf("order name mismatch")
	}
	if o.Class == "" {
		r.ClassReserved = true
		return r, nil
	}
	if e = os.MkdirAll(Root(), 0700); e != nil {
		return r, e
	}
	unlock, e := lockRoot()
	if e != nil {
		return r, e
	}
	defer unlock()
	if exists(filepath.Join(Root(), q.Name)) {
		return r, fmt.Errorf("%w: %s", errRunExists, q.Name)
	}
	live, e := liveNames()
	if e == nil {
		e = addReservations(live)
	}
	if e != nil {
		return r, e
	}
	policy, e := targetPolicy()
	if e != nil {
		return r, fmt.Errorf("target admission config: %w", e)
	}
	if e = admitClass(policy.Classes, o.Class, q.Name, live); e != nil {
		return r, e
	}
	if e = os.MkdirAll(classReservationDir(), 0700); e != nil {
		return r, e
	}
	if e = WriteJSON(classReservationPath(q.Name), classReservation{Run: q.Name, Class: o.Class, Orchestrator: q.Orchestrator, At: time.Now().UTC()}); e != nil {
		return r, e
	}
	r.ClassReserved = true
	return r, nil
}

// releaseRejectedClassSlot drops the reservation of a start this root
// definitively rejected before its run directory existed: a lifecycle, scope,
// budget, cap or class refusal, or a setup failure. An uncertain outcome keeps
// the slot, which a retry re-reserves under the same run name or which lapses
// with the attempt grace. Only the reserving orchestrator's start releases it.
func releaseRejectedClassSlot(run, orchestrator string, err error) {
	if err == nil || sourceUncertain(err) || !ValidName(run) || !exists(classReservationPath(run)) {
		return
	}
	unlock, e := lockRoot()
	if e != nil {
		return
	}
	defer unlock()
	var c classReservation
	if exists(filepath.Join(Root(), run)) || readJSON(classReservationPath(run), &c) != nil || c.Orchestrator != orchestrator {
		return
	}
	_ = os.Remove(classReservationPath(run))
}

// classReservationSlots reads this root's class reservations, under the root
// lock. Reservations whose run exists or whose grace has passed are removed.
func classReservationSlots() ([]occupiedSlot, error) {
	entries, e := os.ReadDir(classReservationDir())
	if e != nil {
		if os.IsNotExist(e) {
			return nil, nil
		}
		return nil, e
	}
	out := []occupiedSlot{}
	for _, d := range entries {
		run, ok := strings.CutSuffix(d.Name(), ".json")
		if !ok || !ValidName(run) {
			continue
		}
		var c classReservation
		if e := readJSON(filepath.Join(classReservationDir(), d.Name()), &c); e != nil {
			return nil, fmt.Errorf("class reservation %s: %w", run, e)
		}
		if exists(filepath.Join(Root(), run, "meta.json")) || time.Since(c.At) > sourceAttemptGrace {
			_ = os.Remove(filepath.Join(classReservationDir(), d.Name()))
			continue
		}
		out = append(out, occupiedSlot{Run: run, Class: c.Class})
	}
	return out, nil
}
