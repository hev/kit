package foh

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
)

func slashResult(t *testing.T, m *TUI, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected asynchronous slash command")
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		msg = batch[0]()
	}
	if _, ok := msg.(hevSlashDoneMsg); !ok {
		t.Fatalf("expected completion, got %T", msg)
	}
	m.Update(msg)
}

func TestHevSlashRunsWithoutAgentAndRecordsBoundedContext(t *testing.T) {
	provider := &scripted{}
	m, a := newTestTUI(t, provider)
	m.opts.HevCommands = []HevSlashCommand{
		{Name: "cluster", DefaultArgs: []string{"ls"}, TakesArg: true},
		{Name: "board", DefaultArgs: []string{"list"}, TakesArg: true},
	}
	var calls []hevCall
	out := "oven down\n" + strings.Repeat("界\n", 2000)
	a.Tools.Hev = fakeHev(&calls, out, nil)
	cmd := submit(m, "/cluster")
	if len(calls) != 0 || len(a.History()) != 0 || m.busy {
		t.Fatal("slash command ran synchronously or started an agent turn")
	}
	if len(m.entries) != 1 || m.entries[0].kind != entryCard || !m.entries[0].running {
		t.Fatalf("expected running card: %+v", m.entries)
	}
	if !strings.Contains(plain(m), "running…") {
		t.Fatal("running card missing from transcript")
	}
	slashResult(t, m, cmd)
	if m.entries[0].running || m.entries[0].card.Summary != "oven down" ||
		!strings.Contains(plain(m), "Ctrl+O") {
		t.Fatalf("completed card failed to summarize output: %s", plain(m))
	}
	m.Update(key(tea.KeyCtrlO))
	if !strings.Contains(plain(m), "界") {
		t.Fatal("expanded card missing output")
	}
	history := a.History()
	if len(history) != 1 || len(history[0].Content) > 4096 || !utf8.ValidString(history[0].Content) ||
		!strings.Contains(history[0].Content, "hev cluster ls (terminal result):\noven down") ||
		!strings.HasSuffix(history[0].Content, "(truncated)") {
		t.Fatal("oversized slash result did not produce bounded UTF-8 context")
	}
	slashResult(t, m, submit(m, `/board search "swim failover" 'a b' c\ d "" '$(literal)'`))
	if len(calls) != 2 || !slices.Equal(calls[0].args, []string{"cluster", "ls"}) ||
		!slices.Equal(calls[1].args, []string{"board", "search", "swim failover", "a b", "c d", "", "$(literal)"}) {
		t.Fatalf("runner argv = %+v", calls)
	}
	if cmd := submit(m, `/board search "unfinished`); cmd != nil || len(calls) != 2 ||
		!strings.Contains(plain(m), "unfinished quote or escape") {
		t.Fatal("malformed quoting should give an error and refuse execution")
	}
}

func TestHevSlashSecretsGuard(t *testing.T) {
	m, a := newTestTUI(t, &scripted{})
	m.opts.HevCommands = []HevSlashCommand{{Name: "secrets", Mode: HevSlashGuarded}}
	var calls []hevCall
	const secret = "test-private-value"
	a.Tools.AddSecret(secret)
	a.Tools.Hev = fakeHev(&calls, "daemon up\n"+secret, nil)
	for _, allowed := range []string{"status --json", "ping", "doctor", "--help", "-h"} {
		slashResult(t, m, submit(m, "/secrets "+allowed))
		e := m.entries[len(m.entries)-1]
		if !e.card.OK || !strings.Contains(e.card.Body, "daemon up") ||
			strings.Contains(e.card.Body, secret) || len(a.History()) != 0 {
			t.Fatal("allowed diagnostic was lost, leaked a known secret or entered history")
		}
	}
	if len(calls) != 5 {
		t.Fatalf("allowed diagnostics made %d runner calls", len(calls))
	}
	for _, refused := range []string{"", "read " + secret, "env " + secret, "run " + secret, "unknown",
		"--help --help=false read " + secret, "-h read " + secret, `"unfinished`} {
		if cmd := submit(m, "/secrets "+refused); cmd != nil {
			t.Fatalf("unsafe form invoked runner: %q", refused)
		}
		e := m.entries[len(m.entries)-1]
		if e.kind != entryError || e.text != secretsSlashError || len(a.History()) != 0 || len(calls) != 5 {
			t.Fatal("unsafe form was echoed, invoked the runner or entered history")
		}
	}
	a.Tools.Hev = fakeHev(&calls, secret, errors.New(secret))
	slashResult(t, m, submit(m, "/secrets status"))
	e := m.entries[len(m.entries)-1]
	if e.card.OK || strings.Contains(e.card.Body, secret) || len(a.History()) != 0 {
		t.Fatal("diagnostic failure leaked its raw output or error")
	}
}

func TestHevSlashReviewUsesTerminalHandoff(t *testing.T) {
	m, a := newTestTUI(t, &scripted{})
	m.opts.HevCommands = []HevSlashCommand{{Name: "review", Mode: HevSlashTerminal, TakesArg: true}}
	a.Tools.Hev = func(context.Context, []string, string) ([]byte, error) {
		t.Fatal("/review used the background runner")
		return nil, nil
	}
	cmd := submit(m, "/review example")
	if cmd == nil {
		t.Fatal("/review did not schedule a terminal handoff")
	}
	// Bubble Tea's exec message is private. Compare its type with the message
	// produced by ExecProcess without starting any terminal subprocess.
	got, want := cmd(), tea.ExecProcess(nil, nil)()
	if reflect.TypeOf(got) != reflect.TypeOf(want) || m.hevRunning != 0 {
		t.Fatalf("review returned %T, want %T", got, want)
	}
}

func TestHevSlashPaletteAndNativePrecedence(t *testing.T) {
	m, _ := newTestTUI(t, &scripted{})
	for _, native := range slashCommands {
		m.opts.HevCommands = append(m.opts.HevCommands, HevSlashCommand{
			Name: strings.TrimPrefix(native.Name, "/"), Group: "cluster", Summary: "shadow",
		})
	}
	m.opts.HevCommands = append(m.opts.HevCommands, HevSlashCommand{Name: "version"})
	for i := 0; i < 22; i++ {
		m.opts.HevCommands = append(m.opts.HevCommands, HevSlashCommand{
			Name: fmt.Sprintf("cluster-%02d", i), Group: "cluster", TakesArg: true,
		})
	}
	typeText(m, "/")
	matches := m.matchingCommands()
	if len(matches) != 33 || !slices.Equal(matches[:len(slashCommands)], slashCommands) ||
		matches[10].Group != "Cluster:" || matches[32].Group != "Other:" {
		t.Fatalf("palette order or deduplication wrong: %+v", matches)
	}
	for i := range matches {
		rows := strings.Split(m.renderCommandSuggestions(100), "\n")
		if len(rows) != 9 || !strings.Contains(rows[8], "… 25 more · type to filter") {
			t.Fatalf("at cursor %d, palette rows = %v", i, rows)
		}
		if !slices.ContainsFunc(rows[:8], func(row string) bool {
			return strings.HasPrefix(strings.TrimSpace(row), "› "+matches[i].Name+" ") || strings.TrimSpace(row) == "› "+matches[i].Name
		}) {
			t.Fatalf("selection %s missing at cursor %d: %v", matches[i].Name, i, rows)
		}
		if i == 10 {
			m.Update(key(tea.KeyUp))
			if m.cmdStart != 3 {
				t.Fatalf("window jumped on upward movement: %d", m.cmdStart)
			}
			m.Update(key(tea.KeyDown))
		}
		if i < len(matches)-1 {
			m.Update(key(tea.KeyDown))
		}
	}
	m.Update(key(tea.KeyTab))
	if m.input.Value() != "/version" {
		t.Fatalf("no-arg completion = %q", m.input.Value())
	}
	m.input.Reset()
	typeText(m, "/cluster-00")
	m.Update(key(tea.KeyTab))
	if m.input.Value() != "/cluster-00 " {
		t.Fatalf("arg completion = %q", m.input.Value())
	}
	m.input.Reset()
	if cmd := submit(m, "/help"); cmd != nil || m.entries[len(m.entries)-1].detail != helpText {
		t.Fatal("Cobra shadowed native /help")
	}
	if cmd := submit(m, "/model"); cmd != nil || m.picker == nil {
		t.Fatal("Cobra shadowed native /model")
	}
}
