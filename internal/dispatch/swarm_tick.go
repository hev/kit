package dispatch

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// SwarmOps is the dispatch surface the reconcile tick uses. The default calls
// the local host's dispatch RPC with the swarm's orchestrator identity, so
// target admission checks scope, authority and budget for code as for rounds.
type SwarmOps interface {
	List(ctx context.Context) (map[string]Row, error)
	// Start returns nil when the target accepted or queued the order.
	Start(ctx context.Context, name, order string) error
	Stop(ctx context.Context, name string) error
	Show(ctx context.Context, name string) (Response, error)
	Retry(ctx context.Context, name, note string) error
	// Pushed reports whether the run's recorded head is on origin.
	Pushed(ctx context.Context, row Row) bool
	// Progress is a signature of log bytes, status and head; a stall is no
	// change for the role's stall duration.
	Progress(ctx context.Context, name string) string
}

type localSwarmOps struct {
	identity string
	host     Host
}

func newLocalSwarmOps(identity string) localSwarmOps {
	h := Host{Local: true}
	if cfg, e := Load(ConfigPath("")); e == nil {
		for _, n := range SortedHostNames(cfg.Hosts) {
			if cfg.Hosts[n].Local {
				h = cfg.Hosts[n]
				break
			}
		}
	}
	return localSwarmOps{identity: identity, host: h}
}

// SwarmUntraced is the reason a swarm's units carry no trace: the
// reconcile tick launches and repairs them, and the ask that started the
// swarm is not carried into its supervisor.
func SwarmUntraced(identity string) string {
	return "swarm " + identity + " launched this unit from its reconcile tick; swarm units do not carry the trace of the ask that started the swarm"
}

// SortedHostNames returns host names in a stable order.
func SortedHostNames(m map[string]Host) []string {
	out := make([]string, 0, len(m))
	for n := range m {
		out = append(out, n)
	}
	slices.Sort(out)
	return out
}

func (o localSwarmOps) rpc(ctx context.Context, q Request) (Response, error) {
	q.Orchestrator = o.identity
	return RPC(ctx, o.host, q)
}

func (o localSwarmOps) List(ctx context.Context) (map[string]Row, error) {
	r, e := o.rpc(ctx, Request{Action: "list"})
	if e != nil {
		return nil, e
	}
	out := map[string]Row{}
	for _, row := range r.Rows {
		if row.Kind != "orch" {
			out[row.Name] = row
		}
	}
	return out, nil
}

func (o localSwarmOps) Start(ctx context.Context, name, text string) error {
	ord, e := Parse(text)
	if e != nil {
		return e
	}
	h := o.host
	// The tick launches units on its own schedule, not in answer to an ask,
	// so a unit is untraced and says why (as orchestrator source events do).
	q := Request{Action: "start", Name: name, Order: text, Meta: Meta{Order: ord, Namespace: h.Namespace, HarnessWrapper: h.HarnessWrapper, ConfigPath: h.ConfigPath, Untraced: SwarmUntraced(o.identity)}, Cap: h.Cap, Workspace: h.Workspace}
	_, e = o.rpc(ctx, q)
	return e
}

func (o localSwarmOps) Stop(ctx context.Context, name string) error {
	_, e := o.rpc(ctx, Request{Action: "stop", Name: name})
	return e
}

func (o localSwarmOps) Show(ctx context.Context, name string) (Response, error) {
	return o.rpc(ctx, Request{Action: "show", Name: name})
}

func (o localSwarmOps) Retry(ctx context.Context, name, note string) error {
	_, e := o.rpc(ctx, Request{Action: "retry", Name: name, Note: note})
	return e
}

func (o localSwarmOps) Pushed(ctx context.Context, row Row) bool {
	if row.Head == "" || row.Branch == "" {
		return false
	}
	out, e := command(filepath.Join(Root(), row.Name, "wt"), "git", "ls-remote", "origin", "refs/heads/"+row.Branch)
	return e == nil && strings.HasPrefix(out, row.Head)
}

func (o localSwarmOps) Progress(_ context.Context, name string) string {
	dir := filepath.Join(Root(), name)
	var size int64
	if info, e := os.Stat(filepath.Join(dir, "log")); e == nil {
		size = info.Size()
	}
	status, _ := os.ReadFile(filepath.Join(dir, "status.md"))
	head, _ := command(filepath.Join(dir, "wt"), "git", "rev-parse", "HEAD")
	return fmt.Sprintf("%d:%x:%s", size, sha256.Sum256(status), head)
}

// swarmBeforeLaunch lets tests crash the tick between writing an attempting
// record and the launch.
var swarmBeforeLaunch func(SwarmUnit) error

// swarmSupervisorTick is the supervisor's reconcile step.
func swarmSupervisorTick(ctx context.Context, name string) error {
	return SwarmTick(ctx, name, newLocalSwarmOps(name), time.Now().UTC())
}

type swarmTick struct {
	ctx  context.Context
	spec SwarmSpec
	orch OrchestratorSpec
	st   *SwarmState
	ops  SwarmOps
	now  time.Time
	host string
}

// SwarmTick is one code-only reconcile pass (RFC 0013 §5): observe each unit,
// take the safe repair, settle races, launch pending units up to replicas,
// and write swarm.json. Judgement goes to a round; the rest stops and tells
// reply. It does nothing while the swarm is paused, stopped or down.
func SwarmTick(ctx context.Context, name string, ops SwarmOps, now time.Time) error {
	unlock, e := lockSwarm(name)
	if e != nil {
		return e
	}
	defer unlock()
	spec, st, e := loadSwarm(name)
	if e != nil {
		return e
	}
	dir := OrchestratorDir(name)
	if st.Down || exists(filepath.Join(dir, "stop")) || exists(filepath.Join(dir, "pause")) {
		return nil
	}
	orch, e := LoadOrchestrator(name)
	if e != nil {
		return e
	}
	host, _ := swarmLocalHostName(spec)
	t := &swarmTick{ctx: ctx, spec: spec, orch: orch, st: &st, ops: ops, now: now, host: host}
	rows, e := ops.List(ctx)
	if e != nil {
		return fmt.Errorf("swarm %s inventory: %w", name, e)
	}
	st.Ticks++
	st.LastTick = now
	for i := range st.Units {
		if e = t.observe(&st.Units[i], rows); e != nil {
			_ = t.save()
			return e
		}
	}
	t.races()
	if e = t.launch(); e != nil {
		_ = t.save()
		return e
	}
	return t.save()
}

func (t *swarmTick) save() error { return saveSwarmState(t.st) }

func (t *swarmTick) log(format string, args ...any) {
	swarmLog(t.st.Name, t.now, format, args...)
}

func (t *swarmTick) role(name string) SwarmRole {
	r, _ := t.spec.role(name)
	return r
}

func (t *swarmTick) roleState(name string) *SwarmRoleState {
	for i := range t.st.Roles {
		if t.st.Roles[i].Name == name {
			return &t.st.Roles[i]
		}
	}
	return &SwarmRoleState{}
}

// notice queues one notice to reply per key and lifetime.
func (t *swarmTick) notice(key, msg string) {
	_ = queueOrchestratorNotice(t.orch, "swarm", t.st.Lifetime.Format(time.RFC3339Nano)+":"+key, t.orch.Reply, noticeText(t.st.Name, "swarm "+t.st.Name+": "+msg))
}

// roundNote hands judgement to the next round, once per key and lifetime.
func (t *swarmTick) roundNote(key, msg string) {
	if t.st.Noted[key] {
		return
	}
	if t.st.Noted == nil {
		t.st.Noted = map[string]bool{}
	}
	dir := OrchestratorDir(t.st.Name)
	unlock, e := lockRoot()
	if e != nil {
		return
	}
	defer unlock()
	f, e := os.OpenFile(filepath.Join(dir, "inbox.md"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if e != nil {
		return
	}
	_, e = fmt.Fprintf(f, "[swarm tick %s] %s\n", t.now.UTC().Format(time.RFC3339), msg)
	if f.Close() != nil || e != nil {
		return
	}
	t.st.Noted[key] = true
	_ = os.WriteFile(filepath.Join(dir, "wake"), []byte("swarm"), 0600)
}

func (t *swarmTick) hold(u *SwarmUnit, reason string, notify bool) {
	u.State, u.Reason, u.HeldAt, u.NextTry = "held", reason, t.now, time.Time{}
	t.log("%s held: %s", u.Run, reason)
	if notify {
		t.notice(fmt.Sprintf("held:%s:%d:%d", u.Run, u.Restarts, u.Stalls), fmt.Sprintf("%s/%s (%s) held: %s", u.Role, swarmUnitLabel(u.Unit), u.Run, reason))
	}
}

func (t *swarmTick) observe(u *SwarmUnit, rows map[string]Row) error {
	if u.Run == "" {
		return nil
	}
	row, ok := rows[u.Run]
	switch u.State {
	case "pending", "superseded", "lost":
		return nil
	case "attempting":
		if u.Op == "retry" {
			if ok && (row.State == "running" || row.State == "launch-unconfirmed" || row.Start.After(u.Attempted)) {
				t.becameLive(u)
				break
			}
			if !ok {
				t.hold(u, "run missing from inventory during a retry", true)
				return nil
			}
			// The retry never reached the target: rework of an ended run
			// is refused while it runs, so sending it again is safe.
			t.retry(u)
			return nil
		}
		if !ok {
			// The start may never have reached the target. Run names are
			// unique per target and a duplicate start is refused
			// atomically, so sending the same start again cannot make two.
			return t.start(u, false)
		}
		t.becameLive(u)
	case "held", "done":
		if ok && row.State == "running" && row.Start.After(maxTime(u.HeldAt, u.DoneAt)) {
			// A round or a person retried it; observe it again.
			t.becameLive(u)
			u.Reason = "relaunched outside the tick"
			break
		}
		return nil
	case "failed":
		if ok && row.State == "running" {
			t.becameLive(u)
			break
		}
		if !t.now.Before(u.NextTry) {
			t.retry(u)
		}
		return nil
	}
	if !ok {
		if u.State == "queued" {
			// A cancelled queued order had no effects; launch it again later.
			u.State, u.Reason = "pending", "queued order cancelled"
			return nil
		}
		t.hold(u, "run missing from inventory", true)
		return nil
	}
	u.RunState, u.Validation = row.State, row.Validation
	switch row.State {
	case "queued":
		u.State = "queued"
	case "running", "launch-unconfirmed":
		u.State = "live"
		t.stall(u)
	case "done":
		if t.ops.Pushed(t.ctx, row) {
			u.State, u.Head, u.DoneAt, u.Reason = "done", row.Head, t.now, ""
			t.log("%s done at %s", u.Run, row.Head)
			return nil
		}
		t.failure(u, row, "done without a pushed head")
	case "failed", "timeout", "died":
		t.failure(u, row, row.State)
	case "blocked", "needs-review":
		why := row.State
		if q := strings.TrimSpace(row.Question + " " + row.Reason); q != "" {
			why += ": " + q
		}
		t.hold(u, why, true)
		t.roundNote("ask:"+u.Run+":"+row.Start.Format(time.RFC3339Nano), fmt.Sprintf("%s/%s (%s) ended %s. Answer it, retry or rework it, or tell reply.", u.Role, swarmUnitLabel(u.Unit), u.Run, why))
	}
	return nil
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func (t *swarmTick) becameLive(u *SwarmUnit) {
	u.State, u.Op, u.Progress, u.ProgressAt = "live", "", "", t.now
}

// confirmStopped stops a run and waits for its host to report a terminal
// state. Only a confirmed stop permits a repair (RFC 0013 §5).
func (t *swarmTick) confirmStopped(name string) (Result, bool) {
	if e := t.ops.Stop(t.ctx, name); e != nil {
		return Result{}, false
	}
	deadline := time.Now().Add(swarmStopWait)
	for {
		r, e := t.ops.Show(t.ctx, name)
		if e == nil && !r.Active {
			var res Result
			if json.Unmarshal([]byte(r.Files["result.json"]), &res) == nil && !res.End.IsZero() {
				return res, true
			}
		}
		if !time.Now().Before(deadline) || t.ctx.Err() != nil {
			return Result{}, false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// failure runs repair 1: stop, confirm a terminal state, then schedule a
// retry in place after the backoff. It holds and asks where the RFC says so.
func (t *swarmTick) failure(u *SwarmUnit, row Row, why string) {
	res, ok := t.confirmStopped(u.Run)
	if !ok {
		t.hold(u, why+"; stop could not confirm a terminal state", true)
		return
	}
	role := t.role(u.Role)
	if role.effects() == "pr" && (why != "failed" || row.PR != "") {
		t.hold(u, "effects: pr run ended "+why+"; not repaired automatically", true)
		return
	}
	if limit := t.spec.restartLimit(); u.Restarts >= limit {
		t.hold(u, fmt.Sprintf("restart limit %d reached; last run %s", limit, why), true)
		return
	}
	u.Failure = fmt.Sprintf("%s, exit %d", why, res.ExitCode)
	u.State, u.Reason = "failed", u.Failure
	u.NextTry = t.now.Add(t.spec.backoff(u.Restarts))
	t.log("%s %s; stop confirmed; retry in place at %s", u.Run, u.Failure, u.NextTry.Format(time.RFC3339))
}

func (t *swarmTick) stall(u *SwarmUnit) {
	role := t.role(u.Role)
	if role.Stall == "" {
		return
	}
	limit, _ := OrchestratorDuration(role.Stall)
	sig := t.ops.Progress(t.ctx, u.Run)
	if sig != u.Progress || u.ProgressAt.IsZero() {
		u.Progress, u.ProgressAt = sig, t.now
		return
	}
	if t.now.Sub(u.ProgressAt) < limit {
		return
	}
	u.Stalls++
	why := "stall: no new log bytes, status or commit for " + role.Stall
	if u.Stalls >= 2 {
		if _, ok := t.confirmStopped(u.Run); !ok {
			t.hold(u, why+"; stop could not confirm a terminal state", true)
			return
		}
		t.hold(u, "second "+why+"; a round judges before a third attempt", true)
		t.roundNote(fmt.Sprintf("stall:%s:%d", u.Run, u.Stalls), fmt.Sprintf("%s/%s (%s) stalled twice. Judge it before a third attempt: retry with guidance, rework, or tell reply.", u.Role, swarmUnitLabel(u.Unit), u.Run))
		return
	}
	t.failure(u, Row{}, why)
}

func (t *swarmTick) retry(u *SwarmUnit) {
	u.State, u.Op, u.Attempted = "attempting", "retry", time.Now().UTC()
	if e := t.save(); e != nil {
		return
	}
	limit := t.spec.restartLimit()
	note := fmt.Sprintf("Swarm repair in place (restart %d of %d): the previous attempt ended %s. Continue from this worktree and branch; push after every commit.", u.Restarts+1, limit, u.Failure)
	e := t.ops.Retry(t.ctx, u.Run, note)
	u.Restarts++
	t.st.Restarts++
	if e != nil {
		if u.Restarts >= limit {
			t.hold(u, "retry refused: "+e.Error()+"; restart limit reached", true)
			return
		}
		u.State, u.Op, u.Reason = "failed", "", "retry refused: "+e.Error()
		u.NextTry = t.now.Add(t.spec.backoff(u.Restarts))
		return
	}
	t.becameLive(u)
	u.Reason = "repaired in place after " + u.Failure
	t.log("%s retried in place (restart %d)", u.Run, u.Restarts)
}

// roleComplete: every unit done, or a race with its chosen unit done.
func (t *swarmTick) roleComplete(name string) bool {
	rs := t.roleState(name)
	any := false
	for _, u := range t.st.Units {
		if u.Role != name {
			continue
		}
		any = true
		if rs.Shape == "race" {
			if u.Unit == rs.Chosen && u.State == "done" {
				return true
			}
			continue
		}
		if u.State != "done" {
			return false
		}
	}
	return any && rs.Shape != "race"
}

func (t *swarmTick) races() {
	for _, r := range t.spec.Roles {
		if r.Shape != "race" {
			continue
		}
		rs := t.roleState(r.Name)
		if rs.Chosen == "" && r.choose() == "first-green" {
			var best *SwarmUnit
			for i := range t.st.Units {
				u := &t.st.Units[i]
				green := r.Order.Validation != "browser" || u.Validation == "pass"
				if u.Role == r.Name && u.State == "done" && green && (best == nil || u.DoneAt.Before(best.DoneAt)) {
					best = u
				}
			}
			if best != nil {
				_ = chooseSwarmUnit(t.spec, t.st, r.Name+"/"+best.Unit, "first done with a pushed head", "first-green", t.now)
			}
		}
		if rs.Chosen == "" {
			settled, done := true, 0
			for _, u := range t.st.Units {
				if u.Role == r.Name {
					settled = settled && !swarmActive(u.State) && u.State != "pending"
					if u.State == "done" {
						done++
					}
				}
			}
			if settled && done > 0 {
				msg := fmt.Sprintf("race %s is ready for a choice: %d of %d variants done.", r.Name, done, len(r.units()))
				if r.choose() == "person" {
					t.notice("race:"+r.Name, msg+fmt.Sprintf(" Choose with: hev swarm say %s choose %s/vN", t.st.Name, r.Name))
				} else {
					t.roundNote("race:"+r.Name, msg+" Compare reports and branches and write choice.json.")
				}
			}
			continue
		}
		// Chosen: stop the others. Losing branches are kept.
		for i := range t.st.Units {
			u := &t.st.Units[i]
			if u.Role != r.Name || u.Unit == rs.Chosen {
				continue
			}
			switch {
			case u.State == "pending":
				u.State, u.Reason = "superseded", "race chose "+rs.Chosen
			case swarmActive(u.State):
				if _, ok := t.confirmStopped(u.Run); !ok {
					t.hold(u, "race chose "+rs.Chosen+"; stop could not confirm", true)
					continue
				}
				u.State, u.Reason = "superseded", "race chose "+rs.Chosen
				t.log("%s stopped: race chose %s", u.Run, rs.Chosen)
			}
		}
	}
}

func (t *swarmTick) inputs(r SwarmRole) []swarmInput {
	var out []swarmInput
	for _, a := range r.After {
		rs := t.roleState(a)
		for _, u := range t.st.Units {
			if u.Role == a && u.State == "done" && (rs.Shape != "race" || u.Unit == rs.Chosen) {
				out = append(out, swarmInput{Role: a, Unit: u.Unit, Branch: "d/" + u.Run, Head: u.Head})
			}
		}
	}
	return out
}

func (t *swarmTick) launch() error {
	for _, r := range t.spec.Roles {
		ready := true
		for _, a := range r.After {
			ready = ready && t.roleComplete(a)
		}
		if !ready || t.roleState(r.Name).Chosen != "" {
			continue
		}
		active := 0
		for _, u := range t.st.Units {
			if u.Role == r.Name && swarmActive(u.State) {
				active++
			}
		}
		for i := range t.st.Units {
			u := &t.st.Units[i]
			if u.Role != r.Name || u.State != "pending" || t.now.Before(u.NextTry) {
				continue
			}
			if active >= r.replicas() {
				break
			}
			if e := t.start(u, true); e != nil {
				return e
			}
			active++
		}
	}
	return nil
}

func swarmOrderPath(name, run string) string {
	return filepath.Join(OrchestratorDir(name), "orders", run+".md")
}

// start writes the attempting record before the launch. A fresh start
// renders the order; a resend reuses the recorded one.
func (t *swarmTick) start(u *SwarmUnit, fresh bool) error {
	r := t.role(u.Role)
	path := swarmOrderPath(t.st.Name, swarmRunName(t.st.Name, r, u.Unit, max(1, u.Gen)))
	var text []byte
	if fresh || !exists(path) {
		u.Gen = max(1, u.Gen)
		u.Run = swarmRunName(t.st.Name, r, u.Unit, u.Gen)
		order := swarmOrderText(t.spec, r, u.Unit, u.Gen, t.host, t.inputs(r))
		if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
			return e
		}
		if e := os.WriteFile(path, []byte(order), 0600); e != nil {
			return e
		}
		text = []byte(order)
		o, _ := Parse(order)
		u.Host, u.Base = t.host, o.Base
		u.State, u.Op, u.Attempted, u.Reason = "attempting", "start", time.Now().UTC(), ""
		if e := t.save(); e != nil {
			return e
		}
	} else {
		b, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		text = b
		t.log("%s attempting with no run in inventory; sending the same start again", u.Run)
	}
	if swarmBeforeLaunch != nil {
		if e := swarmBeforeLaunch(*u); e != nil {
			return e
		}
	}
	e := t.ops.Start(t.ctx, u.Run, string(text))
	switch {
	case e == nil:
		t.log("%s start sent", u.Run)
	case errors.Is(e, errRunExists) || strings.Contains(e.Error(), errRunExists.Error()):
		// The first start landed; the next tick observes it.
		t.log("%s already exists on the target; not started twice", u.Run)
	default:
		u.State, u.Op, u.Reason = "pending", "", "launch refused: "+e.Error()
		u.NextTry = t.now.Add(t.spec.backoff(0))
		t.log("%s launch refused: %v", u.Run, e)
	}
	return nil
}
