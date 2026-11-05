package dispatch

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// occupiedSlot is a run that holds a class slot on this host before its run
// directory exists.
type occupiedSlot struct{ Run, Class string }

// occupiedReceipts lists the receipt kinds that hold a class slot. A later
// receipt kind joins by appending its reader here.
var occupiedReceipts = []func() ([]occupiedSlot, error){sourceAttemptingSlots}

// admitClass is called under the root lock for every launch: start, retry,
// rework and amend restarts, from any caller. It counts this host's running,
// reserved and launch-unconfirmed runs of the class, plus occupied receipts
// whose run has not appeared yet. self is the run being admitted.
func admitClass(classes map[string]int, class, self string, live map[string]bool) error {
	limit, ok := classes[class]
	if class == "" || !ok {
		return nil
	}
	n := 0
	seen := map[string]bool{self: true}
	dirs, e := os.ReadDir(Root())
	if e != nil && !os.IsNotExist(e) {
		return e
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
			return fmt.Errorf("class %s: %w", class, e)
		}
		for _, s := range slots {
			if s.Class == class && !seen[s.Run] {
				seen[s.Run] = true
				n++
			}
		}
	}
	if n >= limit {
		return fmt.Errorf("class %s full (%d/%d)", class, n, limit)
	}
	return nil
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

// sourceAttemptingSlots reads attempting source receipts held in this host's
// dispatch root that target this host: a local host entry or this machine's
// own entry. A receipt whose host is unknown here is counted, not ignored.
func sourceAttemptingSlots() ([]occupiedSlot, error) {
	orchs, e := os.ReadDir(Root())
	if e != nil {
		if os.IsNotExist(e) {
			return nil, nil
		}
		return nil, e
	}
	var cfg Config
	var self string
	loaded := false
	out := []occupiedSlot{}
	for _, d := range orchs {
		name, ok := strings.CutPrefix(d.Name(), "orch-")
		if !ok || !d.IsDir() {
			continue
		}
		receipts, e := loadSourceReceipts(OrchestratorSpec{Name: name})
		if e != nil {
			return nil, e
		}
		for _, r := range receipts {
			if r.Mode != "attempting" || r.Class == "" || r.Run == "" {
				continue
			}
			if !loaded {
				cfg, _ = Load(ConfigPath(""))
				hostname, _ := os.Hostname()
				self = legacyTargetHost(cfg.Hosts, hostname)
				loaded = true
			}
			if h, ok := cfg.Hosts[r.Host]; ok && !h.Local && r.Host != self {
				continue
			}
			out = append(out, occupiedSlot{Run: r.Run, Class: r.Class})
		}
	}
	return out, nil
}
