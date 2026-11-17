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
	busy, err := BusyWork()
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"live-run", "orch-in-round"}; !reflect.DeepEqual(busy, want) {
		t.Fatalf("busy = %v, want %v", busy, want)
	}
}
