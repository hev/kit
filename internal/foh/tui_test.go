package foh

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

var testCatalog = []Model{
	{ID: "anthropic/claude-sonnet-5.5", Name: "Claude Sonnet", ContextLength: 200000, PromptPrice: 3, CompletionPrice: 15},
	{ID: "openai/gpt-5", Name: "GPT-5", ContextLength: 400000, PromptPrice: 1.25, CompletionPrice: 10},
	{ID: "google/gemini-2.5-pro", Name: "Gemini 2.5 Pro", ContextLength: 1048576, PromptPrice: 1.25, CompletionPrice: 10},
	{ID: "meta-llama/llama-4", Name: "Llama 4", ContextLength: 128000, PromptPrice: 0, CompletionPrice: 0},
}

func newTestTUI(t *testing.T, p Provider) (*TUI, *Agent) {
	t.Helper()
	isolateHome(t)
	a := NewAgent(p, NewToolset(t.TempDir(), noHev), "anthropic/claude-sonnet-5.5", "SYS")
	m := NewTUI(TUIOptions{
		Agent: a, Favorites: LoadFavorites(""), Version: "v0.7.0-test", Cwd: "/work/kit", Workspace: "hev",
		Composer: PromptComposer{Cwd: t.TempDir(), Header: func() string { return "host: test" }},
		Catalog: func(context.Context) ([]Model, CatalogSource, error) {
			return testCatalog, SourceCache, nil
		},
	})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 50})
	// Deliver the catalog as Init would.
	m.Update(m.loadCatalog()())
	return m, a
}

func key(t tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: t} }
func runes(s string) tea.KeyMsg    { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
func plain(m *TUI) string          { return ansi.Strip(m.View()) }
func typeText(m *TUI, s string)    { m.Update(runes(s)) }
func submit(m *TUI, s string) tea.Cmd {
	typeText(m, s)
	_, cmd := m.Update(key(tea.KeyEnter))
	return cmd
}

func TestTUISplash(t *testing.T) {
	m, _ := newTestTUI(t, &scripted{})
	v := plain(m)
	for _, want := range []string{"hev foh", "v0.7.0-test", "/work/kit", "workspace  hev", "anthropic/claude-sonnet-5.5", "Ctrl+M model picker", "/prompt", "/help"} {
		if !strings.Contains(v, want) {
			t.Fatalf("splash missing %q:\n%s", want, v)
		}
	}
	// The mascot sits left of the metadata (ASCII here: no color option).
	first := strings.Split(v, "\n")[0]
	if !strings.Contains(first, "@") || !strings.Contains(first, "hev foh") || strings.Index(first, "@") > strings.Index(first, "hev foh") {
		t.Fatalf("mascot not left of header: %q", first)
	}
	// Color mode renders the truecolor mascot at this width.
	m.opts.Color = true
	if !strings.Contains(m.View(), "\x1b[38;2;") {
		t.Fatal("color splash has no truecolor mascot")
	}
}

func TestTUIModelPicker(t *testing.T) {
	m, a := newTestTUI(t, &scripted{})
	// Ctrl+M (the same key as Enter) on an empty prompt opens the picker.
	m.Update(key(tea.KeyCtrlM))
	if m.picker == nil || !strings.Contains(plain(m), "model picker") {
		t.Fatal("Ctrl+M did not open the picker")
	}
	v := plain(m)
	for _, want := range []string{"openai/gpt-5", "400k", "$1.25/$10.00 per M", "1M", "free/free", "(active)"} {
		if !strings.Contains(v, want) {
			t.Fatalf("picker missing %q:\n%s", want, v)
		}
	}
	// Fuzzy filter.
	typeText(m, "gem")
	if len(m.picker.filtered) == 0 || m.picker.filtered[0].ID != "google/gemini-2.5-pro" {
		t.Fatalf("filter gem = %v", m.picker.filtered)
	}
	m.Update(key(tea.KeyBackspace))
	m.Update(key(tea.KeyBackspace))
	m.Update(key(tea.KeyBackspace))
	typeText(m, "gpt5")
	if len(m.picker.filtered) != 1 || m.picker.filtered[0].ID != "openai/gpt-5" {
		t.Fatalf("filter gpt5 = %v", m.picker.filtered)
	}
	typeText(m, "zzz")
	if len(m.picker.filtered) != 0 {
		t.Fatalf("non-matching filter = %v", m.picker.filtered)
	}
	m.Update(key(tea.KeyEsc))
	if m.picker != nil {
		t.Fatal("Esc did not close")
	}

	// Favorite with Ctrl+F; it persists under HOME and floats to the top.
	m.Update(key(tea.KeyEnter))
	typeText(m, "llama")
	m.Update(key(tea.KeyCtrlF))
	path := filepath.Join(os.Getenv("HOME"), ".hev", "foh-favorites.json")
	b, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(b), "meta-llama/llama-4") {
		t.Fatalf("favorites file = %q %v", b, err)
	}
	m.Update(key(tea.KeyEsc))
	m.Update(key(tea.KeyEnter))
	if m.picker.filtered[0].ID != "meta-llama/llama-4" || !strings.Contains(plain(m), "★") {
		t.Fatalf("favorite not first: %v", m.picker.filtered[:2])
	}
	if LoadFavorites("").Has("meta-llama/llama-4") != true {
		t.Fatal("favorite not reloaded from disk")
	}
	// In list focus, f toggles too (and typing f in search focus does not).
	m.Update(key(tea.KeyTab))
	m.Update(runes("f"))
	if LoadFavorites("").Has("meta-llama/llama-4") {
		t.Fatal("f in list focus did not unstar")
	}
	m.Update(key(tea.KeyTab))
	typeText(m, "f")
	if m.picker.query != "f" || len(LoadFavorites("").IDs) != 0 {
		t.Fatalf("f in search focus = query %q favs %v", m.picker.query, LoadFavorites("").IDs)
	}
	m.Update(key(tea.KeyBackspace))

	// Select a model: hot-swap plus a transcript badge.
	typeText(m, "gpt")
	m.Update(key(tea.KeyEnter))
	if m.picker != nil || a.Model() != "openai/gpt-5" {
		t.Fatalf("select: picker=%v model=%s", m.picker != nil, a.Model())
	}
	if !strings.Contains(plain(m), "⇄ model anthropic/claude-sonnet-5.5 → openai/gpt-5") {
		t.Fatalf("no model badge:\n%s", plain(m))
	}
	// /model SLUG switches directly.
	submit(m, "/model google/gemini-2.5-pro")
	if a.Model() != "google/gemini-2.5-pro" {
		t.Fatal("/model slug")
	}
	submit(m, "/model")
	if m.picker == nil {
		t.Fatal("/model did not open the picker")
	}
}

// drain runs cmds until the turn finishes, feeding messages back to Update.
func drain(t *testing.T, m *TUI, cmd tea.Cmd) {
	t.Helper()
	for i := 0; cmd != nil && i < 200; i++ {
		msg := cmd()
		_, cmd = m.Update(msg)
		if _, done := msg.(agentDoneMsg); done {
			return
		}
	}
	if m.busy {
		t.Fatal("turn did not finish")
	}
}

func TestTUITurnAndHevCards(t *testing.T) {
	p := &scripted{replies: []ChatResult{
		{ToolCalls: []ToolCall{call("c1", "hev_dispatch", `{"action":"ls"}`), call("c2", "glob", `{"pattern":"*.none"}`)}},
		{Content: "Two runs are active."},
	}}
	m, a := newTestTUI(t, p)
	var calls []hevCall
	a.Tools = NewToolset(t.TempDir(), fakeHev(&calls, `{"runs":[{"name":"a"},{"name":"b"}]}`, nil))
	drain(t, m, submit(m, "what is running?"))
	v := plain(m)
	for _, want := range []string{"you › what is running?", "◆ hev_dispatch ls", "dispatch ls · 2 runs", "╭", "· glob *.none", "Two runs are active."} {
		if !strings.Contains(v, want) {
			t.Fatalf("transcript missing %q:\n%s", want, v)
		}
	}
	// A generic tool is not drawn as a card.
	for _, line := range strings.Split(v, "\n") {
		if strings.Contains(line, "glob") && strings.Contains(line, "│") {
			t.Fatalf("generic tool rendered as a card: %q", line)
		}
	}
	// Long card bodies are bounded and expand with Ctrl+O.
	body := strings.Repeat("line\n", 30)
	m.applyEvent(Event{Kind: EventToolStart, Call: call("c9", "hev_board", `{"action":"search"}`)})
	m.applyEvent(Event{Kind: EventToolResult, Call: call("c9", "hev_board", `{}`), Result: ToolResult{Card: &Card{Tool: "hev_board", Action: "search", Summary: "board search · 30", Body: body, OK: true}}})
	if !strings.Contains(plain(m), "… 24 more line(s) · Ctrl+O") {
		t.Fatalf("card body not bounded:\n%s", plain(m))
	}
	m.Update(key(tea.KeyCtrlO))
	if strings.Contains(plain(m), "more line(s) · Ctrl+O") {
		t.Fatal("Ctrl+O did not expand")
	}
}

func TestTUISteerWhileBusy(t *testing.T) {
	m, a := newTestTUI(t, &scripted{})
	m.busy = true
	typeText(m, "use staging")
	m.Update(key(tea.KeyEnter))
	if got := a.takeSteers(); len(got) != 1 || got[0] != "use staging" {
		t.Fatalf("steer = %v", got)
	}
	if !strings.Contains(plain(m), "steer queued") {
		t.Fatal("no steer notice")
	}
}

func TestTUISlashCommands(t *testing.T) {
	m, a := newTestTUI(t, &scripted{})
	submit(m, "/help")
	if !strings.Contains(plain(m), "/reload") {
		t.Fatal("/help")
	}
	submit(m, "/prompt")
	if v := plain(m); !strings.Contains(v, "system prompt: base(built-in") || !strings.Contains(v, "live(workspace") {
		t.Fatalf("/prompt:\n%s", v)
	}
	submit(m, "/reload")
	if !strings.Contains(a.System(), "hev foh") || !strings.Contains(a.System(), "host: test") {
		t.Fatalf("/reload system = %q", a.System())
	}
	a.append(Message{Role: "user", Content: "x"})
	submit(m, "/clear")
	if len(a.History()) != 0 || len(m.entries) != 0 {
		t.Fatal("/clear")
	}
	submit(m, "/bogus")
	if !strings.Contains(plain(m), "unknown command /bogus") {
		t.Fatal("unknown command")
	}
	if cmd := submit(m, "/quit"); cmd == nil {
		t.Fatal("/quit returned no command")
	} else if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("/quit did not quit")
	}
}

func TestTUIFavoritesMissingFromCatalog(t *testing.T) {
	m, _ := newTestTUI(t, &scripted{})
	m.opts.Favorites.Toggle("custom/private-model")
	m.Update(key(tea.KeyEnter))
	if m.picker.filtered[0].ID != "custom/private-model" {
		t.Fatalf("uncatalogued favorite = %v", m.picker.filtered[0])
	}
	b, _ := json.Marshal(m.opts.Favorites.IDs)
	if string(b) != `["custom/private-model"]` {
		t.Fatalf("ids = %s", b)
	}
}

func TestFuzzyScore(t *testing.T) {
	if fuzzyScore("gpt", "openai/gpt-5") <= fuzzyScore("gpt", "google/gemini-pro-turbo") {
		t.Fatal("contiguous word-start match must rank higher")
	}
	if fuzzyScore("xyz", "openai/gpt-5") != -1 || fuzzyScore("", "anything") != 0 {
		t.Fatal("non-match / empty")
	}
}
