package dispatch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hev/kit/internal/attention"
	"github.com/hev/kit/internal/cost"
)

// limitObservation is dispatch's own record of a classified limit exit, so
// the next launch on this host skips a model it already knows is out.
type limitObservation struct {
	ModelStep
	LimitType  string    `json:"limit_type,omitempty"`
	ResetsAt   time.Time `json:"resets_at,omitempty"`
	ObservedAt time.Time `json:"observed_at"`
	Run        string    `json:"run,omitempty"`
}

// observationTTL bounds an observation without a reset time, the same hour
// after which internal/cost marks a plan observation stale.
const observationTTL = time.Hour

func limitObservationsPath() string { return filepath.Join(Root(), ".limits.json") }

// fresh is true while an observation still says the model is out: before
// its reset when it has one, else within observationTTL of being seen.
func (o limitObservation) fresh(now time.Time) bool {
	if o.ObservedAt.IsZero() || o.ObservedAt.After(now.Add(time.Minute)) {
		return false
	}
	if !o.ResetsAt.IsZero() {
		return now.Before(o.ResetsAt) && now.Sub(o.ObservedAt) < 8*24*time.Hour
	}
	return now.Sub(o.ObservedAt) < observationTTL
}

func loadLimitObservations() map[string]limitObservation {
	out := map[string]limitObservation{}
	_ = readJSON(limitObservationsPath(), &out)
	return out
}

// recordLimitObservation notes a limit exit. Best effort: a lost write only
// means the next launch tries the model and degrades again.
func recordLimitObservation(step ModelStep, hit *LimitHit, run string, now time.Time) {
	if hit == nil {
		return
	}
	all := loadLimitObservations()
	for k, o := range all {
		if !o.fresh(now) {
			delete(all, k)
		}
	}
	all[step.String()] = limitObservation{ModelStep: step, LimitType: hit.LimitType, ResetsAt: hit.ResetsAt, ObservedAt: now.UTC(), Run: run}
	if err := os.MkdirAll(Root(), 0700); err == nil {
		_ = WriteJSON(limitObservationsPath(), all)
	}
}

// knownExhausted returns a fresh observation that step is out of usage:
// dispatch's own limit records, then the plan observations in the configured
// factory directory (internal/cost). A stale observation is never used.
var knownExhausted = func(d DegradeConfig, step ModelStep, now time.Time) *limitObservation {
	if o, ok := loadLimitObservations()[step.String()]; ok && o.fresh(now) {
		return &o
	}
	if d.Observations == "" {
		return nil
	}
	dir := Expand(d.Observations)
	cfg, err := cost.LoadConfig(filepath.Join(dir, "prices.toml"), filepath.Join(dir, "subscriptions.toml"))
	if err != nil {
		return nil
	}
	rows, _ := cost.ReadRows(filepath.Join(dir, "sessions.jsonl"))
	limits := cfg.Limits(rows, now)
	_ = cost.ApplyClaudeUsage(filepath.Join(dir, "claude-usage.json"), limits, now)
	l, ok := cost.Exhausted(cfg.Plans, limits, step.Harness, step.Model, now)
	if !ok {
		return nil
	}
	return &limitObservation{ModelStep: step, LimitType: l.Name, ResetsAt: time.UnixMilli(l.ResetsAt).UTC(), ObservedAt: time.UnixMilli(l.ObservedAt).UTC()}
}

// degradeHookTimeout is on_degrade's bound when hook_timeout is unset.
const degradeHookTimeout = 10 * time.Second

// runDegradeHook runs on_degrade with the event as JSON on standard input.
// It is fire-and-forget: it runs in the background, is killed at its
// timeout, and a failure is only logged (to logPath, when set).
var runDegradeHook = func(d DegradeConfig, ev DegradeEvent, logPath string) <-chan struct{} {
	done := make(chan struct{})
	if len(d.OnDegrade) == 0 {
		close(done)
		return done
	}
	timeout := degradeHookTimeout
	if t, err := time.ParseDuration(d.HookTimeout); err == nil && t > 0 {
		timeout = t
	}
	payload, _ := json.Marshal(ev)
	go func() {
		defer close(done)
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		argv := append([]string{}, d.OnDegrade...)
		argv[0] = Expand(argv[0])
		c := exec.CommandContext(ctx, argv[0], argv[1:]...)
		c.Stdin = bytes.NewReader(append(payload, '\n'))
		c.Env = append(notificationEnvironment(), "HEV_DEGRADE_EVENT="+ev.Event, "HEV_DEGRADE_RUN="+ev.Run)
		c.WaitDelay = time.Second
		var out bytes.Buffer
		c.Stdout, c.Stderr = &out, &out
		setupChild(c)
		c.Cancel = func() error { killTree(c); return nil }
		err := c.Run()
		if err != nil {
			logDegrade(logPath, map[string]any{"type": "dispatch.degrade.hook_error", "event": ev.Event, "error": err.Error(), "output": strings.TrimSpace(out.String())})
			fmt.Fprintf(os.Stderr, "hev: on_degrade hook for %s: %v\n", ev.Run, err)
		}
	}()
	return done
}

// logDegrade appends one JSON record to a run's log (or a round's trace).
func logDegrade(path string, v any) {
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	_ = json.NewEncoder(f).Encode(v)
}

// degradeMessage is the peers notice text for an event.
func degradeMessage(ev DegradeEvent) string {
	what := "dispatch " + ev.Run
	if ev.Step == "orchestrator" {
		what = "orchestrator " + ev.Run
	}
	why := ev.LimitType
	if why == "" {
		why = "plan limit"
	}
	if ev.Reason == "observed" {
		why += ", known from a fresh observation"
	}
	resets := ""
	if ev.ResetsAt != nil {
		resets = "; resets " + ev.ResetsAt.UTC().Format(time.RFC3339)
	}
	if ev.Event == "exhausted" {
		return fmt.Sprintf("[urgent] %s on %s (%s): all models exhausted; last tried %s (%s)%s. Blocked until a model resets.", what, ev.Host, ev.Step, ev.From, why, resets)
	}
	return fmt.Sprintf("%s on %s (%s) degraded %s → %s (%s)%s.", what, ev.Host, ev.Step, ev.From, ev.To, why, resets)
}

// sendRunDegradeNotice delivers a run's event to its reply session as a
// peers notice from HOST/d-NAME. Tests replace it.
var sendRunDegradeNotice = func(ctx context.Context, m Meta, ev DegradeEvent, id string) error {
	if m.Reply == "" {
		return nil
	}
	exe, err := notificationExecutable()
	if err != nil {
		return err
	}
	from := notificationHost() + "/d-" + m.Name
	host, _, _ := strings.Cut(from, "/")
	return executeNotification(ctx, []string{exe, "peers", "send", "--from", from, "--id", host + ":" + id, "--inbox-only", "--", m.Reply, degradeMessage(ev)}, true)
}

// degradeAttention is the attention notice of an event: an exhausted chain
// needs someone (urgent); a degrade is a done-kind record.
func degradeAttention(kind, source, host, name, key, text string, links attention.Links, requestedBy string) attention.Notice {
	state := "degraded"
	if kind == attention.KindNeedsYou {
		state = "blocked"
	}
	return attention.New(kind, source, host, name, key, state, text, links).For(requestedBy)
}

// emitRunDegrade records, announces and hooks one event of a run step
// (worker, rework or validate). It never blocks the run for long: the hook
// runs in the background and the notice has a short deadline.
func emitRunDegrade(dir string, m Meta, cfg DegradeConfig, ev DegradeEvent, index int) {
	logDegrade(filepath.Join(dir, "log"), map[string]any{"type": "dispatch." + ev.Event, "degrade": ev})
	hook := runDegradeHook(cfg, ev, filepath.Join(dir, "log"))
	id := fmt.Sprintf("dispatch:%s:%s:%s:%d", m.Name, m.Start.UTC().Format(time.RFC3339Nano), ev.Event, index)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	if err := sendRunDegradeNotice(ctx, m, ev, id); err != nil {
		logDegrade(filepath.Join(dir, "log"), map[string]any{"type": "dispatch.degrade.notice_error", "event": ev.Event, "error": err.Error()})
	}
	cancel()
	if m.Parent == "" && m.Forward == nil {
		kind := attention.KindDone
		if ev.Event == "exhausted" {
			kind = attention.KindNeedsYou
		}
		key := runLineage(m) + "|" + ev.Event + "|" + fmt.Sprint(index)
		emitAttention(degradeAttention(kind, attention.SourceRun, attentionHost(m.Host), m.Name, key, degradeMessage(ev), attention.Links{Run: runShowLink(m)}, requester(m)), dir)
	}
	// Give a quick hook its moment; a slow one finishes in the background.
	select {
	case <-hook:
	case <-time.After(degradeHookGrace):
	}
}

// degradeHookGrace is how long an emitter waits for its hook before moving
// on; the hook keeps running until its own timeout.
var degradeHookGrace = 2 * time.Second
