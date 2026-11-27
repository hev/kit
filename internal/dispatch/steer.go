package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hev/kit/internal/harness"
	"github.com/hev/kit/internal/hooks"
	"github.com/hev/kit/internal/layerenv"
	"github.com/hev/kit/internal/peers/compat"
	"github.com/hev/kit/internal/steer"
	"github.com/hev/kit/internal/version"
)

// wiredFile marks a launch whose harness got kit's hooks; its presence is
// what makes a run's steering tool-boundary rather than turn-boundary.
const wiredFile = steer.WiredFile

// wireHooks adds kit's hook layer to one harness launch: invocation hooks
// (never a settings file), and the environment that lets each hook find this
// target's steer store. It records the wiring for hev steer. Harnesses
// without kit invocation hooks (agy) launch unchanged: no wired marker is
// written, so a halt falls back to the turn-boundary interrupt.
func wireHooks(name, steerDir, target string, args []string, env []string) ([]string, []string) {
	exe, err := os.Executable()
	capability := harness.HooksFor(name)
	if err != nil || capability == nil || capability.Invocation == "" {
		// A fallback may follow a harness that supported hooks in this directory.
		_ = os.Remove(filepath.Join(steerDir, wiredFile))
		return args, env
	}
	if len(args) == 0 {
		return args, env
	}
	prompt := args[len(args)-1]
	head := append([]string{}, args[:len(args)-1]...)
	switch capability.Invocation {
	case "settings":
		head = append(head, "--settings", compat.Dumps(compat.NewObj("hooks", hooks.InvocationHooks(exe, name))))
	case "config":
		head = append(head, hooks.ConfigInvocationArgs(exe, name)...)
	}
	env = append(env, steer.EnvDir+"="+steerDir, steer.EnvTarget+"="+target, "HEV_HOOK_INVOCATION=1")
	if os.MkdirAll(steerDir, 0700) == nil {
		_ = WriteJSON(filepath.Join(steerDir, wiredFile), map[string]any{"hooks": hooks.Schema, "harness": name, "at": time.Now().UTC(), "version": version.Version})
	}
	return append(head, prompt), env
}

func steerLabel(c *Caller) string {
	if c == nil {
		return ""
	}
	parts := []string{}
	if c.Sender != "" {
		parts = append(parts, c.Sender)
	}
	if c.Session != "" {
		parts = append(parts, c.Session)
	}
	return strings.Join(parts, " ")
}

func haltTimeout(q Request) time.Duration {
	if d, err := time.ParseDuration(q.HaltTimeout); err == nil && d > 0 {
		return d
	}
	cfg, _ := hooks.LoadConfig(layerenv.ConfigPath())
	return cfg.HaltTimeout
}

// steerAck is the acknowledgement command an agent is told to run.
func steerAck(id string) string { return "hev steer ack " + id }

// handleSteer is the "steer" RPC on the run's (or orchestrator's) host.
func handleSteer(q Request) (Response, error) {
	r := Response{Version: version.Version}
	level := q.Level
	if level == "" {
		level = steer.Note
	}
	if !steer.ValidLevel(level) {
		return r, fmt.Errorf("unknown level %q (want %s)", level, strings.Join(steer.Levels, ", "))
	}
	if strings.TrimSpace(q.Note) == "" {
		return r, fmt.Errorf("message required")
	}
	from := steerLabel(q.Caller)
	if name, ok := strings.CutPrefix(q.Name, "o-"); ok {
		return steerOrchestrator(q, name, level, from)
	}
	if !ValidName(q.Name) {
		return r, fmt.Errorf("invalid name")
	}
	dir := filepath.Join(Root(), q.Name)
	var m Meta
	if readJSON(filepath.Join(dir, "meta.json"), &m) != nil {
		return r, fmt.Errorf("order not found: %s", q.Name)
	}
	if m.Forward != nil {
		if q.APILocalOnly {
			return r, fmt.Errorf("order not found")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		return routeRPC(ctx, *m.Forward, q)
	}
	live, err := liveNames()
	if err != nil {
		return r, err
	}
	running := live[q.Name]
	store := steer.RunStore(Root(), q.Name)
	wired := exists(filepath.Join(store.Dir, wiredFile))
	delivery := steer.TurnBoundary
	if running && wired && level != steer.Note {
		delivery = steer.ToolBoundary
	}
	// say delivers through dispatch say. On a live run that path records
	// the receipt itself (sayID), at this level; on an ended run it reworks
	// and this function records it.
	sayID := ""
	say := func(note string) (Response, error) {
		s := q
		s.Action, s.Note, s.Level = "say", note, level
		sr, err := Handle(s)
		sayID = sr.SteerID
		return sr, err
	}
	var how string
	switch level {
	case steer.Note:
		sr, err := say(q.Note)
		if err != nil {
			return r, err
		}
		if sr.SteerID == "" {
			how = "note: " + sr.Message + "; queued for next-turn prompt injection"
		} else {
			how = "if unacknowledged, the note can also be injected at the next turn's first prompt"
		}
	case steer.Urgent:
		switch {
		case !running:
			sr, err := say(q.Note)
			if err != nil {
				return r, err
			}
			how = "the run had ended, so this reworks it (" + sr.Message + "); the message is injected at the new session's first prompt and mutating calls wait for its ack"
		case wired:
			how = "tool-boundary: injected at the worker's next tool call; file writes, git commit/push, hev dispatch and gh pr writes are blocked until it runs " + steerAck("STEER_ID")
		default:
			if _, err := say(q.Note); err != nil {
				return r, err
			}
			how = "turn-boundary fallback for a run launched without kit hooks"
		}
	case steer.HaltL, steer.Redirect:
		if !running {
			if level == steer.HaltL {
				return r, fmt.Errorf("%s is not running; nothing to halt", q.Name)
			}
			msg, err := store.Enqueue(level, q.Note, from, steer.TurnBoundary, 0)
			if err != nil {
				return r, err
			}
			if r.Message, err = redirectEnded(q, msg.ID); err != nil {
				return r, err
			}
			r.SteerID, r.Delivery = msg.ID, steer.TurnBoundary
			return r, nil
		}
		timeout := haltTimeout(q)
		if wired {
			how = fmt.Sprintf("tool-boundary: every tool call is blocked with \"end your turn now\" at the next boundary; the run shows halted when the harness exits; hard stop after %s if it has not", timeout)
		} else {
			how = fmt.Sprintf("turn-boundary fallback: no kit hooks in this launch, so kit sent the harness its own interrupt (SIGINT); hard stop after %s if it has not ended", timeout)
		}
		if level == steer.Redirect {
			how += "; then the worktree is committed as a checkpoint and the same run is reworked with this message (branch and PR kept)"
		}
		msg, err := store.Enqueue(level, q.Note, from, delivery, timeout)
		if err != nil {
			return r, err
		}
		if !wired {
			if err := deliverByInterrupt(store, msg.ID, func() error { return interruptHarness(dir) }); err != nil {
				how += "; interrupt failed (" + err.Error() + "), so the halt stays queued for the hard stop"
			}
		}
		r.SteerID, r.Delivery = msg.ID, delivery
		r.Message = fmt.Sprintf("%s %s queued for %s. Delivery: %s.", msg.ID, level, q.Name, how)
		return r, nil
	}
	var msg steer.Message
	if sayID != "" {
		msg, err = store.Get(sayID)
	} else {
		msg, err = store.Enqueue(level, q.Note, from, delivery, 0)
	}
	if err != nil {
		return r, err
	}
	r.SteerID, r.Delivery = msg.ID, msg.Delivery
	r.Message = fmt.Sprintf("%s %s %s for %s. Delivery: %s.", msg.ID, level, msg.State, q.Name, strings.ReplaceAll(how, "STEER_ID", msg.ID))
	if sayID != "" {
		r.Message = fmt.Sprintf("%s %s delivered to inbox.md for %s. Delivery: %s. After handling, run %s.", msg.ID, level, q.Name, how, steerAck(msg.ID))
	}
	return r, nil
}

func steerOrchestrator(q Request, name, level, from string) (Response, error) {
	r := Response{Version: version.Version}
	if _, err := LoadOrchestrator(name); err != nil {
		return r, err
	}
	if level == steer.Redirect {
		return r, fmt.Errorf("redirect needs a dispatch run; an orchestrator takes a note (hev orchestrate say) or a halt")
	}
	store := steer.OrchestratorStore(Root(), name)
	round := processRecordLive(filepath.Join(OrchestratorDir(name), "harness.json"))
	delivery, how := steer.TurnBoundary, "next round (orchestrate say)"
	sayID := ""
	switch level {
	case steer.Note:
		id, err := SayOrchestratorReceipt(name, q.Note, level, from)
		if err != nil {
			return r, err
		}
		sayID = id
		if err := LaunchOrchestrator(name); err != nil {
			return r, err
		}
	case steer.Urgent:
		if round && exists(filepath.Join(store.Dir, wiredFile)) {
			delivery, how = steer.ToolBoundary, "tool-boundary in the running round; mutations wait for its ack"
		} else {
			id, err := SayOrchestratorReceipt(name, q.Note, level, from)
			if err != nil {
				return r, err
			}
			sayID = id
			how = "no round is running: inbox.md for the next round, and injected at its first prompt"
		}
	case steer.HaltL:
		if !round {
			return r, fmt.Errorf("orchestrator %s has no round running; nothing to halt (hev orchestrate stop ends it)", name)
		}
		if exists(filepath.Join(store.Dir, wiredFile)) {
			delivery, how = steer.ToolBoundary, "the running round ends its turn at the next tool call"
		} else {
			how = "turn-boundary fallback: no kit hooks in this round, so kit sent the harness its own interrupt (SIGINT)"
		}
		how += fmt.Sprintf("; hard stop after %s if it has not ended; the orchestrator itself keeps its schedule (hev orchestrate stop ends it)", haltTimeout(q))
	}
	var msg steer.Message
	var err error
	if sayID != "" {
		msg, err = store.Get(sayID)
	} else {
		msg, err = store.Enqueue(level, q.Note, from, delivery, haltTimeout(q))
	}
	if err != nil {
		return r, err
	}
	if level == steer.HaltL && delivery == steer.TurnBoundary {
		if err := deliverByInterrupt(store, msg.ID, func() error { return interruptHarness(OrchestratorDir(name)) }); err != nil {
			how += "; interrupt failed (" + err.Error() + "), so the halt stays queued for the hard stop"
		}
	}
	r.SteerID, r.Delivery = msg.ID, delivery
	r.Message = fmt.Sprintf("%s %s queued for o-%s. Delivery: %s.", msg.ID, level, name, how)
	return r, nil
}

// deliverByInterrupt is the no-hook halt: the harness's own interrupt is
// the delivery, so the receipt is marked injected before it is sent (the
// harness may exit, and the supervisor close the halt, at once). If the
// interrupt cannot be sent the receipt goes back to queued.
func deliverByInterrupt(store steer.Store, id string, interrupt func() error) error {
	st := store.LoadState()
	if _, err := store.Update(id, func(m *steer.Message) {
		m.State = steer.Injected
		m.Injected = &steer.Mark{At: time.Now().UTC(), Event: "interrupt", Session: st.Session, Turn: st.Turn, ToolCall: st.ToolCalls, By: "harness interrupt (no kit hooks in this launch)"}
	}); err != nil {
		return err
	}
	if err := interrupt(); err != nil {
		_, _ = store.Update(id, func(m *steer.Message) {
			if m.State == steer.Injected && m.Injected != nil && m.Injected.Event == "interrupt" {
				m.State, m.Injected = steer.Queued, nil
			}
		})
		return err
	}
	return nil
}

// checkpoint commits the worktree so a redirect keeps the halted work.
func checkpoint(wt, id string) (string, error) {
	status, err := command(wt, "git", "status", "--porcelain")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(status) == "" {
		return "clean; no checkpoint commit needed", nil
	}
	if _, err := command(wt, "git", "add", "-A"); err != nil {
		return "", err
	}
	if _, err := command(wt, "git", "commit", "--no-verify", "-m", "checkpoint: redirect "+id+" (hev steer)"); err != nil {
		return "", err
	}
	head, _ := command(wt, "git", "rev-parse", "--short", "HEAD")
	return "checkpoint commit " + head, nil
}

func redirectNote(m steer.Message) string {
	return fmt.Sprintf("Redirect (%s, hev steer): %s\n\nThe previous attempt was halted and its worktree committed as a checkpoint. Continue from that checkpoint with this instruction.", m.ID, m.Text)
}

// redirectEnded checkpoints an ended run and reworks it with the redirect.
func redirectEnded(q Request, id string) (string, error) {
	store := steer.RunStore(Root(), q.Name)
	msg, err := store.Get(id)
	if err != nil {
		return "", err
	}
	note, err := checkpoint(filepath.Join(Root(), q.Name, "wt"), id)
	if err != nil {
		return "", fmt.Errorf("checkpoint: %w", err)
	}
	s := q
	s.Action, s.Note, s.Level = "say", redirectNote(msg), ""
	sr, err := Handle(s)
	if err != nil {
		return "", err
	}
	_, _ = store.Update(id, func(m *steer.Message) {
		m.State, m.Redirected = steer.Acked, "rework"
		m.Acked = &steer.Mark{At: time.Now().UTC(), Event: "redirect", By: "run had ended"}
	})
	return fmt.Sprintf("%s redirect for %s: the run had ended; %s; %s.", id, q.Name, note, sr.Message), nil
}

// FollowRedirect runs detached after a redirected run's supervisor exits: it
// waits for the run to stop being live, then reworks it with the redirect.
func FollowRedirect(name, id string) error {
	store := steer.RunStore(Root(), name)
	msg, err := store.Get(id)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(5 * time.Minute)
	for {
		live, err := liveNames()
		if err == nil && !live[name] {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s still live after 5m; redirect %s not delivered", name, id)
		}
		time.Sleep(500 * time.Millisecond)
	}
	sr, err := Handle(Request{Action: "say", Name: name, Note: redirectNote(msg), Caller: &Caller{Sender: msg.From}})
	_, _ = store.Update(id, func(m *steer.Message) {
		m.Redirected = "rework"
		if err != nil {
			m.Redirected = "rework failed: " + err.Error()
		}
	})
	if err != nil {
		return err
	}
	fmt.Println(sr.Message)
	return nil
}

// afterHalt runs in the supervisor once the harness has exited: it closes the
// halt receipts, marks the result, and for a redirect checkpoints the
// worktree and starts the follower that reworks the run.
func afterHalt(dir, name string, r *Result) {
	store := steer.Store{Dir: filepath.Join(dir, "steer"), Target: "dispatch:" + name}
	ended, found, err := closeHalts(store, r.HaltEscalated)
	if err == nil && !found {
		// A Stop hook (Codex fires one) may already have closed the halt
		// that ended this attempt's turn.
		if st := store.LoadState(); st.Halted && st.HaltedAt != nil && !st.HaltedAt.Before(r.Start) {
			if m, e := store.Get(st.HaltID); e == nil {
				ended, found = m, true
			}
		}
	}
	if err == nil && !found && r.HaltEscalated {
		r.Halted = true
	}
	if err != nil || !found {
		return
	}
	r.Halted, r.HaltID = true, ended.ID
	if ended.Level != steer.Redirect {
		return
	}
	note, err := checkpoint(filepath.Join(dir, "wt"), ended.ID)
	if err != nil {
		r.Error = strings.TrimSpace(r.Error + "\nredirect checkpoint: " + err.Error())
		return
	}
	r.Redirect = ended.ID + ": " + note
	if err := startRedirectFollower(dir, name, ended.ID); err != nil {
		r.Error = strings.TrimSpace(r.Error + "\nredirect follow-up: " + err.Error())
	}
}

// startRedirectFollower starts `hev dispatch redirect-follow` detached; a
// variable so tests do not start one.
var startRedirectFollower = func(dir, name, id string) error {
	exe, err := os.Executable()
	if err == nil {
		c := exec.Command(exe, "dispatch", "redirect-follow", name, id)
		c.Stdout, c.Stderr = nil, nil
		if f, e := os.OpenFile(filepath.Join(dir, "steer", "redirect.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600); e == nil {
			c.Stdout, c.Stderr = f, f
			defer f.Close()
		}
		err = startDetached(c)
	}
	return err
}

// closeHalts closes a target's halt receipts once its harness has exited and
// reports the halt that ended the turn. After a hard stop, a halt nothing had
// shown the agent counts as delivered by it, so a redirect still continues.
func closeHalts(store steer.Store, escalated bool) (steer.Message, bool, error) {
	by := "turn ended (harness exited)"
	if escalated {
		by = "hard stop after halt timeout"
		if m, ok := store.ActiveHalt(); ok && m.State == steer.Queued {
			_, _ = store.Update(m.ID, func(m *steer.Message) {
				m.State = steer.Injected
				m.Injected = &steer.Mark{At: time.Now().UTC(), Event: "hard-stop", By: by}
			})
		}
	}
	return store.EndHalts(by)
}

// roundHaltPoll is how often an orchestrator round checks its halt deadline.
var roundHaltPoll = 2 * time.Second

// superviseOrchestratorRound waits for an orchestrator round's harness. A halt that
// outlives its timeout becomes a hard stop, as for dispatch runs, and the
// round's halt receipts are closed when the harness exits (Claude fires no
// Stop hook after a halt, so nothing else would).
func superviseOrchestratorRound(orchDir string, wait func() error, kill func()) error {
	done := make(chan error, 1)
	go func() { done <- wait() }()
	tick := time.NewTicker(roundHaltPoll)
	defer tick.Stop()
	escalated := false
	var err error
	for waiting := true; waiting; {
		select {
		case err = <-done:
			waiting = false
		case <-tick.C:
			if !escalated && escalateHalt(orchDir) {
				escalated = true
				kill()
			}
		}
	}
	_, _, _ = closeHalts(steer.Store{Dir: filepath.Join(orchDir, "steer")}, escalated)
	if escalated {
		return fmt.Errorf("halt not honoured within its timeout; the round was hard-stopped: %v", err)
	}
	return err
}

// escalateHalt reports whether a pending halt has outlived its timeout, and
// records the escalation on its receipt.
func escalateHalt(dir string) bool {
	store := steer.Store{Dir: filepath.Join(dir, "steer")}
	m, ok := store.ActiveHalt()
	if !ok {
		return false
	}
	timeout, err := time.ParseDuration(m.HaltTimeout)
	if err != nil || timeout <= 0 {
		timeout = hooks.DefaultHaltTimeout
	}
	if time.Since(m.QueuedAt) < timeout {
		return false
	}
	_, _ = store.Update(m.ID, func(m *steer.Message) {
		m.Escalated = &steer.Mark{At: time.Now().UTC(), Event: "hard-stop", By: "halt not honoured within " + timeout.String()}
	})
	return true
}

// steerCounts summarizes a run's receipts for ls and show.
func steerCounts(dir string) *steer.Counts {
	c := steer.Store{Dir: filepath.Join(dir, "steer")}.Counts()
	if c.Total() == 0 && !c.Halted {
		return nil
	}
	return &c
}

// steerReceipts is show's steer.json: every receipt, oldest first.
func steerReceipts(dir string) string {
	ms, _ := steer.Store{Dir: filepath.Join(dir, "steer")}.List()
	if len(ms) == 0 {
		return ""
	}
	b, _ := json.MarshalIndent(ms, "", "  ")
	return string(b)
}
