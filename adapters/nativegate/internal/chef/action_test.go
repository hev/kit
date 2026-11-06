package chef

import (
	"errors"
	"testing"
	"time"
)

type delayedCanonical struct {
	o     Observation
	delay time.Duration
}

func (c delayedCanonical) Current(string) (Observation, error)      { return c.o, nil }
func (c delayedCanonical) Authorize(string, string, Decision) error { time.Sleep(c.delay); return nil }
func TestAuthorizationCannotOutliveLease(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	o := Observation{Order: "order", Revision: "revision", Open: true, Unfinished: true, Idle: true, Evidence: "fixture:canonical"}
	id, e := s.Observe(o)
	if e != nil {
		t.Fatal(e)
	}
	l, e := s.Acquire("chef", time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	d := Decision{Assessment: "fixture:advice", Reason: "wait on named dependency", Action: "wait", WaitOwner: "owner", WaitCondition: "condition"}
	if e = s.Decide(l, id, d, delayedCanonical{o, 0}); e != nil {
		t.Fatal(e)
	}
	l, e = s.Renew(l, 10*time.Millisecond)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Apply(l, id, delayedCanonical{o, 30 * time.Millisecond}, nil); !errors.Is(e, ErrFence) {
		t.Fatalf("expired authorization committed: %v", e)
	}
	st, e := s.Status()
	if e != nil || st.Cursor != 0 || st.Attention[0].Status != "decided" {
		t.Fatal("expired wait changed cursor", st, e)
	}
}
func TestPriorityPreservesContiguousCursor(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	o := Observation{Order: "older", Revision: "revision", Open: true, Unfinished: true, Idle: true, Evidence: "fixture:canonical", Priority: 10}
	old, e := s.Observe(o)
	if e != nil {
		t.Fatal(e)
	}
	o.Order = "urgent"
	o.Priority = 90
	urgent, e := s.Observe(o)
	if e != nil {
		t.Fatal(e)
	}
	l, e := s.Acquire("chef", time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	p, e := s.Next(l)
	if e != nil || p.Event.ID != urgent {
		t.Fatal(p, e)
	}
	d := Decision{Assessment: "fixture:advice", Reason: "wait owned", Action: "wait", WaitOwner: "owner", WaitCondition: "condition"}
	if e = s.Decide(l, urgent, d, delayedCanonical{o, 0}); e != nil {
		t.Fatal(e)
	}
	if e = s.Apply(l, urgent, delayedCanonical{o, 0}, nil); e != nil {
		t.Fatal(e)
	}
	st, e := s.Status()
	if e != nil || st.Cursor != 0 {
		t.Fatal("later result skipped older input", st, e)
	}
	p, e = s.Next(l)
	if e != nil || p.Event.ID != old {
		t.Fatal("older obligation lost", p, e)
	}
}
