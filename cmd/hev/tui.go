package main

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/hev/kit/internal/layer"
	tracepkg "github.com/hev/kit/internal/trace"
	"github.com/spf13/cobra"
)

var tuiCmd = &cobra.Command{
	Use:     "browse",
	Aliases: []string{"tui", "watch"},
	Short:   "Interactive trace browser",
	RunE:    runTUI,
}

func runTUI(cmd *cobra.Command, args []string) error {
	cl, err := client("")
	if err != nil {
		return err
	}
	p := tea.NewProgram(newTUIModel(cl), tea.WithAltScreen())
	_, err = p.Run()
	return err
}

type tuiScreen int

const (
	screenList tuiScreen = iota
	screenTrace
)

type sessionsLoadedMsg struct {
	sessions []tracepkg.SessionRow
	err      error
}

type traceLoadedMsg struct {
	sess  tracepkg.SessionRow
	turns []tracepkg.Turn
	err   error
}

type tuiModel struct {
	client *layer.Client

	screen   tuiScreen
	sessions []tracepkg.SessionRow

	table      table.Model
	viewport   viewport.Model
	vpReady    bool
	traceSess  tracepkg.SessionRow
	showTools  bool
	traceTurns []tracepkg.Turn

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

func newTUIModel(cl *layer.Client) tuiModel {
	cols := []table.Column{
		{Title: "Started", Width: 12},
		{Title: "Model", Width: 12},
		{Title: "Host", Width: 14},
		{Title: "Tokens", Width: 13},
		{Title: "Prompts", Width: 7},
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
		client:     cl,
		screen:     screenList,
		table:      t,
		loadingMsg: "Loading sessions…",
	}
}

func (m tuiModel) Init() tea.Cmd {
	return loadSessionsCmd(m.client)
}

// tuiWindow is how far back the browser lists sessions.
const tuiWindow = 5 * 24 * time.Hour

func loadSessionsCmd(cl *layer.Client) tea.Cmd {
	return func() tea.Msg {
		filter := []any{"start", "Gte", time.Now().Add(-tuiWindow).UnixMilli()}
		sessions, err := cl.ListSessionRows(1000, filter)
		if err != nil {
			return sessionsLoadedMsg{err: err}
		}
		sort.Slice(sessions, func(i, j int) bool { return sessions[i].Start > sessions[j].Start })
		return sessionsLoadedMsg{sessions: sessions}
	}
}

func loadTraceCmd(cl *layer.Client, sess tracepkg.SessionRow) tea.Cmd {
	return func() tea.Msg {
		turns, err := sessionTurns(cl, sess.SessionID)
		return traceLoadedMsg{sess: sess, turns: turns, err: err}
	}
}

// renderTraceText renders the loaded trace with the current options
// (showTools, etc.).
func (m *tuiModel) renderTraceText() string {
	var buf bytes.Buffer
	renderNamespaceTurns(m.traceTurns, &buf, renderOpts{showTools: m.showTools})
	return buf.String()
}

// renderTrace re-renders the loaded trace into the viewport. Called both on
// load and on `t` toggle.
func (m *tuiModel) renderTrace() {
	m.viewport.SetContent(m.renderTraceText())
}

// yankTrace copies the currently-viewed trace to the system clipboard with
// ANSI styling stripped. Respects the active showTools toggle so the user
// gets exactly what they're looking at.
func (m *tuiModel) yankTrace() error {
	return clipboard.WriteAll(ansi.Strip(m.renderTraceText()))
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
		m.traceTurns = msg.turns
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
				return m, loadTraceCmd(m.client, m.sessions[cur])
			case "r":
				m.loadingMsg = "Reloading…"
				return m, loadSessionsCmd(m.client)
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
		started := formatStarted(time.UnixMilli(sess.Start), now)
		tokens := formatTokenPair(sess.InputTokens+sess.CacheReadTokens+sess.CacheCreationTokens, sess.OutputTokens)
		rows = append(rows, table.Row{
			started,
			formatModel(sess.Model),
			sess.Host,
			tokens,
			fmt.Sprintf("%d", sess.PromptCount),
			oneLine(sessionLabel(sess)),
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
	title := oneLine(sessionLabel(m.traceSess))
	id := m.traceSess.SessionID
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
	// Truncate to one row inside the style's padding: lipgloss wraps content
	// wider than Width, and a wrapped header pushes the footer off screen.
	content := ansi.Truncate(left+"  "+title, max(0, width-tuiHeaderStyle.GetHorizontalFrameSize()), "…")
	// Apply width to the same style so the background fills the full row
	// rather than wrapping a separately-styled inner element.
	return tuiHeaderStyle.Width(width).Render(content)
}

// sessionLabel is the title shown for a session: its summary, else its first
// prompt, else its id.
func sessionLabel(sess tracepkg.SessionRow) string {
	for _, s := range []string{sess.Summary, sess.FirstPromptShort, sess.FirstPrompt} {
		if s != "" {
			return s
		}
	}
	return sess.SessionID
}

// oneLine collapses runs of whitespace, newlines included, to single spaces.
// A table cell or the header bar holding a newline breaks the layout.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
