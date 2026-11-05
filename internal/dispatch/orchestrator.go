package dispatch

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hev/kit/internal/platformops"
)

type OrchestratorState struct {
	LastNotice          *OrchestratorNotice `json:"last_notice"`
	ConsecutiveFailures int                 `json:"consecutive_failures"`
	Name                string              `json:"name"`
	State               string              `json:"state"`
	Goal                string              `json:"goal"`
	Host                string              `json:"host,omitempty"`
	Reply               string              `json:"reply"`
	Mode                string              `json:"mode"`
	Start               time.Time           `json:"start"`
	Next                time.Time           `json:"next_wake,omitempty"`
	Rounds              int                 `json:"rounds"`
	Lineage             int                 `json:"lineage"`
	Digest              string              `json:"digest,omitempty"`
	Error               string              `json:"error,omitempty"`
	ManagedRuns         int                 `json:"managed_runs"`
	RunsToday           int                 `json:"runs_today"`
	SpendUSD            *float64            `json:"spend_usd"`
	// Deferred names work a source round left for later; state stays waiting.
	Deferred string         `json:"deferred,omitempty"`
	Signals  []IntakeSignal `json:"signals,omitempty"`
}
type OrchestratorPacket struct {
	Source     SourceBatch            `json:"source"`
	Spec       OrchestratorSpec       `json:"spec"`
	Checkpoint string                 `json:"checkpoint"`
	Inbox      string                 `json:"inbox"`
	Evidence   []OrchestratorEvidence `json:"evidence"`
	Rotate     bool                   `json:"rotate"`
}
type OrchestratorEvidence struct {
	Host  string            `json:"host"`
	Row   Row               `json:"run"`
	Files map[string]string `json:"files"`
	PR    json.RawMessage   `json:"pr,omitempty"`
}

// GatherOrchestratorEvidence generalizes the inventory/inspect evidence path
// used by the expediter: select inventory, then inspect run and PR/CI state.
func GatherOrchestratorEvidence(ctx context.Context, s OrchestratorSpec) ([]OrchestratorEvidence, error) {
	cfg, e := Load(ConfigPath(""))
	if e != nil {
		return nil, e
	}
	inv := Inventory(cfg, "")
	out := []OrchestratorEvidence{}
	rows, e := SelectInventoryEvidence(inv, func(row Row) bool { return row.Kind != "orch" && s.Includes(row.Order) })
	if e != nil {
		return nil, e
	}
	for _, r := range rows {
		host, row := r.Host, r.Row
		detail, e := RPC(ctx, cfg.Hosts[host], Request{Action: "show", Name: row.Name})
		if e != nil {
			return nil, e
		}
		files := map[string]string{}
		for _, f := range []string{"status.md", "report.md", "result.json", "validation.json", "verdict.json", "inbox.md"} {
			files[f] = detail.Files[f]
		}
		// Supervisor timestamps change on every inventory; they are not evidence.
		row.Schedule = Schedule{}
		ev := OrchestratorEvidence{Host: host, Row: row, Files: files}
		if row.PR != "" {
			b, e := helperCommand(ctx, "gh", "pr", "view", row.PR, "--json", "headRefOid,state,statusCheckRollup").Output()
			if e != nil {
				return nil, fmt.Errorf("PR evidence for %s: %w", row.Name, e)
			}
			ev.PR = b
		}
		out = append(out, ev)
	}
	return out, nil
}

// OrchestratorDriver makes both evidence and fresh sessions injectable. Tests
// never require network access or a model process.
type OrchestratorDriver struct {
	Gather  func(context.Context, OrchestratorSpec) ([]OrchestratorEvidence, error)
	Session func(context.Context, OrchestratorSpec, string, OrchestratorPacket) error
	Now     func() time.Time
	Wait    func(context.Context) error
	Notify  OrchestratorNotifier
}

func DefaultOrchestratorDriver() OrchestratorDriver {
	return OrchestratorDriver{Gather: GatherOrchestratorEvidence, Session: orchestratorSession, Now: time.Now}
}
func orchestratorSession(ctx context.Context, s OrchestratorSpec, dir string, p OrchestratorPacket) error {
	prompt := "Read packet.json. Act only through authorized hev dispatch verbs. Do not write code or merge. Write checkpoint.md here before exiting: goal status, open threads by run name, named waits and release conditions, next action, and reply notes. When the owner should know about a decision needed, blocker, ready-to-merge result (include head SHA and evidence), goal met, or rework sent, also write a short reply.md here. Nothing to say means no reply.md. Keep reply notes in checkpoint.md as the record; code delivers reply.md after acceptance when notify authority is granted. Do not send it yourself. If the goal is complete, write goal-met.json containing {\"met\":true}."
	if p.Rotate {
		prompt += " This is a rotation round: write a short summary checkpoint that replaces the previous lineage."
	}
	args := []string{"exec", "--json", "--dangerously-bypass-approvals-and-sandbox"}
	if s.Harness == "claude" {
		args = []string{"-p", "--verbose", "--output-format", "stream-json", "--dangerously-skip-permissions"}
	}
	if s.Model != "" {
		args = append(args, "--model", s.Model)
	}
	args = append(args, prompt)
	// Fresh Codex sessions may run outside a repository in the checkpoint store.
	if s.Harness == "codex" {
		args = append(args[:len(args)-1], "--skip-git-repo-check", prompt)
	}
	c := exec.CommandContext(ctx, s.Harness, args...)
	setupChild(c)
	c.Cancel = func() error { killTree(c); return nil }
	c.Dir = dir
	c.Env = childEnvironment(s.Name)
	f, e := os.OpenFile(filepath.Join(dir, "trace"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	c.Stdout = f
	c.Stderr = f
	cleanup, e := startChild(c)
	if e != nil {
		return e
	}
	defer cleanup()
	if e = recordHarness(OrchestratorDir(s.Name), c); e != nil {
		killTree(c)
		_ = c.Wait()
		return e
	}
	defer os.Remove(filepath.Join(OrchestratorDir(s.Name), "harness.json"))
	if exists(filepath.Join(OrchestratorDir(s.Name), "stop")) {
		killTree(c)
	}
	return c.Wait()
}
func orchestratorState(name string) (OrchestratorState, error) {
	var s OrchestratorState
	e := readJSON(filepath.Join(OrchestratorDir(name), "state.json"), &s)
	_ = readJSON(filepath.Join(OrchestratorDir(name), "last-notice.json"), &s.LastNotice)
	return s, e
}
func ListOrchestrators() ([]OrchestratorState, error) {
	entries, e := os.ReadDir(OrchestratorConfigDir())
	if os.IsNotExist(e) {
		return []OrchestratorState{}, nil
	}
	if e != nil {
		return nil, e
	}
	out := []OrchestratorState{}
	for _, f := range entries {
		if !strings.HasSuffix(f.Name(), ".md") {
			continue
		}
		n := strings.TrimSuffix(f.Name(), ".md")
		s, e := orchestratorState(n)
		if e != nil {
			return nil, e
		}
		if s.Signals, e = OrchestratorSignals(n); e != nil {
			return nil, e
		}
		if exists(filepath.Join(OrchestratorDir(n), "stop")) {
			s.State = "stopped"
		} else if exists(filepath.Join(OrchestratorDir(n), "pause")) {
			s.State = "paused"
		}
		var b struct {
			Day  string `json:"day"`
			Runs int    `json:"runs"`
		}
		for _, f := range []string{"budget.json", "budget-owner.json", "source-budget.json"} {
			b.Day = ""
			b.Runs = 0
			if readJSON(filepath.Join(OrchestratorDir(n), f), &b) == nil && b.Day == time.Now().UTC().Format("2006-01-02") && b.Runs > s.RunsToday {
				s.RunsToday = b.Runs
			}
		}
		out = append(out, s)
	}
	return out, nil
}
func exists(p string) bool { _, e := os.Stat(p); return e == nil }
func ApplyOrchestrator(text string, dry bool) (OrchestratorSpec, error) {
	return applyOrchestrator(text, dry, true)
}
func applyOrchestrator(text string, dry, notices bool) (OrchestratorSpec, error) {
	s, e := ParseOrchestrator(text)
	if e != nil {
		return s, e
	}
	if parent := currentOrchestratorIdentity(); parent != "" {
		if e := admitOrchestratorChild(parent, s.Name, s); e != nil {
			return s, e
		}
	}
	if notices && s.Reply != "" {
		if _, e = noticeSenderAddress(s); e != nil {
			return s, e
		}
	}
	var unlock func()
	if !dry {
		if e = os.MkdirAll(Root(), 0700); e != nil {
			return s, e
		}
		unlock, e = lockRoot()
		if e != nil {
			return s, e
		}
		defer unlock()
	}
	specs := map[string]OrchestratorSpec{}
	entries, e := os.ReadDir(OrchestratorConfigDir())
	if e != nil && !os.IsNotExist(e) {
		return s, e
	}
	for _, f := range entries {
		if strings.HasSuffix(f.Name(), ".md") {
			n := strings.TrimSuffix(f.Name(), ".md")
			if n == s.Name {
				continue
			}
			other, e := LoadOrchestrator(n)
			if e != nil {
				return s, e
			}
			specs[n] = other
		}
	}
	specs[s.Name] = s
	for _, parent := range specs {
		for _, n := range parent.Manages.Runs.Names {
			if child, ok := specs[strings.TrimPrefix(n, "orch-")]; ok {
				if e := orchestratorSubset(parent, child); e != nil {
					return s, e
				}
			}
		}
	}
	if e = ValidateOrchestratorNesting(specs); e != nil {
		return s, e
	}
	if dry {
		return s, nil
	}
	if e = os.MkdirAll(OrchestratorConfigDir(), 0700); e != nil {
		return s, e
	}
	dir := OrchestratorDir(s.Name)
	if e = os.MkdirAll(dir, 0700); e != nil {
		return s, e
	}
	if processRecordLive(filepath.Join(dir, "supervisor.json")) {
		return s, fmt.Errorf("stop orchestrator before applying changes")
	}
	if e = os.WriteFile(filepath.Join(OrchestratorConfigDir(), s.Name+".md"), []byte(text), 0600); e != nil {
		return s, e
	}
	if e = os.WriteFile(filepath.Join(dir, "spec.md"), []byte(text), 0600); e != nil {
		return s, e
	}
	// Reapplying preserves checkpoints and round numbers, but starts a new lifetime.
	st, _ := orchestratorState(s.Name)
	st.Name = s.Name
	st.Goal = s.Goal
	st.Host = s.Host
	st.Reply = s.Reply
	st.Mode = s.Mode
	st.State = "ready"
	st.Start = time.Now().UTC()
	st.Next = time.Time{}
	st.Error = ""
	st.Deferred = ""
	st.ConsecutiveFailures = 0
	st.Digest = "" // A changed spec/brief must get a new model round.
	for _, f := range []string{"stop", "pause", "wake", "handoff.json", "scope-owner.json"} {
		_ = os.Remove(filepath.Join(dir, f))
	}
	if e = WriteJSON(filepath.Join(dir, "state.json"), st); e != nil {
		return s, e
	}
	if notices {
		// Notices queued before this apply belong to the old lineage.
		if e = staleOrchestratorNotices(s.Name); e != nil {
			return s, e
		}
		// Apply only queues; the launched supervisor delivers.
		_ = orchestratorLifecycle(s, st, "started/applied", "applied")
	}
	return s, nil
}
func LaunchOrchestrator(name string) error {
	if parent := currentOrchestratorIdentity(); parent != "" {
		if e := admitOrchestratorChild(parent, name); e != nil {
			return e
		}
	}
	if _, e := LoadOrchestrator(name); e != nil {
		return e
	}
	dir := OrchestratorDir(name)
	if exists(filepath.Join(dir, "scope-owner.json")) {
		return fmt.Errorf("scope-only registration cannot launch rounds")
	}
	st, e := orchestratorState(name)
	if e != nil {
		return e
	}
	if st.State == "handed-off" || st.State == "complete" {
		return fmt.Errorf("orchestrator is terminal; apply again before launching")
	}
	if processRecordLive(filepath.Join(dir, "supervisor.json")) {
		return nil
	}
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	if handled, err := launchOrchestratorTmux(exe, name); handled {
		return err
	}
	return startOrchestratorSupervisor(dir, name, orchestratorSuperviseArgs(exe, name))
}

// orchestratorSuperviseArgs is replaced by runtime tests that launch a helper.
var orchestratorSuperviseArgs = func(exe, name string) []string {
	return []string{exe, "orchestrate", "supervise", name}
}

// OrchestratorControl queues lifecycle notices under the root lock and
// delivers only after unlocking. A live supervisor delivers them itself.
func OrchestratorControl(name, action, to string) error {
	spec, e := LoadOrchestrator(name)
	if e != nil {
		return e
	}
	if e = orchestratorControl(spec, action, to); e != nil {
		return e
	}
	if !processRecordLive(filepath.Join(OrchestratorDir(name), "supervisor.json")) {
		flushOrchestratorNotices(spec, orchestratorNoticeSender)
	}
	return nil
}
func orchestratorControl(spec OrchestratorSpec, action, to string) error {
	name := spec.Name
	dir := OrchestratorDir(name)
	if parent := currentOrchestratorIdentity(); parent != "" {
		if e := admitOrchestratorChild(parent, name); e != nil {
			return e
		}
	}
	unlock, e := lockRoot()
	if e != nil {
		return e
	}
	defer unlock()
	st, e := orchestratorState(name)
	if e != nil {
		return e
	}
	if (action == "wake" || action == "handoff" || action == "resume") && (st.State == "complete" || st.State == "handed-off") {
		return fmt.Errorf("orchestrator is terminal; apply again")
	}
	switch action {
	case "stop":
		if e := os.WriteFile(filepath.Join(dir, action), []byte("stop"), 0600); e != nil {
			return e
		}
		if e := stopHarness(dir); e != nil {
			return e
		}
		_ = orchestratorLifecycle(spec, st, "stopped", "stopped")
		return nil
	case "pause", "wake":
		stamp := time.Now().UTC().Format(time.RFC3339Nano)
		if e := os.WriteFile(filepath.Join(dir, action), []byte(stamp), 0600); e != nil {
			return e
		}
		if action == "pause" {
			_ = orchestratorLifecycle(spec, st, "paused", "paused:"+stamp)
		}
		return nil
	case "resume":
		if exists(filepath.Join(dir, "stop")) {
			return fmt.Errorf("stopped orchestrator must be applied again")
		}
		_ = os.Remove(filepath.Join(dir, "pause"))
		if e := os.WriteFile(filepath.Join(dir, "wake"), []byte("resume"), 0600); e != nil {
			return e
		}
		_ = orchestratorLifecycle(spec, st, "resumed", "resumed:"+time.Now().UTC().Format(time.RFC3339Nano))
		return LaunchOrchestrator(name)
	case "handoff":
		if strings.TrimSpace(to) == "" {
			return fmt.Errorf("handoff requires --to SESSION")
		}
		return WriteJSON(filepath.Join(dir, "handoff.json"), map[string]string{"to": to})
	}
	return fmt.Errorf("unknown control %s", action)
}

// RunOrchestratorRound runs one round, then delivers its notices after every
// lock the round took is released.
func RunOrchestratorRound(ctx context.Context, s OrchestratorSpec, st *OrchestratorState, d OrchestratorDriver) error {
	before := st.Rounds
	e := runOrchestratorRound(ctx, s, st, d)
	if s.Source.Plugin != "" && (e == nil || errors.Is(e, ErrSourceDeferred)) {
		noteSourceCap(s, *st, e == nil && st.Rounds > before)
	}
	flushOrchestratorNotices(s, noticeSender(d))
	return e
}
func runOrchestratorRound(ctx context.Context, s OrchestratorSpec, st *OrchestratorState, d OrchestratorDriver) error {
	dir := OrchestratorDir(s.Name)
	st.Deferred = ""
	if s.Source.Plugin != "" {
		// Signals queued before a crash are delivered even when the round
		// is paused or skipped.
		if e := deliverSourceSignals(s, st); e != nil {
			return e
		}
		if sourcePaused(s) {
			return deferSourceRound(st, "pause_file present")
		}
	}
	ev, e := d.Gather(ctx, s)
	if e != nil {
		return e
	}
	var batch SourceBatch
	if s.Source.Plugin != "" {
		batch, e = fetchSource(ctx, s)
		if e != nil {
			return e
		}
		if e = advanceSourcePoll(s); e != nil {
			return e
		}
	}
	st.ManagedRuns = len(ev)
	checkpoint, _ := os.ReadFile(filepath.Join(dir, "checkpoint.md"))
	inbox, _ := os.ReadFile(filepath.Join(dir, "inbox.md"))
	pending := sourcePendingWork(s)
	b, _ := json.Marshal(struct {
		Evidence []OrchestratorEvidence
		Source   SourceBatch
		Inbox    string
		Pending  []string `json:",omitempty"`
	}{ev, batch, string(inbox), pending})
	digest := fmt.Sprintf("%x", sha256.Sum256(b))
	if s.Idle == "skip" && st.Digest == digest {
		st.State = "idle"
		return nil
	}
	number := st.Rounds + 1
	round := filepath.Join(dir, "rounds", fmt.Sprintf("%03d", number))
	p := OrchestratorPacket{Source: batch, Spec: s, Checkpoint: string(checkpoint), Inbox: string(inbox), Evidence: ev, Rotate: s.Handoff.RotateAfter > 0 && (st.Lineage+1) >= s.Handoff.RotateAfter}
	created := false
	// The round directory is created on first use, so a fully deferred source
	// round leaves no directory and no round number behind.
	create := func() (string, error) {
		if created {
			return round, nil
		}
		if e := os.MkdirAll(filepath.Dir(round), 0700); e != nil {
			return "", e
		}
		if e := os.Mkdir(round, 0700); e != nil {
			return "", e
		}
		created = true
		st.Rounds = number
		st.State = "running"
		if e := WriteJSON(filepath.Join(round, "packet.json"), p); e != nil {
			return "", e
		}
		if s.Agent == "off" {
			if e := os.WriteFile(filepath.Join(round, "trace"), []byte("agent=off source="+s.Source.Plugin+"\n"), 0600); e != nil {
				return "", e
			}
		}
		// Persist numbering before invoking the model, including crash/failed rounds.
		return round, WriteJSON(filepath.Join(dir, "state.json"), st)
	}
	var out sourceOutcome
	if s.Agent == "off" {
		out, e = runSourceRules(ctx, s, st, create, batch, ev)
		if e == nil && !created && out.Deferred != "" {
			return deferSourceRound(st, out.Deferred)
		}
		if e == nil {
			if _, e = create(); e == nil {
				note := "Source events processed. Next: poll from accepted cursor.\n"
				if out.Deferred != "" {
					note += "Deferred: " + out.Deferred + ". Next: retry from the kept cursor.\n"
				}
				e = os.WriteFile(filepath.Join(round, "checkpoint.md"), []byte(note), 0600)
			}
		}
	} else if _, e = create(); e == nil {
		e = d.Session(ctx, s, round, p)
	}
	if e != nil {
		return fmt.Errorf("round %d: %w", number, e)
	}
	next, e := os.ReadFile(filepath.Join(round, "checkpoint.md"))
	if e != nil || strings.TrimSpace(string(next)) == "" {
		return fmt.Errorf("round %d missing required checkpoint.md", st.Rounds)
	}
	if p.Rotate {
		archive := filepath.Join(dir, "lineages", fmt.Sprintf("%03d.md", st.Rounds))
		if e = os.MkdirAll(filepath.Dir(archive), 0700); e != nil {
			return e
		}
		if e = os.WriteFile(archive, checkpoint, 0600); e != nil {
			return e
		}
		st.Lineage = 0
	} else {
		st.Lineage++
	}
	// Atomic replacement keeps the previous accepted checkpoint on failure.
	tmp := filepath.Join(dir, "checkpoint.next")
	if e = os.WriteFile(tmp, next, 0600); e != nil {
		return e
	}
	if e = platformops.Rename(tmp, filepath.Join(dir, "checkpoint.md")); e != nil {
		return e
	}
	if len(inbox) > 0 {
		unlock, err := lockRoot()
		if err != nil {
			return err
		}
		defer unlock()
		current, err := os.ReadFile(filepath.Join(dir, "inbox.md"))
		if err != nil {
			return err
		}
		if !strings.HasPrefix(string(current), string(inbox)) {
			return fmt.Errorf("inbox changed during round; refusing to consume")
		}
		if err = os.WriteFile(filepath.Join(round, "inbox.md"), inbox, 0600); err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(dir, "inbox.md"), current[len(inbox):], 0600); err != nil {
			return err
		}
	}
	// Compare future packets against evidence with the consumed inbox removed.
	b, _ = json.Marshal(struct {
		Evidence []OrchestratorEvidence
		Source   SourceBatch
		Inbox    string
		Pending  []string `json:",omitempty"`
	}{ev, batch, "", sourcePendingWork(s)})
	digest = fmt.Sprintf("%x", sha256.Sum256(b))
	if s.Source.Plugin != "" {
		// Acks follow the accepted checkpoint and precede the cursor. A failure
		// keeps the cursor, so the replay acks again: plugins must be idempotent.
		if e = ackSource(ctx, s, batch, out.Acks); e != nil {
			return e
		}
		if out.Deferred == "" {
			if e = WriteJSON(filepath.Join(dir, "source-cursor.json"), map[string]string{"cursor": batch.Cursor}); e != nil {
				return e
			}
		}
	}
	st.Digest = digest
	if out.Deferred != "" {
		st.Digest = "" // Deferred events must be retried even if the batch repeats.
	}
	st.Deferred = out.Deferred
	st.State = "waiting"
	st.Error = ""
	var met struct {
		Met bool `json:"met"`
	}
	if s.Until == "goal" && readJSON(filepath.Join(round, "goal-met.json"), &met) == nil && met.Met {
		st.State = "complete"
	}
	orchestratorReply(s, *st)
	return nil
}

func deferSourceRound(st *OrchestratorState, reason string) error {
	st.State, st.Deferred, st.Error = "waiting", reason, ""
	return fmt.Errorf("%w: %s", ErrSourceDeferred, reason)
}

func orchestratorEnvironment(name string) []string {
	env := []string{}
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "HEV_ORCHESTRATOR=") {
			env = append(env, v)
		}
	}
	if name != "" {
		env = append(env, "HEV_ORCHESTRATOR="+name)
	}
	return env
}

// RegisterOrchestratorScope applies an admission-only copy on a target. The
// owner remains the sole lifecycle authority; this target cannot run rounds.
func RegisterOrchestratorScope(text string, owner Host, dry bool) (OrchestratorSpec, error) {
	s, e := applyOrchestrator(text, dry, false)
	if e != nil || dry {
		return s, e
	}
	return s, WriteJSON(filepath.Join(OrchestratorDir(s.Name), "scope-owner.json"), owner)
}

// SayOrchestrator appends under the same lock used by checkpoint consumption.
// The wake marker is durable even when a round is currently running.
func SayOrchestrator(name, message string) error {
	if parent := currentOrchestratorIdentity(); parent != "" {
		if e := admitOrchestratorChild(parent, name); e != nil {
			return e
		}
	}
	if strings.TrimSpace(message) == "" {
		return fmt.Errorf("message required")
	}
	if _, e := LoadOrchestrator(name); e != nil {
		return e
	}
	unlock, e := lockRoot()
	if e != nil {
		return e
	}
	defer unlock()
	st, e := orchestratorState(name)
	if e != nil {
		return e
	}
	if st.State == "complete" || st.State == "handed-off" || exists(filepath.Join(OrchestratorDir(name), "stop")) {
		return fmt.Errorf("orchestrator is terminal; apply again")
	}
	f, e := os.OpenFile(filepath.Join(OrchestratorDir(name), "inbox.md"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if e != nil {
		return e
	}
	_, e = fmt.Fprintln(f, message)
	closeErr := f.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	return os.WriteFile(filepath.Join(OrchestratorDir(name), "wake"), []byte("message"), 0600)
}

func addOrchestratorRows(r *Response) error {
	for i := range r.Rows {
		if r.Rows[i].Kind == "" {
			r.Rows[i].Kind = "run"
		}
	}
	states, e := ListOrchestrators()
	if e != nil {
		return e
	}
	for _, st := range states {
		spec, e := LoadOrchestrator(st.Name)
		if e != nil {
			return e
		}
		for i := range r.Rows {
			if r.Rows[i].Kind != "orch" && spec.Includes(r.Rows[i].Order) {
				if r.Rows[i].Orchestrator != "" {
					r.Rows[i].Orchestrator += ","
				}
				r.Rows[i].Orchestrator += st.Name
			}
		}
		if exists(filepath.Join(OrchestratorDir(st.Name), "scope-owner.json")) {
			continue
		}
		until := ""
		if exists(filepath.Join(OrchestratorDir(st.Name), "order.md")) {
			until = "all followed runs merged or abandoned"
		}
		r.Rows = append(r.Rows, Row{Kind: "orch", Meta: Meta{Order: Order{Name: "o-" + st.Name, Host: st.Host, Reply: spec.Reply, Until: until}, Start: st.Start}, State: st.State, Schedule: Schedule{Rounds: st.Rounds, Next: st.Next}, Error: st.Error})
	}
	return nil
}
