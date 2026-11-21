package subscriptions

import (
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/hev/kit/internal/cost"
)

// ConfiguredPlan is a subscriptions.toml plan, reduced to what a status
// shows: which configured spend plan covers the same harness and any explicit
// plan/tier annotation (for example plan = "ultra" on agy).
type ConfiguredPlan struct {
	Name    string
	Harness string
	Tier    string
}

// LoadPlans reads [[plans]] and optional [agy] annotations of a
// subscriptions.toml. The file stays the spend configuration for hev cost-price
// and hev serve; this only names plans and optional tier labels.
func LoadPlans(path string) ([]ConfiguredPlan, error) {
	var c struct {
		Plans []cost.Plan `toml:"plans"`
		Agy   struct {
			Plan string `toml:"plan"`
			Tier string `toml:"tier"`
		} `toml:"agy"`
	}
	if _, err := toml.DecodeFile(path, &c); err != nil {
		return nil, err
	}
	out := []ConfiguredPlan{}
	hasAgyTier := false
	for _, p := range c.Plans {
		tier := strings.TrimSpace(p.PlanLabel)
		if tier == "" {
			tier = strings.TrimSpace(p.Tier)
		}
		if p.Harness != "" && (p.Name != "" || tier != "") {
			if strings.EqualFold(p.Harness, "agy") && tier != "" {
				hasAgyTier = true
			}
			out = append(out, ConfiguredPlan{Name: p.Name, Harness: p.Harness, Tier: tier})
		}
	}
	agyTier := strings.TrimSpace(c.Agy.Plan)
	if agyTier == "" {
		agyTier = strings.TrimSpace(c.Agy.Tier)
	}
	if agyTier != "" && !hasAgyTier {
		out = append(out, ConfiguredPlan{Harness: "agy", Tier: agyTier})
	}
	return out, nil
}
