// Package subscriptions reads which flat-rate agent plans (a Claude
// subscription, a ChatGPT plan for Codex) this machine is signed in to, and
// how much of each usage window is used. It is the plan side of kit; the
// spend side stays in internal/cost and subscriptions.toml.
//
// Readings come from local files where the harness already records them
// (Codex rollouts) or from one rate-limited provider request whose last good
// answer is cached (Claude). Credentials are read at use and never stored,
// logged or returned.
package subscriptions

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Schema names the JSON shape of `hev subscriptions --json`. Bump the version
// on any change that removes or retypes a field.
const Schema = "hev.subscriptions/v1"

// Login states.
const (
	SignedIn     = "signed_in"
	NotSignedIn  = "not_signed_in"
	LoginExpired = "login_expired"
	Unknown      = "unknown"
)

// Meter kinds.
const (
	KindSession      = "session"       // short rolling window (about five hours)
	KindWeekly       = "weekly"        // the plan-wide weekly window
	KindWeeklyScoped = "weekly_scoped" // a weekly window for one model
	KindWindow       = "window"        // a window of another length
	KindCredits      = "credits"       // a credit balance
	KindExtraUsage   = "extra_usage"   // paid overflow beyond the plan
)

// Meter is one usage window or balance. Percent is nil when it is not known,
// and PercentReason then says why; an unknown value is never reported as 0.
type Meter struct {
	Kind          string     `json:"kind"`
	Label         string     `json:"label"`
	Scope         string     `json:"scope,omitempty"`
	WindowMinutes int        `json:"window_minutes,omitempty"`
	Percent       *float64   `json:"percent"`
	PercentReason string     `json:"percent_reason,omitempty"`
	ResetsAt      *time.Time `json:"resets_at"`
	ResetReason   string     `json:"resets_at_reason,omitempty"`
	// RolledOver marks a window whose reading predates its reset: it is
	// reported as 0 used until a newer reading arrives.
	RolledOver bool   `json:"rolled_over,omitempty"`
	Detail     string `json:"detail,omitempty"`
}

// Status is everything known about one subscription on this machine.
type Status struct {
	Name           string     `json:"name"`
	Harness        string     `json:"harness"`
	Plan           *string    `json:"plan"`
	PlanReason     string     `json:"plan_reason,omitempty"`
	State          string     `json:"state"`
	StateReason    string     `json:"state_reason,omitempty"`
	Meters         []Meter    `json:"meters"`
	Weekly         *Meter     `json:"weekly"`
	WeeklyReason   string     `json:"weekly_reason,omitempty"`
	Source         string     `json:"source"`
	ObservedAt     *time.Time `json:"observed_at"`
	AgeSeconds     *int64     `json:"age_seconds"`
	ObservedReason string     `json:"observed_reason,omitempty"`
	Stale          bool       `json:"stale"`
	StaleReason    string     `json:"stale_reason,omitempty"`
	// ConfiguredPlans names subscriptions.toml plans on the same harness.
	ConfiguredPlans []string `json:"configured_plans"`
	Host            string   `json:"host,omitempty"`
}

// List is the --json shape of `hev subscriptions ls`.
type List struct {
	Schema        string    `json:"schema"`
	GeneratedAt   time.Time `json:"generated_at"`
	Subscriptions []Status  `json:"subscriptions"`
}

// Detail is the --json shape of `hev subscriptions show NAME`.
type Detail struct {
	Schema       string    `json:"schema"`
	GeneratedAt  time.Time `json:"generated_at"`
	Subscription Status    `json:"subscription"`
}

// Env is where a read looks. Tests replace every field; nothing in it
// defaults to the network or a real keychain inside a test.
type Env struct {
	Home     string
	StateDir string // kit's state directory, ~/.hev
	Now      func() time.Time
	Getenv   func(string) string
	// Keychain returns the Claude Code credential JSON from the OS keychain,
	// or an error when there is none. Nil means no keychain.
	Keychain func(ctx context.Context) ([]byte, error)
	HTTP     *http.Client
	// ClaudeUsageURL overrides the Claude OAuth usage endpoint.
	ClaudeUsageURL string
	// MinInterval is the minimum age before a cached provider reading is
	// read again. Zero means DefaultMinInterval.
	MinInterval time.Duration
	// Refresh forces one provider request regardless of the cache.
	Refresh bool
	// Plans are subscriptions.toml plans, used only to name the matching one.
	Plans []ConfiguredPlan
}

// DefaultEnv reads this machine as the current user.
func DefaultEnv() (Env, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Env{}, err
	}
	return Env{
		Home:     home,
		StateDir: filepath.Join(home, ".hev"),
		Now:      time.Now,
		Getenv:   os.Getenv,
		Keychain: systemKeychain,
		HTTP:     &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (e Env) now() time.Time {
	if e.Now == nil {
		return time.Now()
	}
	return e.Now()
}

func (e Env) getenv(k string) string {
	if e.Getenv == nil {
		return ""
	}
	return e.Getenv(k)
}

func (e Env) minInterval() time.Duration {
	if e.MinInterval <= 0 {
		return DefaultMinInterval
	}
	return e.MinInterval
}

// Provider checks one subscription.
type Provider struct {
	Name    string
	Harness string
	Read    func(ctx context.Context, env Env) Status
}

// Providers lists every subscription kit knows how to check, by name. A new
// subscription is one Provider here and its Read function.
func Providers() []Provider {
	p := []Provider{
		{Name: "claude", Harness: "claude", Read: readClaude},
		{Name: "codex", Harness: "codex", Read: readCodex},
	}
	sort.Slice(p, func(i, j int) bool { return p[i].Name < p[j].Name })
	return p
}

// Names lists the valid subscription names.
func Names() []string {
	out := []string{}
	for _, p := range Providers() {
		out = append(out, p.Name)
	}
	return out
}

// UnknownNameError is returned for a name kit cannot check.
type UnknownNameError struct{ Name string }

func (e UnknownNameError) Error() string {
	return fmt.Sprintf("unknown subscription %q; valid names: %s", e.Name, strings.Join(Names(), ", "))
}

// Lookup finds a provider by name.
func Lookup(name string) (Provider, error) {
	for _, p := range Providers() {
		if p.Name == strings.ToLower(strings.TrimSpace(name)) {
			return p, nil
		}
	}
	return Provider{}, UnknownNameError{Name: name}
}

// ReadAll reads every subscription.
func ReadAll(ctx context.Context, env Env) List {
	l := List{Schema: Schema, GeneratedAt: env.now().UTC(), Subscriptions: []Status{}}
	for _, p := range Providers() {
		l.Subscriptions = append(l.Subscriptions, read(ctx, env, p))
	}
	return l
}

// ReadOne reads one subscription by name.
func ReadOne(ctx context.Context, env Env, name string) (Detail, error) {
	p, err := Lookup(name)
	if err != nil {
		return Detail{}, err
	}
	return Detail{Schema: Schema, GeneratedAt: env.now().UTC(), Subscription: read(ctx, env, p)}, nil
}

func read(ctx context.Context, env Env, p Provider) Status {
	s := p.Read(ctx, env)
	s.Name, s.Harness = p.Name, p.Harness
	if s.Meters == nil {
		s.Meters = []Meter{}
	}
	if s.Plan == nil && s.PlanReason == "" {
		switch s.State {
		case NotSignedIn:
			s.PlanReason = "not signed in"
		default:
			s.PlanReason = "the provider did not report a plan tier"
		}
	}
	now := env.now()
	if s.ObservedAt != nil {
		t := s.ObservedAt.UTC()
		s.ObservedAt = &t
		age := int64(now.Sub(t).Seconds())
		if age < 0 {
			age = 0
		}
		s.AgeSeconds = &age
	} else if s.ObservedReason == "" {
		s.ObservedReason = "no reading"
	}
	for i := range s.Meters {
		if s.Meters[i].Percent == nil && s.Meters[i].PercentReason == "" {
			s.Meters[i].PercentReason = "the provider did not report a percentage"
		}
		if s.Meters[i].ResetsAt == nil && s.Meters[i].ResetReason == "" {
			s.Meters[i].ResetReason = "the provider did not report a reset time"
		}
		if r := s.Meters[i].ResetsAt; r != nil {
			t := r.UTC()
			s.Meters[i].ResetsAt = &t
			// A reading taken before its window reset says nothing about the
			// new window except that it started at 0.
			if !t.After(now) && s.Meters[i].Percent != nil {
				s.Meters[i].RolledOver = true
				s.Meters[i].Percent = ptr(0.0)
				s.Meters[i].ResetsAt = nil
				s.Meters[i].ResetReason = "window reset at " + t.Format(time.RFC3339) + " after this reading; next reset unknown until a newer reading"
			}
		}
	}
	for i := range s.Meters {
		if s.Meters[i].Kind == KindWeekly {
			m := s.Meters[i]
			s.Weekly = &m
			break
		}
	}
	if s.Weekly == nil && s.WeeklyReason == "" {
		switch {
		case s.State == NotSignedIn:
			s.WeeklyReason = "not signed in"
		case s.State == LoginExpired:
			s.WeeklyReason = "login expired"
		case len(s.Meters) == 0:
			s.WeeklyReason = "no reading"
		default:
			s.WeeklyReason = "the provider did not report a weekly window"
		}
	}
	s.ConfiguredPlans = []string{}
	for _, c := range env.Plans {
		if c.Harness == p.Harness {
			s.ConfiguredPlans = append(s.ConfiguredPlans, c.Name)
		}
	}
	return s
}

func ptr[T any](v T) *T { return &v }
