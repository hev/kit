package steer

import (
	"fmt"
	"strings"
	"time"

	"github.com/hev/kit/internal/hooks"
)

// Handle is the built-in steering handler: it advances the target's turn and
// tool counters, injects queued messages at the boundary their level allows,
// and holds tool calls while an urgent waits for its ack or a halt is
// pending. ack is the command line the agent should run to acknowledge.
func (s Store) Handle(ev hooks.Event, ack string, now time.Time) (hooks.Response, error) {
	unlock, err := s.Lock(time.Second)
	if err != nil {
		return hooks.Response{}, err
	}
	defer unlock()
	st := s.LoadState()
	ms, err := s.List()
	if err != nil {
		return hooks.Response{}, err
	}
	now = now.UTC()
	if ev.SessionID != "" && ev.SessionID != st.Session {
		st.Session, st.Turn, st.ToolCalls = ev.SessionID, 0, 0
	}
	st.Harness, st.LastEvent, st.LastEventAt = ev.Harness, ev.Event, now
	mark := func(event string) *Mark {
		m := &Mark{At: now, Event: event, Session: st.Session, Turn: st.Turn, ToolCall: st.ToolCalls}
		if ev.Tool != nil {
			m.Tool = ev.Tool.Name
		}
		return m
	}
	var changed []Message
	put := func(m *Message) { changed = append(changed, *m) }
	// endHalts closes halts whose turn has ended. At a session or turn start
	// (starting), a queued halt admitted for this attempt is kept: a
	// supervisor accepts halts while it launches the harness, before any hook
	// has fired, and the first prompt delivers them. Any other queued halt is
	// left over from an ended attempt (or, in a peers session, from an idle
	// one) and was never needed: nothing was running.
	endHalts := func(by string, starting bool) {
		for i := range ms {
			if ms[i].Halting() {
				if starting && ms[i].State == Queued && s.CurrentAttempt(st, ms[i]) {
					continue
				}
				if ms[i].State == Injected {
					st.Halted, st.HaltedAt, st.HaltID = true, &now, ms[i].ID
				}
				ms[i].State = Acked
				ms[i].Acked = mark(ev.Event)
				ms[i].Acked.By = by
				put(&ms[i])
			}
		}
	}
	var r hooks.Response
	switch ev.Event {
	case "session-start":
		if ev.Source != "resume" && ev.Source != "compact" {
			endHalts("new session", true)
		}
	case "prompt-submit":
		st.Turn++
		endHalts("turn ended", true)
		st.Halted = false
		// A halt that reached a supervised run before its first tool call
		// stops the prompt itself; notes wait for the turn after it.
		for i := range ms {
			m := &ms[i]
			if m.Halting() && m.State == Queued {
				m.State, m.Injected = Injected, mark(ev.Event)
				put(m)
				r = hooks.Response{Decision: hooks.Halt, Reason: haltReason(*m)}
				break
			}
		}
		if r.Decision == hooks.Halt {
			break
		}
		var parts []string
		for i := range ms {
			m := &ms[i]
			// Unacknowledged inbox messages still reach the next prompt.
			inInbox := m.State == Injected && m.Injected != nil && m.Injected.Event == InboxEvent
			if (m.State == Queued || inInbox) && (m.Level == Note || m.Level == Urgent) {
				m.State, m.Injected = Injected, mark(ev.Event)
				put(m)
				parts = append(parts, inject(*m, ack))
			}
		}
		r.Context = strings.Join(parts, "\n\n")
	case "pre-tool":
		call := st.ToolCalls + 1
		for i := range ms {
			m := &ms[i]
			if !m.Halting() {
				continue
			}
			if m.State == Queued {
				m.State, m.Injected = Injected, mark(ev.Event)
				m.Injected.ToolCall = call
			}
			m.Blocked++
			put(m)
			r = hooks.Response{Decision: hooks.Halt, Reason: haltReason(*m)}
			break
		}
		if r.Decision == hooks.Halt || ev.Tool == nil || ev.Tool.Kind != hooks.KindMutate {
			break
		}
		var fresh, waiting []string
		for i := range ms {
			m := &ms[i]
			if m.Level != Urgent || m.State == Acked {
				continue
			}
			if m.State == Queued {
				m.State, m.Injected = Injected, mark(ev.Event)
				m.Injected.ToolCall = call
				fresh = append(fresh, inject(*m, ack))
			} else {
				waiting = append(waiting, m.ID)
			}
			m.Blocked++
			put(m)
		}
		if len(fresh)+len(waiting) > 0 {
			reason := "Blocked by kit steering: this " + ev.Tool.Name + " call changes state, and an urgent steer message is unacknowledged."
			if len(fresh) > 0 {
				reason += "\n\n" + strings.Join(fresh, "\n\n")
			}
			if len(waiting) > 0 {
				reason += fmt.Sprintf("\n\nRead the urgent message(s) %s shown earlier (or `%s`), take them into account, then run: %s %s. Reads stay allowed.", strings.Join(waiting, ", "), strings.Replace(ack, " ack", " show", 1)+" "+waiting[0], ack, strings.Join(waiting, " "))
			}
			r = hooks.Response{Decision: hooks.Block, Reason: reason}
		}
	case "post-tool":
		st.ToolCalls++
		var parts []string
		for i := range ms {
			m := &ms[i]
			if m.State != Queued || (m.Level != Urgent && !m.Halting()) {
				continue
			}
			m.State, m.Injected = Injected, mark(ev.Event)
			put(m)
			if m.Halting() {
				r.Decision, r.Reason = hooks.Halt, haltReason(*m)
				continue
			}
			parts = append(parts, inject(*m, ack))
		}
		if r.Decision != hooks.Halt {
			// An injected halt that let a call through (a parallel call already
			// past pre-tool) stops the turn here.
			for i := range ms {
				if ms[i].Halting() && ms[i].State == Injected {
					r.Decision, r.Reason = hooks.Halt, haltReason(ms[i])
					break
				}
			}
		}
		r.Context = strings.Join(parts, "\n\n")
	case "stop":
		endHalts("turn ended", false)
	}
	for _, m := range changed {
		if err := s.Put(m); err != nil {
			return hooks.Response{}, err
		}
	}
	if err := s.SaveState(st); err != nil {
		return hooks.Response{}, err
	}
	return r, nil
}

func inject(m Message, ack string) string {
	from := m.From
	if from == "" {
		from = "the dispatcher"
	}
	switch m.Level {
	case Urgent:
		return fmt.Sprintf("URGENT steering message %s from %s (delivered by kit at this tool boundary; it supersedes earlier instructions where they conflict):\n\n%s\n\nRead it now and adjust your plan. Until you acknowledge it, kit blocks file writes and edits, git commit/push, hev dispatch and gh pr writes; reads stay allowed. When you have taken it into account, run: %s %s", m.ID, from, m.Text, ack, m.ID)
	default:
		return fmt.Sprintf("Steering note %s from %s:\n\n%s\n\nHandle it after the current request. When handled, run: %s %s", m.ID, from, m.Text, ack, m.ID)
	}
}

func haltReason(m Message) string {
	from := m.From
	if from == "" {
		from = "the dispatcher"
	}
	what := "HALT"
	if m.Level == Redirect {
		what = "REDIRECT"
	}
	return fmt.Sprintf("%s %s from %s: %s\n\nEnd your turn now. Do not call any more tools. Reply with one line saying where you stopped. Kit will not kill this process; the run continues only when the dispatcher says so.", what, m.ID, from, m.Text)
}
