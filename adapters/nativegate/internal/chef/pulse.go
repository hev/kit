package chef

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Observation is a bounded canonical projection from an explicitly admitted
// source. Revision binds scope, authority, plan, execution and acceptance. A
// caller must not feed untrusted peer claims into ownership/authority fields.
// Release changes when a dependency or named wait condition actually changes.
type Observation struct {
	Priority      int               `json:"priority"`
	Order         string            `json:"order"`
	Revision      string            `json:"revision"`
	Open          bool              `json:"open"`
	Unfinished    bool              `json:"unfinished"`
	Idle          bool              `json:"idle"`
	WaitOwner     string            `json:"waitOwner,omitempty"`
	WaitCondition string            `json:"waitCondition,omitempty"`
	Release       string            `json:"releaseDigest"`
	Evidence      string            `json:"evidence"`
	Workers       map[string]string `json:"workers"` // part -> immutable worker
}
type Notification struct {
	Worker   string `json:"worker"`
	Part     string `json:"part"`
	Kind     string `json:"kind"`
	Fact     string `json:"fact"`
	Request  string `json:"request"`
	Evidence string `json:"evidence"`
}
type Attention struct {
	Priority int       `json:"priority"`
	Sequence uint64    `json:"sequence"`
	ID       string    `json:"id"`
	Order    string    `json:"order"`
	Revision string    `json:"revision"`
	Fact     string    `json:"fact"`
	Request  string    `json:"request"`
	Evidence string    `json:"evidence"`
	Status   string    `json:"status"` // pending, decided, applying, done, held, stale
	Decision *Decision `json:"decision,omitempty"`
	Outcome  string    `json:"outcome,omitempty"`
}

// Observe is one model-free Pulse reconciliation of one canonical Order. It
// creates attention only; Argo alone owns ready nodes, retries and progression.
// An explicit unchanged wait suppresses heartbeat calls. An idle unfinished
// order without a wait gets one durable obligation, never a completion claim.
func (s Store) Observe(o Observation) (string, error) {
	if err := validObservation(o); err != nil {
		return "", err
	}
	return s.attend(o, nil)
}
func validObservation(o Observation) error {
	if o.Priority < 0 || o.Priority > 100 {
		return errors.New("attention priority must be 0 to 100")
	}
	if o.Order == "" || o.Revision == "" || o.Evidence == "" {
		return errors.New("canonical order, revision and evidence required")
	}
	if (o.WaitOwner == "") != (o.WaitCondition == "") {
		return errors.New("wait needs owner and release condition")
	}
	return bounded(o, 8192)
}
func (s Store) Notify(o Observation, n Notification) (string, error) {
	if e := validObservation(o); e != nil {
		return "", e
	}
	if o.Workers[n.Part] != n.Worker || n.Worker == "" || n.Part == "" {
		return "", errors.New("notification not from assigned worker")
	}
	switch n.Kind {
	case "result", "blocker", "plan-change", "recovery":
	default:
		return "", errors.New("internal/routine noise is not attention")
	}
	if strings.TrimSpace(n.Fact) == "" || strings.TrimSpace(n.Request) == "" || strings.TrimSpace(n.Evidence) == "" {
		return "", errors.New("fact/request/evidence required")
	}
	if e := bounded(n, 4096); e != nil {
		return "", e
	}
	return s.attend(o, &n)
}
func (s Store) attend(o Observation, n *Notification) (string, error) {
	var id string
	e := s.locked(func(st *State) error {
		if !o.Open {
			return nil
		} // never reopen closed history
		key := o.Order
		fingerprint := digest(struct {
			Revision, Release, Owner, Condition string
			Idle, Unfinished                    bool
		}{o.Revision, o.Release, o.WaitOwner, o.WaitCondition, o.Idle, o.Unfinished})
		if n != nil {
			key += "/notification/" + digest(n)
			fingerprint = digest(struct {
				Revision     string
				Notification *Notification
			}{o.Revision, n})
		}
		if st.Observations[key] == fingerprint {
			return nil
		}
		// Store closed/no-work observations too, but do not make a model decision.
		old := st.Observations[key]
		if old == "" && len(st.Observations) >= 1000 {
			return errors.New("observation capacity reached; explicit admission/retention required")
		}
		st.Observations[key] = fingerprint
		if n == nil && (!o.Unfinished || !o.Idle && old == "" || o.WaitOwner != "" && old == "") {
			return s.commit(st)
		}
		eventID := digest(struct{ Order, Key, Fingerprint string }{o.Order, key, fingerprint})
		for _, oldEvent := range st.Attention {
			if oldEvent.ID == eventID {
				return s.commit(st)
			}
		}
		if _, err := os.Stat(filepath.Join(s.Dir, "receipt-"+eventID+".json")); err == nil {
			return s.commit(st)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if len(st.Attention) >= MaxAttention {
			return errors.New("attention capacity reached; archive settled entries explicitly")
		}
		if st.Sequence == ^uint64(0) {
			return errors.New("attention sequence exhausted")
		}
		st.Sequence++
		a := Attention{Priority: o.Priority, Sequence: st.Sequence, Order: o.Order, Revision: o.Revision, Status: "pending", Evidence: o.Evidence, Fact: "Canonical release or idle unfinished obligation changed", Request: "Assess remaining authorized obligations; choose next action or explicit wait"}
		if n != nil {
			a.Fact = n.Fact
			a.Request = n.Request
			a.Evidence = n.Evidence
		}
		a.ID = eventID
		id = a.ID
		st.Attention = append(st.Attention, a)
		return s.commit(st)
	})
	return id, e
}

// Packet is bounded consumed context, not history. No caller receives an
// unbounded floor snapshot. Actual model use/usage belongs in Assessment.
type Packet struct {
	Version    string    `json:"version"`
	Lease      Lease     `json:"lease"`
	Cursor     uint64    `json:"handledCursor"`
	Checkpoint string    `json:"checkpoint"`
	Event      Attention `json:"event"`
}

func (s Store) Next(l Lease) (*Packet, error) {
	var p *Packet
	e := s.locked(func(st *State) error {
		if e := check(st, l); e != nil {
			return e
		}
		var selected *Attention
		for i := range st.Attention {
			a := &st.Attention[i]
			if a.Status == "done" || a.Status == "stale" {
				continue
			}
			if selected == nil || a.Priority > selected.Priority {
				selected = a
			}
		}
		if selected != nil {
			v := Packet{Version, l, st.Cursor, st.Handoff, *selected}
			if e := bounded(v, PacketLimit); e != nil {
				return e
			}
			p = &v
		}
		return nil
	})
	return p, e
}
func find(st *State, id string) (*Attention, error) {
	for i := range st.Attention {
		if st.Attention[i].ID == id {
			return &st.Attention[i], nil
		}
	}
	return nil, fmt.Errorf("unknown attention %s", id)
}
