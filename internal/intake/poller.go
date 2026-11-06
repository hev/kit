package intake

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Cached struct {
	Updated, State                   string
	AssignmentActor, TransitionActor string
	Checked                          time.Time
	Entered                          time.Time
	AuthorizerBot                    string
	HumanVersion                     int
	NonHumanEvent                    bool
	Allowed                          bool
	Reason                           string
}
type Decision struct {
	Keys            []string `json:"keys,omitempty"`
	AssignmentActor string   `json:"assignment_actor,omitempty"`
	TransitionActor string   `json:"transition_actor,omitempty"`
	Identifier      string   `json:"identifier,omitempty"`
	URL             string   `json:"url,omitempty"`
	Status          string   `json:"status,omitempty"`
	Action          string   `json:"action,omitempty"`
	Name            string   `json:"name,omitempty"`
	Decision        string   `json:"decision"`
	Reason          string   `json:"reason,omitempty"`
}
type Daily struct {
	Requests   int        `json:"linear_requests"`
	Dispatches int        `json:"live_dispatches"`
	Decisions  []Decision `json:"decisions"`
}
type TicketFailure struct {
	Count, Skip int
}
type BatchRun struct {
	Rule, Name, Keys string
	At               time.Time
}
type State struct {
	Effects     map[string]string
	Sources     SourceState
	CommentRuns map[string]*CommentRun

	Batches        []BatchRun
	PendingSince   map[string]time.Time
	NeedsReconcile map[string]string
	TicketFailures map[string]TicketFailure
	LedgerOffset   int
	Starts         []time.Time
	LastPoll       time.Time
	BackoffUntil   time.Time
	SkipNext       bool
	Failures       int
	Days           map[string]*Daily
	Cache          map[string]Cached
	Pending        map[string]string
}
type Poller struct {
	cfg           Config
	key, endpoint string
	client        *http.Client
	now           func() time.Time
	out           io.Writer
	state         State
	active        map[string]bool
	admitted      map[string]bool
	command       func(context.Context, ...string) ([]byte, error)
}

func newPoller(c Config, key string, out io.Writer) (*Poller, error) {
	p := &Poller{cfg: c, key: key, endpoint: endpoint, client: &http.Client{Timeout: 30 * time.Second}, now: time.Now, out: out, admitted: map[string]bool{}}
	p.command = func(ctx context.Context, args ...string) ([]byte, error) {
		a := append(append([]string{}, c.Dispatch...), args...)
		cmd := exec.CommandContext(ctx, a[0], a[1:]...)
		for _, entry := range os.Environ() {
			if !secretEntry(entry, c.Linear.APIKeyEnv) && (c.Email.TokenEnv == "" || !secretEntry(entry, c.Email.TokenEnv)) {
				cmd.Env = append(cmd.Env, entry)
			}
		}
		return cmd.Output()
	}
	b, e := os.ReadFile(filepath.Join(c.StateDir, "state.json"))
	if e == nil {
		if e = json.Unmarshal(b, &p.state); e != nil {
			return nil, fmt.Errorf("invalid state: %w", e)
		}
	} else if !os.IsNotExist(e) {
		return nil, e
	}
	if p.state.Days == nil {
		p.state.Days = map[string]*Daily{}
	}
	if p.state.Cache == nil {
		p.state.Cache = map[string]Cached{}
	}
	if p.state.Pending == nil {
		p.state.Pending = map[string]string{}
	}

	if p.state.PendingSince == nil {
		p.state.PendingSince = map[string]time.Time{}
	}
	if p.state.NeedsReconcile == nil {
		p.state.NeedsReconcile = map[string]string{}
	}
	if p.state.TicketFailures == nil {
		p.state.TicketFailures = map[string]TicketFailure{}
	}
	b, e = os.ReadFile(filepath.Join(c.StateDir, "admitted.jsonl"))
	if e != nil && !os.IsNotExist(e) {
		return nil, e
	}
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if line != "" {
			p.admitted[line] = true
		}
	}
	if e = p.replayReservations(); e != nil {
		return nil, e
	}
	return p, nil
}
func (p *Poller) day() *Daily {
	key := p.now().UTC().Format("2006-01-02")
	if p.state.Days[key] == nil {
		p.state.Days[key] = &Daily{}
	}
	return p.state.Days[key]
}
func (p *Poller) save() error {
	if e := os.MkdirAll(p.cfg.StateDir, 0700); e != nil {
		return e
	}
	b, e := json.MarshalIndent(p.state, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(p.cfg.StateDir, ".state-*")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if _, e = f.Write(b); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return replaceState(name, filepath.Join(p.cfg.StateDir, "state.json"))
}
func (p *Poller) start() error {
	now := p.now()
	recent := []time.Time{}
	for _, t := range p.state.Starts {
		if now.Sub(t) < time.Hour {
			recent = append(recent, t)
		}
	}
	if len(recent) >= 3 {
		return fmt.Errorf("restart guard: more than 3 starts in one hour; wait before restarting")
	}
	p.state.Starts = append(recent, now)
	return p.save()
}
func (p *Poller) log(d Decision) error {
	p.day().Decisions = append(p.day().Decisions, d)
	b, e := json.Marshal(d)
	if e != nil {
		return e
	}
	b = append(b, '\n')
	if _, e = p.out.Write(b); e != nil {
		return e
	}
	if e = os.MkdirAll(p.cfg.StateDir, 0700); e != nil {
		return e
	}
	f, e := os.OpenFile(filepath.Join(p.cfg.StateDir, p.now().UTC().Format("2006-01-02")+".jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	_, e = f.Write(b)
	ce := f.Close()
	return errors.Join(e, ce)
}
func lease(i Issue, r Rule, t time.Time) string {
	h := sha256.Sum256([]byte(i.ID + "\x00" + i.State.ID + "\x00" + t.UTC().Format(time.RFC3339Nano)))
	return strings.ToLower(i.Identifier + "-" + r.Action + "-" + hex.EncodeToString(h[:])[:6])
}

type Row struct {
	Name     string
	State    string
	Exit     *int `json:"exit"`
	ExitCode *int `json:"exit_code"`
}
type HostInventory struct {
	Names []string
	Rows  []Row
	Error string
}

func (p *Poller) inventory(ctx context.Context) (map[string]bool, int, error) {
	b, e := p.command(ctx, "ls", "--all", "--json")
	if e != nil {
		return nil, 0, fmt.Errorf("dispatch inventory failed")
	}
	var inv map[string]HostInventory
	if e = json.Unmarshal(b, &inv); e != nil {
		return nil, 0, fmt.Errorf("invalid dispatch inventory")
	}
	names := map[string]bool{}
	p.active = map[string]bool{}
	running := 0
	for _, h := range inv {
		if h.Error != "" {
			return nil, 0, fmt.Errorf("dispatch host unreachable; inventory incomplete")
		}
		rows := map[string]bool{}
		for _, r := range h.Rows {
			rows[r.Name] = true
		}
		for _, name := range h.Names {
			names[name] = true
			if !rows[name] {
				p.active[name] = true
			}
			if !rows[name] {
				for _, rule := range p.allRules() {
					if rule.Concurrency == "code" && p.matchesLease(name, rule.Action) {
						running++
						break
					}
				}
			}
		}
		for _, r := range h.Rows {
			if run := p.state.CommentRuns[r.Name]; run != nil {
				run.Terminal = r.State == "done" || r.State == "failed" || r.State == "stopped"
				run.Exit = r.Exit
				if r.ExitCode != nil {
					run.Exit = r.ExitCode
				}
			}
			names[r.Name] = true
			if r.State == "running" || r.State == "reserved" || r.State == "launch-unconfirmed" { // Unknown jobs consume a code slot conservatively.
				p.active[r.Name] = true
				code := false
				for _, rule := range p.allRules() {
					if p.matchesLease(r.Name, rule.Action) {
						code = code || rule.Concurrency == "code"
					}
				}
				if code {
					running++
				}
			}
		}
	}
	for name, kind := range p.state.Pending {
		if names[name] {
			if e := p.admit(name); e != nil {
				return nil, 0, e
			}
			if e := p.reservation(name, kind, "done"); e != nil {
				return nil, 0, e
			}
			continue
		}
		since := p.state.PendingSince[name]
		if since.IsZero() {
			p.state.PendingSince[name] = p.now()
			since = p.now()
		}
		if p.now().Sub(since) >= pendingGrace {
			if e := p.reservation(name, kind, "needs-reconcile"); e != nil {
				return nil, 0, e
			}
			continue
		}
		if kind == "code" && !names[name] {
			running++
		}
	}
	// Unknown feedback outcomes keep their code slot until inventory or an
	// operator establishes what happened.
	for name, kind := range p.state.NeedsReconcile {
		if kind == "code" && !names[name] && p.matchesLease(name, "feedback") {
			running++
		}
	}
	return names, running, nil
}
func (p *Poller) poll(ctx context.Context) (err error) {
	defer func() {
		if errors.Is(err, errQuota) {
			err = p.log(Decision{Decision: "skipped", Reason: "rate-limit threshold"})
		}
		err = errors.Join(err, p.save())
	}()
	for day := range p.state.Days {
		if day < p.now().UTC().AddDate(0, 0, -6).Format("2006-01-02") {
			delete(p.state.Days, day)
		}
	}
	p.initSources()
	p.state.LastPoll = p.now()
	paused, e := exists(p.cfg.PauseFile)
	if e != nil {
		return e
	}
	if paused {
		return p.log(Decision{Decision: "skipped", Reason: "paused"})
	}
	if p.now().Before(p.state.BackoffUntil) {
		return p.log(Decision{Decision: "skipped", Reason: "rate-limit reset pending"})
	}
	if p.state.SkipNext {
		p.state.SkipNext = false
		return p.log(Decision{Decision: "skipped", Reason: "rate-limit backoff"})
	}
	snapshot, e := p.trackerSource().Fetch(ctx)
	issues, bot := snapshot.Issues, snapshot.Bot
	if e != nil {
		return e
	}
	current := map[string]bool{}
	for _, i := range issues {
		current[i.ID] = true
	}
	for id := range p.state.Cache {
		if !current[id] {
			delete(p.state.Cache, id)
		}
	}
	names, running, e := p.inventory(ctx)
	if e != nil {
		return e
	}
	if e := p.pollSources(ctx, bot, names, &running); e != nil {
		return e
	}
	p.checkStopped(ctx, bot)
	defer func() { p.pruneLeases(issues, names) }()
	batches := map[string][]Issue{}
candidateLoop:
	for _, i := range issues {
		if e := ctx.Err(); e != nil {
			return e
		}
		if p.state.Sources.EmailIssues[i.ID] && i.State.Name == p.cfg.Email.Status {
			if !p.state.Sources.Suppressed[i.ID] {
				p.state.Sources.Suppressed[i.ID] = true
				if e := p.log(Decision{Identifier: i.Identifier, Decision: "skipped", Reason: "email triage already reserved"}); e != nil {
					return e
				}
			}
			continue
		}
		var r Rule
		for _, rule := range p.cfg.Status {
			if rule.Name == i.State.Name {
				r = rule
				break
			}
		}
		if r.Name == "" {
			continue
		}
		d := Decision{Identifier: i.Identifier, URL: safeURL(i.URL, i.Identifier), Status: i.State.Name, Action: r.Action, Decision: "skipped"}
		if r.Mode == "off" {
			d.Reason = "status off"
			if e = p.log(d); e != nil {
				return e
			}
			continue
		}
		c, ok := p.state.Cache[i.ID]
		if !ok || c.HumanVersion != 3 || c.AuthorizerBot != bot || c.Updated != i.UpdatedAt || c.State != i.State.ID {
			c, e = p.history(ctx, i, bot)
			if e != nil {
				if r.Batch && !errors.Is(e, errQuota) && !p.now().Before(p.state.BackoffUntil) && !p.state.SkipNext {
					d.Reason = "ticket authorization lookup failed"
					if le := p.log(d); le != nil {
						return le
					}
					continue candidateLoop
				}
				return e
			}
			p.state.Cache[i.ID] = c
		}
		interval, _ := time.ParseDuration(p.cfg.Interval)
		if interval < 5*time.Minute {
			interval = 5 * time.Minute
		}
		if p.now().Sub(c.Checked) >= interval {
			allowed, e := p.member(ctx, c.AssignmentActor)
			if e != nil {
				if r.Batch && !errors.Is(e, errQuota) && !p.now().Before(p.state.BackoffUntil) && !p.state.SkipNext {
					d.Reason = "ticket authorization lookup failed"
					if le := p.log(d); le != nil {
						return le
					}
					continue candidateLoop
				}
				return e
			}
			transition, e := p.member(ctx, c.TransitionActor)
			if e != nil {
				if r.Batch && !errors.Is(e, errQuota) && !p.now().Before(p.state.BackoffUntil) && !p.state.SkipNext {
					d.Reason = "ticket authorization lookup failed"
					if le := p.log(d); le != nil {
						return le
					}
					continue candidateLoop
				}
				return e
			}
			c.Checked = p.now()
			if c.NonHumanEvent || c.Reason == "no human authorizer" || nonHuman(c.AssignmentActor, bot, allowed) || nonHuman(c.TransitionActor, bot, transition) {
				c.NonHumanEvent = true
				c.Reason = "no human authorizer"
			} else if (c.Reason == "" || c.Reason == "assigning actor outside allowed team" || c.Reason == "transition actor outside allowed team") && !p.allowed(History{Actor: allowed}, bot) {
				c.Reason = "assigning actor outside allowed team"
			} else if (c.Reason == "" || c.Reason == "assigning actor outside allowed team" || c.Reason == "transition actor outside allowed team") && !p.allowed(History{Actor: transition}, bot) {
				c.Reason = "transition actor outside allowed team"
			} else if c.Reason == "assigning actor outside allowed team" || c.Reason == "transition actor outside allowed team" {
				c.Reason = ""
			}
			p.state.Cache[i.ID] = c
		}
		d.AssignmentActor, d.TransitionActor = c.AssignmentActor, c.TransitionActor
		if !c.Entered.IsZero() {
			d.Name = lease(i, r, c.Entered)
		}
		switch {
		case c.Reason != "":
			d.Reason = c.Reason
		case !slug.MatchString(d.Name):
			d.Reason = "invalid lease name"
		case !r.Batch && (names[d.Name] || p.admitted[d.Name]):
			d.Reason = "dispatch record exists"
		case p.state.Pending[d.Name] != "" || p.state.NeedsReconcile[d.Name] != "":
			d.Reason = "dispatch outcome requires reconciliation"
		case p.state.TicketFailures[d.Name].Skip > 0:
			f := p.state.TicketFailures[d.Name]
			f.Skip--
			p.state.TicketFailures[d.Name] = f
			d.Reason = "ticket failure backoff"
		case r.Effect != "" && r.Effect != "dispatch":
			d.Decision, d.Reason, e = p.applyEffect(ctx, i, r, d.Name)
			if e != nil {
				return e
			}
		case r.Batch && validateValues(i.Identifier, i.URL, i.Title, i.State.Name) != nil:
			d.Reason = "template value contains control characters"
		case r.Batch:
			batches[r.Name] = append(batches[r.Name], i)
			d.Decision = "included"
		case r.Mode == "dry-run":
			d.Decision = "would-dispatch"
		case p.day().Dispatches >= p.cfg.DailyCap:
			d.Reason = "daily cap"
		case r.Concurrency == "code" && running >= p.cfg.Limits.CodeJobs:
			d.Reason = "code-job limit"
		default:
			if e = p.launch(ctx, i, r, d.Name); e != nil {
				d.Reason = "dispatch failed or uncertain; inspect named record"
				if strings.Contains(e.Error(), "deterministic rejection") {
					d.Decision = "failed"
					d.Reason = "deterministic dispatch rejection"
				} else if p.state.Pending[d.Name] == "" {
					d.Decision = "failed"
					d.Reason = "invalid order or local preparation failed"
				}
				if le := p.log(d); le != nil {
					return errors.Join(e, le)
				}
				if errors.Is(e, errHostFull) {
					continue
				}
				if ctx.Err() != nil && p.state.Pending[d.Name] == "" {
					return ctx.Err()
				}
				failure := p.state.TicketFailures[d.Name]
				failure.Count++
				failure.Skip = 1 << min(failure.Count-1, 6)
				p.state.TicketFailures[d.Name] = failure
				if p.state.Pending[d.Name] != "" {
					return nil
				}
				continue
			}
			delete(p.state.TicketFailures, d.Name)
			d.Decision = "dispatched"
			names[d.Name] = true
			if r.Concurrency == "code" {
				running++
			}
		}
		if e = p.log(d); e != nil {
			return e
		}
	}
	for _, r := range p.cfg.Status {
		if len(batches[r.Name]) > 0 {
			if e := p.dispatchBatch(ctx, batches[r.Name], r, names, &running); e != nil {
				return e
			}
			if p.state.Pending[batchName(batches[r.Name], r, p.now())] != "" {
				return nil
			}
		}
	}
	if !p.now().Before(p.state.BackoffUntil) {
		p.state.Failures = 0
	}
	return nil
}
func (p *Poller) launch(ctx context.Context, i Issue, r Rule, name string) error {
	rendered, e := renderOrder(i, r, name)
	if e != nil {
		// Live logs must not expose ticket values embedded in parser errors.
		return fmt.Errorf("template validation failed")
	}
	if e := p.trackRun(name, r, []Issue{i}); e != nil {
		return e
	}
	return p.launchRendered(ctx, rendered, r, name)
}
func (p *Poller) launchRendered(ctx context.Context, rendered string, r Rule, name string) error {
	var e error
	tmp := filepath.Join(p.cfg.StateDir, "tmp")
	if e = os.MkdirAll(tmp, 0700); e != nil {
		return e
	}
	if e = privateDirectory(tmp); e != nil {
		return e
	}
	f, e := os.CreateTemp(tmp, "hev-intake-*.md")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if _, e = f.WriteString(rendered); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	// Reserve before subprocess: a crash or lost response must not cause duplicate work or bypass the cap.
	if e = p.reservation(name, r.Concurrency, "reserve"); e != nil {
		return e
	}
	if _, e = p.command(ctx, f.Name(), "--host", r.Host); e != nil {
		var exit *exec.ExitError
		if errors.As(e, &exit) && exit.ExitCode() == 1 && deterministic(string(exit.Stderr)) {
			if err := p.reservation(name, r.Concurrency, "reject"); err != nil {
				return err
			}
			if hostFull(rejectionMessage(string(exit.Stderr))) {
				return errHostFull
			}
			return fmt.Errorf("dispatch failed: deterministic rejection")
		}
		return fmt.Errorf("dispatch outcome uncertain; inspect %s", name)
	}
	if e = p.admit(name); e != nil {
		return e
	}
	return p.reservation(name, r.Concurrency, "done")
}

func secretEntry(entry, key string) bool {
	name, _, _ := strings.Cut(entry, "=")
	return strings.EqualFold(name, key)
}
func safeURL(raw, id string) string {
	j := strings.Index(raw, "/"+id)
	if j < 0 {
		return ""
	}
	return raw[:j+1+len(id)]
}
func (p *Poller) matchesLease(name, action string) bool {
	if regexp.MustCompile(`^` + regexp.QuoteMeta(action) + `-[0-9]{8}-[a-f0-9]{6}$`).MatchString(name) {
		return true
	}
	return regexp.MustCompile(`^` + regexp.QuoteMeta(strings.ToLower(p.cfg.Linear.Team)) + `-[0-9]+-` + regexp.QuoteMeta(action) + `-[a-f0-9]{6}$`).MatchString(name)
}

const pendingGrace = time.Hour

func deterministic(s string) bool {
	s = rejectionMessage(s)
	for _, exact := range []string{"order must start with ---", "expected flat key: value", "unclosed quote", "invalid scalar", "expected scalar", "unclosed frontmatter", "missing ## Acceptance section", "invalid timeout", "invalid name", "repo required", "invalid harness", "invalid validation", "invalid base"} {
		if s == exact {
			return true
		}
	}
	if hostFull(s) {
		return true
	}
	for _, prefix := range []string{"unknown key: ", "duplicate key: ", "empty field: "} {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}
func (p *Poller) cleanTemp() error {
	dir := filepath.Join(p.cfg.StateDir, "tmp")
	files, e := filepath.Glob(filepath.Join(dir, "hev-intake-*.md"))
	if e != nil {
		return e
	}
	for _, file := range files {
		if e = os.Remove(file); e != nil {
			return e
		}
	}
	return nil
}
func (p *Poller) reconcile(name string, admitted bool) error {
	if p.state.Effects[name] == "needs-reconcile" || p.state.Effects[name] == "pending" {
		if admitted {
			p.state.Effects[name] = "completed"
		} else {
			delete(p.state.Effects, name)
		}
		delete(p.state.NeedsReconcile, name)
		return p.save()
	}
	kind := p.state.NeedsReconcile[name]
	if kind == "" {
		return fmt.Errorf("lease does not need reconciliation")
	}
	if admitted {
		if e := p.admit(name); e != nil {
			return e
		}
	}
	// Uncertain attempts retain their original daily charge, even when not admitted.
	if e := p.reservation(name, kind, "done"); e != nil {
		return e
	}
	return p.save()
}
func (p *Poller) admit(name string) error {
	f, e := os.OpenFile(filepath.Join(p.cfg.StateDir, "admitted.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	_, e = f.WriteString(name + "\n")
	if e == nil {
		e = f.Sync()
	}
	e = errors.Join(e, f.Close())
	if e == nil {
		p.admitted[name] = true
	}
	return e
}

type reservationEvent struct {
	Name, Kind, Action, Day string
	At                      time.Time
}

func (p *Poller) applyReservation(ev reservationEvent) {
	if p.state.Days[ev.Day] == nil {
		p.state.Days[ev.Day] = &Daily{}
	}
	switch ev.Action {
	case "reserve":
		p.state.Pending[ev.Name] = ev.Kind
		p.state.PendingSince[ev.Name] = ev.At
		p.state.Days[ev.Day].Dispatches++
	case "reject":
		delete(p.state.Pending, ev.Name)
		delete(p.state.PendingSince, ev.Name)
		p.state.Days[ev.Day].Dispatches--
	case "needs-reconcile":
		p.state.NeedsReconcile[ev.Name] = ev.Kind
		delete(p.state.Pending, ev.Name)
		delete(p.state.PendingSince, ev.Name)
	case "done":
		delete(p.state.NeedsReconcile, ev.Name)
		delete(p.state.PendingSince, ev.Name)
		delete(p.state.Pending, ev.Name)
	}
}
func (p *Poller) replayReservations() error {
	b, e := os.ReadFile(filepath.Join(p.cfg.StateDir, "reservations.jsonl"))
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	if p.state.LedgerOffset > len(b) {
		return fmt.Errorf("reservation ledger truncated")
	}
	for _, line := range strings.Split(string(b[p.state.LedgerOffset:]), "\n") {
		if line == "" {
			continue
		}
		var ev reservationEvent
		if e = json.Unmarshal([]byte(line), &ev); e != nil {
			return fmt.Errorf("invalid reservation ledger")
		}
		p.applyReservation(ev)
	}
	p.state.LedgerOffset = len(b)
	return nil
}
func (p *Poller) reservation(name, kind, action string) error {
	ev := reservationEvent{name, kind, action, p.now().UTC().Format("2006-01-02"), p.now()}
	b, _ := json.Marshal(ev)
	b = append(b, '\n')
	f, e := os.OpenFile(filepath.Join(p.cfg.StateDir, "reservations.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	_, e = f.Write(b)
	if e == nil {
		e = f.Sync()
	}
	e = errors.Join(e, f.Close())
	if e != nil {
		return e
	}
	p.applyReservation(ev)
	p.state.LedgerOffset += len(b)
	return nil
}

var errHostFull = errors.New("dispatch failed: deterministic rejection: host full")

func hostFull(s string) bool {
	return regexp.MustCompile(`^host full \([0-9]+/[0-9]+\)$`).MatchString(s)
}

// Cobra includes usage between its Error line and repeated final message.
func rejectionMessage(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	last := strings.TrimSpace(lines[len(lines)-1])
	diagnostic := regexp.MustCompile(`^[^ :]+: ([0-9]+ free|unreachable: .+)$`)
	for index, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Error: ") {
			message := strings.TrimPrefix(line, "Error: ")
			if message == last {
				return message
			}
			return ""
		}
		if !diagnostic.MatchString(line) {
			// A bare final error is also emitted by older dispatch versions.
			if index == len(lines)-1 {
				return strings.TrimPrefix(last, "error: ")
			}
			return ""
		}
	}
	return ""
}

func (p *Poller) pruneLeases(issues []Issue, names map[string]bool) {
	keep := map[string]bool{}
	for name := range names {
		keep[name] = true
	}
	for _, i := range issues {
		c, ok := p.state.Cache[i.ID]
		// A quota/cancellation/uncertain stop may leave later candidates unchecked.
		// Retain their gates until history establishes the current lease.
		if !ok || c.Updated != i.UpdatedAt || c.State != i.State.ID {
			prefix := strings.ToLower(i.Identifier) + "-"
			for name := range p.state.TicketFailures {
				if strings.HasPrefix(name, prefix) {
					keep[name] = true
				}
			}
			continue
		}
		for _, r := range p.cfg.Status {
			if r.Name == i.State.Name {
				keep[lease(i, r, c.Entered)] = true
			}
		}
	}
	for name := range p.state.TicketFailures {
		if !keep[name] {
			delete(p.state.TicketFailures, name)
		}
	}
	// NeedsReconcile is an operator gate: only reconcile may clear it.
}
