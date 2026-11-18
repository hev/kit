package dispatch

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// The daemon's automatic update defers while a dispatch run or an
// orchestrator round runs here (BusyWork); an idle orchestrator, a finished
// run and an empty host do not defer it.
func TestBusyWorkNamesRunningRunsAndRounds(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if busy, err := BusyWork(); err != nil || len(busy) != 0 {
		t.Fatalf("no dispatch root: %v %v", busy, err)
	}
	mk := func(name string) string {
		dir := filepath.Join(Root(), name)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	live := func(dir string) {
		if err := writeProcessRecord(filepath.Join(dir, "supervisor.json"), os.Getpid()); err != nil {
			t.Fatal(err)
		}
	}
	state := func(dir, s string) {
		if err := WriteJSON(filepath.Join(dir, "state.json"), OrchestratorState{State: s}); err != nil {
			t.Fatal(err)
		}
	}
	mk("finished-run") // no live supervisor
	live(mk("live-run"))
	idle := mk("orch-idle")
	live(idle)
	state(idle, "waiting")
	round := mk("orch-in-round")
	live(round)
	state(round, "running")
	stale := mk("orch-crashed")
	state(stale, "running") // the round's supervisor is gone
	// Dispatch runs named orch-NAME (they have meta.json) are runs: a live
	// supervisor or an admission reservation makes them busy, whatever any
	// orchestrator state says; a finished one does not.
	meta := func(dir string) {
		if err := WriteJSON(filepath.Join(dir, "meta.json"), Meta{Order: Order{Name: filepath.Base(dir), Host: "local"}}); err != nil {
			t.Fatal(err)
		}
	}
	orchRun := mk("orch-named-run")
	meta(orchRun)
	live(orchRun)
	admitting := mk("orch-named-admitting")
	meta(admitting)
	if err := writeProcessRecord(filepath.Join(admitting, "admission.json"), os.Getpid()); err != nil {
		t.Fatal(err)
	}
	orchDone := mk("orch-named-finished")
	meta(orchDone)
	state(orchDone, "running")
	busy, err := BusyWork()
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"live-run", "orch-in-round", "orch-named-admitting", "orch-named-run"}; !reflect.DeepEqual(busy, want) {
		t.Fatalf("busy = %v, want %v", busy, want)
	}
}
