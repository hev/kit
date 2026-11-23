package foh

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type peerRow struct {
	Session string
	Agent   string
	State   string
	Cwd     string
	Role    string
	You     bool
}

type peerExchange struct {
	Time time.Time
	Dir  string // "→" or "←"
	Peer string
	Body string
}

type peersOverlay struct {
	query    string
	cursor   int
	loading  bool
	all      []peerRow
	filtered []peerRow
	note     string
}

type peersListMsg struct {
	rows []peerRow
	err  error
}

type peerSendMsg struct {
	target string
	body   string
	err    error
}

type peerBgDoneMsg struct {
	sender    string
	replyTo   string
	replyBody string
	err       error
}

const peerSidecarPrompt = `You are handling a background peer-to-peer message for this hev kit session.
Stay brief, answer concrete questions like a colleague using hev_peers send (action: "send") to the reply target in the header, and never run destructive or outward-facing actions without the user.
Stop rule: if the peer message is purely an acknowledgment or closing pleasantry (e.g. "thanks", "sounds good", "got it", "here if needed") with no new question or task, do NOT send another message — end your turn silently.`

func (m *TUI) startBackgroundPeerTurn(sender, text string) tea.Cmd {
	if m.opts.Agent == nil || m.opts.Agent.Provider == nil {
		return nil
	}
	m.peerBusy = true
	p := m.opts.Agent.Provider
	tools := m.opts.Agent.Tools
	model := m.opts.Agent.Model()
	sidecar := NewAgent(p, tools, model, peerSidecarPrompt)
	// Session is nil so background peer chatter never pollutes the main trace.
	return tea.Batch(shimmerTick(), func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		var replyTo, replyBody string
		err := sidecar.Run(ctx, text, func(ev Event) {
			if ev.Kind == EventToolStart && ev.Call.Function.Name == "hev_peers" {
				var a hevArgs
				if json.Unmarshal([]byte(ev.Call.Function.Arguments), &a) == nil && a.Action == "send" {
					replyTo = a.Target
					replyBody = a.Message
				}
			}
		})
		return peerBgDoneMsg{sender: sender, replyTo: replyTo, replyBody: replyBody, err: err}
	})
}

const maxPeerLog = 50

func (m *TUI) recordPeer(dir, peer, body string) {
	summary := fmt.Sprintf("⇄ %s %s: %s", dir, peer, truncateLine(body, 48))
	m.peerActivity = summary
	m.peerLog = append(m.peerLog, peerExchange{Time: time.Now(), Dir: dir, Peer: peer, Body: truncateLine(body, 100)})
	if len(m.peerLog) > maxPeerLog {
		m.peerLog = m.peerLog[len(m.peerLog)-maxPeerLog:]
	}
}

func (m *TUI) openPeersOverlay(initialQuery string) tea.Cmd {
	m.peers = &peersOverlay{query: initialQuery, loading: true}
	return m.fetchPeersList()
}

func (m *TUI) fetchPeersList() tea.Cmd {
	var execFn HevRunner
	if m.opts.Agent != nil && m.opts.Agent.Tools != nil {
		execFn = m.opts.Agent.Tools.Hev
	}
	if execFn == nil {
		execFn = ExecHev
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		raw, err := execFn(ctx, []string{"peers", "ls", "--json"}, "")
		if err != nil {
			return peersListMsg{err: err}
		}
		var doc struct {
			Panes []struct {
				Session string `json:"session"`
				Address string `json:"address"`
				Agent   string `json:"agent"`
				State   string `json:"state"`
				Cwd     string `json:"cwd"`
				Role    string `json:"role"`
				You     bool   `json:"you"`
			} `json:"panes"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			return peersListMsg{err: err}
		}
		rows := make([]peerRow, 0, len(doc.Panes))
		for _, p := range doc.Panes {
			name := p.Session
			if name == "" {
				name = p.Address
			}
			rows = append(rows, peerRow{
				Session: name,
				Agent:   p.Agent,
				State:   p.State,
				Cwd:     p.Cwd,
				Role:    p.Role,
				You:     p.You,
			})
		}
		return peersListMsg{rows: rows}
	}
}

func (m *TUI) sendPeerDirect(target, body string) tea.Cmd {
	var execFn HevRunner
	if m.opts.Agent != nil && m.opts.Agent.Tools != nil {
		execFn = m.opts.Agent.Tools.Hev
	}
	if execFn == nil {
		execFn = ExecHev
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_, err := execFn(ctx, []string{"peers", "send", target, body}, "")
		return peerSendMsg{target: target, body: body, err: err}
	}
}

func (m *TUI) refilterPeers() {
	p := m.peers
	if p == nil {
		return
	}
	p.filtered = p.filtered[:0]
	q := strings.TrimSpace(p.query)
	for _, r := range p.all {
		if q == "" ||
			fuzzyScore(q, r.Session) >= 0 ||
			fuzzyScore(q, r.Agent) >= 0 ||
			fuzzyScore(q, r.State) >= 0 ||
			fuzzyScore(q, r.Cwd) >= 0 {
			p.filtered = append(p.filtered, r)
		}
	}
	if p.cursor >= len(p.filtered) {
		p.cursor = max(0, len(p.filtered)-1)
	}
}

func (m *TUI) updatePeersOverlay(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := m.peers
	switch msg.Type {
	case tea.KeyEsc, tea.KeyCtrlC:
		m.peers = nil
		return m, nil
	case tea.KeyEnter:
		if len(p.filtered) > 0 {
			sel := p.filtered[p.cursor]
			m.peers = nil
			m.input.SetValue("/peers " + sel.Session + " ")
			m.input.CursorEnd()
			m.syncInputHeight()
			return m, nil
		}
		m.peers = nil
		return m, nil
	case tea.KeyUp, tea.KeyCtrlP:
		p.cursor = max(0, p.cursor-1)
	case tea.KeyDown, tea.KeyCtrlN:
		p.cursor = min(max(0, len(p.filtered)-1), p.cursor+1)
	case tea.KeyCtrlR:
		p.loading = true
		return m, m.fetchPeersList()
	case tea.KeyBackspace:
		if p.query != "" {
			runes := []rune(p.query)
			p.query = string(runes[:len(runes)-1])
			p.cursor = 0
			m.refilterPeers()
		}
	case tea.KeyRunes, tea.KeySpace:
		s := string(msg.Runes)
		if msg.Type == tea.KeySpace {
			s = " "
		}
		p.query += s
		p.cursor = 0
		m.refilterPeers()
	}
	return m, nil
}

func (m *TUI) peersOverlayView(width, height int) string {
	p := m.peers
	var b strings.Builder
	status := fmt.Sprintf("  %d live peer(s)", len(p.filtered))
	if p.loading {
		status = "  loading peers…"
	}
	b.WriteString(badge.Render("peers") + dimStyle.Render(status) + "\n")
	b.WriteString("filter › " + p.query + "▏\n")
	logBudget := 0
	if len(m.peerLog) > 0 {
		logBudget = min(len(m.peerLog), 5) + 2
	}
	rows := max(3, height-4-logBudget)
	start := 0
	if p.cursor >= rows {
		start = p.cursor - rows + 1
	}
	cwdMax := max(14, width-38)
	for i := start; i < len(p.filtered) && i < start+rows; i++ {
		r := p.filtered[i]
		cur := "  "
		hBadge := fmt.Sprintf("%-8s", "["+r.Agent+"]")
		youTag := ""
		if r.You {
			youTag = " (you)"
		}
		line := fmt.Sprintf("%-14s %s %-8s %s%s",
			ansi.Truncate(r.Session, 14, "…"),
			hBadge,
			r.State,
			ansi.Truncate(r.Cwd, cwdMax, "…"),
			youTag)
		if i == p.cursor {
			cur = lipgloss.NewStyle().Foreground(accent).Render("› ")
			line = lipgloss.NewStyle().Bold(true).Render(line)
		}
		b.WriteString(cur + line + "\n")
	}
	if !p.loading && len(p.filtered) == 0 {
		b.WriteString(dimStyle.Render("  no matching peers") + "\n")
	}
	if len(m.peerLog) > 0 {
		b.WriteString("\n" + dimStyle.Render("recent exchanges:") + "\n")
		logStart := max(0, len(m.peerLog)-5)
		for _, ex := range m.peerLog[logStart:] {
			b.WriteString("  " + sysStyle.Render(fmt.Sprintf("%s %s %s: %s", ex.Time.Format("15:04:05"), ex.Dir, ex.Peer, ansi.Truncate(ex.Body, max(16, width-28), "…"))) + "\n")
		}
	}
	if p.note != "" {
		b.WriteString(errStyle.Render("  "+p.note) + "\n")
	}
	b.WriteString(dimStyle.Render("Enter message peer · Ctrl+R refresh · type to filter · Esc close"))
	return b.String()
}
