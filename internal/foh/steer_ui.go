package foh

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type steerTargetRow struct {
	Name  string
	Host  string
	Kind  string // run | orch
	State string
}

func (r steerTargetRow) Target() string {
	if r.Host != "" {
		return r.Host + "/" + r.Name
	}
	return r.Name
}

type steerOverlay struct {
	query    string
	cursor   int
	loading  bool
	all      []steerTargetRow
	filtered []steerTargetRow
	note     string
}

type steerListMsg struct {
	rows []steerTargetRow
	err  error
}

type steerSendMsg struct {
	target string
	level  string
	body   string
	id     string
	out    string
	err    error
}

type watchedSteer struct {
	Target string
	ID     string
	Level  string
	Body   string
	State  string // queued | injected | acked
}

type steerPollTickMsg struct{}
type steerPollMsg struct {
	updates map[string]string // steer ID -> state
}

var steerIDRe = regexp.MustCompile(`st-[A-Za-z0-9_-]+`)

func extractSteerID(out string) string {
	return steerIDRe.FindString(out)
}

func looksLikeSteerTarget(tok string, watched []watchedRun) bool {
	if strings.Contains(tok, "/") || strings.HasPrefix(tok, "o-") || strings.HasPrefix(tok, "d-") {
		return true
	}
	for _, w := range watched {
		if w.Name == tok || w.Label() == tok {
			return true
		}
	}
	return false
}

func (m *TUI) scheduleSteerPoll() tea.Cmd {
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg { return steerPollTickMsg{} })
}

func (m *TUI) trackSteer(ws watchedSteer) tea.Cmd {
	if ws.ID == "" {
		return nil
	}
	first := len(m.watchedSteers) == 0
	m.watchedSteers = append(m.watchedSteers, ws)
	if first {
		return m.scheduleSteerPoll()
	}
	return nil
}

func (m *TUI) pollWatchedSteers() tea.Cmd {
	items := append([]watchedSteer(nil), m.watchedSteers...)
	var execFn HevRunner
	if m.opts.Agent != nil && m.opts.Agent.Tools != nil {
		execFn = m.opts.Agent.Tools.Hev
	}
	if execFn == nil {
		execFn = ExecHev
	}
	return func() tea.Msg {
		updates := map[string]string{}
		seenTarget := map[string]string{}
		for _, ws := range items {
			out, ok := seenTarget[ws.Target]
			if !ok {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				raw, err := execFn(ctx, []string{"steer", "ls", ws.Target}, "")
				cancel()
				if err != nil {
					continue
				}
				out = string(raw)
				seenTarget[ws.Target] = out
			}
			for _, line := range strings.Split(out, "\n") {
				if !strings.Contains(line, ws.ID) {
					continue
				}
				low := strings.ToLower(line)
				switch {
				case strings.Contains(low, "acked"):
					updates[ws.ID] = "acked"
				case strings.Contains(low, "injected"):
					updates[ws.ID] = "injected"
				case strings.Contains(low, "queued"):
					updates[ws.ID] = "queued"
				}
				break
			}
		}
		return steerPollMsg{updates: updates}
	}
}

func (m *TUI) openSteerOverlay(initialQuery string) tea.Cmd {
	m.steerer = &steerOverlay{query: initialQuery, loading: true}
	return m.fetchSteerTargets()
}

func (m *TUI) fetchSteerTargets() tea.Cmd {
	var execFn HevRunner
	if m.opts.Agent != nil && m.opts.Agent.Tools != nil {
		execFn = m.opts.Agent.Tools.Hev
	}
	if execFn == nil {
		execFn = ExecHev
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		raw, err := execFn(ctx, []string{"dispatch", "ls", "--json"}, "")
		if err != nil {
			return steerListMsg{err: err}
		}
		rows := parseSteerTargets(raw)
		return steerListMsg{rows: rows}
	}
}

func parseSteerTargets(raw []byte) []steerTargetRow {
	var doc map[string]any
	if json.Unmarshal(raw, &doc) != nil {
		return nil
	}
	var rows []steerTargetRow
	// Support both {"runs": [...]} and {"host": {"rows": [...]}} shapes.
	if arr, ok := doc["runs"].([]any); ok {
		for _, item := range arr {
			if r, ok := parseOneSteerRow("", item); ok {
				rows = append(rows, r)
			}
		}
	} else {
		for host, val := range doc {
			hm, ok := val.(map[string]any)
			if !ok {
				continue
			}
			if arr, ok := hm["rows"].([]any); ok {
				for _, item := range arr {
					if r, ok := parseOneSteerRow(host, item); ok {
						rows = append(rows, r)
					}
				}
			}
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		// Active (running/ready) first, then alphabetical.
		ai := isActiveSteerState(rows[i].State)
		aj := isActiveSteerState(rows[j].State)
		if ai != aj {
			return ai
		}
		return rows[i].Target() < rows[j].Target()
	})
	return rows
}

func parseOneSteerRow(defaultHost string, item any) (steerTargetRow, bool) {
	m, ok := item.(map[string]any)
	if !ok {
		return steerTargetRow{}, false
	}
	name := pickStr(m, "name")
	if name == "" {
		return steerTargetRow{}, false
	}
	state := pickStr(m, "state", "status")
	if strings.EqualFold(state, "closed") || strings.EqualFold(state, "done") || strings.EqualFold(state, "stopped") {
		return steerTargetRow{}, false
	}
	host := pickStr(m, "host")
	if host == "" {
		host = defaultHost
	}
	kind := pickStr(m, "kind")
	if kind == "" {
		if strings.HasPrefix(name, "o-") {
			kind = "orch"
		} else {
			kind = "run"
		}
	}
	return steerTargetRow{Name: name, Host: host, Kind: kind, State: state}, true
}

func isActiveSteerState(state string) bool {
	s := strings.ToLower(strings.TrimSpace(state))
	return s == "running" || s == "ready" || s == "waiting" || s == "active"
}

func (m *TUI) sendSteerDirect(target, level, body string) tea.Cmd {
	var execFn HevRunner
	if m.opts.Agent != nil && m.opts.Agent.Tools != nil {
		execFn = m.opts.Agent.Tools.Hev
	}
	if execFn == nil {
		execFn = ExecHev
	}
	if level == "" {
		level = "note"
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		argv := []string{"steer", target, body, "--level", level}
		out, err := execFn(ctx, argv, "")
		outStr := strings.TrimSpace(string(out))
		return steerSendMsg{target: target, level: level, body: body, id: extractSteerID(outStr), out: outStr, err: err}
	}
}

func (m *TUI) refilterSteer() {
	s := m.steerer
	if s == nil {
		return
	}
	s.filtered = s.filtered[:0]
	q := strings.TrimSpace(s.query)
	for _, r := range s.all {
		if q == "" ||
			fuzzyScore(q, r.Name) >= 0 ||
			fuzzyScore(q, r.Host) >= 0 ||
			fuzzyScore(q, r.Kind) >= 0 ||
			fuzzyScore(q, r.State) >= 0 {
			s.filtered = append(s.filtered, r)
		}
	}
	if s.cursor >= len(s.filtered) {
		s.cursor = max(0, len(s.filtered)-1)
	}
}

func (m *TUI) updateSteerOverlay(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := m.steerer
	switch msg.Type {
	case tea.KeyEsc, tea.KeyCtrlC:
		m.steerer = nil
		return m, nil
	case tea.KeyEnter:
		if len(s.filtered) > 0 {
			sel := s.filtered[s.cursor]
			m.steerer = nil
			m.input.SetValue("/steer " + sel.Target() + " ")
			m.input.CursorEnd()
			m.syncInputHeight()
			return m, nil
		}
		m.steerer = nil
		return m, nil
	case tea.KeyUp, tea.KeyCtrlP:
		s.cursor = max(0, s.cursor-1)
	case tea.KeyDown, tea.KeyCtrlN:
		s.cursor = min(max(0, len(s.filtered)-1), s.cursor+1)
	case tea.KeyCtrlR:
		s.loading = true
		return m, m.fetchSteerTargets()
	case tea.KeyBackspace:
		if s.query != "" {
			runes := []rune(s.query)
			s.query = string(runes[:len(runes)-1])
			s.cursor = 0
			m.refilterSteer()
		}
	case tea.KeyRunes, tea.KeySpace:
		txt := string(msg.Runes)
		if msg.Type == tea.KeySpace {
			txt = " "
		}
		s.query += txt
		s.cursor = 0
		m.refilterSteer()
	}
	return m, nil
}

func (m *TUI) steerOverlayView(width, height int) string {
	s := m.steerer
	var b strings.Builder
	status := fmt.Sprintf("  %d active BOH target(s)", len(s.filtered))
	if s.loading {
		status = "  loading active runs & orchestrators…"
	}
	b.WriteString(badge.Render("steer BOH") + dimStyle.Render(status) + "\n")
	b.WriteString("filter › " + s.query + "▏\n")
	rows := max(3, height-4)
	start := 0
	if s.cursor >= rows {
		start = s.cursor - rows + 1
	}
	nameMax := max(18, width-32)
	for i := start; i < len(s.filtered) && i < start+rows; i++ {
		r := s.filtered[i]
		cur := "  "
		kBadge := fmt.Sprintf("%-6s", "["+r.Kind+"]")
		line := fmt.Sprintf("%-10s %s %-10s %s",
			ansi.Truncate(r.Host, 10, "…"),
			kBadge,
			r.State,
			ansi.Truncate(r.Name, nameMax, "…"))
		if i == s.cursor {
			cur = lipgloss.NewStyle().Foreground(accent).Render("› ")
			line = lipgloss.NewStyle().Bold(true).Render(line)
		}
		b.WriteString(cur + line + "\n")
	}
	if !s.loading && len(s.filtered) == 0 {
		b.WriteString(dimStyle.Render("  no active runs or orchestrators to steer") + "\n")
	}
	if s.note != "" {
		b.WriteString(errStyle.Render("  "+s.note) + "\n")
	}
	b.WriteString(dimStyle.Render("Enter steer target · /steer [--urgent|--halt|--redirect] TARGET MSG · Ctrl+R refresh · Esc close"))
	return b.String()
}
