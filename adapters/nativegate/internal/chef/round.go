package chef

import (
	"encoding/json"
	"errors"
)

// Advice binds the shared assessment receipt to the exact captured event. A
// real adapter records actual model/provider and usage in that immutable receipt;
// consumed prompt bytes are reported only when the adapter can observe them.
type Advice struct {
	Receipt       string   `json:"receipt"`
	Order         string   `json:"order"`
	Revision      string   `json:"revision"`
	Ready         bool     `json:"ready"`
	Actions       []string `json:"actions"`
	Model         string   `json:"model"`
	Provider      string   `json:"provider"`
	ConsumedBytes *int     `json:"consumedBytes,omitempty"`
}
type Assessor interface{ Assess(Packet) (Advice, error) }
type Chooser interface {
	Choose(Packet, Advice) (Decision, error)
}
type RoundResult struct {
	Attention     string `json:"attention,omitempty"`
	Assessment    string `json:"assessment,omitempty"`
	PacketBytes   int    `json:"packetBytes"`
	ConsumedBytes *int   `json:"consumedBytes,omitempty"`
	Status        string `json:"status"`
}

// Round handles at most one durable attention event in the foreground. It is
// not a scheduler: no polling, ready queue, retry/backoff or host fallback. A
// quiet floor makes no assessment call. Decided intents resume without another
// assessment; started effects remain held. Caller retains any transport timeout.
func (s Store) Round(l Lease, c Canonical, assessor Assessor, chooser Chooser, effect Effect) (RoundResult, error) {
	out := RoundResult{Status: "quiet"}
	p, e := s.Next(l)
	if e != nil || p == nil {
		return out, e
	}
	raw, _ := json.Marshal(p)
	out.Attention = p.Event.ID
	out.PacketBytes = len(raw)
	if p.Event.Status == "applying" || p.Event.Status == "held" {
		out.Status = "held"
		return out, ErrHeld
	}
	if p.Event.Status == "pending" {
		if assessor == nil || chooser == nil {
			return out, errors.New("shared assessor and chef chooser required")
		}
		advice, e := assessor.Assess(*p)
		if e != nil {
			return out, e
		}
		if advice.Receipt == "" || advice.Order != p.Event.Order || advice.Revision != p.Event.Revision || !advice.Ready {
			return out, errors.New("assessment is missing, hypothetical or stale")
		}
		if e = bounded(advice, 4096); e != nil {
			return out, e
		}
		d, e := chooser.Choose(*p, advice)
		if e != nil {
			return out, e
		}
		eligible := false
		for _, action := range advice.Actions {
			if action == d.Action {
				eligible = true
			}
		}
		if !eligible || d.Assessment != advice.Receipt {
			return out, errors.New("chef action not supported by bound assessment")
		}
		if e = s.Decide(l, p.Event.ID, d, c); e != nil {
			return out, e
		}
		out.Assessment = advice.Receipt
		out.ConsumedBytes = advice.ConsumedBytes
	} else if p.Event.Decision != nil {
		out.Assessment = p.Event.Decision.Assessment
	}
	e = s.Apply(l, p.Event.ID, c, effect)
	out.Status = "done"
	if e != nil {
		out.Status = "unfinished"
	}
	return out, e
}
