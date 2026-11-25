package foh

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/hev/kit/internal/cluster"
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

func TestTUICatalogDeadlineAllowsScopedBudgetWait(t *testing.T) {
	for _, tc := range []struct {
		name     string
		provider Provider
		timeout  time.Duration
	}{
		{"legacy", NewRouter(&scripted{}), 30 * time.Second},
		{"scoped", NewRouter(&scopedInference{}), scopeResolveTimeout + scopeVerifyTimeout + catalogTimeout},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := newTestTUI(t, tc.provider)
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			m.opts.CatalogContext = parent
			m.opts.Catalog = func(ctx context.Context) ([]Model, CatalogSource, error) {
				deadline, ok := ctx.Deadline()
				if left := time.Until(deadline); !ok || left > tc.timeout || left < tc.timeout-time.Second {
					t.Errorf("catalog deadline remaining %v, want about %v", left, tc.timeout)
				}
				return nil, "", ctx.Err()
			}
			load := m.loadCatalog()
			m.models = nil
			if duplicate := m.openPicker(); duplicate != nil {
				t.Fatal("opening the picker must reuse the startup catalog load")
			}
			result := load().(catalogMsg)
			if result.err != nil {
				t.Fatal(result.err)
			}
			m.Update(result)
			cancel()
			if result := m.loadCatalog()().(catalogMsg); !errors.Is(result.err, context.Canceled) {
				t.Fatal("catalog must honor cancellation from the FOH session")
			}
		})
	}
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
	for _, want := range []string{"hev kit", "v0.7.0-test", "/work/kit", "workspace  hev", "anthropic/claude-sonnet-5.5", "Ctrl+M model picker", "/prompt", "/help", "hev kit runs without permissions.", "compute."} {
		if !strings.Contains(v, want) {
			t.Fatalf("splash missing %q:\n%s", want, v)
		}
	}
	// The mascot sits left of the metadata (ASCII here: no color option).
	first := strings.Split(v, "\n")[0]
	if !strings.Contains(first, "@") || !strings.Contains(first, "hev kit") || strings.Index(first, "@") > strings.Index(first, "hev kit") {
		t.Fatalf("mascot not left of header: %q", first)
	}
	// Color mode renders the truecolor mascot at this width.
	m.opts.Color = true
	if !strings.Contains(m.View(), "\x1b[38;2;") {
		t.Fatal("color splash has no truecolor mascot")
	}
	// Mascot stays in the scroll pane after a message is added.
	m.system("hello")
	if !strings.Contains(plain(m), "hev kit runs without permissions.") {
		t.Fatalf("splash disappeared from scroll pane after entry:\n%s", plain(m))
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
	for _, want := range []string{"openai/gpt-5", "400k", "[OR] $1.25/$10.00", "1M", "free/free", "(active)"} {
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
	queue := []tea.Cmd{}
	if cmd != nil {
		queue = append(queue, cmd)
	}
	for i := 0; len(queue) > 0 && i < 200; i++ {
		c := queue[0]
		queue = queue[1:]
		msg := c()
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, sub := range batch {
				if sub != nil {
					queue = append(queue, sub)
				}
			}
			continue
		}
		if _, tick := msg.(shimmerTickMsg); tick {
			continue
		}
		if _, tick := msg.(watchTickMsg); tick {
			continue
		}
		_, next := m.Update(msg)
		if _, done := msg.(agentDoneMsg); done {
			return
		}
		if evs, ok := msg.(agentEventsMsg); ok && evs.done {
			return
		}
		if next != nil {
			queue = append(queue, next)
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
	for _, want := range []string{"you › what is running?", "◆ hev dispatch ls", "dispatch ls · 2 runs", "· glob *.none", "Two runs are active."} {
		if !strings.Contains(v, want) {
			t.Fatalf("transcript missing %q:\n%s", want, v)
		}
	}
	// A generic tool is not drawn with a hev card badge.
	for _, line := range strings.Split(v, "\n") {
		if strings.Contains(line, "glob") && strings.Contains(line, "◆") {
			t.Fatalf("generic tool rendered as a hev card: %q", line)
		}
	}
	// Long card bodies are collapsed to one line and expand with Ctrl+O.
	body := strings.Repeat("line\n", 30)
	m.applyEvent(Event{Kind: EventToolStart, Call: call("c9", "hev_board", `{"action":"search"}`)})
	m.applyEvent(Event{Kind: EventToolResult, Call: call("c9", "hev_board", `{}`), Result: ToolResult{Card: &Card{Tool: "hev_board", Action: "search", Summary: "board search · 30", Body: body, OK: true}}})
	if !strings.Contains(plain(m), "(30 lines · Ctrl+O)") {
		t.Fatalf("card body not collapsed:\n%s", plain(m))
	}
	m.Update(key(tea.KeyCtrlO))
	if strings.Contains(plain(m), "(30 lines · Ctrl+O)") {
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
	if !strings.Contains(a.System(), "hev kit") || !strings.Contains(a.System(), "host: test") {
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

func TestTUIBackgroundGrid(t *testing.T) {
	lines := []string{"", "", " hello", "", "", "", ""}
	out := applyBackgroundGrid(lines, 40)
	// Row 2 has text " hello" (len 6) + margin 3 -> first dot at col 12.
	if !strings.HasPrefix(out[2], " hello") || !strings.Contains(out[2], "·") {
		t.Fatalf("expected grid dots to the right of text on row 2, got %q", out[2])
	}
	// Row 6 is blank and matches rowStep=4, rowOff=2 -> full dot row.
	if !strings.Contains(out[6], "·") {
		t.Fatalf("expected grid dots on blank row 6, got %q", out[6])
	}
	// Non-grid rows stay untouched.
	if out[0] != "" || out[3] != "" {
		t.Fatalf("non-grid rows modified: row0=%q row3=%q", out[0], out[3])
	}
}

func TestTUIInputWrapAndSinglePromptCaret(t *testing.T) {
	m, _ := newTestTUI(t, &scripted{})
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 24})
	m.input.SetValue("line one\nline two")
	m.syncInputHeight()
	if m.input.Height() < 2 {
		t.Fatalf("height = %d, want >= 2", m.input.Height())
	}
	view := ansi.Strip(m.input.View())
	if strings.Count(view, "›") != 1 {
		t.Fatalf("expected single top caret, got:\n%s", view)
	}
	// Long sentence must word-wrap and grow height automatically.
	m.input.SetValue("this is a long sentence with multiple words that should soft wrap across lines")
	m.syncInputHeight()
	if m.input.Height() < 2 {
		t.Fatalf("wrapped height = %d, want >= 2", m.input.Height())
	}
}

func TestHevDocsMultiSiteResolution(t *testing.T) {
	ts := NewToolset(t.TempDir(), nil)
	site, base, err := ts.docsBaseURL("layer")
	if err != nil || site != "layer" || base != DefaultLayerDocsURL {
		t.Fatalf("layer = %q %q %v", site, base, err)
	}
	site, base, err = ts.docsBaseURL("mind")
	if err != nil || site != "mind" || base != DefaultMindDocsURL {
		t.Fatalf("mind = %q %q %v", site, base, err)
	}
	if _, _, err := ts.docsBaseURL("evil"); err == nil {
		t.Fatal("expected unknown site error")
	}
}

func TestTUIToolCallsHaveNoBoxOutline(t *testing.T) {
	m, _ := newTestTUI(t, &scripted{})
	m.opts.Color = true
	m.applyEvent(Event{Kind: EventToolStart, Call: call("c1", "hev_dispatch", `{"action":"ls"}`)})
	m.applyEvent(Event{Kind: EventToolResult, Call: call("c1", "hev_dispatch", `{}`), Result: ToolResult{
		Card: &Card{Tool: "hev_dispatch", Action: "ls", Summary: "2 runs", Body: "a\nb\nc", OK: true},
	}})
	m.applyEvent(Event{Kind: EventToolStart, Call: call("c2", "bash", `{"command":"echo hi"}`)})
	m.applyEvent(Event{Kind: EventToolResult, Call: call("c2", "bash", `{}`), Result: ToolResult{Content: "hi"}})

	checkNoBox := func(label, s string) {
		t.Helper()
		for _, ch := range []string{"╭", "╮", "╰", "╯", "┌", "┐", "└", "┘"} {
			if strings.Contains(s, ch) {
				t.Fatalf("%s rendered box-outline char %q:\n%s", label, ch, s)
			}
		}
	}
	checkNoBox("collapsed", plain(m))
	m.Update(key(tea.KeyCtrlO))
	checkNoBox("expanded", plain(m))
}

func TestTUIWaitEventCoalescesText(t *testing.T) {
	ch := make(chan Event, 8)
	ch <- Event{Kind: EventText, Text: "hel"}
	ch <- Event{Kind: EventText, Text: "lo "}
	ch <- Event{Kind: EventText, Text: "world"}
	ch <- Event{Kind: EventToolStart, Call: call("c1", "glob", `{}`)}
	ch <- Event{Kind: EventText, Text: "after"}
	close(ch)

	msg := waitEvent(ch)()
	batch, ok := msg.(agentEventsMsg)
	if !ok {
		t.Fatalf("got %T, want agentEventsMsg", msg)
	}
	if !batch.done {
		t.Fatal("expected done=true when channel closed in batch")
	}
	if len(batch.evs) != 3 {
		t.Fatalf("len(evs) = %d, want 3: %#v", len(batch.evs), batch.evs)
	}
	if batch.evs[0].Kind != EventText || batch.evs[0].Text != "hello world" {
		t.Fatalf("evs[0] = %#v, want merged text", batch.evs[0])
	}
	if batch.evs[1].Kind != EventToolStart {
		t.Fatalf("evs[1] = %#v, want EventToolStart", batch.evs[1])
	}
	if batch.evs[2].Kind != EventText || batch.evs[2].Text != "after" {
		t.Fatalf("evs[2] = %#v, want trailing text", batch.evs[2])
	}
}

func TestTUIStreamingMarkdownCacheAndThrottle(t *testing.T) {
	m, _ := newTestTUI(t, &scripted{})
	m.busy = true
	m.applyEvent(Event{Kind: EventText, Text: "first chunk"})
	v1 := plain(m)
	if !strings.Contains(v1, "first chunk") {
		t.Fatalf("initial stream text missing:\n%s", v1)
	}
	// Immediate second delta within throttle window reuses cached render.
	m.applyEvent(Event{Kind: EventText, Text: " second chunk"})
	v2 := plain(m)
	if strings.Contains(v2, "second chunk") {
		t.Fatalf("expected streaming markdown render to throttle within %v", streamMarkdownInterval)
	}
	// Once the interval elapses, streaming render refreshes.
	m.entries[len(m.entries)-1].cacheAt = time.Now().Add(-streamMarkdownInterval - time.Millisecond)
	if !strings.Contains(plain(m), "second chunk") {
		t.Fatal("streaming markdown did not refresh after throttle interval")
	}
	// Completing the turn always flushes immediately regardless of throttle.
	m.applyEvent(Event{Kind: EventText, Text: " final chunk"})
	m.busy = false
	if !strings.Contains(plain(m), "final chunk") {
		t.Fatal("final text did not flush when turn finished")
	}
}

func TestTUITerminalNoiseFilteredFromInput(t *testing.T) {
	m, _ := newTestTUI(t, &scripted{})

	// Refused: leaked SGR mouse, OSC color report, CSI focus/DA/CPR, and stray Alt sequences.
	noise := []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("[<64;45;12M")},
		{Type: tea.KeyRunes, Runes: []rune("64;45;12m")},
		{Type: tea.KeyRunes, Runes: []rune("11;rgb:1111/1111/1111")},
		{Type: tea.KeyRunes, Runes: []rune("[?62;1;2c")},
		{Type: tea.KeyRunes, Runes: []rune("[12;34R")},
		{Type: tea.KeyRunes, Runes: []rune("[I")},
		{Type: tea.KeyRunes, Runes: []rune("O"), Alt: true},
		// Split OSC: ESC ] 11;rgb:1111/1111/1111 ESC \
		{Type: tea.KeyRunes, Runes: []rune("]"), Alt: true},
		{Type: tea.KeyRunes, Runes: []rune("1")},
		{Type: tea.KeyRunes, Runes: []rune("1")},
		{Type: tea.KeyRunes, Runes: []rune(";rgb:1111/1111/1111")},
		{Type: tea.KeyRunes, Runes: []rune(`\`), Alt: true},
		// Split CSI: ESC [ ? 6 2 ; 1 c
		{Type: tea.KeyRunes, Runes: []rune("["), Alt: true},
		{Type: tea.KeyRunes, Runes: []rune("?62;1")},
		{Type: tea.KeyRunes, Runes: []rune("c")},
	}
	for _, msg := range noise {
		m.Update(msg)
	}
	if got := m.input.Value(); got != "" {
		t.Fatalf("terminal noise leaked into input: %q", got)
	}

	// Kept: normal typing and bracketed paste.
	typeText(m, "hello [1, 2] rgb(1,2,3)")
	if got := m.input.Value(); got != "hello [1, 2] rgb(1,2,3)" {
		t.Fatalf("normal typing was dropped: %q", got)
	}
	m.input.Reset()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[<64;45;12M"), Paste: true})
	if got := m.input.Value(); got != "[<64;45;12M" {
		t.Fatalf("explicit bracketed paste was dropped: %q", got)
	}
}

func TestTUIFooterDoesNotDuplicateMainShimmer(t *testing.T) {
	m, _ := newTestTUI(t, &scripted{})
	n := 2
	m.chrome = ChromeSnapshot{
		Members: []cluster.Member{
			{
				Facts: cluster.Facts{Name: "mini", OS: "darwin", Dispatch: true, Activity: &cluster.Activity{AgentsWorking: &n}},
				State: "alive",
			},
		},
	}
	m.busy = true
	m.thinkVerb = "traversing the bowl…"

	v := plain(m)
	if count := strings.Count(v, m.thinkVerb); count != 1 {
		t.Fatalf("expected shimmer verb once (main area only), got %d:\n%s", count, v)
	}
	footer := ansi.Strip(m.statusChromeBar(120))
	if strings.Contains(footer, m.thinkVerb) {
		t.Fatalf("footer duplicated main shimmer: %q", footer)
	}
	if !strings.Contains(footer, "peers 2") || !strings.Contains(footer, "mini") {
		t.Fatalf("footer lost cluster counts while busy: %q", footer)
	}

	// With no cluster chrome, busy footer shows plain hint (no shimmer verb).
	m.chrome = ChromeSnapshot{}
	footerEmpty := ansi.Strip(m.statusChromeBar(120))
	if strings.Contains(footerEmpty, m.thinkVerb) {
		t.Fatalf("empty-chrome footer duplicated shimmer: %q", footerEmpty)
	}
	if !strings.Contains(footerEmpty, "Enter steers") {
		t.Fatalf("empty-chrome busy footer missing steer hint: %q", footerEmpty)
	}
}
