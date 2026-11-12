package dispatch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Swarm ceilings and defaults from RFC 0013, section 7.
const (
	SwarmMaxReplicas       = 8
	SwarmMaxUnits          = 32
	swarmDefaultEvery      = "30m"
	swarmDefaultReconcile  = 2 * time.Minute
	swarmDefaultLimit      = 3
	swarmDefaultBackoff    = 2 * time.Minute
	swarmDefaultMaxBackoff = 30 * time.Minute
)

// SwarmSpec is a kind: swarm file. The orchestrator block keeps RFC 0008's
// field names; role order fields are order frontmatter keys. Fields owned by
// later build phases decode so a file is portable, and up refuses them.
type SwarmSpec struct {
	Kind         string `yaml:"kind" json:"kind"`
	Name         string `yaml:"name" json:"name"`
	Goal         string `yaml:"goal" json:"goal"`
	Reply        string `yaml:"reply,omitempty" json:"reply,omitempty"`
	Host         string `yaml:"host,omitempty" json:"host,omitempty"`
	Every        string `yaml:"every,omitempty" json:"every,omitempty"`
	MaxLifetime  string `yaml:"max_lifetime,omitempty" json:"max_lifetime,omitempty"`
	Harness      string `yaml:"harness,omitempty" json:"harness,omitempty"`
	Model        string `yaml:"model,omitempty" json:"model,omitempty"`
	RoundTimeout string `yaml:"round_timeout,omitempty" json:"round_timeout,omitempty"`
	NotifyRoute  string `yaml:"notify_route,omitempty" json:"notify_route,omitempty"`
	Budget       struct {
		ConcurrentRuns int     `yaml:"concurrent_runs,omitempty" json:"concurrent_runs"`
		RunsPerDay     int     `yaml:"runs_per_day,omitempty" json:"runs_per_day"`
		USDPerDay      float64 `yaml:"usd_per_day,omitempty" json:"usd_per_day,omitempty"`
	} `yaml:"budget,omitempty" json:"budget"`
	ReconcileEvery string        `yaml:"reconcile_every,omitempty" json:"reconcile_every,omitempty"`
	Restart        SwarmRestart  `yaml:"restart,omitempty" json:"restart"`
	Roles          []SwarmRole   `yaml:"roles" json:"roles"`
	Standby        []string      `yaml:"standby,omitempty" json:"standby,omitempty"`
	StateRepo      string        `yaml:"state_repo,omitempty" json:"state_repo,omitempty"`
	HostGrace      string        `yaml:"host_grace,omitempty" json:"host_grace,omitempty"`
	Plans          *SwarmPlans   `yaml:"plans,omitempty" json:"plans,omitempty"`
	MaxRuns        int           `yaml:"max_runs,omitempty" json:"max_runs,omitempty"`
	Brief          string        `yaml:"-" json:"-"`
	Sections       swarmSections `yaml:"-" json:"-"`
}

type SwarmRestart struct {
	Limit      *int   `yaml:"limit,omitempty" json:"limit,omitempty"`
	Backoff    string `yaml:"backoff,omitempty" json:"backoff,omitempty"`
	MaxBackoff string `yaml:"max_backoff,omitempty" json:"max_backoff,omitempty"`
	SwarmLimit int    `yaml:"swarm_limit,omitempty" json:"swarm_limit,omitempty"`
}

type SwarmPlans struct {
	HoldAt   int `yaml:"hold_at,omitempty" json:"hold_at,omitempty"`
	ShrinkAt int `yaml:"shrink_at,omitempty" json:"shrink_at,omitempty"`
}

type SwarmRole struct {
	Name      string          `yaml:"name" json:"name"`
	Shape     string          `yaml:"shape" json:"shape"`
	Replicas  int             `yaml:"replicas,omitempty" json:"replicas"`
	Pieces    SwarmPieces     `yaml:"pieces,omitempty" json:"pieces,omitempty"`
	Variants  []SwarmOrder    `yaml:"variants,omitempty" json:"variants,omitempty"`
	Choose    string          `yaml:"choose,omitempty" json:"choose,omitempty"`
	After     []string        `yaml:"after,omitempty" json:"after,omitempty"`
	Effects   string          `yaml:"effects,omitempty" json:"effects"`
	Stall     string          `yaml:"stall,omitempty" json:"stall,omitempty"`
	Order     SwarmOrder      `yaml:"order" json:"order"`
	Placement *SwarmPlacement `yaml:"placement,omitempty" json:"placement,omitempty"`
}

// SwarmOrder holds the order keys a role may set. name, host, reply and the
// repeat keys belong to the swarm, so they are not fields here.
type SwarmOrder struct {
	Repo       string `yaml:"repo,omitempty" json:"repo,omitempty"`
	Base       string `yaml:"base,omitempty" json:"base,omitempty"`
	Harness    string `yaml:"harness,omitempty" json:"harness,omitempty"`
	Model      string `yaml:"model,omitempty" json:"model,omitempty"`
	Validation string `yaml:"validation,omitempty" json:"validation,omitempty"`
	Issue      string `yaml:"issue,omitempty" json:"issue,omitempty"`
	Linear     string `yaml:"linear,omitempty" json:"linear,omitempty"`
	Class      string `yaml:"class,omitempty" json:"class,omitempty"`
	Preview    string `yaml:"preview,omitempty" json:"preview,omitempty"`
	Timeout    int    `yaml:"timeout,omitempty" json:"timeout,omitempty"`
}

type SwarmPlacement struct {
	Hosts   []string `yaml:"hosts,omitempty" json:"hosts,omitempty"`
	OS      string   `yaml:"os,omitempty" json:"os,omitempty"`
	Spread  bool     `yaml:"spread,omitempty" json:"spread,omitempty"`
	Exclude []string `yaml:"exclude,omitempty" json:"exclude,omitempty"`
}

// SwarmPieces is a list of piece names, or the scalar orchestrator.
type SwarmPieces struct {
	Names        []string
	Orchestrator bool
}

func (p *SwarmPieces) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		if n.Value != "orchestrator" {
			return fmt.Errorf("pieces must be a list or orchestrator")
		}
		p.Orchestrator = true
		return nil
	}
	if n.Kind != yaml.SequenceNode {
		return fmt.Errorf("pieces must be a list or orchestrator")
	}
	return n.Decode(&p.Names)
}

func (p SwarmPieces) MarshalJSON() ([]byte, error) {
	if p.Orchestrator {
		return []byte(`"orchestrator"`), nil
	}
	if p.Names == nil {
		return []byte("null"), nil
	}
	return json.Marshal(p.Names)
}

func (p SwarmPieces) MarshalYAML() (any, error) {
	if p.Orchestrator {
		return "orchestrator", nil
	}
	return p.Names, nil
}

// swarmSections are the body parts a compiled order is assembled from.
type swarmSections struct {
	Problem    string
	Acceptance string
	Roles      map[string]swarmRoleSection
}

type swarmRoleSection struct {
	Brief      string
	Acceptance string
}

// ParseSwarm decodes a swarm file strictly and validates it. It accepts the
// fields of later phases; SwarmLaterPhase reports them.
func ParseSwarm(text string) (SwarmSpec, error) {
	var s SwarmSpec
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return s, fmt.Errorf("swarm file must start with ---")
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
	sections, e := parseSwarmBody(s.Brief)
	if e != nil {
		return s, e
	}
	s.Sections = sections
	return s, s.Validate()
}

// parseSwarmBody splits ## Problem, ## Acceptance and ## Role: NAME (with its
// ### Acceptance). Other sections stay in the orchestrator brief only.
func parseSwarmBody(body string) (swarmSections, error) {
	out := swarmSections{Roles: map[string]swarmRoleSection{}}
	var title string
	var buf []string
	flush := func() error {
		text := strings.TrimSpace(strings.Join(buf, "\n"))
		switch {
		case title == "Problem":
			out.Problem = text
		case title == "Acceptance":
			out.Acceptance = text
		case strings.HasPrefix(title, "Role:"):
			name := strings.TrimSpace(strings.TrimPrefix(title, "Role:"))
			if _, dup := out.Roles[name]; dup {
				return fmt.Errorf("duplicate ## Role: %s section", name)
			}
			brief, acceptance, _ := strings.Cut(text, "### Acceptance")
			out.Roles[name] = swarmRoleSection{Brief: strings.TrimSpace(brief), Acceptance: strings.TrimSpace(acceptance)}
		}
		return nil
	}
	fence := false
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fence = !fence
		}
		if !fence && strings.HasPrefix(line, "## ") {
			if e := flush(); e != nil {
				return out, e
			}
			title, buf = strings.TrimSpace(line[3:]), nil
			continue
		}
		buf = append(buf, line)
	}
	return out, flush()
}

func (s SwarmSpec) Validate() error {
	if s.Kind != "swarm" {
		return fmt.Errorf("kind must be swarm")
	}
	if !ValidName(s.Name) {
		return fmt.Errorf("invalid swarm name %q", s.Name)
	}
	if strings.TrimSpace(s.Goal) == "" {
		return fmt.Errorf("goal required")
	}
	if strings.TrimSpace(s.Reply) == "" {
		return fmt.Errorf("reply required: the swarm stops and tells reply when it cannot repair")
	}
	if s.MaxLifetime == "" {
		return fmt.Errorf("max_lifetime required")
	}
	if s.Budget.ConcurrentRuns <= 0 {
		return fmt.Errorf("budget.concurrent_runs must be positive")
	}
	for _, f := range []struct{ name, v string }{{"every", s.Every}, {"max_lifetime", s.MaxLifetime}, {"reconcile_every", s.ReconcileEvery}, {"host_grace", s.HostGrace}, {"restart.backoff", s.Restart.Backoff}, {"restart.max_backoff", s.Restart.MaxBackoff}} {
		if f.v == "" {
			continue
		}
		if _, e := OrchestratorDuration(f.v); e != nil {
			return fmt.Errorf("%s: %w", f.name, e)
		}
	}
	if s.Restart.Limit != nil && *s.Restart.Limit < 0 {
		return fmt.Errorf("restart.limit must be nonnegative")
	}
	if s.Restart.SwarmLimit < 0 || s.MaxRuns < 0 {
		return fmt.Errorf("restart.swarm_limit and max_runs must be nonnegative")
	}
	if s.Plans != nil && (s.Plans.HoldAt < 0 || s.Plans.HoldAt > 100 || s.Plans.ShrinkAt < 0 || s.Plans.ShrinkAt > 100) {
		return fmt.Errorf("plans thresholds are percentages")
	}
	if s.Sections.Problem == "" {
		return fmt.Errorf("body needs a ## Problem section")
	}
	if len(s.Roles) == 0 {
		return fmt.Errorf("at least one role required")
	}
	roles := map[string]SwarmRole{}
	units, prRoles := 0, 0
	for _, r := range s.Roles {
		if !ValidLabel(r.Name) {
			return fmt.Errorf("invalid role name %q", r.Name)
		}
		if _, dup := roles[r.Name]; dup {
			return fmt.Errorf("duplicate role %s", r.Name)
		}
		roles[r.Name] = r
		if e := r.validate(); e != nil {
			return fmt.Errorf("role %s: %w", r.Name, e)
		}
		sec, ok := s.Sections.Roles[r.Name]
		if !ok || sec.Acceptance == "" {
			return fmt.Errorf("role %s needs a ## Role: %s section with ### Acceptance", r.Name, r.Name)
		}
		if r.effects() == "pr" {
			prRoles++
		}
		units += len(r.units())
	}
	for name := range s.Sections.Roles {
		if _, ok := roles[name]; !ok {
			return fmt.Errorf("## Role: %s names no role", name)
		}
	}
	if prRoles > 1 {
		return fmt.Errorf("at most one effects: pr role; a swarm opens one draft PR")
	}
	if units > SwarmMaxUnits {
		return fmt.Errorf("%d units exceed the ceiling of %d per swarm", units, SwarmMaxUnits)
	}
	for _, r := range s.Roles {
		for _, a := range r.After {
			if a == r.Name {
				return fmt.Errorf("role %s: after names itself", r.Name)
			}
			if _, ok := roles[a]; !ok {
				return fmt.Errorf("role %s: after names unknown role %s", r.Name, a)
			}
		}
	}
	if cycle := swarmAfterCycle(s.Roles); cycle != "" {
		return fmt.Errorf("cycle in after: %s", cycle)
	}
	for _, r := range s.Roles {
		for _, u := range r.units() {
			if n := swarmRunName(s.Name, r, u, 1); !ValidName(n) {
				return fmt.Errorf("role %s unit %s: run name %s is invalid", r.Name, u, n)
			}
		}
	}
	return nil
}

func (r SwarmRole) validate() error {
	switch r.Shape {
	case "partition", "race", "single":
	default:
		return fmt.Errorf("shape must be partition, race or single")
	}
	if r.Replicas < 0 {
		return fmt.Errorf("replicas must be positive")
	}
	if r.Replicas > SwarmMaxReplicas {
		return fmt.Errorf("replicas %d exceed the ceiling of %d", r.Replicas, SwarmMaxReplicas)
	}
	if r.Effects != "" && r.Effects != "branch" && r.Effects != "pr" {
		return fmt.Errorf("effects must be branch or pr")
	}
	if r.effects() == "pr" && r.Shape != "single" {
		return fmt.Errorf("effects: pr requires shape single")
	}
	if r.Stall != "" {
		if _, e := OrchestratorDuration(r.Stall); e != nil {
			return fmt.Errorf("stall: %w", e)
		}
	}
	if (len(r.Pieces.Names) > 0 || r.Pieces.Orchestrator) != (r.Shape == "partition") {
		return fmt.Errorf("pieces belong to partition roles, and a partition needs pieces")
	}
	if r.Shape == "partition" && !r.Pieces.Orchestrator {
		seen := map[string]bool{}
		for _, p := range r.Pieces.Names {
			if !ValidLabel(p) || seen[p] {
				return fmt.Errorf("invalid or duplicate piece %q", p)
			}
			seen[p] = true
		}
	}
	if len(r.Variants) > 0 && r.Shape != "race" {
		return fmt.Errorf("variants belong to race roles")
	}
	if r.Choose != "" && r.Shape != "race" {
		return fmt.Errorf("choose belongs to race roles")
	}
	switch r.Choose {
	case "", "orchestrator", "first-green", "person":
	default:
		return fmt.Errorf("choose must be orchestrator, first-green or person")
	}
	if r.Shape == "race" && len(r.Variants) == 0 && r.Replicas < 2 {
		return fmt.Errorf("a race needs variants or replicas of at least 2")
	}
	if r.Shape == "single" && r.Replicas > 1 {
		return fmt.Errorf("a single role runs one unit")
	}
	if len(r.Variants) > SwarmMaxReplicas*SwarmMaxUnits {
		return fmt.Errorf("too many variants")
	}
	return nil
}

func (r SwarmRole) effects() string {
	if r.Effects == "" {
		return "branch"
	}
	return r.Effects
}

func (r SwarmRole) choose() string {
	if r.Choose == "" {
		return "orchestrator"
	}
	return r.Choose
}

// units names the role's units: pieces, v1..vN, or "" for a single.
func (r SwarmRole) units() []string {
	switch r.Shape {
	case "partition":
		return r.Pieces.Names
	case "race":
		n := len(r.Variants)
		if n == 0 {
			n = r.Replicas
		}
		out := make([]string, n)
		for i := range out {
			out[i] = "v" + strconv.Itoa(i+1)
		}
		return out
	}
	return []string{""}
}

// replicas is the concurrency limit; a race defaults to all its variants.
func (r SwarmRole) replicas() int {
	if r.Replicas > 0 {
		return r.Replicas
	}
	if r.Shape == "race" {
		return min(len(r.units()), SwarmMaxReplicas)
	}
	return 1
}

func swarmRunName(swarm string, r SwarmRole, unit string, gen int) string {
	n := swarm + "-" + r.Name
	if unit != "" {
		n += "-" + unit
	}
	return n + "-g" + strconv.Itoa(gen)
}

func swarmAfterCycle(roles []SwarmRole) string {
	after := map[string][]string{}
	for _, r := range roles {
		after[r.Name] = r.After
	}
	state := map[string]int{}
	var path []string
	var visit func(string) string
	visit = func(n string) string {
		switch state[n] {
		case 1:
			i := slices.Index(path, n)
			return strings.Join(append(slices.Clone(path[i:]), n), " -> ")
		case 2:
			return ""
		}
		state[n] = 1
		path = append(path, n)
		for _, a := range after[n] {
			if c := visit(a); c != "" {
				return c
			}
		}
		path = path[:len(path)-1]
		state[n] = 2
		return ""
	}
	for _, r := range roles {
		if c := visit(r.Name); c != "" {
			return c
		}
	}
	return ""
}

// SwarmLaterPhase names the first field this build cannot honour. up refuses
// such a file rather than silently ignoring the field.
func SwarmLaterPhase(s SwarmSpec, localHost string) error {
	need := func(field string, phase int, what string) error {
		return fmt.Errorf("%s needs swarm phase %d (%s); this build implements phase 1, a single-host swarm", field, phase, what)
	}
	switch {
	case len(s.Standby) > 0:
		return need("standby", 4, "durable owner and takeover")
	case s.StateRepo != "":
		return need("state_repo", 4, "durable owner and lease")
	case s.HostGrace != "":
		return need("host_grace", 3, "cross-host placement and host-loss repair")
	case s.Plans != nil:
		return need("plans", 2, "bounds and scale")
	case s.MaxRuns != 0:
		return need("max_runs", 2, "bounds and scale")
	case s.Restart.SwarmLimit != 0:
		return need("restart.swarm_limit", 2, "bounds and scale")
	}
	for _, r := range s.Roles {
		if r.Pieces.Orchestrator {
			return need("role "+r.Name+": pieces: orchestrator", 2, "bounds and scale")
		}
		p := r.Placement
		if p == nil {
			continue
		}
		switch {
		case p.Spread:
			return need("role "+r.Name+": placement.spread", 3, "cross-host placement")
		case len(p.Exclude) > 0:
			return need("role "+r.Name+": placement.exclude", 3, "cross-host placement")
		case p.OS != "" && p.OS != runtime.GOOS:
			return need("role "+r.Name+": placement.os "+p.OS, 3, "cross-host placement")
		}
		for _, h := range p.Hosts {
			if h != localHost && h != s.Host {
				return need("role "+r.Name+": placement.hosts "+h, 3, "cross-host placement")
			}
		}
	}
	return nil
}

func (s SwarmSpec) restartLimit() int {
	if s.Restart.Limit != nil {
		return *s.Restart.Limit
	}
	return swarmDefaultLimit
}

func (s SwarmSpec) backoff(restarts int) time.Duration {
	base, ceiling := swarmDefaultBackoff, swarmDefaultMaxBackoff
	if s.Restart.Backoff != "" {
		base, _ = OrchestratorDuration(s.Restart.Backoff)
	}
	if s.Restart.MaxBackoff != "" {
		ceiling, _ = OrchestratorDuration(s.Restart.MaxBackoff)
	}
	d := base
	for i := 0; i < restarts && d < ceiling; i++ {
		d *= 2
	}
	return min(d, ceiling)
}

func (s SwarmSpec) reconcileEvery() time.Duration {
	if s.ReconcileEvery == "" {
		return swarmDefaultReconcile
	}
	d, _ := OrchestratorDuration(s.ReconcileEvery)
	return d
}

func (s SwarmSpec) role(name string) (SwarmRole, bool) {
	for _, r := range s.Roles {
		if r.Name == name {
			return r, true
		}
	}
	return SwarmRole{}, false
}

// CompileSwarmSpec renders the orchestrator spec RFC 0008 already runs.
func CompileSwarmSpec(s SwarmSpec) (string, error) {
	type flowBudget struct {
		ConcurrentRuns int     `yaml:"concurrent_runs,omitempty"`
		RunsPerDay     int     `yaml:"runs_per_day,omitempty"`
		USDPerDay      float64 `yaml:"usd_per_day,omitempty"`
	}
	type runs struct {
		Selector string `yaml:"selector"`
	}
	type manages struct {
		Runs  runs     `yaml:"runs,flow"`
		Repos []string `yaml:"repos,flow,omitempty"`
	}
	front := struct {
		Kind         string     `yaml:"kind"`
		Name         string     `yaml:"name"`
		Goal         string     `yaml:"goal"`
		Host         string     `yaml:"host,omitempty"`
		Mode         string     `yaml:"mode"`
		Every        string     `yaml:"every"`
		Until        string     `yaml:"until"`
		MaxLifetime  string     `yaml:"max_lifetime"`
		Harness      string     `yaml:"harness,omitempty"`
		Model        string     `yaml:"model,omitempty"`
		RoundTimeout string     `yaml:"round_timeout,omitempty"`
		Reply        string     `yaml:"reply"`
		NotifyRoute  string     `yaml:"notify_route,omitempty"`
		Manages      manages    `yaml:"manages"`
		Authority    []string   `yaml:"authority,flow"`
		Budget       flowBudget `yaml:"budget,flow"`
		Idle         string     `yaml:"idle"`
	}{Kind: "orchestrator", Name: s.Name, Goal: s.Goal, Host: s.Host, Mode: "loop", Every: s.Every, Until: "goal", MaxLifetime: s.MaxLifetime, Harness: s.Harness, Model: s.Model, RoundTimeout: s.RoundTimeout, Reply: s.Reply, NotifyRoute: s.NotifyRoute, Idle: "skip"}
	if front.Every == "" {
		front.Every = swarmDefaultEvery
	}
	front.Manages.Runs.Selector = "swarm=" + s.Name
	front.Authority = []string{"say", "notify", "rework", "retry", "dispatch", "stop"}
	for _, r := range s.Roles {
		for _, o := range append([]SwarmOrder{r.Order}, r.Variants...) {
			if o.Repo != "" && !slices.Contains(front.Manages.Repos, o.Repo) {
				front.Manages.Repos = append(front.Manages.Repos, o.Repo)
			}
			if o.Validation == "browser" && !slices.Contains(front.Authority, "validate") {
				front.Authority = append(front.Authority, "validate")
			}
		}
	}
	front.Budget = flowBudget{s.Budget.ConcurrentRuns, s.Budget.RunsPerDay, s.Budget.USDPerDay}
	b, e := yaml.Marshal(front)
	if e != nil {
		return "", e
	}
	return "---\n" + string(b) + "---\n" + swarmOrchestratorBrief(s), nil
}

func swarmOrchestratorBrief(s SwarmSpec) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You orchestrate swarm %s. Code launches, observes and repairs its units every reconcile tick; you judge. Never write code, merge, or launch units yourself: the reconcile tick owns launches. The unit table is ../../swarm.json and packet.json carries it as swarm.\n\n", s.Name)
	b.WriteString("Your judgement:\n")
	b.WriteString("- Race roles with choose: orchestrator: when the tick reports a race ready, compare the variants' reports and branches and write choice.json in this round directory: {\"role\":\"ROLE\",\"unit\":\"vN\",\"reason\":\"...\"}. Only a done unit with a pushed head can be chosen, and a role is chosen once.\n")
	b.WriteString("- Held units (restart limit, blocked, needs-review, a second stall) wait for you or a person. Retry or rework them through dispatch, or explain them in reply.md.\n")
	b.WriteString("- When the swarm acceptance is met, write goal-met.json and a reply.md with the PR or chosen branch and its head.\n\n")
	b.WriteString(s.Brief)
	b.WriteString("\n")
	return b.String()
}

// swarmOrderText assembles one unit's flat order: the role's order keys plus
// name, host and reply; the body is Problem, the Role section, the unit
// assignment and the role's acceptance as ## Acceptance.
func swarmOrderText(s SwarmSpec, r SwarmRole, unit string, gen int, host string, inputs []swarmInput) string {
	o := r.Order
	if r.Shape == "race" && len(r.Variants) > 0 {
		i, _ := strconv.Atoi(strings.TrimPrefix(unit, "v"))
		if i >= 1 && i <= len(r.Variants) {
			o = mergeSwarmOrder(o, r.Variants[i-1])
		}
	}
	name := swarmRunName(s.Name, r, unit, gen)
	var b strings.Builder
	b.WriteString("---\n")
	field := func(k, v string) {
		if v != "" {
			fmt.Fprintf(&b, "%s: %s\n", k, swarmScalar(v))
		}
	}
	field("name", name)
	field("host", host)
	field("reply", s.Reply)
	field("repo", o.Repo)
	field("base", o.Base)
	field("harness", o.Harness)
	field("model", o.Model)
	field("validation", o.Validation)
	field("issue", o.Issue)
	field("linear", o.Linear)
	field("class", o.Class)
	field("preview", o.Preview)
	if o.Timeout > 0 {
		field("timeout", strconv.Itoa(o.Timeout))
	}
	b.WriteString("---\n")
	sec := s.Sections.Roles[r.Name]
	fmt.Fprintf(&b, "## Problem\n\n%s\n\n", s.Sections.Problem)
	fmt.Fprintf(&b, "## Role: %s\n\n", r.Name)
	if sec.Brief != "" {
		b.WriteString(sec.Brief + "\n\n")
	}
	b.WriteString("## Assignment\n\n")
	fmt.Fprintf(&b, "Swarm `%s`, role `%s` (%s)", s.Name, r.Name, r.Shape)
	switch r.Shape {
	case "partition":
		fmt.Fprintf(&b, ", piece `%s`", unit)
	case "race":
		fmt.Fprintf(&b, ", variant `%s` of %d. Every variant gets the same task; one is chosen", unit, len(r.units()))
	}
	fmt.Fprintf(&b, ", generation %d. Your branch is `d/%s`.\n", gen, name)
	if r.Shape == "partition" {
		fmt.Fprintf(&b, "\nPiece: %s\n", unit)
	}
	if len(inputs) > 0 {
		b.WriteString("\nInputs, the branches of finished units (current generation only):\n\n")
		for _, in := range inputs {
			head := in.Head
			if head == "" {
				head = "its pushed head"
			}
			fmt.Fprintf(&b, "- `%s` (%s/%s) at %s\n", in.Branch, in.Role, swarmUnitLabel(in.Unit), head)
		}
	}
	b.WriteString("\n## Swarm rules\n\n")
	if r.effects() == "branch" {
		fmt.Fprintf(&b, "- Commit and push your branch `d/%s` after every commit. Do not open a pull request, comment on one, deploy, or message people: this order overrides the default to open a draft PR. Write ../report.md as usual.\n", name)
	} else {
		b.WriteString("- You produce the swarm's one draft pull request. Push your branch after every commit and open exactly one draft PR.\n")
	}
	fmt.Fprintf(&b, "- Send findings other units need with `hev dispatch say o-%s \"...\"`.\n", s.Name)
	b.WriteString("- Your unit is complete only when this run ends done with its head pushed.\n")
	fmt.Fprintf(&b, "\n## Acceptance\n\n%s\n", sec.Acceptance)
	return b.String()
}

type swarmInput struct {
	Role, Unit, Branch, Head string
}

func mergeSwarmOrder(base, v SwarmOrder) SwarmOrder {
	pick := func(a, b string) string {
		if b != "" {
			return b
		}
		return a
	}
	base.Repo, base.Base, base.Harness, base.Model = pick(base.Repo, v.Repo), pick(base.Base, v.Base), pick(base.Harness, v.Harness), pick(base.Model, v.Model)
	base.Validation, base.Issue, base.Linear = pick(base.Validation, v.Validation), pick(base.Issue, v.Issue), pick(base.Linear, v.Linear)
	base.Class, base.Preview = pick(base.Class, v.Class), pick(base.Preview, v.Preview)
	if v.Timeout > 0 {
		base.Timeout = v.Timeout
	}
	return base
}

// swarmScalar quotes a value the flat order parser would otherwise reject.
func swarmScalar(v string) string {
	if strings.ContainsAny(v, "#[]{}\t'\"") || strings.TrimSpace(v) != v {
		if !strings.Contains(v, "\"") {
			return "\"" + v + "\""
		}
		return "'" + v + "'"
	}
	return v
}

func swarmUnitLabel(u string) string {
	if u == "" {
		return "-"
	}
	return u
}

// SwarmPlan is what up compiles: the orchestrator spec and one order per unit.
type SwarmPlan struct {
	Spec      SwarmSpec
	Host      string
	OrchSpec  string
	Orch      OrchestratorSpec
	Orders    []SwarmPlannedOrder
	LaterNote string
}

type SwarmPlannedOrder struct {
	Role, Unit, Run, Text string
}

// CompileSwarm validates a swarm file for this host and renders every order.
// Orders for after roles list their inputs' planned branches.
func CompileSwarm(text string) (SwarmPlan, error) {
	s, e := ParseSwarm(text)
	if e != nil {
		return SwarmPlan{}, e
	}
	host, e := swarmLocalHostName(s)
	if e != nil {
		return SwarmPlan{}, e
	}
	if e = SwarmLaterPhase(s, host); e != nil {
		return SwarmPlan{}, e
	}
	p := SwarmPlan{Spec: s, Host: host}
	if p.OrchSpec, e = CompileSwarmSpec(s); e != nil {
		return p, e
	}
	if p.Orch, e = ParseOrchestrator(p.OrchSpec); e != nil {
		return p, fmt.Errorf("compiled orchestrator spec: %w", e)
	}
	for _, r := range s.Roles {
		var inputs []swarmInput
		for _, a := range r.After {
			up, _ := s.role(a)
			for _, u := range up.units() {
				if up.Shape == "race" {
					inputs = append(inputs, swarmInput{Role: a, Unit: "(chosen variant)", Branch: "d/" + swarmRunName(s.Name, up, "vN", 1)})
					break
				}
				inputs = append(inputs, swarmInput{Role: a, Unit: u, Branch: "d/" + swarmRunName(s.Name, up, u, 1)})
			}
		}
		for _, u := range r.units() {
			t := swarmOrderText(s, r, u, 1, host, inputs)
			if _, e := Parse(t); e != nil {
				return p, fmt.Errorf("role %s unit %s: compiled order: %w", r.Name, swarmUnitLabel(u), e)
			}
			p.Orders = append(p.Orders, SwarmPlannedOrder{Role: r.Name, Unit: u, Run: swarmRunName(s.Name, r, u, 1), Text: t})
		}
	}
	return p, nil
}

// swarmLocalHostName is the label runs get on this host. Phase 1 places every
// unit here, so a host field naming a remote dispatch host is refused.
func swarmLocalHostName(s SwarmSpec) (string, error) {
	cfg, e := Load(ConfigPath(""))
	if e != nil {
		cfg = Config{}
	}
	if s.Host != "" {
		if h, ok := cfg.Hosts[s.Host]; ok && !h.Local {
			return "", fmt.Errorf("host %s is a remote dispatch host; an owner on another host needs swarm phase 3; run up on that host", s.Host)
		}
		return s.Host, nil
	}
	for n, h := range cfg.Hosts {
		if h.Local {
			return n, nil
		}
	}
	return "", nil
}
