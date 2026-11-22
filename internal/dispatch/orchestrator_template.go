package dispatch

import (
	"fmt"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

// OrchestratorTemplate describes the two layers before runtime compilation.
type OrchestratorTemplate struct {
	SwarmSpec
	Climb *ClimbTemplate `toml:"climb" yaml:"-"`
}

// ClimbTemplate is the initial generation configuration. Scorer admission and
// execution belong to the climb runtime.
type ClimbTemplate struct {
	Repo      string `toml:"repo"`
	Base      string `toml:"base"`
	Objective struct {
		Scorer    string `toml:"scorer"`
		Direction string `toml:"direction"`
	} `toml:"objective"`
	Population struct {
		Width int `toml:"width"`
		TopK  int `toml:"top_k"`
	} `toml:"population"`
	Stop struct {
		MaxGenerations int `toml:"max_generations"`
	} `toml:"stop"`
}

func ParseOrchestratorTemplate(text string) (OrchestratorTemplate, error) {
	var t OrchestratorTemplate
	meta, err := toml.Decode(text, &t)
	if err != nil {
		return t, err
	}
	if unknown := meta.Undecoded(); len(unknown) != 0 {
		return t, fmt.Errorf("unknown template field %s", unknown[0])
	}
	switch t.Kind {
	case "":
		switch {
		case t.Climb != nil:
			t.Kind = "climb"
		case meta.IsDefined("roles"):
			t.Kind = "swarm"
		default:
			return t, fmt.Errorf("template requires kind swarm, [[roles]] or [climb]")
		}
	case "swarm", "climb":
	default:
		return t, fmt.Errorf("template kind must be swarm or climb")
	}
	if t.Kind == "climb" && t.Climb == nil {
		return t, fmt.Errorf("kind climb requires [climb]")
	}
	if t.Climb != nil {
		if !ValidName(t.Name) || strings.TrimSpace(t.Goal) == "" ||
			strings.TrimSpace(t.Host) == "" || strings.TrimSpace(t.Reply) == "" {
			return t, fmt.Errorf("climb requires a valid name, goal, scoring host and reply")
		}
		if err := t.Climb.validate(); err != nil {
			return t, err
		}
	}
	if t.Kind == "swarm" || meta.IsDefined("roles") {
		if err := t.SwarmSpec.setBrief(t.Brief); err != nil {
			return t, err
		}
		swarm := t.SwarmSpec
		swarm.Kind = "swarm"
		if err := swarm.Validate(); err != nil {
			return t, err
		}
	}
	return t, nil
}

func (c ClimbTemplate) validate() error {
	if strings.TrimSpace(c.Repo) == "" || strings.TrimSpace(c.Base) == "" ||
		strings.TrimSpace(c.Objective.Scorer) == "" {
		return fmt.Errorf("climb requires repo, base and objective.scorer")
	}
	if c.Objective.Direction != "max" && c.Objective.Direction != "min" {
		return fmt.Errorf("climb objective.direction must be max or min")
	}
	if c.Population.TopK <= 0 || c.Population.Width < c.Population.TopK {
		return fmt.Errorf("climb population requires 1 <= top_k <= width")
	}
	if c.Stop.MaxGenerations <= 0 {
		return fmt.Errorf("climb stop.max_generations must be positive")
	}
	return nil
}

// SwarmMarkdown gives the existing swarm runtime its durable input format.
// Reject climb before handing any part of the template to that runtime.
func (t OrchestratorTemplate) SwarmMarkdown() (string, error) {
	if t.Climb != nil {
		return "", fmt.Errorf("climb templates require the climb runtime (PRs #322/#323)")
	}
	front, err := yaml.Marshal(t.SwarmSpec)
	if err != nil {
		return "", err
	}
	return "---\n" + string(front) + "---\n" + t.Brief + "\n", nil
}

func (p *SwarmPieces) UnmarshalTOML(value any) error {
	switch v := value.(type) {
	case string:
		if v == "orchestrator" {
			p.Orchestrator = true
			return nil
		}
	case []any:
		for _, entry := range v {
			name, ok := entry.(string)
			if !ok {
				return fmt.Errorf("pieces must contain only strings")
			}
			p.Names = append(p.Names, name)
		}
		return nil
	}
	return fmt.Errorf("pieces must be a list or orchestrator")
}
