package main

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/hev/kit/internal/store"
	"github.com/spf13/cobra"
)

var tuiCmd = &cobra.Command{
	Use:     "browse",
	Aliases: []string{"tui", "watch"},
	Short:   "Interactive trace browser",
	RunE:    runTUI,
}

func runTUI(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	s, err := openStore()
	if err != nil {
		return err
	}
	p := tea.NewProgram(newTUIModel(ctx, s), tea.WithAltScreen())
	_, err = p.Run()
	return err
}

type tuiScreen int

const (
	screenList tuiScreen = iota
	screenTrace
)

type sessionsLoadedMsg struct {
	sessions []store.OTLPSession
	err      error
}

type traceLoadedMsg struct {
	sess        store.OTLPSession
	otlpEvents  []store.OTLPEvent
	codexEvents []store.CodexRolloutEvent
	err         error
}

type tuiModel struct {
	ctx   context.Context
	store *store.Store

	screen   tuiScreen
	sessions []store.OTLPSession

	table      table.Model
	viewport   viewport.Model
	vpReady    bool
	traceSess  store.OTLPSession
	showTools  bool
	traceCache []store.OTLPEvent
	codexCache []store.CodexRolloutEvent

	loadingMsg string
	err        error
	status     string

	width, height int
}

var (
	tuiTitleStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("215")).Bold(true)
	tuiFooterStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	tuiErrStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
	tuiOkStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("114")).Bold(true)
	tuiHeaderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Background(lipgloss.Color("63")).Bold(true).Padding(0, 1)
)

func newTUIModel(ctx context.Context, s *store.Store) tuiModel {
	cols := []table.Column{
		{Title: "Started", Width: 12},
		{Title: "Model", Width: 12},
		{Title: "GH User", Width: 14},
		{Title: "Tokens", Width: 13},
		{Title: "Events", Width: 7},
		{Title: "Title", Width: 60},
	}
	t := table.New(
		table.WithColumns(cols),
		table.WithFocused(true),
		table.WithHeight(10),
	)
	st := table.DefaultStyles()
	st.Header = st.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("244")).
		BorderBottom(true).
		Bold(true).
		Foreground(lipgloss.Color("252"))
	st.Selected = st.Selected.
		Foreground(lipgloss.Color("231")).
		Background(lipgloss.Color("63")).
		Bold(true)
	t.SetStyles(st)

	return tuiModel{
		ctx:        ctx,
		store:      s,
		screen:     screenList,
		table:      t,
		loadingMsg: "Loading sessions…",
	}
}

func (m tuiModel) Init() tea.Cmd {
	return loadSessionsCmd(m.ctx, m.store)
}

func loadSessionsCmd(ctx context.Context, s *store.Store) tea.Cmd {
	return func() tea.Msg {
		now := time.Now()
		since := 5 * 24 * time.Hour
		cutoff := now.Add(-since)
		dates := datesInWindow(now, since)
		var all []store.OTLPSession
		for _, date := range dates {
			sessions, err := s.ListOTLPSessions(ctx, date)
			if err != nil {
				sessions = nil
			}
			codexSessions, _ := s.ListCodexRolloutSessions(ctx, date)
			for _, sess := range append(sessions, codexSessions...) {
				if !sess.FirstTS.IsZero() && sess.FirstTS.Before(cutoff) {
					continue
				}
				all = append(all, sess)
			}
		}
		sort.Slice(all, func(i, j int) bool { return all[i].FirstTS.After(all[j].FirstTS) })
		s.LoadTitles(ctx, all)
		return sessionsLoadedMsg{sessions: all}
	}
}

func loadTraceCmd(ctx context.Context, s *store.Store, sess store.OTLPSession) tea.Cmd {
	return func() tea.Msg {
		if sess.Harness == "codex_cli" {
			full, events, err := s.GetCodexRolloutSession(ctx, sess.Date, sess.ID)
			if err != nil {
				return traceLoadedMsg{sess: sess, err: err}
			}
			return traceLoadedMsg{sess: full, codexEvents: events}
		}

		full, events, err := s.GetOTLPSession(ctx, sess.Date, sess.ID)
		if err != nil {
			full, codexEvents, codexErr := s.GetCodexRolloutSession(ctx, sess.Date, sess.ID)
			if codexErr != nil {
				return traceLoadedMsg{sess: sess, err: err}
			}
			return traceLoadedMsg{sess: full, codexEvents: codexEvents}
		}
		return traceLoadedMsg{sess: full, otlpEvents: events}
	}
}

// renderTrace re-renders the cached trace into the viewport using current
// model options (showTools, etc.). Called both on load and on `t` toggle.
func (m *tuiModel) renderTrace() {
	var buf bytes.Buffer
	opts := renderOpts{showTools: m.showTools}
	if len(m.codexCache) > 0 {
		_ = showCodexSessionOpts(m.traceSess, m.codexCache, &buf, opts)
	} else {
		_ = showOTLPSessionOpts(m.ctx, m.store, m.traceSess, m.traceCache, &buf, opts)
	}
	m.viewport.SetContent(buf.String())
}

// yankTrace copies the currently-viewed trace to the system clipboard with
// ANSI styling stripped. Respects the active showTools toggle so the user
// gets exactly what they're looking at.
func (m *tuiModel) yankTrace() error {
	var buf bytes.Buffer
	opts := renderOpts{showTools: m.showTools}
	if len(m.codexCache) > 0 {
		if err := showCodexSessionOpts(m.traceSess, m.codexCache, &buf, opts); err != nil {
			return err
		}
	} else {
		if err := showOTLPSessionOpts(m.ctx, m.store, m.traceSess, m.traceCache, &buf, opts); err != nil {
			return err
		}
	}
	return clipboard.WriteAll(ansi.Strip(buf.String()))
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.table.SetWidth(msg.Width)
		m.table.SetHeight(msg.Height - 4)
		// last column flexes to fill remaining width
		cols := m.table.Columns()
		fixed := 0
		for i := 0; i < len(cols)-1; i++ {
			fixed += cols[i].Width
		}
		// account for table internal padding between cells
		padding := 2 * len(cols)
		last := msg.Width - fixed - padding
		if last < 20 {
			last = 20
		}
		cols[len(cols)-1].Width = last
		m.table.SetColumns(cols)

		// Trace screen: header (1) + blank (1) + footer (1) = 3 lines reserved.
		vpHeight := msg.Height - 3
		if vpHeight < 5 {
			vpHeight = 5
		}
		if !m.vpReady {
			m.viewport = viewport.New(msg.Width, vpHeight)
			m.vpReady = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = vpHeight
		}

	case sessionsLoadedMsg:
		m.loadingMsg = ""
		m.err = msg.err
		m.sessions = msg.sessions
		m.refreshRows()

	case traceLoadedMsg:
		m.loadingMsg = ""
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.traceSess = msg.sess
		m.traceCache = msg.otlpEvents
		m.codexCache = msg.codexEvents
		m.renderTrace()
		m.viewport.GotoTop()
		m.screen = screenTrace

	case tea.KeyMsg:
		// Dismiss any sticky error or transient status on next key.
		m.err = nil
		m.status = ""
		switch m.screen {
		case screenList:
			switch msg.String() {
			case "q", "ctrl+c":
				return m, tea.Quit
			case "enter":
				if len(m.sessions) == 0 {
					return m, nil
				}
				cur := m.table.Cursor()
				if cur < 0 || cur >= len(m.sessions) {
					return m, nil
				}
				m.loadingMsg = "Loading trace…"
				return m, loadTraceCmd(m.ctx, m.store, m.sessions[cur])
			case "r":
				m.loadingMsg = "Reloading…"
				return m, loadSessionsCmd(m.ctx, m.store)
			}
		case screenTrace:
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "q", "esc", "backspace":
				m.screen = screenList
				return m, nil
			case "t":
				m.showTools = !m.showTools
				m.renderTrace()
				return m, nil
			case "y":
				if err := m.yankTrace(); err != nil {
					m.err = fmt.Errorf("yank failed: %w", err)
				} else {
					m.status = "Copied trace to clipboard"
				}
				return m, nil
			}
		}
	}

	var cmd tea.Cmd
	if m.screen == screenList {
		m.table, cmd = m.table.Update(msg)
	} else {
		m.viewport, cmd = m.viewport.Update(msg)
	}
	return m, cmd
}

func (m *tuiModel) refreshRows() {
	now := time.Now()
	rows := make([]table.Row, 0, len(m.sessions))
	for _, sess := range m.sessions {
		started := formatStarted(sess.FirstTS, now)
		tokens := formatTokenPair(sess.InputTokens+sess.CacheReadTokens+sess.CacheCreationTokens, sess.OutputTokens)
		label := sess.Title
		if label == "" {
			label = sess.FirstPrompt
		}
		rows = append(rows, table.Row{
			started,
			formatModel(sess.Model),
			sess.GHUser,
			tokens,
			fmt.Sprintf("%d", sess.EventCount),
			label,
		})
	}
	m.table.SetRows(rows)
}

func (m tuiModel) View() string {
	if m.loadingMsg != "" {
		return "\n  " + m.loadingMsg + "\n"
	}

	switch m.screen {
	case screenList:
		title := tuiTitleStyle.Render("hev traces")
		help := tuiFooterStyle.Render("↑/↓ select  ·  enter view  ·  r reload  ·  q quit")
		if m.err != nil {
			help = tuiErrStyle.Render(m.err.Error()) + "  " + help
		}
		return title + "\n" + m.table.View() + "\n" + help
	case screenTrace:
		toolHint := "t show tools"
		if m.showTools {
			toolHint = "t hide tools"
		}
		help := tuiFooterStyle.Render(fmt.Sprintf("↑/↓ scroll  ·  %s  ·  y yank  ·  esc back  ·  q quit  ·  %3.0f%%",
			toolHint, m.viewport.ScrollPercent()*100))
		switch {
		case m.err != nil:
			help = tuiErrStyle.Render(m.err.Error()) + "  " + help
		case m.status != "":
			help = tuiOkStyle.Render(m.status) + "  " + help
		}
		return m.renderHeader() + "\n" + m.viewport.View() + "\n" + help
	}
	return ""
}

// renderHeader produces the pinned title bar shown above the trace viewport.
// It uses the same fg/bg as the table selector so visually the row "carries"
// into the detail view.
func (m tuiModel) renderHeader() string {
	title := m.traceSess.Title
	if title == "" {
		title = m.traceSess.FirstPrompt
	}
	if title == "" {
		title = m.traceSess.ID
	}
	id := m.traceSess.ID
	if len(id) > 8 {
		id = id[:8]
	}
	left := id
	if model := formatModel(m.traceSess.Model); model != "" {
		left += "  " + model
	}
	width := m.width
	if width <= 0 {
		width = 80
	}
	content := left + "  " + title
	// Apply width to the same style so the background fills the full row
	// rather than wrapping a separately-styled inner element.
	return tuiHeaderStyle.Width(width).Render(content)
}
