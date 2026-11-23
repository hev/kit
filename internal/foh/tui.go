package foh

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// CatalogLoader returns the model catalog for the picker.
type CatalogLoader func(ctx context.Context) ([]Model, CatalogSource, error)

// TUIOptions configures the meta-harness TUI.
type TUIOptions struct {
	Agent     *Agent
	Catalog   CatalogLoader
	Favorites *Favorites
	Composer  PromptComposer
	Version   string
	Cwd       string
	Workspace string
	Color     bool
}

type entryKind int

const (
	entryUser entryKind = iota
	entryAssistant
	entryTool
	entryCard
	entrySystem
	entryError
)

type entry struct {
	kind   entryKind
	text   string
	detail string
	card   *Card
	callID string
}

// TUI is the Bubble Tea model for hev foh.
type TUI struct {
	opts   TUIOptions
	width  int
	height int
	input  textinput.Model

	entries     []entry
	busy        bool
	cancel      context.CancelFunc
	events      chan Event
	expandCards bool

	models        []Model
	catalogSource CatalogSource
	picker        *picker
	quitting      bool
	peers         *nativePeers
}

// picker is the in-harness model picker overlay.
type picker struct {
	query     string
	cursor    int
	focusList bool
	filtered  []Model
	note      string
}

type agentEventMsg struct{ ev Event }
type agentDoneMsg struct{ err error }
type catalogMsg struct {
	models []Model
	source CatalogSource
	err    error
}

// NewTUI builds the TUI model.
func NewTUI(opts TUIOptions) *TUI {
	in := textinput.New()
	in.Placeholder = "Ask hev foh…  (/help)"
	in.Prompt = "› "
	in.CharLimit = 32 << 10
	in.Focus()
	if opts.Favorites == nil {
		opts.Favorites = LoadFavorites("")
	}
	return &TUI{opts: opts, input: in, width: 100, height: 40}
}

// Init loads the catalog in the background.
func (m *TUI) Init() tea.Cmd { return tea.Batch(textinput.Blink, m.loadCatalog(), m.pollPeers()) }

func (m *TUI) loadCatalog() tea.Cmd {
	if m.opts.Catalog == nil {
		return nil
	}
	load := m.opts.Catalog
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		models, src, err := load(ctx)
		return catalogMsg{models, src, err}
	}
}

func waitEvent(ch <-chan Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return agentDoneMsg{}
		}
		return agentEventMsg{ev}
	}
}

// Update handles keys, agent events and catalog loads.
func (m *TUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case nativePeerTick:
		return m, m.receivePeers(msg)
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.input.Width = max(10, msg.Width-4)
		return m, nil
	case catalogMsg:
		m.models, m.catalogSource = msg.models, msg.source
		if m.picker != nil {
			m.refilter()
		}
		return m, nil
	case agentEventMsg:
		m.applyEvent(msg.ev)
		return m, waitEvent(m.events)
	case agentDoneMsg:
		m.setBusy(false)
		m.events = nil
		if m.cancel != nil {
			m.cancel()
			m.cancel = nil
		}
		return m, m.finishPeerSteer()
	case tea.KeyMsg:
		if m.picker != nil {
			return m.updatePicker(msg)
		}
		return m.updateKeys(msg)
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *TUI) updateKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.Type == tea.KeyCtrlC:
		if m.busy && m.cancel != nil {
			m.cancel()
			m.system("turn cancelled")
			return m, nil
		}
		m.quitting = true
		return m, tea.Quit
	case msg.Type == tea.KeyCtrlO:
		m.expandCards = !m.expandCards
		return m, nil
	case msg.Alt && msg.String() == "alt+m":
		return m, m.openPicker()
	case msg.Type == tea.KeyEnter: // also Ctrl+M: terminals send the same byte
		text := strings.TrimSpace(m.input.Value())
		if text == "" {
			return m, m.openPicker()
		}
		m.input.Reset()
		if strings.HasPrefix(text, "/") {
			return m, m.command(text)
		}
		if m.busy {
			if m.opts.Agent.Steer(text) {
				m.entries = append(m.entries, entry{kind: entrySystem, text: "↪ steer queued: lands at the next tool boundary", detail: text})
			} else {
				m.system("steer queue full; wait for the turn to finish")
			}
			return m, nil
		}
		return m, m.startTurn(m.withPeerNotes(text))
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *TUI) system(text string) {
	m.entries = append(m.entries, entry{kind: entrySystem, text: text})
}

const helpText = `Enter send (steers while a turn runs) · Ctrl+M or Enter on an empty line: model picker
/model [slug]  pick or switch model    /prompt  show the composed system prompt
/reload        recompose the prompt    /clear   new thread       /help  this help
/quit          exit                    Ctrl+O   expand hev cards  Ctrl+C cancel turn / quit`

func (m *TUI) command(text string) tea.Cmd {
	name, arg, _ := strings.Cut(text, " ")
	arg = strings.TrimSpace(arg)
	switch name {
	case "/model":
		if arg != "" {
			m.switchModel(arg)
			return nil
		}
		return m.openPicker()
	case "/prompt":
		parts, err := m.opts.Composer.Parts()
		if err != nil {
			m.entries = append(m.entries, entry{kind: entryError, text: err.Error()})
			return nil
		}
		var srcs []string
		for _, p := range parts {
			srcs = append(srcs, fmt.Sprintf("%s(%s, %d B)", p.Label, p.Source, len(p.Text)))
		}
		m.entries = append(m.entries, entry{kind: entrySystem, text: "system prompt: " + strings.Join(srcs, " + "), detail: m.opts.Agent.System()})
	case "/reload":
		s, err := m.opts.Composer.Compose()
		if err != nil {
			m.entries = append(m.entries, entry{kind: entryError, text: err.Error()})
			return nil
		}
		m.opts.Agent.SetSystem(s)
		m.system(fmt.Sprintf("⟳ system prompt reloaded (%d bytes)", len(s)))
	case "/clear":
		if m.busy {
			m.system("wait for the turn to finish before /clear")
			return nil
		}
		m.opts.Agent.Reset()
		m.entries = nil
	case "/help":
		m.entries = append(m.entries, entry{kind: entrySystem, text: "help", detail: helpText})
	case "/quit", "/exit":
		m.quitting = true
		if m.cancel != nil {
			m.cancel()
		}
		return tea.Quit
	default:
		m.system("unknown command " + name + " (try /help)")
	}
	return nil
}

func (m *TUI) startTurn(text string) tea.Cmd {
	m.entries = append(m.entries, entry{kind: entryUser, text: text})
	m.setBusy(true)
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	ch := make(chan Event, 256)
	m.events = ch
	agent := m.opts.Agent
	go func() {
		defer close(ch)
		_ = agent.Run(ctx, text, func(ev Event) {
			select {
			case ch <- ev:
			case <-ctx.Done():
			}
		})
	}()
	return waitEvent(ch)
}

func (m *TUI) last(kind entryKind) *entry {
	if n := len(m.entries); n > 0 && m.entries[n-1].kind == kind {
		return &m.entries[n-1]
	}
	return nil
}

func (m *TUI) applyEvent(ev Event) {
	switch ev.Kind {
	case EventText:
		if e := m.last(entryAssistant); e != nil {
			e.text += ev.Text
		} else {
			m.entries = append(m.entries, entry{kind: entryAssistant, text: ev.Text})
		}
	case EventToolStart:
		if IsHevTool(ev.Call.Function.Name) {
			m.entries = append(m.entries, entry{kind: entryCard, callID: ev.Call.ID,
				card: &Card{Tool: ev.Call.Function.Name, Action: argField(ev.Call.Function.Arguments, "action"), Summary: "running…"}})
		} else {
			m.entries = append(m.entries, entry{kind: entryTool, callID: ev.Call.ID, text: ev.Call.Function.Name + " " + argsSummary(ev.Call.Function.Arguments)})
		}
	case EventToolResult:
		for i := len(m.entries) - 1; i >= 0; i-- {
			e := &m.entries[i]
			if e.callID != ev.Call.ID {
				continue
			}
			if e.kind == entryCard && ev.Result.Card != nil {
				e.card = ev.Result.Card
			} else {
				e.detail = ev.Result.Content
				if ev.Result.IsError {
					e.text += "  ✗"
				}
			}
			break
		}
	case EventSteer:
		if m.peers != nil && m.peers.steering == ev.Text {
			m.peers.steering = ""
		}
		m.system("↪ steer injected: " + ev.Text)
	case EventError:
		m.entries = append(m.entries, entry{kind: entryError, text: ev.Err.Error()})
	}
}

func argField(args, key string) string {
	var v map[string]any
	if json.Unmarshal([]byte(args), &v) != nil {
		return ""
	}
	s, _ := v[key].(string)
	return s
}

func argsSummary(args string) string {
	var v map[string]any
	if json.Unmarshal([]byte(args), &v) != nil {
		return truncateLine(args, 80)
	}
	for _, k := range []string{"command", "path", "pattern"} {
		if s, ok := v[k].(string); ok {
			return truncateLine(strings.ReplaceAll(s, "\n", " "), 100)
		}
	}
	return truncateLine(args, 80)
}

// --- model picker ---

func (m *TUI) openPicker() tea.Cmd {
	m.picker = &picker{}
	m.refilter()
	if m.models == nil {
		m.picker.note = "loading catalog…"
		return m.loadCatalog()
	}
	return nil
}

func (m *TUI) catalog() []Model {
	if len(m.models) > 0 {
		return m.models
	}
	return StarterModels
}

// fuzzyScore scores a subsequence match of q in s; -1 when q does not match.
func fuzzyScore(q, s string) int {
	q, s = strings.ToLower(q), strings.ToLower(s)
	if q == "" {
		return 0
	}
	score, si, prev := 0, 0, -2
	for _, r := range q {
		i := strings.IndexRune(s[si:], r)
		if i < 0 {
			return -1
		}
		pos := si + i
		score += 10
		if pos == prev+1 {
			score += 15 // contiguous
		}
		if pos == 0 || strings.ContainsRune("/-_.: ", rune(s[pos-1])) {
			score += 10 // word start
		}
		score -= min(i, 10)
		prev, si = pos, pos+len(string(r))
	}
	return score
}

func (m *TUI) refilter() {
	p := m.picker
	type scored struct {
		m     Model
		score int
		fav   bool
		order int
	}
	seen := map[string]bool{}
	var rows []scored
	add := func(model Model, order int) {
		if seen[model.ID] {
			return
		}
		seen[model.ID] = true
		s := fuzzyScore(p.query, model.ID)
		for _, field := range []string{model.Name, model.ProviderID()} {
			if n := fuzzyScore(p.query, field); n > s {
				s = n
			}
		}
		if s >= 0 {
			rows = append(rows, scored{model, s, m.opts.Favorites.Has(model.ID), order})
		}
	}
	for i, model := range m.catalog() {
		add(model, i)
	}
	// Favorites missing from the catalog still show, under the provider
	// their ID implies.
	for i, id := range m.opts.Favorites.IDs {
		add(Model{ID: id, PromptPrice: -1, CompletionPrice: -1}, len(m.catalog())+i)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.fav != b.fav {
			return a.fav
		}
		if a.score != b.score {
			return a.score > b.score
		}
		return a.order < b.order
	})
	p.filtered = p.filtered[:0]
	for _, r := range rows {
		p.filtered = append(p.filtered, r.m)
	}
	if p.cursor >= len(p.filtered) {
		p.cursor = max(0, len(p.filtered)-1)
	}
}

func (m *TUI) toggleFavorite() {
	p := m.picker
	if len(p.filtered) == 0 {
		return
	}
	id := p.filtered[p.cursor].ID
	on, err := m.opts.Favorites.Toggle(id)
	switch {
	case err != nil:
		p.note = "could not save favorites"
	case on:
		p.note = "★ starred " + id
	default:
		p.note = "unstarred " + id
	}
	m.refilter()
	for i, model := range p.filtered {
		if model.ID == id {
			p.cursor = i
		}
	}
}

func (m *TUI) switchModel(id string) {
	from := m.opts.Agent.Model()
	if id == from {
		m.system("model unchanged: " + id)
		return
	}
	m.opts.Agent.SetModel(id)
	text := "⇄ model " + from + " → " + id
	if fp, tp := m.opts.Agent.ProviderFor(from), m.opts.Agent.ProviderFor(id); fp != tp {
		text += " (" + fp + " → " + tp + ")"
	}
	m.entries = append(m.entries, entry{kind: entrySystem, text: text})
}

func (m *TUI) updatePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := m.picker
	switch msg.Type {
	case tea.KeyEsc, tea.KeyCtrlC:
		m.picker = nil
		return m, nil
	case tea.KeyEnter:
		if len(p.filtered) > 0 {
			m.switchModel(p.filtered[p.cursor].ID)
		}
		m.picker = nil
		return m, nil
	case tea.KeyUp, tea.KeyCtrlP:
		p.cursor = max(0, p.cursor-1)
	case tea.KeyDown, tea.KeyCtrlN:
		p.cursor = min(max(0, len(p.filtered)-1), p.cursor+1)
	case tea.KeyTab:
		p.focusList = !p.focusList
	case tea.KeyCtrlF:
		m.toggleFavorite()
	case tea.KeyBackspace:
		if p.query != "" {
			r := []rune(p.query)
			p.query = string(r[:len(r)-1])
			p.cursor = 0
			m.refilter()
		}
		p.focusList = false
	case tea.KeyRunes, tea.KeySpace:
		s := string(msg.Runes)
		if msg.Type == tea.KeySpace {
			s = " "
		}
		if p.focusList {
			switch s {
			case "f":
				m.toggleFavorite()
				return m, nil
			case "j":
				p.cursor = min(max(0, len(p.filtered)-1), p.cursor+1)
				return m, nil
			case "k":
				p.cursor = max(0, p.cursor-1)
				return m, nil
			}
			p.focusList = false
		}
		p.query += s
		p.cursor = 0
		m.refilter()
	}
	return m, nil
}

// --- view ---

var (
	accent    = lipgloss.Color("#eb5768")
	gold      = lipgloss.Color("#fbe476")
	steel     = lipgloss.Color("#5186a5")
	dim       = lipgloss.Color("#8a8a8a")
	userStyle = lipgloss.NewStyle().Bold(true).Foreground(gold)
	sysStyle  = lipgloss.NewStyle().Foreground(steel).Italic(true)
	errStyle  = lipgloss.NewStyle().Foreground(accent).Bold(true)
	dimStyle  = lipgloss.NewStyle().Foreground(dim)
	cardStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accent).Padding(0, 1)
	badge     = lipgloss.NewStyle().Foreground(lipgloss.Color("#000000")).Background(gold).Bold(true).Padding(0, 1)
)

func (m *TUI) splash() string {
	mascot := RenderMascot(m.width, m.opts.Color)
	ws := m.opts.Workspace
	if ws == "" {
		ws = "default"
	}
	info := strings.Join([]string{
		badge.Render("hev foh") + " " + dimStyle.Render(m.opts.Version),
		"",
		"cwd        " + m.opts.Cwd,
		"workspace  " + ws,
		"model      " + userStyle.Render(m.opts.Agent.Model()) + dimStyle.Render(" via "+m.opts.Agent.ProviderFor(m.opts.Agent.Model())),
		"",
		dimStyle.Render("Ctrl+M model picker · /prompt · /help"),
		dimStyle.Render("YOLO: tools run without approval prompts"),
	}, "\n")
	return lipgloss.JoinHorizontal(lipgloss.Top, mascot, "   ", info)
}

func (m *TUI) headerBar() string {
	state := "idle"
	if m.busy {
		state = "working"
	}
	model := m.opts.Agent.Model()
	return badge.Render("hev foh") + " " + userStyle.Render(model) + dimStyle.Render(" ["+m.opts.Agent.ProviderFor(model)+"]  "+state+" · "+m.opts.Cwd)
}

func (m *TUI) renderCard(c *Card, width int) string {
	title := lipgloss.NewStyle().Bold(true).Foreground(accent).Render("◆ " + c.Tool)
	if c.Action != "" {
		title += " " + lipgloss.NewStyle().Foreground(gold).Render(c.Action)
	}
	lines := []string{title}
	if c.Summary != "" {
		lines = append(lines, c.Summary)
	}
	if body := strings.TrimSpace(c.Body); body != "" {
		bl := strings.Split(body, "\n")
		limit := 6
		if m.expandCards {
			limit = 40
		}
		for i, l := range bl {
			if i == limit {
				lines = append(lines, dimStyle.Render(fmt.Sprintf("… %d more line(s) · Ctrl+O", len(bl)-limit)))
				break
			}
			lines = append(lines, dimStyle.Render(ansi.Truncate(l, max(10, width-6), "…")))
		}
	}
	return cardStyle.Width(max(20, width-2)).Render(strings.Join(lines, "\n"))
}

func (m *TUI) renderEntry(e entry, width int) string {
	switch e.kind {
	case entryUser:
		return userStyle.Render("you › ") + e.text
	case entryAssistant:
		return lipgloss.NewStyle().Width(width).Render(e.text)
	case entryTool:
		out := dimStyle.Render("· "+e.text) + "\n"
		if e.detail != "" {
			dl := strings.Split(strings.TrimRight(e.detail, "\n"), "\n")
			for i, l := range dl {
				if i == 4 {
					out += dimStyle.Render(fmt.Sprintf("    … %d more line(s)", len(dl)-4)) + "\n"
					break
				}
				out += dimStyle.Render("    "+ansi.Truncate(l, max(10, width-6), "…")) + "\n"
			}
		}
		return strings.TrimRight(out, "\n")
	case entryCard:
		return m.renderCard(e.card, width)
	case entrySystem:
		s := sysStyle.Render(e.text)
		if e.detail != "" {
			s += "\n" + dimStyle.Render(e.detail)
		}
		return s
	case entryError:
		return errStyle.Render("✗ " + e.text)
	}
	return ""
}

func (m *TUI) pickerView(height int) string {
	p := m.picker
	var b strings.Builder
	focus := "search"
	if p.focusList {
		focus = "list (f star · j/k move)"
	}
	src := string(m.catalogSource)
	if src == "" {
		src = string(SourceStarter)
	}
	b.WriteString(badge.Render("model picker") + dimStyle.Render(fmt.Sprintf("  %d models · %s · focus %s", len(p.filtered), src, focus)) + "\n")
	b.WriteString("search › " + p.query + "▏\n")
	rows := max(3, height-4)
	start := 0
	if p.cursor >= rows {
		start = p.cursor - rows + 1
	}
	for i := start; i < len(p.filtered) && i < start+rows; i++ {
		model := p.filtered[i]
		star := "  "
		if m.opts.Favorites.Has(model.ID) {
			star = lipgloss.NewStyle().Foreground(gold).Render("★ ")
		}
		cur := "  "
		line := fmt.Sprintf("%-44s %6s  %s", ansi.Truncate(model.ID, 44, "…"), model.ContextLabel(), model.Badge())
		if model.ID == m.opts.Agent.Model() {
			line += "  (active)"
		}
		if i == p.cursor {
			cur = lipgloss.NewStyle().Foreground(accent).Render("› ")
			line = lipgloss.NewStyle().Bold(true).Render(line)
		}
		b.WriteString(cur + star + line + "\n")
	}
	if p.note != "" {
		b.WriteString(sysStyle.Render(p.note) + "\n")
	}
	b.WriteString(dimStyle.Render("Enter select · Ctrl+F star · Tab focus list · Esc close"))
	return b.String()
}

// View renders the splash or header, transcript or picker, and input.
func (m *TUI) View() string {
	if m.quitting {
		return ""
	}
	width := max(20, m.width)
	var header string
	if len(m.entries) == 0 && m.picker == nil {
		header = m.splash()
	} else {
		header = m.headerBar()
	}
	footer := m.input.View()
	if m.busy {
		footer = dimStyle.Render("working… Enter steers · Ctrl+C cancels") + "\n" + footer
	}
	if status := m.peerStatus(width); status != "" {
		footer = status + "\n" + footer
	}
	room := max(3, m.height-lipgloss.Height(header)-lipgloss.Height(footer)-1)
	var body string
	if m.picker != nil {
		body = m.pickerView(room)
	} else {
		var parts []string
		for _, e := range m.entries {
			parts = append(parts, m.renderEntry(e, width))
		}
		lines := strings.Split(strings.Join(parts, "\n"), "\n")
		if len(lines) > room {
			lines = lines[len(lines)-room:]
		}
		body = strings.Join(lines, "\n")
	}
	return header + "\n" + body + "\n" + footer
}

// WorkspaceName reads the active hev workspace for the header.
func WorkspaceName() string {
	for _, k := range []string{"HEV_WORKSPACE"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}
