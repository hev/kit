package foh

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// hevToolEmoji maps each first-class hev tool to a distinct icon for TUI cards.
func hevToolEmoji(tool string, color bool) string {
	if !color {
		return "◆"
	}
	switch tool {
	case "hev_dispatch":
		return "🚀"
	case "hev_steer":
		return "🧭"
	case "hev_peers":
		return "💬"
	case "hev_board":
		return "📋"
	case "hev_traces":
		return "🔍"
	case "hev_status":
		return "📡"
	default:
		return "◆"
	}
}

type cardSummaryMsg struct {
	callID  string
	summary string
}

// pickSummaryModel chooses a small/fast model available on the active router.
func pickSummaryModel(p Provider, activeModel string) string {
	if r, ok := p.(*Router); ok {
		if r.Has(ProviderOpenRouter) {
			return "google/gemini-2.5-flash-lite"
		}
		if r.Has(ProviderCodexSub) {
			return "codex/gpt-5.3-codex"
		}
	}
	return activeModel
}

// summarizeCardAsync asks a small/fast model in the background for a 1-line
// human summary of a completed hev tool call. Never blocks the agent loop or UI.
func (m *TUI) summarizeCardAsync(callID, tool, action, rawOutput string) tea.Cmd {
	if m.opts.Agent == nil || m.opts.Agent.Provider == nil {
		return nil
	}
	// Skip tiny or empty outputs where the deterministic summary is already ideal.
	trimmed := strings.TrimSpace(rawOutput)
	if len(trimmed) < 40 {
		return nil
	}
	p := m.opts.Agent.Provider
	model := pickSummaryModel(p, m.opts.Agent.Model())
	if model == "" {
		return nil
	}
	snippet := truncateLine(trimmed, 3000)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		req := ChatRequest{
			Model: model,
			Messages: []Message{
				{Role: "system", Content: "Summarize what this hev CLI command did or returned in ONE short, plain-English sentence (max 14 words). No markdown, no quotes, no secrets."},
				{Role: "user", Content: fmt.Sprintf("command: %s %s\noutput:\n%s", strings.ReplaceAll(tool, "_", " "), action, snippet)},
			},
		}
		res, err := p.Stream(ctx, req, nil)
		if err != nil {
			return nil
		}
		line := firstLine(strings.TrimSpace(res.Content), 110)
		if line == "" {
			return nil
		}
		return cardSummaryMsg{callID: callID, summary: line}
	}
}
