package dispatch

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hev/kit/internal/platformops"
	"github.com/hev/kit/internal/update"
)

// Admission registers a run and then waits out an automatic update's switch
// window, so the update's last check either sees the run or the run starts
// on the new build. A switch still under way when the wait runs out fails
// admission instead of letting the work start.
func TestAdmissionWaitsOutAnUpdateSwitch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	lock := update.SwitchingPath(home)
	if err := os.MkdirAll(filepath.Dir(lock), 0o700); err != nil {
		t.Fatal(err)
	}
	unlock, err := platformops.LockPath(lock, false)
	if err != nil {
		t.Fatal(err)
	}
	go func(unlock func()) { time.Sleep(150 * time.Millisecond); unlock() }(unlock)
	start := time.Now()
	if err := awaitUpdateSwitch(); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) < 150*time.Millisecond {
		t.Fatal("admission did not wait for the switch to end")
	}
	start = time.Now()
	if err := awaitUpdateSwitch(); err != nil || time.Since(start) > time.Second {
		t.Fatalf("admission waited with no switch under way: %v", err)
	}

	old := updateSwitchWait
	updateSwitchWait = 100 * time.Millisecond
	t.Cleanup(func() { updateSwitchWait = old })
	unlock, err = platformops.LockPath(lock, false)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if err := awaitUpdateSwitch(); !errors.Is(err, update.ErrSwitching) {
		t.Fatalf("admission outlived a live switch: %v", err)
	}
}
