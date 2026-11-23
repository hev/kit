package foh

import (
	"context"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hev/kit/internal/peers/nativeinbox"
)

func TestTUINativeInboxModesAndPresence(t *testing.T) {
	provider := &scripted{replies: []ChatResult{{Content: "sidecar"}, {Content: "main"}}}
	m, agent := newTestTUI(t, provider)
	t.Setenv("TMUX_PANE", "")
	t.Setenv("HEV_NATIVE_PEER", "")
	closePeer, err := m.RegisterNativePeer(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(closePeer)
	p := m.nativePeer
	deliver := func(msg nativeinbox.Message) tea.Cmd {
		t.Helper()
		if err := p.store.Send(p.owner, msg); err != nil {
			t.Fatal(err)
		}
		_, cmd := m.Update(m.pollPeers()())
		return cmd
	}
	background := deliver(nativeinbox.Message{
		ID: "bg", From: "observer", Body: "background context", Mode: nativeinbox.Background,
	})
	// Run the sidecar deterministically without executing the recurring poll.
	for _, cmd := range background().(tea.BatchMsg)[1]().(tea.BatchMsg) {
		if done, ok := cmd().(peerBgDoneMsg); ok {
			m.Update(done)
		}
	}
	if len(agent.History()) != 0 || len(provider.requests) != 1 ||
		!strings.Contains(provider.requests[0].Messages[1].Content, "background context") {
		t.Fatalf("background did not stay in the sidecar: %+v", provider.requests)
	}
	deliver(nativeinbox.Message{
		ID: "int", From: "dispatcher", Body: "interrupt prompt", Mode: nativeinbox.Interrupt,
	})
	if got, err := p.store.Lookup(p.owner.Name); err != nil || got.State != "working" {
		t.Fatalf("working presence: %+v, %v", got, err)
	}
	view := plain(m)
	if !strings.Contains(view, "background context") || !strings.Contains(view, "interrupt prompt") {
		t.Fatalf("both modes must appear in the footer: %q", m.peerStatus(120))
	}
	drain(t, m, waitEvent(m.events))
	history := agent.History()
	if len(history) != 2 || !strings.Contains(history[0].Content, "interrupt prompt") ||
		strings.Contains(history[0].Content, "background context") {
		t.Fatalf("interrupt did not reach the main turn: %+v", history)
	}
	if got, err := p.store.Lookup(p.owner.Name); err != nil || got.State != "idle" {
		t.Fatalf("idle presence: %+v, %v", got, err)
	}
	closePeer()
	if _, err := p.store.Lookup(p.owner.Name); !os.IsNotExist(err) {
		t.Fatalf("close left a visible peer: %v", err)
	}
}

func TestTUINativeInterruptSteersOrStartsNextTurn(t *testing.T) {
	m, agent := newTestTUI(t, &scripted{replies: []ChatResult{{Content: "continued"}}})
	m.nativePeer = &nativePeers{}
	incoming := nativePeerTick{messages: []nativeinbox.Message{
		{ID: "mid-turn", From: "sender", Body: "use staging", Mode: nativeinbox.Interrupt},
	}}
	m.busy = true
	m.receivePeers(incoming)
	steers := agent.takeSteers()
	if len(steers) != 1 || !strings.Contains(steers[0], "use staging") || m.nativePeer.steering != steers[0] {
		t.Fatal("busy peer did not queue the interrupt as a steer")
	}
	m.applyEvent(Event{Kind: EventSteer, Text: steers[0]})
	if _, cmd := m.Update(agentDoneMsg{}); cmd != nil || m.busy {
		t.Fatal("an injected steer started a duplicate turn")
	}

	// Simulate a message arriving after the loop's final steering boundary.
	m.busy = true
	incoming.messages[0].ID = "late"
	m.receivePeers(incoming)
	_, next := m.Update(agentDoneMsg{})
	if next == nil || !m.busy {
		t.Fatal("the late interrupt was dropped")
	}
	drain(t, m, next)
	history := agent.History()
	if len(history) != 2 || !strings.Contains(history[0].Content, "use staging") {
		t.Fatalf("late interrupt did not reach the next turn once: %+v", history)
	}
}
