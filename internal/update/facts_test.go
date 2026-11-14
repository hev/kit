package update

import (
	"testing"
	"time"
)

func TestObserveUpdateState(t *testing.T) {
	u := Updater{Home: t.TempDir()}
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	if e := u.saveState(State{Pending: &Pending{At: now}, LastCheck: now, LastError: "private raw-error /private/path"}); e != nil {
		t.Fatal(e)
	}
	o, e := u.Observe()
	if e != nil || !o.Staged || o.Outcome != "failing" || !o.LastCheck.Equal(now) {
		t.Fatal(o, e)
	}
	if e := u.saveState(State{LastCheck: now}); e != nil {
		t.Fatal(e)
	}
	o, e = u.Observe()
	if e != nil || o.Staged || o.Outcome != "ok" {
		t.Fatal(o, e)
	}
}
