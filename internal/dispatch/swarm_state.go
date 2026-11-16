package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"
	"time"
)

// SwarmState is swarm.json: desired state and the unit table (RFC 0013 §5).
type SwarmState struct {
	Name     string           `json:"name"`
	Lifetime time.Time        `json:"lifetime"`
	Down     bool             `json:"down,omitempty"`
	Roles    []SwarmRoleState `json:"roles"`
	Units    []SwarmUnit      `json:"units"`
	Restarts int              `json:"restarts"`
	Ticks    int              `json:"ticks"`
	LastTick time.Time        `json:"last_tick,omitempty"`
	// Noted keys round notes already sent this lifetime.
	Noted map[string]bool `json:"noted,omitempty"`
}

type SwarmRoleState struct {
	Name         string    `json:"name"`
	Shape        string    `json:"shape"`
	Replicas     int       `json:"replicas"`
	After        []string  `json:"after,omitempty"`
	Effects      string    `json:"effects"`
	Choose       string    `json:"choose,omitempty"`
	Chosen       string    `json:"chosen,omitempty"`
	ChosenBy     string    `json:"chosen_by,omitempty"`
	ChoiceReason string    `json:"choice_reason,omitempty"`
	ChosenAt     time.Time `json:"chosen_at,omitempty"`
}

// SwarmUnit is one unit record. States: pending, attempting, queued, live,
// done, failed (a repair in place is scheduled), lost, held, superseded.
type SwarmUnit struct {
	Role     string `json:"role"`
	Unit     string `json:"unit"`
	Gen      int    `json:"gen"`
	Run      string `json:"run,omitempty"`
	Host     string `json:"host,omitempty"`
	State    string `json:"state"`
	Base     string `json:"base,omitempty"`
	Head     string `json:"head,omitempty"`
	Restarts int    `json:"restarts"`
	// NextTry is the earliest launch or repair; it carries the backoff.
	NextTry time.Time `json:"next_try,omitempty"`
	// Op and Attempted describe an attempting record: start or retry, and
	// when the record was written, before the call.
	Op         string       `json:"op,omitempty"`
	Attempted  time.Time    `json:"attempted,omitempty"`
	RunState   string       `json:"run_state,omitempty"`
	Validation string       `json:"validation,omitempty"`
	Reason     string       `json:"reason,omitempty"`
	Failure    string       `json:"failure,omitempty"`
	Stalls     int          `json:"stalls,omitempty"`
	Progress   string       `json:"progress,omitempty"`
	ProgressAt time.Time    `json:"progress_at,omitempty"`
	HeldAt     time.Time    `json:"held_at,omitempty"`
	DoneAt     time.Time    `json:"done_at,omitempty"`
	Chosen     bool         `json:"chosen,omitempty"`
	Fenced     []SwarmFence `json:"fenced"`
}

// SwarmFence records a fenced generation. Phase 1 repairs in place and never
// fences; the field keeps the record shape stable for phase 3.
type SwarmFence struct {
	Gen  int       `json:"gen"`
	Run  string    `json:"run"`
	Host string    `json:"host"`
	At   time.Time `json:"at"`
	Head string    `json:"head,omitempty"`
}

func swarmActive(state string) bool {
	return state == "attempting" || state == "queued" || state == "live" || state == "failed"
}

func IsSwarm(name string) bool {
	return ValidName(name) && exists(filepath.Join(OrchestratorDir(name), "swarm.json"))
}

func lockSwarm(name string) (func(), error) {
	p := filepath.Join(OrchestratorDir(name), "swarm-lock")
	if e := os.MkdirAll(p, 0700); e != nil {
		return nil, e
	}
	return lockDirectory(p)
}

func loadSwarmSpec(name string) (SwarmSpec, error) {
	b, e := os.ReadFile(filepath.Join(OrchestratorDir(name), "swarm.md"))
	if e != nil {
		return SwarmSpec{}, e
	}
	return ParseSwarm(string(b))
}

func loadSwarm(name string) (SwarmSpec, SwarmState, error) {
	var st SwarmState
	if !IsSwarm(name) {
		return SwarmSpec{}, st, fmt.Errorf("no swarm named %s", name)
	}
	s, e := loadSwarmSpec(name)
	if e != nil {
		return s, st, e
	}
	return s, st, readJSON(filepath.Join(OrchestratorDir(name), "swarm.json"), &st)
}

func saveSwarmState(st *SwarmState) error {
	return WriteJSON(filepath.Join(OrchestratorDir(st.Name), "swarm.json"), st)
}

func swarmLog(name string, now time.Time, format string, args ...any) {
	f, e := os.OpenFile(filepath.Join(OrchestratorDir(name), "swarm.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if e != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n", now.UTC().Format(time.RFC3339), fmt.Sprintf(format, args...))
}

// swarmDesired builds the unit table from the file, keeping every existing
// record, generation and restart count (RFC 0013: up again keeps records).
func swarmDesired(s SwarmSpec, prev *SwarmState, lifetime time.Time) SwarmState {
	st := SwarmState{Name: s.Name, Lifetime: lifetime, Units: []SwarmUnit{}}
	if prev != nil {
		st.Restarts, st.Ticks, st.LastTick = prev.Restarts, prev.Ticks, prev.LastTick
		if prev.Lifetime.Equal(lifetime) {
			st.Noted = prev.Noted
		}
	}
	old := map[string]SwarmUnit{}
	oldRoles := map[string]SwarmRoleState{}
	if prev != nil {
		for _, u := range prev.Units {
			old[u.Role+"/"+u.Unit] = u
		}
		for _, r := range prev.Roles {
			oldRoles[r.Name] = r
		}
	}
	for _, r := range s.Roles {
		rs := SwarmRoleState{Name: r.Name, Shape: r.Shape, Replicas: r.replicas(), After: r.After, Effects: r.effects()}
		if r.Shape == "race" {
			rs.Choose = r.choose()
		}
		if o, ok := oldRoles[r.Name]; ok {
			rs.Chosen, rs.ChosenBy, rs.ChoiceReason, rs.ChosenAt = o.Chosen, o.ChosenBy, o.ChoiceReason, o.ChosenAt
		}
		st.Roles = append(st.Roles, rs)
		for _, u := range r.units() {
			key := r.Name + "/" + u
			if o, ok := old[key]; ok {
				delete(old, key)
				if o.State == "held" && o.Reason == "down" {
					// A new lifetime observes the stopped run and repairs it.
					o.State, o.Reason = "live", "resumed by up"
				}
				st.Units = append(st.Units, o)
				continue
			}
			st.Units = append(st.Units, SwarmUnit{Role: r.Name, Unit: u, State: "pending", Fenced: []SwarmFence{}})
		}
	}
	// Units removed from the file keep their records when they ever ran.
	for _, u := range prev.unitsIn(old) {
		if u.Run == "" {
			continue
		}
		if swarmActive(u.State) || u.State == "pending" {
			u.State, u.Reason = "held", "removed from the swarm file"
		}
		st.Units = append(st.Units, u)
	}
	return st
}

func (p *SwarmState) unitsIn(set map[string]SwarmUnit) []SwarmUnit {
	if p == nil {
		return nil
	}
	var out []SwarmUnit
	for _, u := range p.Units {
		if _, ok := set[u.Role+"/"+u.Unit]; ok {
			out = append(out, u)
		}
	}
	return out
}

// Hooks replaced by tests: launching the orchestrator supervisor, and the
// wait for a stopped run to report a terminal state.
var (
	swarmLaunch   = LaunchOrchestrator
	swarmStopWait = 20 * time.Second
)

// SwarmUp validates and compiles a swarm file. A dry run writes nothing.
// Otherwise it applies the orchestrator, writes swarm.md and swarm.json, and
// launches the supervisor, whose reconcile tick launches the units.
func SwarmUp(text string, dry bool) (SwarmPlan, error) {
	plan, e := CompileSwarm(text)
	if e != nil {
		return plan, e
	}
	if dry {
		_, e = ApplyOrchestrator(plan.OrchSpec, true)
		return plan, e
	}
	name := plan.Spec.Name
	if _, e := LoadOrchestrator(name); e == nil && !IsSwarm(name) {
		return plan, fmt.Errorf("orchestrator %s exists and is not a swarm", name)
	}
	dir := OrchestratorDir(name)
	if processRecordLive(filepath.Join(dir, "supervisor.json")) {
		cur, _ := os.ReadFile(filepath.Join(OrchestratorConfigDir(), name+".md"))
		if string(cur) != plan.OrchSpec {
			return plan, fmt.Errorf("swarm %s is running and its orchestrator fields changed; run hev swarm down %s, then up", name, name)
		}
		// Desired state changes in place; the next tick acts on it.
		return plan, writeSwarmFiles(plan, text, false)
	}
	if _, e = ApplyOrchestrator(plan.OrchSpec, false); e != nil {
		return plan, e
	}
	if e = writeSwarmFiles(plan, text, true); e != nil {
		return plan, e
	}
	return plan, swarmLaunch(name)
}

func writeSwarmFiles(plan SwarmPlan, text string, lifetime bool) error {
	name := plan.Spec.Name
	unlock, e := lockSwarm(name)
	if e != nil {
		return e
	}
	defer unlock()
	dir := OrchestratorDir(name)
	var prev *SwarmState
	var old SwarmState
	if readJSON(filepath.Join(dir, "swarm.json"), &old) == nil {
		prev = &old
	}
	start := time.Now().UTC()
	if !lifetime && prev != nil {
		start = prev.Lifetime
	}
	st := swarmDesired(plan.Spec, prev, start)
	if e = os.WriteFile(filepath.Join(dir, "swarm.md"), []byte(text), 0600); e != nil {
		return e
	}
	if e = saveSwarmState(&st); e != nil {
		return e
	}
	swarmLog(name, start, "up: %d roles, %d units", len(st.Roles), len(st.Units))
	return nil
}

// WriteSwarmPlan prints up's summary, and with full the compiled spec and
// every order, each between --- NAME --- separators.
func WriteSwarmPlan(w io.Writer, p SwarmPlan, full bool) {
	s := p.Spec
	host := p.Host
	if host == "" {
		host = "this host"
	}
	most := 0
	for _, r := range s.Roles {
		most += r.replicas()
	}
	most = min(most, s.Budget.ConcurrentRuns)
	fmt.Fprintf(w, "swarm %s: owner %s, %d roles, %d units, max %d concurrent\n", s.Name, host, len(s.Roles), len(p.Orders), most)
	fmt.Fprintf(w, "  orchestrator o-%s  authority %s\n", s.Name, strings.Join(p.Orch.Authority, ","))
	for _, r := range s.Roles {
		n := len(r.units())
		desc := fmt.Sprintf("%d unit", n)
		switch r.Shape {
		case "partition":
			desc = fmt.Sprintf("%d pieces  replicas %d", n, r.replicas())
		case "race":
			desc = fmt.Sprintf("%d variants  replicas %d  choose %s", n, r.replicas(), r.choose())
		}
		if len(r.After) > 0 {
			desc += "  after " + strings.Join(r.After, ",")
		}
		fmt.Fprintf(w, "  %-10s %s  effects %s  placement %s\n", r.Name, desc, r.effects(), host)
	}
	fmt.Fprintln(w, "  state      swarm.json on this host (phase 1: no state ref, no lease)")
	if !full {
		return
	}
	fmt.Fprintf(w, "--- orchestrator %s ---\n%s", s.Name, p.OrchSpec)
	for _, o := range p.Orders {
		fmt.Fprintf(w, "--- order %s ---\n%s", o.Run, o.Text)
	}
}

// SwarmView is what ps shows.
type SwarmView struct {
	Name         string           `json:"name"`
	Host         string           `json:"host,omitempty"`
	Orchestrator string           `json:"orchestrator_state"`
	Rounds       int              `json:"rounds"`
	Down         bool             `json:"down,omitempty"`
	Ticks        int              `json:"ticks"`
	LastTick     time.Time        `json:"last_tick,omitempty"`
	Restarts     int              `json:"restarts"`
	Roles        []SwarmRoleState `json:"roles"`
	Units        []SwarmUnit      `json:"units"`
}

func SwarmPS(name string) (SwarmView, error) {
	_, st, e := loadSwarm(name)
	if e != nil {
		return SwarmView{}, e
	}
	ost, _ := orchestratorState(name)
	v := SwarmView{Name: name, Host: ost.Host, Orchestrator: ost.State, Rounds: ost.Rounds, Down: st.Down, Ticks: st.Ticks, LastTick: st.LastTick, Restarts: st.Restarts, Roles: st.Roles, Units: st.Units}
	dir := OrchestratorDir(name)
	switch {
	case exists(filepath.Join(dir, "stop")):
		v.Orchestrator = "stopped"
	case exists(filepath.Join(dir, "pause")):
		v.Orchestrator = "paused"
	}
	return v, nil
}

// ListSwarms returns every swarm applied on this host.
func ListSwarms() ([]SwarmView, error) {
	entries, e := os.ReadDir(Root())
	if os.IsNotExist(e) {
		return []SwarmView{}, nil
	}
	if e != nil {
		return nil, e
	}
	out := []SwarmView{}
	for _, f := range entries {
		n, ok := strings.CutPrefix(f.Name(), "orch-")
		if !ok || !IsSwarm(n) {
			continue
		}
		v, e := SwarmPS(n)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, nil
}

func WriteSwarmPS(w io.Writer, v SwarmView) {
	state := v.Orchestrator
	if v.Down {
		state = "down"
	}
	fmt.Fprintf(w, "swarm %s: %s, round %d, tick %d, restarts %d\n", v.Name, state, v.Rounds, v.Ticks, v.Restarts)
	roles := map[string]SwarmRoleState{}
	for _, r := range v.Roles {
		roles[r.Name] = r
	}
	done := func(role string) bool {
		r := roles[role]
		any := false
		for _, u := range v.Units {
			if u.Role != role {
				continue
			}
			any = true
			if r.Shape == "race" {
				if u.Chosen && u.State == "done" {
					return true
				}
				continue
			}
			if u.State != "done" {
				return false
			}
		}
		return any && r.Shape != "race"
	}
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "ROLE\tUNIT\tGEN\tHOST\tSTATE\tHEAD\tRESTARTS\tNOTE")
	for _, u := range v.Units {
		gen, host, head, state, note := "-", "-", "-", u.State, u.Reason
		if u.Gen > 0 {
			gen = fmt.Sprint(u.Gen)
		}
		if u.Host != "" {
			host = u.Host
		} else if u.Run != "" {
			host = "local"
		}
		if u.Head != "" {
			head = u.Head[:min(7, len(u.Head))]
		}
		if u.State == "pending" {
			var waiting []string
			for _, a := range roles[u.Role].After {
				if !done(a) {
					waiting = append(waiting, a)
				}
			}
			if len(waiting) > 0 {
				state, note = "waiting", "(after "+strings.Join(waiting, ",")+")"
			}
		}
		if u.Chosen {
			note = strings.TrimSpace("chosen by " + roles[u.Role].ChosenBy + " " + note)
		}
		if u.State == "failed" && !u.NextTry.IsZero() {
			note = strings.TrimPrefix(note+"; retry at "+u.NextTry.UTC().Format("15:04:05Z"), "; ")
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\n", u.Role, swarmUnitLabel(u.Unit), gen, host, state, head, u.Restarts, note)
	}
	tw.Flush()
}

// SwarmPause and SwarmResume reuse orchestrate pause and resume. A pause
// stops rounds, launches and repairs; running workers continue.
func SwarmPause(name string) error {
	if !IsSwarm(name) {
		return fmt.Errorf("no swarm named %s", name)
	}
	return OrchestratorControl(name, "pause", "")
}

func SwarmResume(name string) error {
	if !IsSwarm(name) {
		return fmt.Errorf("no swarm named %s", name)
	}
	return OrchestratorControl(name, "resume", "")
}

// SwarmDownReport says what down stopped and what it left in place.
type SwarmDownReport struct {
	Name      string   `json:"name"`
	Stopped   []string `json:"stopped"`
	Unstopped []string `json:"unconfirmed,omitempty"`
	Branches  []string `json:"branches"`
	Worktrees []string `json:"worktrees"`
}

// SwarmDown stops the orchestrator, then every current run. Worktrees,
// branches and records stay. It acts as the operator: a stopped orchestrator's
// identity is refused by admission, and down must still reach its runs.
func SwarmDown(ctx context.Context, name string, ops SwarmOps) (SwarmDownReport, error) {
	rep := SwarmDownReport{Name: name, Stopped: []string{}, Branches: []string{}, Worktrees: []string{}}
	if !IsSwarm(name) {
		return rep, fmt.Errorf("no swarm named %s", name)
	}
	if e := OrchestratorControl(name, "stop", ""); e != nil {
		return rep, e
	}
	if ops == nil {
		ops = newLocalSwarmOps("")
	}
	unlock, e := lockSwarm(name)
	if e != nil {
		return rep, e
	}
	defer unlock()
	spec, st, e := loadSwarm(name)
	if e != nil {
		return rep, e
	}
	now := time.Now().UTC()
	t := &swarmTick{ctx: ctx, spec: spec, st: &st, ops: ops, now: now}
	for i := range st.Units {
		u := &st.Units[i]
		if u.Run != "" && (u.State == "attempting" || u.State == "queued" || u.State == "live") {
			if _, ok := t.confirmStopped(u.Run); ok {
				rep.Stopped = append(rep.Stopped, u.Run)
			} else {
				rep.Unstopped = append(rep.Unstopped, u.Run)
			}
		}
		if swarmActive(u.State) {
			u.State, u.Reason, u.HeldAt = "held", "down", now
		}
		if u.Run != "" {
			rep.Branches = append(rep.Branches, "d/"+u.Run)
			if wt := filepath.Join(Root(), u.Run, "wt"); exists(wt) {
				rep.Worktrees = append(rep.Worktrees, wt)
			}
		}
	}
	st.Down = true
	swarmLog(name, now, "down: stopped %d runs", len(rep.Stopped))
	return rep, saveSwarmState(&st)
}

func WriteSwarmDown(w io.Writer, r SwarmDownReport) {
	fmt.Fprintf(w, "stopped o-%s\n", r.Name)
	if len(r.Stopped) == 0 {
		fmt.Fprintln(w, "runs: 0 stopped (none live)")
	} else {
		fmt.Fprintf(w, "runs: %d stopped (%s)\n", len(r.Stopped), strings.Join(r.Stopped, ", "))
	}
	if len(r.Unstopped) > 0 {
		fmt.Fprintf(w, "runs: %d did not confirm a stop; inspect with hev dispatch show: %s\n", len(r.Unstopped), strings.Join(r.Unstopped, ", "))
	}
	fmt.Fprintf(w, "kept: %d branches\n", len(r.Branches))
	for _, b := range r.Branches {
		fmt.Fprintf(w, "  %s\n", b)
	}
	fmt.Fprintf(w, "kept: %d worktrees\n", len(r.Worktrees))
	fmt.Fprintf(w, "records: hev swarm ps %s\n", r.Name)
}

// SwarmUnitRun resolves ROLE/UNIT, or ROLE for a single role, to its run.
func SwarmUnitRun(name, address string) (string, bool) {
	_, st, e := loadSwarm(name)
	if e != nil {
		return "", false
	}
	role, unit, slash := strings.Cut(address, "/")
	for _, u := range st.Units {
		if u.Role == role && (u.Unit == unit || (!slash && u.Unit == "")) && u.Run != "" {
			return u.Run, true
		}
	}
	return "", false
}

// SwarmChoose records a race's one chosen unit. by is first-green (code),
// orchestrator (a round's choice.json) or person (hev swarm say NAME choose).
func SwarmChoose(name, address, reason, by string) error {
	unlock, e := lockSwarm(name)
	if e != nil {
		return e
	}
	defer unlock()
	spec, st, e := loadSwarm(name)
	if e != nil {
		return e
	}
	if e = chooseSwarmUnit(spec, &st, address, reason, by, time.Now().UTC()); e != nil {
		return e
	}
	return saveSwarmState(&st)
}

func chooseSwarmUnit(spec SwarmSpec, st *SwarmState, address, reason, by string, now time.Time) error {
	role, unit, ok := strings.Cut(address, "/")
	if !ok {
		// A bare variant is accepted when exactly one race has it.
		unit, role = address, ""
		for _, r := range spec.Roles {
			if r.Shape == "race" && slices.Contains(r.units(), unit) {
				if role != "" {
					return fmt.Errorf("variant %s is ambiguous; name ROLE/%s", unit, unit)
				}
				role = r.Name
			}
		}
	}
	r, found := spec.role(role)
	if !found || r.Shape != "race" {
		return fmt.Errorf("%s is not a race unit", address)
	}
	if r.choose() != by {
		return fmt.Errorf("role %s is chosen by %s, not %s", role, r.choose(), by)
	}
	ri := slices.IndexFunc(st.Roles, func(x SwarmRoleState) bool { return x.Name == role })
	ui := slices.IndexFunc(st.Units, func(x SwarmUnit) bool { return x.Role == role && x.Unit == unit })
	if ri < 0 || ui < 0 {
		return fmt.Errorf("%s is not a race unit", address)
	}
	if c := st.Roles[ri].Chosen; c != "" {
		if c == unit {
			return nil
		}
		return fmt.Errorf("role %s already chose %s", role, c)
	}
	if st.Units[ui].State != "done" {
		return fmt.Errorf("%s/%s is %s; only a done unit with a pushed head can be chosen", role, unit, st.Units[ui].State)
	}
	st.Roles[ri].Chosen, st.Roles[ri].ChosenBy, st.Roles[ri].ChoiceReason, st.Roles[ri].ChosenAt = unit, by, reason, now
	st.Units[ui].Chosen = true
	swarmLog(st.Name, now, "%s chose %s/%s: %s", by, role, unit, reason)
	return nil
}

// applySwarmRoundChoice records a round's choice.json after the round's
// checkpoint is accepted. A refused choice is logged, never fatal.
func applySwarmRoundChoice(name, round string) {
	if !IsSwarm(name) {
		return
	}
	var c struct {
		Role   string `json:"role"`
		Unit   string `json:"unit"`
		Reason string `json:"reason"`
	}
	if readJSON(filepath.Join(round, "choice.json"), &c) != nil {
		return
	}
	if e := SwarmChoose(name, c.Role+"/"+c.Unit, c.Reason, "orchestrator"); e != nil {
		swarmLog(name, time.Now(), "round choice refused: %v", e)
	}
}

// swarmPacket gives a round the unit table.
func swarmPacket(name string) json.RawMessage {
	if !IsSwarm(name) {
		return nil
	}
	b, e := os.ReadFile(filepath.Join(OrchestratorDir(name), "swarm.json"))
	if e != nil || !json.Valid(b) {
		return nil
	}
	return b
}

// SwarmRunLogs reads a unit's current run log from offset, as the operator.
func SwarmRunLogs(ctx context.Context, run string, offset int64) (Response, error) {
	return newLocalSwarmOps("").rpc(ctx, Request{Action: "logs", Name: run, Offset: offset})
}

// SwarmSayRun sends a message to a unit's current run, as the operator.
func SwarmSayRun(ctx context.Context, run, msg string) (Response, error) {
	return newLocalSwarmOps("").rpc(ctx, Request{Action: "say", Name: run, Note: msg})
}

// SwarmSayOrchestrator messages the swarm's orchestrator and wakes it.
func SwarmSayOrchestrator(name, msg string) error {
	if !IsSwarm(name) {
		return fmt.Errorf("no swarm named %s", name)
	}
	if e := SayOrchestrator(name, msg); e != nil {
		return e
	}
	return orchestratorLaunch(name)
}
