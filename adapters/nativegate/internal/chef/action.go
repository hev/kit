package chef

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

// Canonical is a trusted, explicitly admitted Order authority adapter, never a
// peer message. Authorize rechecks current scope, authority, acceptance, owner,
// attempt and revision invariants. Its implementation must serialize competing
// canonical mutations with an effect (e.g. canonical CAS in the effect itself).
// No installed adapter is implied by this source API.
type Canonical interface {
	Current(order string) (Observation, error)
	Authorize(order, revision string, decision Decision) error
}
type Decision struct {
	Assessment    string          `json:"assessment"`
	Reason        string          `json:"reason"`
	Action        string          `json:"action"`
	Payload       json.RawMessage `json:"payload,omitempty"`
	WaitOwner     string          `json:"waitOwner,omitempty"`
	WaitCondition string          `json:"waitCondition,omitempty"`
}

func (d Decision) valid() error {
	if strings.TrimSpace(d.Assessment) == "" || strings.TrimSpace(d.Reason) == "" {
		return errors.New("assessment receipt and chef reason required")
	}
	switch d.Action {
	case "admit-workflow", "request-validation", "send", "wait", "complete":
	default:
		return errors.New("unsupported chef action; Argo alone advances/retries DAGs")
	}
	if d.Action == "wait" && (d.WaitOwner == "" || d.WaitCondition == "") {
		return errors.New("wait requires owner and release condition")
	}
	if len(d.Payload) > 0 && !json.Valid(d.Payload) {
		return errors.New("invalid action payload")
	}
	return bounded(d, 8192)
}
func verify(c Canonical, a *Attention) error {
	o, e := c.Current(a.Order)
	if e != nil {
		return e
	}
	if e = validObservation(o); e != nil {
		return e
	}
	if o.Order != a.Order || !o.Open || o.Revision != a.Revision {
		return errors.New("canonical order closed or revision changed")
	}
	return nil
}

// Decide retains assessment, explicit reason and immutable action intent before
// any external effect. Assessment/model execution happens outside the journal
// lock; a late response cannot act after ownership or canonical scope changed.
func (s Store) Decide(l Lease, id string, d Decision, c Canonical) error {
	if e := d.valid(); e != nil {
		return e
	}
	return s.locked(func(st *State) error {
		if e := check(st, l); e != nil {
			return e
		}
		a, e := find(st, id)
		if e != nil {
			return e
		}
		if a.Status != "pending" {
			if a.Decision != nil && digest(*a.Decision) == digest(d) {
				return nil
			}
			return errors.New("attention already decided")
		}
		if e = noUnknown(st, a); e != nil {
			return e
		}
		if e = verify(c, a); e != nil {
			return e
		}
		if e = c.Authorize(a.Order, a.Revision, d); e != nil {
			return e
		}
		if e = check(st, l); e != nil {
			return e
		}
		// Detach RawMessage from the caller's mutable buffer.
		b, _ := json.Marshal(d)
		var captured Decision
		if e = json.Unmarshal(b, &captured); e != nil {
			return e
		}
		a.Decision = &captured
		a.Status = "decided"
		return s.commit(st)
	})
}

type Intent struct {
	ID           string    `json:"id"`
	Order        string    `json:"order"`
	Revision     string    `json:"revision"`
	Fence        uint64    `json:"fence"`
	Owner        string    `json:"owner"`
	Decision     Decision  `json:"decision"`
	LeaseExpires time.Time `json:"leaseExpires"`
}

// Effect is a trusted action adapter. Preflight may run before Guard.Do, but ALL
// mutations must occur within its callback. Remote mutation requires an
// independently enforcing receiver: pass Intent.Fence/Owner and lease deadline,
// use original Intent.ID for idempotency, and reject expired/stale authority at
// that receiver's mutation boundary. The local journal is not a remote lease.
type Effect interface {
	Execute(Intent, Guard) (string, error)
}
type Guard struct {
	run func(func() (string, error)) (string, error)
}

func (g Guard) Do(effect func() (string, error)) (string, error) {
	if g.run == nil || effect == nil {
		return "", errors.New("missing action-side guard")
	}
	return g.run(effect)
}

// Apply can be called after restart/handoff for a decided intent. Once a guard
// has started, no automatic replay is permitted, even with a newer chef fence.
func (s Store) Apply(l Lease, id string, c Canonical, effect Effect) error {
	var intent Intent
	err := s.locked(func(st *State) error {
		if e := check(st, l); e != nil {
			return e
		}
		a, e := find(st, id)
		if e != nil {
			return e
		}
		if a.Status == "done" {
			return nil
		}
		if a.Status == "applying" || a.Status == "held" {
			return ErrHeld
		}
		if a.Status != "decided" || a.Decision == nil {
			return errors.New("no action intent")
		}
		intent = Intent{a.ID, a.Order, a.Revision, l.Fence, l.Owner, *a.Decision, st.Lease.Expires}
		return nil
	})
	if err != nil || intent.ID == "" {
		return err
	}
	// Wait is a durable decision with no external effect.
	if intent.Decision.Action == "wait" {
		return s.locked(func(st *State) error {
			if e := check(st, l); e != nil {
				return e
			}
			a, e := find(st, id)
			if e != nil {
				return e
			}
			if a.Status != "decided" {
				return errors.New("action state changed")
			}
			if e = noUnknown(st, a); e != nil {
				return e
			}
			if e = verify(c, a); e != nil {
				return e
			}
			if e = c.Authorize(a.Order, a.Revision, *a.Decision); e != nil {
				return e
			}
			if e = check(st, l); e != nil {
				return e
			}
			a.Status = "done"
			a.Outcome = "waiting: " + a.Decision.WaitOwner + "; " + a.Decision.WaitCondition
			advance(st)
			return s.commit(st)
		})
	}
	if effect == nil {
		return errors.New("effect adapter required")
	}
	var used atomic.Bool
	var closed atomic.Bool
	guard := Guard{run: func(fn func() (string, error)) (string, error) {
		var outcome string
		if closed.Load() {
			return "", errors.New("action guard is closed")
		}
		e := s.locked(func(st *State) error {
			if closed.Load() {
				return errors.New("action guard is closed")
			}
			if e := check(st, l); e != nil {
				return e
			}
			a, e := find(st, id)
			if e != nil {
				return e
			}
			if a.Status != "decided" || a.Decision == nil || digest(*a.Decision) != digest(intent.Decision) {
				return ErrHeld
			}
			if e = noUnknown(st, a); e != nil {
				return e
			}
			if e = verify(c, a); e != nil {
				return e
			}
			if e = c.Authorize(a.Order, a.Revision, *a.Decision); e != nil {
				return e
			}
			// Canonical queries may block: recheck real time immediately before commit.
			if e = check(st, l); e != nil {
				return e
			}
			a.Status = "applying"
			if e = s.commit(st); e != nil {
				return e
			}
			used.Store(true)
			// Ownership cannot transfer while this lock is held. Expiry after intent
			// commit is still rejected before starting the mutation.
			if e = check(st, l); e != nil {
				return e
			}
			outcome, e = fn()
			if e != nil {
				a.Status = "held"
				a.Outcome = "effect may have committed; inspect using intent " + id
			} else if outcome == "" || len(outcome) > 4096 {
				a.Status = "held"
				a.Outcome = "missing or oversized effect evidence"
				e = ErrHeld
			} else {
				a.Status = "done"
				a.Outcome = outcome
				advance(st)
			}
			if ce := s.commit(st); ce != nil {
				return ce
			}
			return e
		})
		return outcome, e
	}}
	_, err = effect.Execute(intent, guard)
	closed.Store(true)
	if !used.Load() && err == nil {
		return errors.New("adapter omitted action-side guard")
	}
	return err
}

// advance is a contiguous handled-event cursor; a later result never hides an
// earlier pending/uncertain obligation from the next chef.
func advance(st *State) {
	for _, a := range st.Attention {
		if a.Sequence <= st.Cursor {
			continue
		}
		if a.Sequence != st.Cursor+1 || a.Status != "done" && a.Status != "stale" {
			break
		}
		st.Cursor = a.Sequence
	}
}

// Reconcile records inspected effect evidence as terminal; it never authorizes a
// replay. A required new effect needs a new attention fact and canonical intent.
func (s Store) Reconcile(l Lease, id, evidence, outcome string) error {
	return s.locked(func(st *State) error {
		if e := check(st, l); e != nil {
			return e
		}
		a, e := find(st, id)
		if e != nil {
			return e
		}
		if a.Status != "held" && a.Status != "applying" {
			return errors.New("no uncertain effect to reconcile")
		}
		if evidence == "" || outcome == "" || len(evidence)+len(outcome) > 4096 {
			return errors.New("bounded inspection evidence and observed outcome required")
		}
		a.Status = "done"
		a.Outcome = fmt.Sprintf("inspection %s: %s", evidence, outcome)
		advance(st)
		return s.commit(st)
	})
}

// Supersede closes only pre-effect stale attention after proving canonical drift.
// It cannot settle unknown effects or discard a live obligation.
func (s Store) Supersede(l Lease, id string, c Canonical) error {
	return s.locked(func(st *State) error {
		if e := check(st, l); e != nil {
			return e
		}
		a, e := find(st, id)
		if e != nil {
			return e
		}
		if a.Status != "pending" && a.Status != "decided" {
			return errors.New("cannot supersede started effect")
		}
		o, e := c.Current(a.Order)
		if e != nil {
			return e
		}
		if o.Order != a.Order || o.Open && o.Revision == a.Revision {
			return errors.New("canonical drift not proven")
		}
		a.Status = "stale"
		a.Outcome = "canonical revision changed or order closed"
		advance(st)
		return s.commit(st)
	})
}

func noUnknown(st *State, a *Attention) error {
	for _, other := range st.Attention {
		if other.ID != a.ID && other.Order == a.Order && (other.Status == "applying" || other.Status == "held") {
			return ErrHeld
		}
	}
	return nil
}
