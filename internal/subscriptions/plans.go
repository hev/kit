package subscriptions

import (
	"github.com/BurntSushi/toml"
	"github.com/hev/kit/internal/cost"
)

// ConfiguredPlan is a subscriptions.toml plan, reduced to what a status
// shows: which configured spend plan covers the same harness.
type ConfiguredPlan struct {
	Name    string
	Harness string
}

// LoadPlans reads the [[plans]] of a subscriptions.toml. The file stays the
// spend configuration for hev cost-price and hev serve; this only names plans.
func LoadPlans(path string) ([]ConfiguredPlan, error) {
	var c struct {
		Plans []cost.Plan `toml:"plans"`
	}
	if _, err := toml.DecodeFile(path, &c); err != nil {
		return nil, err
	}
	out := []ConfiguredPlan{}
	for _, p := range c.Plans {
		if p.Name != "" && p.Harness != "" {
			out = append(out, ConfiguredPlan{Name: p.Name, Harness: p.Harness})
		}
	}
	return out, nil
}
