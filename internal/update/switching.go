package update

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hev/kit/internal/platformops"
)

// An automatic update checks what defers it twice: before it downloads, and
// again in its switch window, right before it points the host's links at the
// new build. The switch window is an exclusive OS file lock (flock, or
// LockFileEx on Windows) on ~/.hev/update/switching, held from before that
// last check until the links are switched. Work that must not run across a
// switch (a dispatch run's admission, an orchestrator round, a hold)
// registers itself first and then takes and drops the same lock with
// AwaitSwitch. Both sides write before they read, so one of them always sees
// the other: either the update's last check sees the new work or hold and
// defers, or the work waits until the switch is done and starts on the new
// build.
//
// The lock belongs to the updater's process for as long as it lives: the
// operating system releases it when that process exits, so no amount of
// elapsed time makes a live updater's switch look abandoned. Waiting is
// bounded; when the bound passes, AwaitSwitch returns ErrSwitching and the
// caller must not start the work.

// switchWait bounds how long AwaitSwitch waits.
const switchWait = time.Minute

// ErrDeferred is returned when an update stopped before it switched because
// something deferred it while it was downloading.
var ErrDeferred = errors.New("update deferred")

// ErrSwitching is returned by AwaitSwitch when an automatic update was still
// switching this host's build after the wait: the work it guards must not
// start (admission fails and may be retried).
var ErrSwitching = errors.New("an automatic update is still switching this host's build")

type deferredError struct{ reason string }

func (e *deferredError) Error() string {
	return "deferred before switching: " + e.reason
}
func (e *deferredError) Unwrap() error { return ErrDeferred }

// DeferredReason returns why err deferred an update, or "".
func DeferredReason(err error) string {
	var d *deferredError
	if errors.As(err, &d) {
		return d.reason
	}
	return ""
}

// SwitchingPath is the switch-window lock file under home. The file stays;
// the lock on it marks the window.
func SwitchingPath(home string) string {
	return filepath.Join(home, ".hev", "update", "switching")
}

// lockSwitch takes the switch-window lock, waiting at most max; it returns
// ErrSwitching when the lock was still held at the deadline, and whether it
// had to wait.
func lockSwitch(home string, max, poll time.Duration) (unlock func(), waited bool, err error) {
	path := SwitchingPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, false, err
	}
	deadline := time.Now().Add(max)
	for {
		unlock, err := platformops.LockPath(path, true)
		if err == nil {
			return unlock, waited, nil
		}
		if !platformops.IsLockBusy(err) {
			return nil, waited, fmt.Errorf("switch lock %s: %w", path, err)
		}
		if !time.Now().Before(deadline) {
			return nil, true, fmt.Errorf("%w (waited %s for %s)", ErrSwitching, max, path)
		}
		waited = true
		time.Sleep(poll)
	}
}

// beginSwitch opens the switch window; the returned function ends it.
func (u *Updater) beginSwitch() (func(), error) {
	unlock, _, err := lockSwitch(u.Home, switchWait, 50*time.Millisecond)
	if err != nil {
		return nil, fmt.Errorf("open the switch window: %w", err)
	}
	done := false
	return func() {
		if !done {
			done = true
			unlock()
		}
	}, nil
}

// switchHook runs inside the switch window, after the lock is taken and
// before the last check; tests use it to land work in the window.
var switchHook func()

// AwaitSwitch waits while an automatic update on this home is in its switch
// window, for at most a minute. Call it after registering the work or hold
// the update must see. waited reports whether a switch was under way. A nil
// error means no switch can now begin without seeing what was registered;
// ErrSwitching means a switch was still under way at the deadline, and the
// caller must not start the work.
func AwaitSwitch(home string) (waited bool, err error) {
	return AwaitSwitchWithin(home, switchWait)
}

// AwaitSwitchWithin is AwaitSwitch with its own bound.
func AwaitSwitchWithin(home string, max time.Duration) (waited bool, err error) {
	return awaitSwitch(home, max, 50*time.Millisecond)
}

func awaitSwitch(home string, max, poll time.Duration) (bool, error) {
	unlock, waited, err := lockSwitch(home, max, poll)
	if err != nil {
		return waited, err
	}
	unlock()
	return waited, nil
}
