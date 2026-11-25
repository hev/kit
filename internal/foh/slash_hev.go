package foh

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type HevSlashMode int

const (
	HevSlashBackground HevSlashMode = iota
	HevSlashTerminal
	HevSlashGuarded
)

// HevSlashCommand describes one visible top-level Cobra command.
type HevSlashCommand struct {
	Name        string
	Summary     string
	Group       string
	DefaultArgs []string
	Mode        HevSlashMode
	TakesArg    bool
}

type hevSlashDoneMsg struct {
	card       *Card
	background bool
	body       string
	err        error
}

type hevSlashTickMsg struct{}

func hevSlashTick() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg { return hevSlashTickMsg{} })
}

const secretsSlashError = "/secrets accepts ping, status, doctor, --help or -h. Secret values are never printed into TUI scrollback or LLM context."

func (m *TUI) runHevSlash(command HevSlashCommand, arg string) tea.Cmd {
	guarded := command.Mode == HevSlashGuarded || command.Name == "secrets"
	args, err := splitSlashArgs(arg)
	switch {
	case guarded && (err != nil || !safeSecretsSlash(args)):
		m.entries = append(m.entries, entry{kind: entryError, text: secretsSlashError})
		return nil
	case err != nil:
		m.entries = append(m.entries, entry{kind: entryError, text: err.Error()})
		return nil
	}
	if len(args) == 0 {
		args = command.DefaultArgs
	}
	argv := append([]string{command.Name}, args...)
	tools := m.opts.Agent.Tools
	action := strings.Join(args, " ")
	if guarded {
		action = args[0]
	}
	secrets := tools.secrets()
	card := &Card{Tool: "hev " + command.Name, Action: truncateLine(ansi.Strip(Scrub(action, secrets...)), 80)}
	m.entries = append(m.entries, entry{kind: entryCard, card: card, running: true})
	if command.Mode == HevSlashTerminal {
		exe, err := os.Executable()
		if err != nil {
			exe = "hev"
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		cmd := exec.CommandContext(ctx, exe, argv...)
		cmd.Env = toolEnv()
		return tea.ExecProcess(cmd, func(err error) tea.Msg {
			cancel()
			return hevSlashDoneMsg{card: card, err: scrubErr(err, secrets...)}
		})
	}

	run := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), defaultHevTimeout)
		defer cancel()
		out, err := tools.Hev(ctx, argv, "")
		if guarded && err != nil {
			// Cobra errors can echo arguments. Only display successful diagnostic output.
			return hevSlashDoneMsg{card: card, background: true, err: fmt.Errorf("secrets diagnostic failed")}
		}
		return hevSlashDoneMsg{card: card, background: true, body: ansi.Strip(Scrub(string(out), secrets...)), err: scrubErr(err, secrets...)}
	}
	m.hevRunning++
	if m.hevRunning == 1 {
		return tea.Batch(run, hevSlashTick())
	}
	return run
}

func safeSecretsSlash(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "status", "ping", "doctor":
		return true
	case "--help", "-h":
		// Cobra can route a subcommand after a flag. Standalone help
		// must never be used as a prefix for another command.
		return len(args) == 1
	default:
		return false
	}
}

func (m *TUI) finishHevSlash(msg hevSlashDoneMsg) {
	for i := len(m.entries) - 1; i >= 0; i-- {
		e := &m.entries[i]
		if e.card != msg.card {
			continue
		}
		e.running = false
		e.card.OK = msg.err == nil
		e.card.Body = msg.body
		e.card.Summary = "Completed"
		if msg.err != nil {
			e.card.Summary = "Failed: " + firstLine(msg.err.Error(), 120)
			e.card.Body = strings.TrimSpace(msg.err.Error() + "\n" + msg.body)
		} else if msg.body != "" {
			e.card.Summary = firstLine(msg.body, 120)
		}
		if e.card.Tool != "hev secrets" {
			record := Message{Role: "user", Content: hevSlashContext(e.card)}
			if m.busy {
				// Keep direct command results outside the active tool-call sequence.
				if len(m.hevContext) == MaxHistory {
					m.hevContext = m.hevContext[1:]
				}
				m.hevContext = append(m.hevContext, record)
			} else {
				m.opts.Agent.append(record)
			}
		}
		return
	}
}

func hevSlashContext(card *Card) string {
	const limit = 4096
	body := card.Body
	if body == "" {
		body = card.Summary
	}
	context := strings.TrimSpace(card.Tool+" "+card.Action) + " (terminal result):\n" + body
	if len(context) <= limit {
		return context
	}
	const suffix = "\n… (truncated)"
	end := limit - len(suffix)
	for !utf8.RuneStart(context[end]) {
		end--
	}
	return context[:end] + suffix
}

// splitSlashArgs tokenizes shell-style quoting without expansions or shell execution.
func splitSlashArgs(input string) ([]string, error) {
	var args []string
	var word strings.Builder
	var quote rune
	started, escape := false, false
	for _, r := range input {
		switch {
		case escape:
			if quote == '"' && !strings.ContainsRune("\"\\$`\n", r) {
				word.WriteRune('\\')
			}
			if r != '\n' {
				word.WriteRune(r)
				started = true
			}
			escape = false
		case r == '\\' && quote != '\'':
			escape = true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				word.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote, started = r, true
		case unicode.IsSpace(r):
			if started {
				args = append(args, word.String())
				word.Reset()
				started = false
			}
		default:
			word.WriteRune(r)
			started = true
		}
	}
	if quote != 0 || escape {
		return nil, fmt.Errorf("unfinished quote or escape in command arguments")
	}
	if started {
		args = append(args, word.String())
	}
	return args, nil
}
