package foh

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// CatalogLoader returns the model catalog for the picker.
type CatalogLoader func(ctx context.Context) ([]Model, CatalogSource, error)

// TUIOptions configures the meta-harness TUI.
type TUIOptions struct {
	Agent        *Agent
	Catalog      CatalogLoader
	Favorites    *Favorites
	Composer     PromptComposer
	Version      string
	Cwd          string
	Workspace    string
	SummaryModel string
	Color        bool
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
	kind    entryKind
	text    string
	detail  string
	card    *Card
	callID  string
	running bool
}

// TUI is the Bubble Tea model for hev kit.
type TUI struct {
	opts   TUIOptions
	width  int
	height int
	input  textarea.Model

	entries     []entry
	busy        bool
	thinkVerb   string
	frame       int
	scroll      int
	cmdCursor   int
	cancel      context.CancelFunc
	events      chan Event
	expandCards bool

	chrome        ChromeSnapshot
	watched       []watchedRun
	watchedSteers []watchedSteer
	peerActivity  string
	peerBusy      bool
	peerLog       []peerExchange
	pendingAuto   []string
	callArgs      map[string]struct{ tool, action, args string }
	models        []Model
	catalogSource CatalogSource
	picker        *picker
	resumer       *resumePicker
	peers         *peersOverlay
	steerer       *steerOverlay
	quitting      bool
}

type resumePicker struct {
	query    string
	cursor   int
	loading  bool
	all      []ResumeSession
	filtered []ResumeSession
	note     string
}

type resumeListMsg struct {
	sessions []ResumeSession
}

type resumeLoadedMsg struct {
	session ResumeSession
	msgs    []Message
	err     error
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
type shimmerTickMsg struct{}
type catalogMsg struct {
	models []Model
	source CatalogSource
	err    error
}

// NewTUI builds the TUI model.
func NewTUI(opts TUIOptions) *TUI {
	in := textarea.New()
	in.Placeholder = "Ask hev kit…  (Shift+Enter newline · /help)"
	in.Prompt = "› "
	in.ShowLineNumbers = false
	in.CharLimit = 32 << 10
	in.SetHeight(1)
	in.SetWidth(96)
	// Keep the input clean and unboxed; horizontal rules frame it in View().
	in.FocusedStyle.CursorLine = lipgloss.NewStyle()
	in.BlurredStyle.CursorLine = lipgloss.NewStyle()
	in.FocusedStyle.Prompt = userStyle
	in.BlurredStyle.Prompt = dimStyle
	in.KeyMap.InsertNewline.SetEnabled(false)
	in.Focus()
	if opts.Favorites == nil {
		opts.Favorites = LoadFavorites("")
	}
	return &TUI{
		opts:     opts,
		input:    in,
		width:    100,
		height:   40,
		callArgs: map[string]struct{ tool, action, args string }{},
	}
}

// Init loads the catalog and cluster chrome in the background.
func (m *TUI) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.loadCatalog(), m.fetchChrome(), m.scheduleChrome())
}

func (m *TUI) fetchChrome() tea.Cmd {
	return func() tea.Msg { return chromeMsg{snap: loadChrome()} }
}

func (m *TUI) scheduleChrome() tea.Cmd {
	return tea.Tick(chromeTick(), func(time.Time) tea.Msg { return chromeTickMsg{} })
}

func (m *TUI) syncInputHeight() {
	lines := strings.Split(m.input.Value(), "\n")
	w := max(10, m.input.Width())
	visual := 0
	for _, l := range lines {
		n := len([]rune(l))
		if n == 0 {
			visual++
			continue
		}
		visual += (n + w - 1) / w
	}
	h := max(1, min(8, visual))
	m.input.SetHeight(h)
}

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

func shimmerTick() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg { return shimmerTickMsg{} })
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
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.input.SetWidth(max(10, msg.Width-4))
		m.syncInputHeight()
		return m, nil
	case chromeMsg:
		m.chrome = msg.snap
		return m, nil
	case chromeTickMsg:
		return m, tea.Batch(m.fetchChrome(), m.scheduleChrome())
	case watchTickMsg:
		if len(m.watched) == 0 {
			return m, nil
		}
		return m, m.pollWatched()
	case watchPollMsg:
		var cmds []tea.Cmd
		if len(msg.results) > 0 {
			doneSet := map[string]bool{}
			for _, wr := range msg.results {
				doneSet[wr.Run.Label()] = true
				summary := fmt.Sprintf("%s · %s", wr.Run.Label(), wr.State)
				if wr.PR != "" {
					summary += " · " + wr.PR
				}
				m.entries = append(m.entries, entry{
					kind: entryCard,
					card: &Card{Tool: "hev_dispatch", Action: "done", Summary: summary, Body: wr.Report, OK: wr.State == "done"},
				})
				m.pendingAuto = append(m.pendingAuto, formatWatchPrompt(wr))
			}
			kept := m.watched[:0]
			for _, w := range m.watched {
				if !doneSet[w.Label()] {
					kept = append(kept, w)
				}
			}
			m.watched = kept
			if !m.busy && len(m.pendingAuto) > 0 {
				next := m.pendingAuto[0]
				m.pendingAuto = m.pendingAuto[1:]
				cmds = append(cmds, m.startAutoTurn(next))
			}
		}
		if len(m.watched) > 0 {
			cmds = append(cmds, m.scheduleWatch())
		}
		return m, tea.Batch(cmds...)
	case catalogMsg:
		m.models, m.catalogSource = msg.models, msg.source
		if m.picker != nil {
			m.refilter()
		}
		return m, nil
	case resumeListMsg:
		if m.resumer != nil {
			m.resumer.loading = false
			m.resumer.all = msg.sessions
			m.refilterResume()
		}
		return m, nil
	case peersListMsg:
		if m.peers != nil {
			m.peers.loading = false
			if msg.err != nil {
				m.peers.note = msg.err.Error()
			} else {
				m.peers.all = msg.rows
				m.peers.note = ""
				m.refilterPeers()
			}
		}
		return m, nil
	case peerSendMsg:
		if msg.err != nil {
			m.peerActivity = fmt.Sprintf("⇄ ✗ %s: %s", msg.target, truncateLine(msg.err.Error(), 48))
		} else {
			m.recordPeer("→", msg.target, msg.body)
		}
		return m, nil
	case peerBgDoneMsg:
		m.peerBusy = false
		if msg.err != nil {
			m.peerActivity = fmt.Sprintf("⇄ ✗ %s: %s", msg.sender, truncateLine(msg.err.Error(), 48))
		} else if msg.replyTo != "" {
			m.recordPeer("→", msg.replyTo, msg.replyBody)
		}
		return m, nil
	case steerListMsg:
		if m.steerer != nil {
			m.steerer.loading = false
			if msg.err != nil {
				m.steerer.note = msg.err.Error()
			} else {
				m.steerer.all = msg.rows
				m.steerer.note = ""
				m.refilterSteer()
			}
		}
		return m, nil
	case steerSendMsg:
		if msg.err != nil {
			m.peerActivity = fmt.Sprintf("↪ ✗ steer %s: %s", msg.target, truncateLine(msg.err.Error(), 60))
			m.entries = append(m.entries, entry{kind: entryError, text: fmt.Sprintf("steer %s failed: %s", msg.target, msg.err.Error())})
			return m, nil
		}
		tag := "→"
		if msg.level != "" && msg.level != "note" {
			tag = "[" + msg.level + "] →"
		}
		suffix := "queued"
		if msg.id != "" {
			suffix = "queued (" + msg.id + ")"
		}
		m.recordPeer(tag, msg.target, msg.body+" · "+suffix)
		if msg.id != "" {
			return m, m.trackSteer(watchedSteer{Target: msg.target, ID: msg.id, Level: msg.level, Body: msg.body, State: "queued"})
		}
		return m, nil
	case steerPollTickMsg:
		if len(m.watchedSteers) == 0 {
			return m, nil
		}
		return m, m.pollWatchedSteers()
	case steerPollMsg:
		kept := m.watchedSteers[:0]
		for _, ws := range m.watchedSteers {
			st := msg.updates[ws.ID]
			if st == "acked" {
				m.recordPeer("✓ acked", ws.Target, ws.Body+" ("+ws.ID+")")
				continue
			}
			if st != "" && st != ws.State {
				ws.State = st
				m.recordPeer("↪ "+st, ws.Target, ws.Body+" ("+ws.ID+")")
			}
			kept = append(kept, ws)
		}
		m.watchedSteers = kept
		if len(m.watchedSteers) > 0 {
			return m, m.scheduleSteerPoll()
		}
		return m, nil
	case cardSummaryMsg:
		for i := len(m.entries) - 1; i >= 0; i-- {
			if m.entries[i].callID == msg.callID && m.entries[i].kind == entryCard && m.entries[i].card != nil {
				m.entries[i].card.Summary = msg.summary
				break
			}
		}
		return m, nil
	case resumeLoadedMsg:
		if msg.err != nil {
			m.entries = append(m.entries, entry{kind: entryError, text: "resume failed: " + msg.err.Error()})
			return m, nil
		}
		m.opts.Agent.LoadHistory(msg.msgs)
		m.entries = nil
		m.scroll = 0
		m.system(fmt.Sprintf("↺ resumed [%s] %s · %d messages (%s)", msg.session.Harness, truncateLine(msg.session.ID, 16), len(msg.msgs), msg.session.ShortDir()))
		start := 0
		if len(msg.msgs) > 20 {
			start = len(msg.msgs) - 20
			m.system(fmt.Sprintf("… showing last 20 of %d messages (full history loaded into context)", len(msg.msgs)))
		}
		for _, msgItem := range msg.msgs[start:] {
			if msgItem.Role == "user" {
				m.entries = append(m.entries, entry{kind: entryUser, text: msgItem.Content})
			} else {
				m.entries = append(m.entries, entry{kind: entryAssistant, text: msgItem.Content})
			}
		}
		return m, nil
	case shimmerTickMsg:
		if !m.busy && !m.peerBusy {
			return m, nil
		}
		m.frame++
		// Rotate the alpine verb every ~10s (125 ticks @ 80ms) on long turns.
		if m.busy && m.frame%125 == 0 {
			m.thinkVerb = pickAlpineVerb()
		}
		return m, shimmerTick()
	case agentEventMsg:
		var cmds []tea.Cmd
		if wcmd := m.applyEvent(msg.ev); wcmd != nil {
			cmds = append(cmds, wcmd)
		}
		cmds = append(cmds, waitEvent(m.events))
		return m, tea.Batch(cmds...)
	case agentDoneMsg:
		m.busy, m.events = false, nil
		if m.cancel != nil {
			m.cancel()
			m.cancel = nil
		}
		if len(m.pendingAuto) > 0 {
			next := m.pendingAuto[0]
			m.pendingAuto = m.pendingAuto[1:]
			return m, m.startAutoTurn(next)
		}
		return m, nil
	case tea.MouseMsg:
		if m.picker != nil {
			return m, nil
		}
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.scroll += 3
		case tea.MouseButtonWheelDown:
			m.scroll = max(0, m.scroll-3)
		}
		return m, nil
	case tea.KeyMsg:
		if m.picker != nil {
			return m.updatePicker(msg)
		}
		if m.resumer != nil {
			return m.updateResumePicker(msg)
		}
		if m.peers != nil {
			return m.updatePeersOverlay(msg)
		}
		if m.steerer != nil {
			return m.updateSteerOverlay(msg)
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
	case msg.Type == tea.KeyPgUp:
		m.scroll += max(3, m.height/2)
		return m, nil
	case msg.Type == tea.KeyPgDown:
		m.scroll = max(0, m.scroll-max(3, m.height/2))
		return m, nil
	case msg.Type == tea.KeyEnd && m.scroll > 0:
		m.scroll = 0
		return m, nil
	case msg.Type == tea.KeyUp && len(m.matchingCommands()) > 0:
		matches := m.matchingCommands()
		m.cmdCursor = (m.cmdCursor - 1 + len(matches)) % len(matches)
		return m, nil
	case msg.Type == tea.KeyDown && len(m.matchingCommands()) > 0:
		matches := m.matchingCommands()
		m.cmdCursor = (m.cmdCursor + 1) % len(matches)
		return m, nil
	case msg.Type == tea.KeyTab && len(m.matchingCommands()) > 0:
		matches := m.matchingCommands()
		sel := matches[min(m.cmdCursor, len(matches)-1)]
		fill := sel.Name
		if sel.TakesArg {
			fill += " "
		}
		m.input.SetValue(fill)
		m.input.CursorEnd()
		m.cmdCursor = 0
		m.syncInputHeight()
		return m, nil
	case msg.Alt && msg.String() == "alt+m":
		return m, m.openPicker()
	case msg.String() == "shift+enter" || msg.String() == "alt+enter" || (msg.Alt && msg.Type == tea.KeyEnter) || msg.Type == tea.KeyCtrlJ:
		m.input.InsertString("\n")
		m.syncInputHeight()
		return m, nil
	case msg.Type == tea.KeyEnter: // also Ctrl+M: terminals send the same byte
		text := strings.TrimSpace(m.input.Value())
		if text == "" {
			return m, m.openPicker()
		}
		// Resolve slash-command prefix (e.g. "/dis" -> "/dispatch") when selecting from typeahead.
		if matches := m.matchingCommands(); len(matches) > 0 {
			sel := matches[min(m.cmdCursor, len(matches)-1)]
			text = sel.Name
		}
		m.input.Reset()
		m.cmdCursor = 0
		m.scroll = 0
		m.syncInputHeight()
		if strings.HasPrefix(text, "/") || strings.HasPrefix(text, ":") {
			return m, m.command(text)
		}
		// Incoming peer messages pasted by `peers send`: always show in the
		// lower status bar; background (agent-only) by default, or interrupt
		// the main user thread when tagged [interrupt] / [urgent].
		if sender, body, interrupt, ok := parsePeerMessage(text); ok {
			m.recordPeer("←", sender, body)
			if interrupt {
				m.system(fmt.Sprintf("⇄ [interrupt] from %s: %s", sender, body))
				if m.busy {
					m.pendingAuto = append(m.pendingAuto, text)
					return m, nil
				}
				return m, m.startAutoTurn(text)
			}
			return m, m.startBackgroundPeerTurn(sender, text)
		}
		if m.busy {
			if m.opts.Agent.Steer(text) {
				m.entries = append(m.entries, entry{kind: entrySystem, text: "↪ steer queued: lands at the next tool boundary", detail: text})
			} else {
				m.system("steer queue full; wait for the turn to finish")
			}
			return m, nil
		}
		return m, m.startTurn(text)
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.cmdCursor = 0
	m.syncInputHeight()
	return m, cmd
}

func (m *TUI) system(text string) {
	m.entries = append(m.entries, entry{kind: entrySystem, text: text})
}

// parsePeerMessage detects `[peer message from SENDER ...]\nBODY` pasted by
// `peers send` and extracts the sender, first body line, and whether it is
// tagged to interrupt the user's main thread ([interrupt] or [urgent]).
func parsePeerMessage(text string) (sender, body string, interrupt, ok bool) {
	if !strings.HasPrefix(text, "[peer message from ") {
		return "", "", false, false
	}
	head, rest, found := strings.Cut(text, "\n")
	inner := strings.TrimPrefix(head, "[peer message from ")
	if idx := strings.IndexAny(inner, " (]"); idx > 0 {
		sender = inner[:idx]
	} else {
		sender = strings.TrimRight(inner, "]")
	}
	lowRest := strings.ToLower(strings.TrimSpace(rest))
	if strings.HasPrefix(lowRest, "[interrupt]") || strings.HasPrefix(lowRest, "[urgent]") {
		interrupt = true
	}
	if found {
		body = firstLine(rest, 80)
	}
	if body == "" {
		body = "message"
	}
	return sender, body, interrupt, sender != ""
}

type slashCmd struct {
	Name     string
	Summary  string
	TakesArg bool
}

var slashCommands = []slashCmd{
	{Name: "/dispatch", Summary: "plan & send a dispatch order (watched automatically)", TakesArg: true},
	{Name: "/steer", Summary: "steer a BOH run or orchestrator (--urgent|--halt|--redirect)", TakesArg: true},
	{Name: "/peers", Summary: "list live peers, view exchanges, or /peers TARGET MSG", TakesArg: true},
	{Name: "/resume", Summary: "resume a local session (hev, claude, codex, agy, pi)", TakesArg: true},
	{Name: "/model", Summary: "pick or switch model", TakesArg: true},
	{Name: "/prompt", Summary: "show the composed system prompt"},
	{Name: "/reload", Summary: "recompose the system prompt"},
	{Name: "/clear", Summary: "start a new thread"},
	{Name: "/help", Summary: "show shortcuts and commands"},
	{Name: "/quit", Summary: "exit hev kit"},
}

// matchingCommands returns command suggestions when the input is typing a
// slash or vim-style colon command (starts with "/" or ":" and has no whitespace yet).
func (m *TUI) matchingCommands() []slashCmd {
	raw := m.input.Value()
	if (!strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, ":")) || strings.ContainsAny(raw, " \t\n") {
		return nil
	}
	prefixChar := raw[:1]
	q := strings.ToLower(strings.TrimPrefix(raw, prefixChar))
	if q == "" {
		return slashCommands
	}
	// Vim quit shortcuts (:q, :q!, :wq) resolve directly to /quit.
	if prefixChar == ":" && (q == "q" || q == "q!" || q == "wq" || q == "qa" || q == "qa!") {
		for _, sc := range slashCommands {
			if sc.Name == "/quit" {
				return []slashCmd{sc}
			}
		}
	}
	var prefix, fuzzy []slashCmd
	for _, sc := range slashCommands {
		bare := strings.TrimPrefix(sc.Name, "/")
		if strings.HasPrefix(bare, q) {
			prefix = append(prefix, sc)
		} else if fuzzyScore(q, bare) >= 0 {
			fuzzy = append(fuzzy, sc)
		}
	}
	return append(prefix, fuzzy...)
}

func (m *TUI) renderCommandSuggestions(width int) string {
	matches := m.matchingCommands()
	if len(matches) == 0 {
		return ""
	}
	cur := min(m.cmdCursor, len(matches)-1)
	var lines []string
	for i, sc := range matches {
		name := fmt.Sprintf("%-12s", sc.Name)
		desc := sc.Summary
		if i == cur {
			if m.opts.Color {
				lines = append(lines, hevBadgeStyle.Render("› ")+hevCmdStyle.Render(name)+" "+chromeName.Render(desc))
			} else {
				lines = append(lines, "› "+name+" "+desc)
			}
		} else {
			if m.opts.Color {
				lines = append(lines, "  "+chromeName.Render(name)+" "+dimStyle.Render(desc))
			} else {
				lines = append(lines, "  "+name+" "+desc)
			}
		}
	}
	return lipgloss.NewStyle().Padding(0, 1).Render(strings.Join(lines, "\n"))
}

const helpText = `Enter send (steers while a turn runs) · Shift+Enter newline · Ctrl+M model picker · PgUp/PgDn or wheel scroll
/dispatch [task]          plan & send a dispatch order (watched automatically)
/peers [target message]   list live peers & exchanges, or message a peer directly
/resume [query]           resume a local session from hev, claude, codex, agy or pi
/model [slug]             pick or switch model       /prompt  show composed system prompt
/reload                   recompose the prompt       /clear   new thread       /help  this help
/quit                     exit                       Ctrl+O   expand hev cards  Ctrl+C cancel / quit`

func (m *TUI) command(text string) tea.Cmd {
	name, arg, _ := strings.Cut(text, " ")
	arg = strings.TrimSpace(arg)
	if strings.HasPrefix(name, ":") {
		name = "/" + strings.TrimPrefix(name, ":")
	}
	switch name {
	case "/steer":
		if arg == "" {
			return m.openSteerOverlay("")
		}
		level := "note"
		for _, flag := range []struct{ pref, lvl string }{
			{"--urgent ", "urgent"}, {"--halt ", "halt"}, {"--redirect ", "redirect"}, {"--note ", "note"},
		} {
			if strings.HasPrefix(arg, flag.pref) {
				level = flag.lvl
				arg = strings.TrimSpace(strings.TrimPrefix(arg, flag.pref))
				break
			}
		}
		if target, body, hasBody := strings.Cut(arg, " "); hasBody && strings.TrimSpace(body) != "" && looksLikeSteerTarget(strings.TrimSpace(target), m.watched) {
			return m.sendSteerDirect(strings.TrimSpace(target), level, strings.TrimSpace(body))
		}
		if !strings.Contains(arg, " ") && !looksLikeSteerTarget(arg, m.watched) {
			return m.openSteerOverlay(arg)
		}
		// Natural-language steer request: let the agent resolve the active BOH target and call hev_steer.
		prompt := fmt.Sprintf("Steer request (level=%s): %s\n\nFind the matching active run or orchestrator via hev_dispatch ls, send the steer via hev_steer send (report the receipt ID and delivery mode), and verify receipt.", level, arg)
		return m.startTurn(prompt)
	case "/peers":
		interrupt := false
		if strings.HasPrefix(arg, "--interrupt ") {
			interrupt = true
			arg = strings.TrimSpace(strings.TrimPrefix(arg, "--interrupt "))
		}
		if target, body, hasBody := strings.Cut(arg, " "); hasBody && strings.TrimSpace(body) != "" {
			msg := strings.TrimSpace(body)
			if interrupt {
				msg = "[interrupt] " + msg
			}
			return m.sendPeerDirect(strings.TrimSpace(target), msg)
		}
		return m.openPeersOverlay(arg)
	case "/resume":
		if m.busy {
			m.system("wait for the current turn to finish before /resume")
			return nil
		}
		return m.openResumePicker(arg)
	case "/dispatch":
		if m.busy {
			m.system("wait for the current turn to finish before /dispatch")
			return nil
		}
		if arg == "" {
			return m.startTurn("Show active and recent dispatched runs using hev_dispatch ls, and ask what I'd like to dispatch.")
		}
		prompt := "Dispatch plan request: " + arg + "\n\n" +
			"If this is a simple smoke/test order, compose a concrete order (with ## Acceptance) and dispatch it now via hev_dispatch send. " +
			"If this is an involved task, draft the full order plan (host, repo, base, harness, ## Acceptance) and confirm with me before sending."
		return m.startTurn(prompt)
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
	case "/quit", "/exit", "/q", "/q!", "/wq", "/qa", "/qa!":
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
	return m.runAgentTurn(text)
}

func (m *TUI) startAutoTurn(text string) tea.Cmd {
	return m.runAgentTurn(text)
}

func (m *TUI) runAgentTurn(text string) tea.Cmd {
	m.busy = true
	m.thinkVerb = pickAlpineVerb()
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
	return tea.Batch(waitEvent(ch), shimmerTick())
}

func (m *TUI) scheduleWatch() tea.Cmd {
	return tea.Tick(watchInterval(), func(time.Time) tea.Msg { return watchTickMsg{} })
}

func (m *TUI) pollWatched() tea.Cmd {
	runs := append([]watchedRun(nil), m.watched...)
	var execFn HevRunner
	if m.opts.Agent != nil && m.opts.Agent.Tools != nil {
		execFn = m.opts.Agent.Tools.Hev
	}
	return func() tea.Msg {
		return watchPollMsg{results: pollWatchedRuns(runs, execFn)}
	}
}

func (m *TUI) trackWatched(run watchedRun) tea.Cmd {
	for _, w := range m.watched {
		if w.Label() == run.Label() {
			return nil
		}
	}
	first := len(m.watched) == 0
	m.watched = append(m.watched, run)
	m.system("◉ dispatched to " + run.Label() + " — watching for completion")
	if first {
		return m.scheduleWatch()
	}
	return nil
}

func (m *TUI) clearWatched(name, host string) {
	kept := m.watched[:0]
	for _, w := range m.watched {
		if w.Name == name && (host == "" || w.Host == "" || w.Host == host) {
			continue
		}
		kept = append(kept, w)
	}
	m.watched = kept
}

func (m *TUI) last(kind entryKind) *entry {
	if n := len(m.entries); n > 0 && m.entries[n-1].kind == kind {
		return &m.entries[n-1]
	}
	return nil
}

func (m *TUI) applyEvent(ev Event) tea.Cmd {
	switch ev.Kind {
	case EventText:
		if e := m.last(entryAssistant); e != nil {
			e.text += ev.Text
		} else {
			m.entries = append(m.entries, entry{kind: entryAssistant, text: ev.Text})
		}
	case EventToolStart:
		action := argField(ev.Call.Function.Arguments, "action")
		if m.callArgs != nil {
			m.callArgs[ev.Call.ID] = struct{ tool, action, args string }{ev.Call.Function.Name, action, ev.Call.Function.Arguments}
		}
		if IsHevTool(ev.Call.Function.Name) {
			m.entries = append(m.entries, entry{kind: entryCard, callID: ev.Call.ID, running: true,
				card: &Card{Tool: ev.Call.Function.Name, Action: action, Summary: "running…"}})
		} else if ev.Call.Function.Name == "bash" {
			if htool, hact, ok := detectBashHevCommand(ev.Call.Function.Arguments); ok {
				if m.callArgs != nil {
					m.callArgs[ev.Call.ID] = struct{ tool, action, args string }{htool, hact, ev.Call.Function.Arguments}
				}
				m.entries = append(m.entries, entry{kind: entryCard, callID: ev.Call.ID, running: true,
					card: &Card{Tool: htool, Action: hact, Summary: "running…"}})
			} else {
				m.entries = append(m.entries, entry{kind: entryTool, callID: ev.Call.ID, running: true, text: ev.Call.Function.Name + " " + argsSummary(ev.Call.Function.Arguments)})
			}
		} else {
			m.entries = append(m.entries, entry{kind: entryTool, callID: ev.Call.ID, running: true, text: ev.Call.Function.Name + " " + argsSummary(ev.Call.Function.Arguments)})
		}
	case EventToolResult:
		for i := len(m.entries) - 1; i >= 0; i-- {
			e := &m.entries[i]
			if e.callID != ev.Call.ID {
				continue
			}
			e.running = false
			if e.kind == entryCard && ev.Result.Card != nil {
				e.card = ev.Result.Card
			} else if e.kind == entryCard && e.card != nil {
				// Promoted `bash hev ...` card.
				e.card.OK = !ev.Result.IsError
				e.card.Body = strings.TrimSpace(ev.Result.Content)
				if ev.Result.IsError {
					e.card.Summary = "failed: " + firstLine(ev.Result.Content, 80)
				} else {
					e.card.Summary = summarize(e.card.Tool, e.card.Action, ev.Result.Content)
				}
			} else {
				e.detail = ev.Result.Content
				if ev.Result.IsError {
					e.text += "  ✗"
				}
			}
			break
		}
		if info, ok := m.callArgs[ev.Call.ID]; ok {
			delete(m.callArgs, ev.Call.ID)
			var cmds []tea.Cmd
			if info.tool == "hev_dispatch" && info.action == "send" && !ev.Result.IsError {
				if run, ok := extractDispatchedRun(info.args, ev.Result.Content); ok {
					if c := m.trackWatched(run); c != nil {
						cmds = append(cmds, c)
					}
				}
			} else if info.tool == "hev_dispatch" && info.action == "show" && !ev.Result.IsError {
				var a hevArgs
				if json.Unmarshal([]byte(info.args), &a) == nil && a.Name != "" {
					if _, done := parseShowResponse(watchedRun{Name: a.Name, Host: a.Host}, []byte(ev.Result.Content)); done {
						m.clearWatched(a.Name, a.Host)
					}
				}
			} else if info.tool == "hev_peers" && info.action == "send" && !ev.Result.IsError {
				var a hevArgs
				if json.Unmarshal([]byte(info.args), &a) == nil && a.Target != "" {
					m.recordPeer("→", a.Target, a.Message)
					// Keep peer exchanges out of the main scroll trace; surface them in the status bar.
					for i := len(m.entries) - 1; i >= 0; i-- {
						if m.entries[i].callID == ev.Call.ID {
							m.entries = append(m.entries[:i], m.entries[i+1:]...)
							break
						}
					}
					return nil
				}
			} else if info.tool == "hev_steer" && info.action == "send" && !ev.Result.IsError {
				var a hevArgs
				if json.Unmarshal([]byte(info.args), &a) == nil && a.Target != "" {
					tag := "→"
					if a.Level != "" && a.Level != "note" {
						tag = "[" + a.Level + "] →"
					}
					id := extractSteerID(ev.Result.Content)
					suffix := "queued"
					if id != "" {
						suffix = "queued (" + id + ")"
					}
					m.recordPeer(tag, a.Target, a.Message+" · "+suffix)
					for i := len(m.entries) - 1; i >= 0; i-- {
						if m.entries[i].callID == ev.Call.ID {
							m.entries = append(m.entries[:i], m.entries[i+1:]...)
							break
						}
					}
					if id != "" {
						return m.trackSteer(watchedSteer{Target: a.Target, ID: id, Level: a.Level, Body: a.Message, State: "queued"})
					}
					return nil
				}
			}
			// Kick off an async small-model summary for kept hev cards (including promoted `bash hev ...` cards).
			if (IsHevTool(info.tool) || strings.HasPrefix(info.tool, "hev ")) && !ev.Result.IsError {
				if c := m.summarizeCardAsync(ev.Call.ID, info.tool, info.action, ev.Result.Content); c != nil {
					cmds = append(cmds, c)
				}
			}
			if len(cmds) > 0 {
				return tea.Batch(cmds...)
			}
		}
	case EventSteer:
		m.system("↪ steer injected: " + ev.Text)
	case EventError:
		m.entries = append(m.entries, entry{kind: entryError, text: ev.Err.Error()})
	}
	return nil
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

// --- resume picker ---

func (m *TUI) openResumePicker(initialQuery string) tea.Cmd {
	m.resumer = &resumePicker{query: initialQuery, loading: true}
	return func() tea.Msg {
		return resumeListMsg{sessions: ListLocalSessions("", nil, 30)}
	}
}

func (m *TUI) refilterResume() {
	r := m.resumer
	if r == nil {
		return
	}
	r.filtered = r.filtered[:0]
	q := strings.TrimSpace(r.query)
	for _, s := range r.all {
		if q == "" ||
			fuzzyScore(q, s.Title) >= 0 ||
			fuzzyScore(q, s.Harness) >= 0 ||
			fuzzyScore(q, s.ShortDir()) >= 0 ||
			fuzzyScore(q, s.ID) >= 0 {
			r.filtered = append(r.filtered, s)
		}
	}
	if r.cursor >= len(r.filtered) {
		r.cursor = max(0, len(r.filtered)-1)
	}
}

func (m *TUI) updateResumePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	r := m.resumer
	switch msg.Type {
	case tea.KeyEsc, tea.KeyCtrlC:
		m.resumer = nil
		return m, nil
	case tea.KeyEnter:
		if len(r.filtered) > 0 {
			sel := r.filtered[r.cursor]
			m.resumer = nil
			return m, func() tea.Msg {
				msgs, err := LoadSessionMessages(sel)
				return resumeLoadedMsg{session: sel, msgs: msgs, err: err}
			}
		}
		m.resumer = nil
		return m, nil
	case tea.KeyUp, tea.KeyCtrlP:
		r.cursor = max(0, r.cursor-1)
	case tea.KeyDown, tea.KeyCtrlN:
		r.cursor = min(max(0, len(r.filtered)-1), r.cursor+1)
	case tea.KeyBackspace:
		if r.query != "" {
			runes := []rune(r.query)
			r.query = string(runes[:len(runes)-1])
			r.cursor = 0
			m.refilterResume()
		}
	case tea.KeyRunes, tea.KeySpace:
		s := string(msg.Runes)
		if msg.Type == tea.KeySpace {
			s = " "
		}
		r.query += s
		r.cursor = 0
		m.refilterResume()
	}
	return m, nil
}

func (m *TUI) resumePickerView(width, height int) string {
	r := m.resumer
	var b strings.Builder
	status := fmt.Sprintf("  %d local sessions on this host", len(r.filtered))
	if r.loading {
		status = "  scanning local harness traces…"
	}
	b.WriteString(badge.Render("resume session") + dimStyle.Render(status) + "\n")
	b.WriteString("filter › " + r.query + "▏\n")
	rows := max(3, height-4)
	start := 0
	if r.cursor >= rows {
		start = r.cursor - rows + 1
	}
	now := time.Now()
	titleMax := max(16, width-32)
	for i := start; i < len(r.filtered) && i < start+rows; i++ {
		s := r.filtered[i]
		cur := "  "
		hBadge := fmt.Sprintf("%-8s", "["+s.Harness+"]")
		dir := ansi.Truncate(s.ShortDir(), 10, "…")
		age := fmt.Sprintf("%3s", s.AgeLabel(now))
		title := ansi.Truncate(s.Title, titleMax, "…")
		line := fmt.Sprintf("%s %s %-10s %s", hBadge, age, dir, title)
		if i == r.cursor {
			cur = lipgloss.NewStyle().Foreground(accent).Render("› ")
			line = lipgloss.NewStyle().Bold(true).Render(line)
		}
		b.WriteString(cur + line + "\n")
	}
	if !r.loading && len(r.filtered) == 0 {
		b.WriteString(dimStyle.Render("  no matching local sessions") + "\n")
	}
	b.WriteString(dimStyle.Render("Enter resume · ↑/↓ move · type to filter by harness/dir/prompt · Esc close"))
	return b.String()
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
	infoWidth := max(24, m.width-lipgloss.Width(mascot)-5)
	info := lipgloss.NewStyle().Width(infoWidth).Render(strings.Join([]string{
		badge.Render("hev kit") + " " + dimStyle.Render(m.opts.Version),
		"",
		"cwd        " + m.opts.Cwd,
		"workspace  " + ws,
		"model      " + userStyle.Render(m.opts.Agent.Model()) + dimStyle.Render(" via "+m.opts.Agent.ProviderFor(m.opts.Agent.Model())),
		"",
		dimStyle.Render("Ctrl+M model picker · /prompt · /help"),
		dimStyle.Render("hev kit runs without permissions. It's your responsibility to secure your compute."),
	}, "\n"))
	return lipgloss.JoinHorizontal(lipgloss.Top, mascot, "   ", info)
}

func (m *TUI) headerBar() string {
	state := "idle"
	if m.busy {
		state = "working"
	}
	model := m.opts.Agent.Model()
	return badge.Render("hev kit") + " " + userStyle.Render(model) + dimStyle.Render(" ["+m.opts.Agent.ProviderFor(model)+"]  "+state+" · "+m.opts.Cwd)
}

func (m *TUI) renderCard(c *Card, width int, running bool) string {
	icon := hevToolEmoji(c.Tool, m.opts.Color)
	title := hevBadgeStyle.Render(icon + " " + strings.ReplaceAll(c.Tool, "_", " "))
	if c.Action != "" {
		title += " " + hevCmdStyle.Render(c.Action)
	}
	lines := []string{title}
	if running {
		lines = append(lines, renderShimmer("running…", m.frame, m.opts.Color))
	} else if c.Summary != "" {
		lines = append(lines, c.Summary)
	}
	if body := strings.TrimSpace(c.Body); body != "" {
		bl := strings.Split(body, "\n")
		// Keep completed cards compact (summary + Ctrl+O hint); show full body on Ctrl+O or error.
		if !m.expandCards && c.OK && len(bl) > 3 {
			lines = append(lines, dimStyle.Render(fmt.Sprintf("… %d line(s) · Ctrl+O to expand", len(bl))))
		} else {
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
	}
	return cardStyle.Width(max(20, width-2)).Render(strings.Join(lines, "\n"))
}

func (m *TUI) renderEntry(e entry, width int) string {
	switch e.kind {
	case entryUser:
		return userStyle.Render("you › ") + e.text
	case entryAssistant:
		return renderMarkdown(e.text, width, m.opts.Color)
	case entryTool:
		if e.running {
			return renderShimmer(e.text, m.frame, m.opts.Color)
		}
		line := e.text
		if m.opts.Color {
			line = styleHevCommands(line)
		}
		out := dimStyle.Render("· ") + line + "\n"
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
		return m.renderCard(e.card, width, e.running)
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

func (m *TUI) horizontalRule(width int) string {
	line := strings.Repeat("─", max(10, width))
	if !m.opts.Color {
		return line
	}
	return ruleStyle.Render(line)
}

func (m *TUI) activeVerb() string {
	if m.thinkVerb != "" {
		return m.thinkVerb
	}
	return alpineVerbs[0]
}

func (m *TUI) statusChromeBar(width int) string {
	inner := max(20, width-2)
	model := m.opts.Agent.Model()
	prov := m.opts.Agent.ProviderFor(model)
	right := userStyle.Render(model) + dimStyle.Render(" · "+prov)
	if !m.opts.Color {
		right = model + " · " + prov
	}
	var left string
	if m.busy {
		left = renderShimmer(m.activeVerb(), m.frame, m.opts.Color) + dimStyle.Render("  Enter steers · Ctrl+C cancels")
	} else {
		left = renderChrome(m.chrome, m.opts.Color)
		if len(m.watched) > 0 {
			wb := fmt.Sprintf("◉ watching %d", len(m.watched))
			if m.opts.Color {
				wb = userStyle.Render(wb)
			}
			if left != "" {
				left = wb + "  " + left
			} else {
				left = wb
			}
		}
		if left == "" {
			left = dimStyle.Render("/dispatch · Ctrl+M model · Shift+Enter newline · /help")
		}
	}
	lw := lipgloss.Width(left)
	rw := lipgloss.Width(right)
	gap := inner - lw - rw
	if gap < 2 {
		return " " + ansi.Truncate(left+"  "+right, inner, "…")
	}
	return " " + left + strings.Repeat(" ", gap) + right
}

// View renders the transcript (with splash art in the scroll pane) or picker,
// framed input area with horizontal rules, and bottom cluster chrome bar.
func (m *TUI) View() string {
	if m.quitting {
		return ""
	}
	width := max(20, m.width)
	innerWidth := max(16, width-2)
	rule := m.horizontalRule(width)
	inputBlock := lipgloss.NewStyle().Padding(0, 1).Render(m.input.View())
	chromeBar := m.statusChromeBar(width)
	if m.peerActivity != "" {
		txt := ansi.Truncate(m.peerActivity, innerWidth, "…")
		var pl string
		if m.peerBusy {
			pl = " " + renderShimmer(txt, m.frame, m.opts.Color)
		} else if m.opts.Color {
			pl = " " + sysStyle.Render(txt)
		} else {
			pl = " " + txt
		}
		chromeBar = chromeBar + "\n" + pl
	}
	suggestions := m.renderCommandSuggestions(innerWidth)
	footer := rule + "\n" + inputBlock + "\n" + rule + "\n" + chromeBar
	if suggestions != "" {
		footer = suggestions + "\n" + footer
	}

	if m.picker != nil {
		header := " " + m.headerBar()
		room := max(3, m.height-lipgloss.Height(header)-lipgloss.Height(footer)-2)
		body := lipgloss.NewStyle().Padding(0, 1).Render(m.pickerView(room))
		return header + "\n\n" + body + "\n" + footer
	}
	if m.resumer != nil {
		header := " " + m.headerBar()
		room := max(3, m.height-lipgloss.Height(header)-lipgloss.Height(footer)-2)
		body := lipgloss.NewStyle().Padding(0, 1).Render(m.resumePickerView(innerWidth, room))
		return header + "\n\n" + body + "\n" + footer
	}
	if m.peers != nil {
		header := " " + m.headerBar()
		room := max(3, m.height-lipgloss.Height(header)-lipgloss.Height(footer)-2)
		body := lipgloss.NewStyle().Padding(0, 1).Render(m.peersOverlayView(innerWidth, room))
		return header + "\n\n" + body + "\n" + footer
	}
	if m.steerer != nil {
		header := " " + m.headerBar()
		room := max(3, m.height-lipgloss.Height(header)-lipgloss.Height(footer)-2)
		body := lipgloss.NewStyle().Padding(0, 1).Render(m.steerOverlayView(innerWidth, room))
		return header + "\n\n" + body + "\n" + footer
	}
	parts := []string{m.splash()}
	hasRunningTool := false
	for _, e := range m.entries {
		if e.running {
			hasRunningTool = true
		}
		parts = append(parts, m.renderEntry(e, innerWidth))
	}
	if m.busy && !hasRunningTool {
		parts = append(parts, renderShimmer(m.activeVerb(), m.frame, m.opts.Color))
	}
	padded := lipgloss.NewStyle().Padding(0, 1).Render(strings.Join(parts, "\n\n"))
	lines := strings.Split(padded, "\n")
	room := max(3, m.height-lipgloss.Height(footer)-1)
	maxScroll := max(0, len(lines)-room)
	scroll := min(m.scroll, maxScroll)
	if len(lines) > room {
		end := len(lines) - scroll
		start := max(0, end-room)
		lines = lines[start:end]
	}
	// Pad short transcripts with blank lines so the input box stays anchored at the bottom.
	for len(lines) < room {
		lines = append(lines, "")
	}
	if scroll > 0 {
		badgeText := fmt.Sprintf(" ↑ scrolled %d line(s) · PgDn/End for live ", scroll)
		if m.opts.Color {
			badgeText = dimStyle.Render(badgeText)
		}
		lines[len(lines)-1] = badgeText
	}
	body := strings.Join(lines, "\n")
	return body + "\n" + footer
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
