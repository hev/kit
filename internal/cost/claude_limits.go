package cost

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// ClaudeUsage is the cached, content-free OAuth usage response captured by an
// operator's authenticated GUI-domain job. Credentials never enter this file.
type ClaudeUsage struct {
	Plan       string `json:"plan"`
	ObservedAt int64  `json:"observed_at"`
	Limits     []struct {
		Kind     string  `json:"kind"`
		Percent  float64 `json:"percent"`
		ResetsAt string  `json:"resets_at"`
	} `json:"limits"`
}

func ApplyClaudeUsage(path string, limits []Limit, now time.Time) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var u ClaudeUsage
	if err = json.Unmarshal(b, &u); err != nil {
		return err
	}
	if u.ObservedAt <= 0 || u.ObservedAt > now.UnixMilli() {
		return fmt.Errorf("invalid Claude usage observation time")
	}
	for _, raw := range u.Limits {
		if raw.Kind != "weekly_all" {
			continue
		}
		reset, err := time.Parse(time.RFC3339, raw.ResetsAt)
		if err != nil {
			return err
		}
		if raw.Percent < 0 || raw.Percent > 100 || invalidNumber(raw.Percent) {
			return fmt.Errorf("invalid Claude weekly usage")
		}
		for i := range limits {
			if limits[i].Name == u.Plan {
				v := raw.Percent
				limits[i] = Limit{Name: u.Plan, Percent: &v, ObservedAt: u.ObservedAt, ResetsAt: reset.UnixMilli(), Source: "Claude OAuth usage", Stale: u.ObservedAt < now.Add(-time.Hour).UnixMilli() || !reset.After(now)}
				return nil
			}
		}
		return fmt.Errorf("Claude usage plan is not configured")
	}
	return fmt.Errorf("Claude usage response has no weekly_all limit")
}
