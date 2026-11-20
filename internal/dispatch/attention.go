package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/hev/kit/internal/attention"
)

// Attention notices (docs/notify.md). Emission happens on the host where the
// moment happens; it runs hook handlers and, with [notify.slack], posts to
// Slack. It never fails a run or a round. Tests replace emitAttention.
var emitAttention = func(n attention.Notice, stateDir string) {
	_ = attention.Emit(n, stateDir)
}

// queueAttention records a notice for the next flush; orchestrator rounds
// use it because they hold locks.
var queueAttention = func(n attention.Notice, stateDir string) {
	_ = attention.Queue(n, stateDir)
}

// flushAttention retries due Slack posts; supervisors call it on each wake.
var flushAttention = func() { attention.Flush(context.Background()) }

// attentionHost is the host label a notice names.
func attentionHost(h string) string {
	if h != "" {
		return h
	}
	return notificationHost()
}

// newAttempt starts a new attempt of a one-shot run (a rework or an amend),
// keeping the first attempt's start as the run's lineage.
func newAttempt(m *Meta) {
	m.Lineage = runLineage(*m)
	m.Start = time.Now().UTC()
}

// runLineage scopes a run's attention notices: the first attempt's start,
// which a rework or an amend does not move.
func runLineage(m Meta) string {
	if m.Lineage != "" {
		return m.Lineage
	}
	return m.Start.UTC().Format(time.RFC3339Nano)
}

func runShowLink(m Meta) string {
	link := "hev dispatch show " + m.Name
	if m.Host != "" {
		link += " --host " + m.Host
	}
	return link
}

// runAttention maps a finished run to its notice. ok is false for states
// nobody needs to hear about here (a stop someone asked for, a soft halt).
func runAttention(m Meta, r Result) (attention.Notice, bool) {
	return runAttentionAt(m, r, "")
}

// runAttentionAt is runAttention for one attempt of the run: "" for a
// one-shot run (its end time names the attempt), or "round:N" for a round of
// a repeating run, whose identity must not move when the repeat ends.
//
// Only two moments raise needs-you: an open decision (the run ended
// needs-decision KEY: QUESTION) and a halt that escalated to a hard stop.
// Their ID is the subject, not the attempt: the run, plus the decision key
// and question, or the halt. Asking the same question again, from a rework or
// a later round, is the same notice and is not raised again; a changed
// question is a new one. Every other end (blocked, failed, timeout,
// needs-review, an admission failure, done) is a record on the opt-in done
// kind, keyed by its attempt.
func runAttentionAt(m Meta, r Result, attempt string) (attention.Notice, bool) {
	state := State(&r, false)
	run := runLineage(m)
	links := attention.Links{PR: r.PR, Run: runShowLink(m)}
	host := attentionHost(m.Host)
	if attempt == "" {
		attempt = r.End.UTC().Format(time.RFC3339Nano)
	}
	switch {
	case r.HaltEscalated:
		halt := r.HaltID
		if halt == "" {
			halt = attempt
		}
		return attention.New(attention.KindNeedsYou, attention.SourceSteer, host, m.Name, run+"|halt|"+halt, state, "halt escalated to a hard stop", links).For(requester(m)), true
	case r.Halted, r.Stopped && !r.Timeout:
		return attention.Notice{}, false
	case state == "needs-review" && r.Outcome == OutcomeNeedsDecision:
		return attention.New(attention.KindNeedsYou, attention.SourceRun, host, m.Name, decisionSubject(run, r.Key, r.Question), state, outcomeSummary(r), links).For(requester(m)), true
	}
	text := ""
	switch state {
	case "needs-review":
		text = "needs review: validation failed twice"
	case "blocked":
		text = outcomeSummary(r)
	case "timeout":
		text = "timed out"
	case "failed":
		text = fmt.Sprintf("failed (exit %d)", r.ExitCode)
		if r.Error != "" {
			text += ": " + r.Error
		}
	case "done":
		text = "done"
	default:
		return attention.Notice{}, false
	}
	return attention.New(attention.KindDone, attention.SourceRun, host, m.Name, run+"|"+attempt+"|"+state, state, text, links).For(requester(m)), true
}

// decisionSubject is the dedupe key of an open decision: what it is about
// (scope), its key and its question, with whitespace collapsed so a
// reflowed question is still the same one.
func decisionSubject(scope, key, question string) string {
	return scope + "|decision|" + strings.TrimSpace(key) + "|" + strings.Join(strings.Fields(question), " ")
}

// noticeRunEnd emits a finished run's notice. Validator children have their
// own reporting. A repeating run's round speaks only when it ended somewhere
// other than done: an open decision or a hard halt (needs-you), or a failed,
// blocked, timed-out or needs-review round (a done-kind record). An
// uneventful round stays silent. The round number is the attempt identity of
// a record, so the same round re-emitted at the repeat's end is one notice.
func noticeRunEnd(dir string, m Meta, r Result) {
	if m.Parent != "" || m.Forward != nil {
		return
	}
	if m.Every == "" {
		if n, ok := runAttention(m, r); ok {
			emitAttention(n, dir)
		}
		return
	}
	if n, ok := runAttentionAt(m, r, repeatAttempt(dir)); ok && n.State != "done" {
		emitAttention(n, dir)
	}
}

func repeatAttempt(dir string) string {
	return fmt.Sprintf("round:%d", loadSchedule(dir).Rounds)
}

// noticeRepeatEnd emits a repeating run's final transition. A last round that
// raised a notice is that same notice (and is not raised twice); an ended
// repeat is the opt-in done kind; a requested stop is quiet.
func noticeRepeatEnd(dir string, m Meta, r Result, reason string) {
	if m.Parent != "" || m.Forward != nil {
		return
	}
	if n, ok := runAttentionAt(m, r, repeatAttempt(dir)); ok && n.State != "done" {
		emitAttention(n, dir)
		return
	}
	if reason == "stopped" || reason == "" {
		return
	}
	key := runLineage(m) + "|end|" + reason
	emitAttention(attention.New(attention.KindDone, attention.SourceRun, attentionHost(m.Host), m.Name, key, "done", "repeat ended: "+reason, attention.Links{PR: r.PR, Run: runShowLink(m)}).For(requester(m)), dir)
}

// noticeAdmissionFailure emits the notice for a queued run whose admission
// failed after its sender had gone; a run that was never recorded, or did not
// fail admission, is left alone.
func noticeAdmissionFailure(dir string) {
	var m Meta
	var r Result
	if readJSON(filepath.Join(dir, "meta.json"), &m) != nil || readJSON(filepath.Join(dir, "result.json"), &r) != nil || !r.AdmissionFailed {
		return
	}
	noticeRunEnd(dir, m, r)
}

// prReady reports a non-draft open PR whose checks all passed.
var prReady = func(ctx context.Context, dir, pr string) bool {
	c := helperCommand(ctx, "gh", "pr", "view", pr, "--json", "isDraft,state,statusCheckRollup")
	c.Dir = dir
	b, err := c.Output()
	if err != nil {
		return false
	}
	var v struct {
		IsDraft bool   `json:"isDraft"`
		State   string `json:"state"`
		Checks  []struct {
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
			State      string `json:"state"`
		} `json:"statusCheckRollup"`
	}
	if json.Unmarshal(b, &v) != nil || v.IsDraft || v.State != "OPEN" || len(v.Checks) == 0 {
		return false
	}
	for _, c := range v.Checks {
		switch strings.ToUpper(c.Conclusion + c.State) {
		case "SUCCESS", "NEUTRAL", "SKIPPED":
		default:
			return false
		}
	}
	return true
}

// noticeRunReady emits the opt-in ready notice once a validation PASS lands
// on a run whose PR is ready with green CI.
func noticeRunReady(dir string, m Meta, r Result) {
	if m.Parent != "" || r.PR == "" || verdictName(r) != "PASS" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	ready := prReady(ctx, filepath.Join(dir, "wt"), r.PR)
	cancel()
	if !ready {
		return
	}
	key := runLineage(m) + "|ready|" + r.Head
	emitAttention(attention.New(attention.KindReady, attention.SourceRun, attentionHost(m.Host), m.Name, key, "ready", "PR ready: green CI and a PASS review", attention.Links{PR: r.PR, Run: runShowLink(m)}).For(requester(m)), dir)
}

// orchestratorSource names an orchestrator's flavour.
func orchestratorSource(name string) string {
	switch {
	case IsSwarm(name):
		return attention.SourceSwarm
	case IsFollowName(name):
		return attention.SourceFollow
	}
	return attention.SourceOrchestrator
}

// orchestratorAttention builds an orchestrator notice; nil for a scope-only
// copy, which must not speak for the owner.
func orchestratorAttention(s OrchestratorSpec, kind, key, state, text string) *attention.Notice {
	if exists(filepath.Join(OrchestratorDir(s.Name), "scope-owner.json")) {
		return nil
	}
	link := "hev orchestrate show " + s.Name
	n := attention.New(kind, orchestratorSource(s.Name), noticeHost(s), s.Name, key, state, text, attention.Links{Run: link}).For(orchestratorRequester(s.Name))
	return &n
}

// replyNeedsYou returns the first line of a reply when it starts [needs you].
func replyNeedsYou(body string) (string, bool) {
	first, _, _ := strings.Cut(strings.TrimSpace(body), "\n")
	first = strings.TrimSpace(first)
	if !strings.HasPrefix(first, "[needs you]") {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(first, "[needs you]")), true
}

// NeedsDecision parses "needs-decision KEY: QUESTION" (the run marker) at
// the start of text, as an orchestrator writes it after
// [needs you] for an open decision. Anything else is not a decision.
func NeedsDecision(text string) (key, question string, ok bool) {
	text = strings.TrimSpace(strings.Trim(strings.TrimSpace(text), "`"))
	rest, found := strings.CutPrefix(text, OutcomeNeedsDecision+" ")
	if !found {
		return "", "", false
	}
	key, question, found = strings.Cut(rest, ":")
	key, question = strings.TrimSpace(key), strings.TrimSpace(question)
	if !found || question == "" || !ValidDecisionKey(key) {
		return "", "", false
	}
	return key, question, true
}

// orchestratorReplyAttention maps a [needs you] reply to its notice. An open
// decision raises needs-you, keyed by the orchestrator and the decision
// (the same question repeated every round is one notice). Any other
// [needs you] reply (a stalled or failed run, a missing tool) is a record
// on the opt-in done kind, one per round.
func orchestratorReplyAttention(s OrchestratorSpec, st OrchestratorState, text string) *attention.Notice {
	scope := orchestratorLineage(st)
	if key, question, ok := NeedsDecision(text); ok {
		return orchestratorAttention(s, attention.KindNeedsYou, decisionSubject(scope, key, question), st.State, text)
	}
	return orchestratorAttention(s, attention.KindDone, scope+":reply:"+fmt.Sprint(st.Rounds), st.State, text)
}

// orchestratorLineage scopes an orchestrator's attention notices: its first
// lifetime's start, which reapplying does not move.
func orchestratorLineage(st OrchestratorState) string {
	if st.FirstStart != "" {
		return st.FirstStart
	}
	return st.Start.Format(time.RFC3339Nano)
}

// noticeOrchestrator queues n for an orchestrator on this host (the round's
// flush posts it once its locks are released), and returns it for the owner
// receipt so a pull route carries it too.
func noticeOrchestrator(s OrchestratorSpec, n *attention.Notice) *attention.Notice {
	if n == nil {
		return nil
	}
	queueAttention(*n, OrchestratorDir(s.Name))
	// The receipt copy leaves this host: a client island's privacy applies
	// before it is persisted.
	out := attention.Outbound(*n)
	return &out
}

// PulledAttention re-emits a notice carried by a pulled receipt on this host
// (notices pull --deliver), so a host with [notify.slack] posts what a pull
// host could not. host is the local alias of the host it came from.
func PulledAttention(ctx context.Context, host string, n *attention.Notice) error {
	if n == nil || n.ID == "" {
		return nil
	}
	c := *n
	c.Host, c.Thread = host, ""
	if c.Source == attention.SourceRun || c.Source == attention.SourceSteer {
		c.Links.Run = "hev dispatch show " + c.Name + " --host " + host
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	return attention.Default().Emit(ctx, c, "")
}

// attentionRows fills Row.Attention for ls.
func attentionStatus(dir string) string { return attention.StatusLine(dir) }
