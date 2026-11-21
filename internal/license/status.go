package license

import (
	"slices"
	"time"
)

type State string

const (
	Licensed State = "licensed"
	Grace    State = "grace"
	Floor    State = "floor"

	DefaultGrace = 14 * 24 * time.Hour
)

type Timeline struct {
	State                 State `json:"state"`
	SecondsToDeadline     int64 `json:"seconds_to_deadline"`
	GraceSecondsRemaining int64 `json:"grace_seconds_remaining"`
}

func ClampGrace(grace time.Duration) time.Duration {
	return min(max(0, grace), DefaultGrace)
}

func Evaluate(now, exp time.Time, tier string, grace time.Duration) Timeline {
	if exp.IsZero() {
		return Timeline{State: Floor}
	}
	if !now.After(exp) {
		return Timeline{State: Licensed, SecondsToDeadline: int64(exp.Sub(now) / time.Second)}
	}
	deadline := exp.Add(ClampGrace(grace))
	if tier != "trial" && !now.After(deadline) {
		remaining := int64(deadline.Sub(now) / time.Second)
		return Timeline{State: Grace, SecondsToDeadline: remaining, GraceSecondsRemaining: remaining}
	}
	return Timeline{State: Floor}
}

type Status struct {
	Timeline
	Valid     bool              `json:"valid"`
	Reason    string            `json:"reason,omitempty"`
	Subject   string            `json:"sub,omitempty"`
	Tier      string            `json:"tier,omitempty"`
	JTI       string            `json:"jti,omitempty"`
	Features  []string          `json:"features,omitempty"`
	Limits    map[string]uint64 `json:"limits,omitempty"`
	Expires   *time.Time        `json:"exp,omitempty"`
	CheckedAt time.Time         `json:"checked_at"`
	Until     time.Time         `json:"until"`
}

func (s Status) Entitled() bool {
	return s.Valid && (s.State == Licensed || s.State == Grace)
}

func (s Status) HasFeature(name string) bool {
	return s.Entitled() && slices.Contains(s.Features, name)
}

func (s Status) Limit(name string) (uint64, bool) {
	value, ok := s.Limits[name]
	return value, ok && s.Entitled()
}

func (s Status) Warning() string {
	switch {
	case s.State == Grace:
		return "Kit Pro license is in grace. Renew before grace ends."
	case s.State == Licensed && s.SecondsToDeadline <= int64(DefaultGrace/time.Second):
		return "Kit Pro license expires within 14 days. Renew before it expires."
	case s.Valid && s.State == Floor:
		return "Kit Pro license has expired."
	default:
		return ""
	}
}
