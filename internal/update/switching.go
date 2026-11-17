package update

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// An automatic update checks what defers it twice: before it downloads, and
// again in its switch window, right before it points the host's links at the
// new build. The switch window is marked by ~/.hev/update/switching. Work
// that must not run across a switch (a dispatch run's admission, an
// orchestrator round, a hold) registers itself first and then waits out the
// marker with AwaitSwitch. Both sides write before they read, so one of them
// always sees the other: either the update's last check sees the new work or
// hold and defers, or the work waits until the switch is done and starts on
// the new build.

// switchStale bounds how long a marker is honoured. A switch takes moments;
// an older marker was left by an updater that died.
const switchStale = 2 * time.Minute

// switchWait bounds how long AwaitSwitch waits.
const switchWait = time.Minute

// ErrDeferred is returned when an update stopped before it switched because
// something deferred it while it was downloading.
var ErrDeferred = errors.New("update deferred")

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

// SwitchingPath is the switch-window marker under home.
func SwitchingPath(home string) string {
	return filepath.Join(home, ".hev", "update", "switching")
}

// beginSwitch marks the switch window; the returned function ends it.
func (u *Updater) beginSwitch() (func(), error) {
	path := SwitchingPath(u.Home)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o600); err != nil {
		return nil, fmt.Errorf("mark the switch: %w", err)
	}
	done := false
	return func() {
		if !done {
			done = true
			_ = os.Remove(path)
		}
	}, nil
}

// switchHook runs inside the switch window, after the marker is written and
// before the last check; tests use it to land work in the window.
var switchHook func()

// AwaitSwitch waits while an automatic update on this home is in its switch
// window, for at most a minute, and reports whether it waited. Call it after
// registering the work or hold the update must see.
func AwaitSwitch(home string) bool {
	return awaitSwitch(home, switchWait, 50*time.Millisecond)
}

func awaitSwitch(home string, max, poll time.Duration) bool {
	path := SwitchingPath(home)
	deadline := time.Now().Add(max)
	waited := false
	for {
		info, err := os.Stat(path)
		if err != nil || time.Since(info.ModTime()) > switchStale || time.Now().After(deadline) {
			return waited
		}
		waited = true
		time.Sleep(poll)
	}
}
