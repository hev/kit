package chef

import (
	"errors"
	"testing"
	"time"
)

type flow02Canonical struct{ o Observation }

func (c *flow02Canonical) Current(string) (Observation, error)      { return c.o, nil }
func (c *flow02Canonical) Authorize(string, string, Decision) error { return nil }

type flow02Effect struct{ calls *int }

func (e flow02Effect) Execute(_ Intent, g Guard) (string, error) {
	return g.Do(func() (string, error) { *e.calls++; return "fixture:observed-effect", nil })
}

func TestFlow02DuplicateStaleOwnerFenceAndGeneration(t *testing.T) {
	for _, mode := range []string{"duplicate", "stale-notifier", "wrong-owner", "old-fence", "expired-fence", "old-generation"} {
		t.Run(mode, func(t *testing.T) {
			s := Store{Dir: t.TempDir()}
			c := &flow02Canonical{o: Observation{Order: "fixture-order", Revision: "generation-1", Open: true, Unfinished: true, Idle: true, Evidence: "fixture:canonical", Workers: map[string]string{"build": "worker-1"}}}
			n := Notification{Worker: "worker-1", Part: "build", Kind: "result", Fact: "candidate ready", Request: "validate", Evidence: "fixture:manifest"}
			id, err := s.Notify(c.o, n)
			if err != nil {
				t.Fatal(err)
			}
			again, err := s.Notify(c.o, n)
			if err != nil || again != "" {
				t.Fatalf("expected duplicate suppressed with no new attention; actual %q %v", again, err)
			}
			if mode == "stale-notifier" {
				n.Worker = "superseded-worker"
				if _, err := s.Notify(c.o, n); err == nil {
					t.Fatal("expected stale notifier rejected; actual accepted")
				}
				return
			}
			l, err := s.Acquire("chef-1", time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			d := Decision{Action: "send", Assessment: "fixture:assessment", Reason: "retained-worker follow-up"}
			if err := s.Decide(l, id, d, c); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "wrong-owner":
				l.Owner = "superseded-chef"
			case "old-fence":
				if err := s.Handoff(l, "fixture:checkpoint"); err != nil {
					t.Fatal(err)
				}
				if _, err := s.Acquire("chef-2", time.Minute); err != nil {
					t.Fatal(err)
				}
			case "expired-fence":
				if err := s.locked(func(st *State) error { st.Lease.Expires = time.Unix(1, 0); return s.commit(st) }); err != nil {
					t.Fatal(err)
				}
			case "old-generation":
				c.o.Revision = "generation-2"
			}
			calls := 0
			err = s.Apply(l, id, c, flow02Effect{&calls})
			if mode == "duplicate" {
				if err != nil {
					t.Fatal(err)
				}
				if err = s.Apply(l, id, c, flow02Effect{&calls}); err != nil || calls != 1 {
					t.Fatalf("expected one effect; actual err=%v calls=%d", err, calls)
				}
			} else {
				if err == nil || calls != 0 {
					t.Fatalf("expected stale action rejected before effect; actual err=%v calls=%d", err, calls)
				}
				if mode != "old-generation" && !errors.Is(err, ErrFence) {
					t.Fatalf("expected ErrFence; actual %v", err)
				}
			}
			t.Logf("expected=%s enforced before duplicate/stale effect; actual calls=%d err=%v", mode, calls, err)
		})
	}
}
