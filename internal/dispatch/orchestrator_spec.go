package dispatch

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// OrchestratorSpec is the opt-in declaration. The body is the standing brief.
type OrchestratorSpec struct {
	Kind        string `yaml:"kind" json:"kind"`
	Name        string `yaml:"name" json:"name"`
	Goal        string `yaml:"goal" json:"goal"`
	Host        string `yaml:"host,omitempty" json:"host,omitempty"`
	Mode        string `yaml:"mode,omitempty" json:"mode"`
	Every       string `yaml:"every,omitempty" json:"every,omitempty"`
	Until       string `yaml:"until,omitempty" json:"until,omitempty"`
	MaxLifetime string `yaml:"max_lifetime,omitempty" json:"max_lifetime,omitempty"`
	Harness     string `yaml:"harness,omitempty" json:"harness"`
	Model       string `yaml:"model,omitempty" json:"model,omitempty"`
	Reply       string `yaml:"reply,omitempty" json:"reply,omitempty"`
	Manages     struct {
		Runs  RunScope `yaml:"runs" json:"runs"`
		Repos []string `yaml:"repos,omitempty" json:"repos,omitempty"`
	} `yaml:"manages" json:"manages"`
	Authority []string `yaml:"authority,omitempty" json:"authority"`
	Budget    struct {
		ConcurrentRuns int     `yaml:"concurrent_runs,omitempty" json:"concurrent_runs"`
		RunsPerDay     int     `yaml:"runs_per_day,omitempty" json:"runs_per_day"`
		USDPerDay      float64 `yaml:"usd_per_day,omitempty" json:"usd_per_day,omitempty"`
	} `yaml:"budget,omitempty" json:"budget"`
	Handoff struct {
		RotateAfter int `yaml:"rotate_after,omitempty" json:"rotate_after"`
	} `yaml:"handoff,omitempty" json:"handoff"`
	Idle        string `yaml:"idle,omitempty" json:"idle"`
	Source      string `yaml:"source,omitempty" json:"source"`
	AllowNested bool   `yaml:"allow_nested,omitempty" json:"allow_nested"`
	Brief       string `yaml:"-" json:"brief"`
}

type RunScope struct {
	Selector string   `yaml:"selector,omitempty" json:"selector,omitempty"`
	Names    []string `yaml:"names,omitempty" json:"names,omitempty"`
}

func (r *RunScope) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		if n.Value != "mine" {
			return fmt.Errorf("runs scalar must be mine")
		}
		r.Selector = "mine"
		return nil
	}
	// Decode explicitly so unknown nested fields cannot silently expand scope.
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("runs must be a mapping or mine")
	}
	seen := map[string]bool{}
	for i := 0; i < len(n.Content); i += 2 {
		if seen[n.Content[i].Value] {
			return fmt.Errorf("duplicate runs field %s", n.Content[i].Value)
		}
		seen[n.Content[i].Value] = true
		switch n.Content[i].Value {
		case "selector":
			if err := n.Content[i+1].Decode(&r.Selector); err != nil {
				return err
			}
		case "names":
			if err := n.Content[i+1].Decode(&r.Names); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown runs field %s", n.Content[i].Value)
		}
	}
	return nil
}

func OrchestratorDuration(v string) (time.Duration, error) {
	if strings.HasSuffix(v, "d") {
		n, e := strconv.ParseFloat(strings.TrimSuffix(v, "d"), 64)
		if e != nil || n <= 0 || n > 3650 {
			return 0, fmt.Errorf("invalid duration %q", v)
		}
		d := time.Duration(n * float64(24*time.Hour))
		if d <= 0 {
			return 0, fmt.Errorf("invalid duration %q", v)
		}
		return d, nil
	}
	d, e := time.ParseDuration(v)
	if e != nil || d <= 0 {
		return 0, fmt.Errorf("invalid duration %q", v)
	}
	return d, nil
}
func ParseOrchestrator(text string) (OrchestratorSpec, error) {
	s := OrchestratorSpec{Mode: "once", Harness: "codex", Idle: "skip", Source: "none", Authority: []string{"say", "notify"}}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return s, fmt.Errorf("spec must start with ---")
	}
	front, body, ok := strings.Cut(text[4:], "\n---")
	if !ok || (body != "" && !strings.HasPrefix(body, "\n")) {
		return s, fmt.Errorf("unclosed frontmatter")
	}
	d := yaml.NewDecoder(bytes.NewBufferString(front))
	d.KnownFields(true)
	if e := d.Decode(&s); e != nil {
		return s, e
	}
	var extra any
	if e := d.Decode(&extra); e != io.EOF {
		return s, fmt.Errorf("one frontmatter document required")
	}
	s.Brief = strings.TrimSpace(body)
	return s, s.Validate()
}
func (s OrchestratorSpec) Validate() error {
	if s.Kind != "orchestrator" || !ValidName(s.Name) || strings.TrimSpace(s.Goal) == "" {
		return fmt.Errorf("kind orchestrator, valid name and goal required")
	}
	if s.Mode != "once" && s.Mode != "loop" {
		return fmt.Errorf("mode must be once or loop")
	}
	if s.Harness != "codex" && s.Harness != "claude" {
		return fmt.Errorf("invalid harness")
	}
	if s.Idle != "skip" && s.Idle != "run" {
		return fmt.Errorf("idle must be skip or run")
	}
	if s.Source != "none" {
		return fmt.Errorf("source plugins are not supported yet")
	}
	if s.Mode == "loop" && s.Every == "" {
		return fmt.Errorf("loop requires every")
	}
	if s.Every != "" {
		if _, e := OrchestratorDuration(s.Every); e != nil {
			return e
		}
	}
	if s.MaxLifetime != "" {
		if _, e := OrchestratorDuration(s.MaxLifetime); e != nil {
			return e
		}
	}
	if s.Mode == "loop" && s.MaxLifetime == "" {
		return fmt.Errorf("loop requires max_lifetime")
	}
	if s.Until != "" && s.Until != "goal" && s.Until != "never" {
		if _, e := time.Parse("2006-01-02", s.Until); e != nil {
			return fmt.Errorf("until must be goal, never or a date")
		}
	}
	if s.Until == "never" && s.MaxLifetime == "" {
		return fmt.Errorf("until never requires max_lifetime")
	}
	r := s.Manages.Runs
	if (r.Selector == "") == (len(r.Names) == 0) {
		return fmt.Errorf("choose one runs selector or names")
	}
	if r.Selector != "" && r.Selector != "mine" {
		k, v, ok := strings.Cut(r.Selector, "=")
		if !ok || k != "issue" || v == "" {
			return fmt.Errorf("selector must be mine or issue=GLOB")
		}
		if _, e := filepath.Match(v, ""); e != nil {
			return e
		}
	}
	if r.Selector == "mine" && s.Reply == "" {
		return fmt.Errorf("mine requires reply")
	}
	seen := map[string]bool{}
	for _, n := range r.Names {
		if !ValidName(n) || seen[n] {
			return fmt.Errorf("invalid or duplicate run name")
		}
		seen[n] = true
	}
	for _, v := range s.Authority {
		switch v {
		case "say", "notify", "rework", "retry", "validate", "dispatch":
		default:
			return fmt.Errorf("unsupported authority %q", v)
		}
	}
	if s.Budget.ConcurrentRuns < 0 || s.Budget.RunsPerDay < 0 || s.Budget.USDPerDay < 0 || s.Handoff.RotateAfter < 0 {
		return fmt.Errorf("budget and rotate_after must be nonnegative")
	}
	for _, r := range s.Manages.Repos {
		if strings.TrimSpace(r) == "" {
			return fmt.Errorf("empty repo scope")
		}
	}
	return nil
}
func OrchestratorConfigDir() string      { return Expand("~/.config/hev/orchestrators") }
func OrchestratorDir(name string) string { return filepath.Join(Root(), "orch-"+name) }
func LoadOrchestrator(name string) (OrchestratorSpec, error) {
	if !ValidName(name) {
		return OrchestratorSpec{}, fmt.Errorf("invalid orchestrator name")
	}
	b, e := os.ReadFile(filepath.Join(OrchestratorConfigDir(), name+".md"))
	if e != nil {
		return OrchestratorSpec{}, e
	}
	return ParseOrchestrator(string(b))
}
func (s OrchestratorSpec) Includes(o Order) bool {
	if len(s.Manages.Repos) > 0 {
		found := false
		for _, r := range s.Manages.Repos {
			if r == o.Repo {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	if s.Manages.Runs.Selector == "mine" {
		return o.Reply == s.Reply
	}
	if p, ok := strings.CutPrefix(s.Manages.Runs.Selector, "issue="); ok {
		match, _ := filepath.Match(p, o.Issue)
		return match
	}
	for _, n := range s.Manages.Runs.Names {
		if n == o.Name {
			return true
		}
	}
	return false
}

// ValidateOrchestratorNesting checks the whole proposed graph, including parents
// already applied. Two orchestrators in a chain is the maximum depth.
func ValidateOrchestratorNesting(specs map[string]OrchestratorSpec) error {
	var visit func(string, map[string]bool, int) error
	visit = func(n string, path map[string]bool, depth int) error {
		if path[n] {
			return fmt.Errorf("orchestrator cycle at %s", n)
		}
		if depth > 2 {
			return fmt.Errorf("orchestrator nesting depth exceeds 2")
		}
		path[n] = true
		defer delete(path, n)
		s := specs[n]
		for _, r := range s.Manages.Runs.Names {
			child := strings.TrimPrefix(r, "orch-")
			if _, ok := specs[child]; !ok {
				if strings.HasPrefix(r, "orch-") {
					return fmt.Errorf("nested orchestrator %s is not applied", child)
				}
				continue
			}
			if !s.AllowNested {
				return fmt.Errorf("%s requires allow_nested", n)
			}
			if e := visit(child, path, depth+1); e != nil {
				return e
			}
		}
		return nil
	}
	for n := range specs {
		if e := visit(n, map[string]bool{}, 1); e != nil {
			return e
		}
	}
	return nil
}
