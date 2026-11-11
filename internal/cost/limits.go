package cost

import "time"

type Limit struct {
	Name       string   `json:"name"`
	Percent    *float64 `json:"percent"`
	ResetsAt   int64    `json:"resets_at"`
	ObservedAt int64    `json:"observed_at"`
	Source     string   `json:"source"`
	Stale      bool     `json:"stale"`
}

// Limits accepts recorded observations only. It does not access a keychain or
// invent a provider percentage from tokens. A configured fallback is labelled.
func (c Config) Limits(rows []Row, now time.Time) []Limit {
	out := []Limit{}
	for _, p := range c.Plans {
		l := Limit{Name: p.Name, Source: "unavailable", Stale: true}
		var tokens int64
		for _, r := range rows {
			if r.Subscription != p.Name {
				continue
			}
			if planWeek(r.Start, p) == planWeek(now.UnixMilli(), p) {
				tokens += r.Tokens
			}
			raw, err := ParseCodexRateLimits(r.Limits)
			if err != nil {
				continue
			}
			for _, v := range raw.Windows() {
				if v.Window == WeeklyMinutes && v.Used >= 0 && v.Used <= 100 && r.End > l.ObservedAt {
					value := v.Used
					l.Percent = &value
					l.ResetsAt = v.ResetsAt * 1000
					l.ObservedAt = r.End
					l.Source = "codex rollout"
					l.Stale = r.End < now.Add(-time.Hour).UnixMilli() || l.ResetsAt <= now.UnixMilli()
				}
			}
		}
		if l.Percent == nil && p.Basis > 0 {
			v := float64(tokens) / float64(p.Basis) * 100
			l.Percent = &v
			l.Source = "token estimate"
			l.Stale = false
		}
		out = append(out, l)
	}
	return out
}
