package foh

import (
	"context"
	"fmt"
	"os"
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
	case "hev_docs":
		return "📖"
	default:
		return "◆"
	}
}

type cardSummaryMsg struct {
	callID  string
	summary string
}

// pickSummaryModel chooses a small/fast model available on the active router.
// HEV_SUMMARY_MODEL or [foh].summary_model overrides the default.
func pickSummaryModel(p Provider, activeModel, configured string) string {
	if v := strings.TrimSpace(os.Getenv("HEV_SUMMARY_MODEL")); v != "" {
		return v
	}
	if v := strings.TrimSpace(configured); v != "" {
		return v
	}
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
	model := pickSummaryModel(p, m.opts.Agent.Model(), m.opts.SummaryModel)
	if model == "" {
		return nil
	}
	snippet := truncateLine(trimmed, 3000)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		req := ChatRequest{
			Model: model,
			Messages: []Message{
				{Role: "system", Content: "Summarize what this hev CLI command did or returned in ONE short, plain-English sentence (max 14 words). No markdown, no quotes, no JSON, no secrets."},
				{Role: "user", Content: fmt.Sprintf("command: %s %s\noutput:\n%s", strings.ReplaceAll(tool, "_", " "), action, snippet)},
			},
		}
		res, err := p.Stream(ctx, req, func(Delta) {})
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
