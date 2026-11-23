package foh

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/hev/kit/internal/peers/nativeinbox"
)

func TestTUINativeInboxModesAndPresence(t *testing.T) {
	m, agent := newTestTUI(t, &scripted{replies: []ChatResult{{Content: "hello"}, {Content: "ok"}}})
	t.Setenv("TMUX_PANE", "")
	t.Setenv("HEV_NATIVE_PEER", "")
	closePeer, err := m.RegisterNativePeer(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(closePeer)
	p := m.peers
	for _, msg := range []nativeinbox.Message{
		{ID: "bg", From: "observer", Body: "background context", Mode: nativeinbox.Background},
		{ID: "int", From: "dispatcher", Body: "interrupt prompt", Mode: nativeinbox.Interrupt},
	} {
		if err := p.store.Send(p.owner, msg); err != nil {
			t.Fatal(err)
		}
	}
	m.Update(m.pollPeers()())
	if got, err := p.store.Lookup(p.owner.Name); err != nil || got.State != "working" {
		t.Fatalf("working presence: %+v, %v", got, err)
	}
	view := plain(m)
	if !strings.Contains(view, "background context") || !strings.Contains(view, "interrupt prompt") {
		t.Fatalf("both modes must appear in the footer: %q", m.peerStatus(120))
	}
	// Await the real loop events, leaving the background note pending.
	drain(t, m, waitEvent(m.events))
	history := agent.History()
	if len(history) == 0 || !strings.Contains(history[0].Content, "interrupt prompt") ||
		strings.Contains(history[0].Content, "background context") {
		t.Fatalf("interrupt prompt/sidecar routing: %+v", history)
	}
	drain(t, m, submit(m, "operator prompt"))
	history = agent.History()
	if len(history) < 3 || !strings.Contains(history[2].Content, "operator prompt") ||
		!strings.Contains(history[2].Content, "background context") {
		t.Fatalf("background context was not attached to the operator prompt: %+v", history)
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
	m.peers = &nativePeers{}
	incoming := nativePeerTick{messages: []nativeinbox.Message{
		{ID: "mid-turn", From: "sender", Body: "use staging", Mode: nativeinbox.Interrupt},
	}}
	m.busy = true
	m.receivePeers(incoming)
	steers := agent.takeSteers()
	if len(steers) != 1 || !strings.Contains(steers[0], "use staging") || m.peers.steering != steers[0] {
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
