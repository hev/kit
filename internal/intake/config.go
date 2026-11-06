package intake

import (
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Interval  string
	Mode      string
	StateDir  string `toml:"state_dir"`
	Dispatch  []string
	DailyCap  int    `toml:"daily_cap"`
	PauseFile string `toml:"pause_file"`
	Linear    struct {
		APIKeyEnv        string `toml:"api_key_env"`
		Team             string
		Assignee         string
		AllowedActorTeam string `toml:"allowed_actor_team"`
	}
	Mention []Rule
	Email   EmailConfig
	Status  []Rule
	Limits  struct {
		CodeJobs     int     `toml:"code_jobs"`
		MinRemaining float64 `toml:"linear_min_remaining_pct"`
	}
}
type Rule struct {
	Name, Action, Mode, Template, Host, Concurrency string
	MarkerPrefix                                    string `toml:"marker_prefix"`
	Batch                                           bool
	BatchCooldown                                   string `toml:"batch_cooldown"`
}

func (r Rule) cooldown() time.Duration {
	if r.BatchCooldown == "" {
		return 6 * time.Hour
	}
	d, _ := time.ParseDuration(r.BatchCooldown)
	return d
}

var slug = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func loadConfig(path string) (Config, error) {
	var c Config
	md, err := toml.DecodeFile(path, &c)
	if err != nil {
		return c, err
	}
	if len(md.Undecoded()) != 0 {
		return c, fmt.Errorf("unknown config keys: %v", md.Undecoded())
	}
	if c.Interval == "" {
		c.Interval = "15m"
	}
	d, err := time.ParseDuration(c.Interval)
	if err != nil || d < 5*time.Minute {
		return c, fmt.Errorf("interval must be at least 5m")
	}
	if c.Mode == "" {
		c.Mode = "dry-run"
	}
	if len(c.Dispatch) == 0 {
		c.Dispatch = []string{"hev", "dispatch"}
	}
	if c.StateDir == "" || c.Linear.APIKeyEnv == "" || c.Linear.Team == "" || c.Linear.Assignee == "" || c.Linear.AllowedActorTeam == "" || (len(c.Status) == 0 && len(c.Mention) == 0 && c.Email.URL == "") {
		return c, fmt.Errorf("state_dir, linear fields and at least one source rule required")
	}
	if c.DailyCap < 0 || c.Limits.CodeJobs < 0 || c.Limits.MinRemaining < 0 || c.Limits.MinRemaining > 100 {
		return c, fmt.Errorf("invalid limits")
	}
	if !validMode(c.Mode) {
		return c, fmt.Errorf("invalid global mode")
	}
	seen := map[string]bool{}
	actions := map[string]string{}
	for i := range c.Status {
		r := &c.Status[i]
		if r.BatchCooldown != "" {
			if d, err := time.ParseDuration(r.BatchCooldown); err != nil || d < 0 {
				return c, fmt.Errorf("invalid batch_cooldown")
			}
		}
		if r.Mode == "" {
			r.Mode = c.Mode
		}
		if !validMode(r.Mode) || r.Name == "" || !slug.MatchString(r.Action) || seen[r.Name] {
			return c, fmt.Errorf("invalid or duplicate status rule")
		}
		seen[r.Name] = true
		if previous, ok := actions[r.Action]; ok && previous != r.Concurrency {
			return c, fmt.Errorf("an action must use one concurrency class")
		}
		actions[r.Action] = r.Concurrency
		if r.Concurrency != "read" && r.Concurrency != "code" {
			return c, fmt.Errorf("concurrency must be read or code")
		}
		if r.Mode == "live" && (r.Template == "" || r.Host == "") {
			return c, fmt.Errorf("live rules require template and host")
		}
	}
	for j := range c.Mention {
		r := &c.Mention[j]
		if r.Mode == "" {
			r.Mode = c.Mode
		}
		if r.Action != "feedback" || !validMode(r.Mode) || r.Concurrency != "code" || r.Template == "" || r.Host == "" || r.Batch {
			return c, fmt.Errorf("invalid mention rule")
		}
		if previous, ok := actions[r.Action]; ok && previous != r.Concurrency {
			return c, fmt.Errorf("an action must use one concurrency class")
		}
	}
	if len(c.Mention) > 1 {
		return c, fmt.Errorf("only one mention rule supported")
	}
	if c.Email.URL != "" {
		e := &c.Email
		if e.MaxAttachmentBytes < 0 {
			return c, fmt.Errorf("invalid email attachment limit")
		}
		if e.Mode == "" {
			e.Mode = c.Mode
		}
		if previous, ok := actions["triage"]; ok && previous != "read" {
			return c, fmt.Errorf("email triage requires read concurrency")
		}
		if !validMode(e.Mode) || e.TokenEnv == "" || len(e.Allow) == 0 || e.Require != "dmarc" || e.Team != c.Linear.Team || e.Status != "Triage" || e.TriageTemplate == "" || e.Host == "" {
			return c, fmt.Errorf("invalid email configuration")
		}
	}
	return c, nil
}
func validMode(s string) bool { return s == "off" || s == "dry-run" || s == "live" }
func exists(path string) (bool, error) {
	_, e := os.Stat(path)
	if os.IsNotExist(e) {
		return false, nil
	}
	return e == nil, e
}
