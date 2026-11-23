package foh

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/hev/kit/internal/peers/nativeinbox"
)

const (
	peerPollInterval = time.Second
	peerQueueSize    = 8
)

type nativePeers struct {
	store          nativeinbox.Store
	owner          nativeinbox.Presence
	background     []peerSidecarMessage
	interrupts     []string
	steering       string
	lastBackground string
	lastInterrupt  string
}

type peerSidecarMessage struct {
	sender string
	text   string
}

type nativePeerTick struct {
	messages []nativeinbox.Message
}

// RegisterNativePeer ties presence to the interactive program's lifetime.
// Child tools inherit the address so peers can attribute their sends.
func (m *TUI) RegisterNativePeer(ctx context.Context) (func(), error) {
	noop := func() {}
	home, err := os.UserHomeDir()
	if err != nil {
		return noop, fmt.Errorf("native peer home unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	git := exec.CommandContext(ctx, "git", "-C", m.opts.Cwd, "branch", "--show-current")
	git.WaitDelay = 250 * time.Millisecond
	branch, _ := git.Output()
	store := nativeinbox.AtHome(home)
	owner, err := store.Register(m.opts.Cwd, strings.TrimSpace(string(branch)), os.Getenv("TMUX_PANE"))
	if err != nil {
		return noop, err
	}
	prior, hadPrior := os.LookupEnv("HEV_NATIVE_PEER")
	if err := os.Setenv("HEV_NATIVE_PEER", owner.Name); err != nil {
		_ = store.Close(owner)
		return noop, fmt.Errorf("native peer address unavailable")
	}
	m.nativePeer = &nativePeers{store: store, owner: owner}
	return func() {
		_ = store.Close(owner)
		if hadPrior {
			_ = os.Setenv("HEV_NATIVE_PEER", prior)
		} else {
			_ = os.Unsetenv("HEV_NATIVE_PEER")
		}
	}, nil
}

func (m *TUI) setBusy(busy bool) {
	m.busy = busy
	if p := m.nativePeer; p != nil {
		_ = p.store.Update(p.owner, busy)
	}
}

func (m *TUI) pollPeers() tea.Cmd {
	p := m.nativePeer
	if p == nil {
		return nil
	}
	limit := peerQueueSize - max(len(p.interrupts), len(p.background))
	return tea.Tick(peerPollInterval, func(time.Time) tea.Msg {
		messages, _ := p.store.Drain(p.owner, limit)
		return nativePeerTick{messages: messages}
	})
}

func (m *TUI) receivePeers(tick nativePeerTick) tea.Cmd {
	p := m.nativePeer
	if p == nil {
		return nil
	}
	m.setBusy(m.busy)
	for _, msg := range tick.messages {
		body := Scrub(msg.Body, providerSecrets(m.opts.Agent.Provider)...)
		preview := strings.Join(strings.Fields(ansi.Strip(body)), " ")
		preview = strings.Map(func(r rune) rune {
			if r < ' ' || r == 127 || r >= 128 && r <= 159 {
				return -1
			}
			return r
		}, preview)
		from := ansi.Strip(msg.From)
		text := fmt.Sprintf("[peer %s from %q, id %q] %q", msg.Mode, from, msg.ID, body)
		if msg.Mode == nativeinbox.Interrupt {
			p.lastInterrupt = fmt.Sprintf("Peer interrupt from %q · %s", from, preview)
			p.interrupts = append(p.interrupts, text)
		} else {
			p.lastBackground = fmt.Sprintf("Peer background from %q · %s", from, preview)
			p.background = append(p.background, peerSidecarMessage{sender: from, text: text})
		}
	}
	turn := m.deliverPeerInterrupt()
	return tea.Batch(m.pollPeers(), m.deliverPeerBackground(), turn)
}

func (m *TUI) deliverPeerInterrupt() tea.Cmd {
	p := m.nativePeer
	if p.steering != "" || len(p.interrupts) == 0 {
		return nil
	}
	text := p.interrupts[0]
	if m.busy {
		if m.opts.Agent.Steer(text) {
			p.steering = text
			p.interrupts = p.interrupts[1:]
		}
		return nil
	}
	p.interrupts = p.interrupts[1:]
	return m.startTurn(text)
}

func (m *TUI) finishPeerSteer() tea.Cmd {
	p := m.nativePeer
	if p == nil {
		return nil
	}
	if p.steering != "" {
		// A steer can arrive after the loop's last check or during cancellation.
		// The worker has finished, so reclaim that message before starting again.
		for _, steer := range m.opts.Agent.takeSteers() {
			if steer != p.steering {
				m.opts.Agent.Steer(steer)
			}
		}
		text := p.steering
		p.steering = ""
		return m.startTurn(text)
	}
	return m.deliverPeerInterrupt()
}

func (m *TUI) deliverPeerBackground() tea.Cmd {
	p := m.nativePeer
	if p == nil || m.peerBusy || len(p.background) == 0 {
		return nil
	}
	next := p.background[0]
	p.background = p.background[1:]
	return m.startBackgroundPeerTurn(next.sender, next.text)
}

func (m *TUI) peerStatus(width int) string {
	if m.nativePeer == nil {
		return ""
	}
	var lines []string
	for _, status := range []string{m.nativePeer.lastBackground, m.nativePeer.lastInterrupt} {
		if status != "" {
			lines = append(lines, dimStyle.Render(ansi.Truncate(status, width, "…")))
		}
	}
	return strings.Join(lines, "\n")
}
