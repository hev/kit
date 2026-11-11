package cost

import (
	"bytes"
	"encoding/json"
	"strings"
)

// CodexWindow is one rate-limit window from a Codex rollout's token-count
// event. Codex puts whichever windows the plan has in primary and secondary,
// so callers classify a window by WindowMinutes, never by its slot.
type CodexWindow struct {
	Used     float64 `json:"used_percent"`
	Window   int     `json:"window_minutes"`
	ResetsAt int64   `json:"resets_at"` // Unix seconds
}

// CodexCredits is the credit balance Codex reports beside its windows.
type CodexCredits struct {
	HasCredits bool            `json:"has_credits"`
	Unlimited  bool            `json:"unlimited"`
	Balance    json.RawMessage `json:"balance"`
}

// BalanceText renders the reported balance, which Codex sends as a string or
// a number, without its quotes or trailing zeros. Empty when not reported.
func (c CodexCredits) BalanceText() string {
	b := bytes.TrimSpace(c.Balance)
	if len(b) == 0 || string(b) == "null" {
		return ""
	}
	s := strings.Trim(string(b), `"`)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	return s
}

// CodexRateLimits is payload.rate_limits from a Codex rollout, and the
// rate_limits column factory records from the same event.
type CodexRateLimits struct {
	LimitID   string        `json:"limit_id"`
	LimitName *string       `json:"limit_name"`
	Primary   *CodexWindow  `json:"primary"`
	Secondary *CodexWindow  `json:"secondary"`
	Credits   *CodexCredits `json:"credits"`
	PlanType  string        `json:"plan_type"`
}

// ParseCodexRateLimits decodes one rate_limits object. Only the windows are
// strict, as cost.Limits always required; a malformed extra field (credits,
// plan, limit name) is dropped rather than discarding the windows. JSON null
// decodes to an empty value with no windows.
func ParseCodexRateLimits(b []byte) (CodexRateLimits, error) {
	var w struct {
		Primary   *CodexWindow `json:"primary"`
		Secondary *CodexWindow `json:"secondary"`
	}
	if err := json.Unmarshal(b, &w); err != nil {
		return CodexRateLimits{}, err
	}
	r := CodexRateLimits{Primary: w.Primary, Secondary: w.Secondary}
	var extra map[string]json.RawMessage
	if json.Unmarshal(b, &extra) == nil {
		_ = json.Unmarshal(extra["limit_id"], &r.LimitID)
		_ = json.Unmarshal(extra["limit_name"], &r.LimitName)
		_ = json.Unmarshal(extra["plan_type"], &r.PlanType)
		var c CodexCredits
		if raw, ok := extra["credits"]; ok && json.Unmarshal(raw, &c) == nil && string(bytes.TrimSpace(raw)) != "null" {
			r.Credits = &c
		}
	}
	return r, nil
}

// Windows returns the reported windows in slot order, skipping absent ones.
func (r CodexRateLimits) Windows() []CodexWindow {
	out := []CodexWindow{}
	for _, w := range []*CodexWindow{r.Primary, r.Secondary} {
		if w != nil {
			out = append(out, *w)
		}
	}
	return out
}

// WeeklyMinutes is the window length Codex uses for its weekly limit.
const WeeklyMinutes = 10080
