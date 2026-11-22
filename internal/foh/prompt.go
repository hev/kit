package foh

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// BasePrompt is the FOH meta-harness instruction every session starts with.
const BasePrompt = `You are hev foh, the front-of-house agent of hev kit, a meta-harness for coding
agents. You run YOLO: tools execute without approval prompts, and isolation is
the job of the host, workspace, admission and gates, not of you asking.

Your job is coordination. Back-of-house work (implementing, testing, long
builds) belongs on native harnesses (claude, codex, agy) dispatched through
hev. Prefer the first-class hev tools over shelling out to hev:
- hev_status: workspace, cluster, active orders and crons.
- hev_dispatch: ls, show, hosts, send orders to workers.
- hev_peers: see, read and message other live agents.
- hev_board: search, read and post shared findings.
- hev_traces: search past sessions and usage stats.
Use read_file, glob, grep, edit_file and bash for local work. Keep replies
short, say what you did, and never print secret values.`

// maxPromptPart bounds each file read into the prompt.
const maxPromptPart = 64 << 10

// PromptPart is one labeled section of the composed system prompt.
type PromptPart struct {
	Label, Source, Text string
}

// PromptComposer assembles the system prompt from its sources.
type PromptComposer struct {
	// Cwd is searched for AGENTS.md and MISSION.md.
	Cwd string
	// UserPath is the user's custom prompt; empty uses
	// ~/.config/hev/foh-prompt.md.
	UserPath string
	// ConfigPrompt is [foh].system_prompt: inline text, or a path when it
	// names an existing file.
	ConfigPrompt string
	// ExtraPath is the --prompt file.
	ExtraPath string
	// Header renders the live workspace/cluster header; nil skips it.
	Header func() string
}

// UserPromptPath is ~/.config/hev/foh-prompt.md.
func UserPromptPath() string { return filepath.Join(homeDir(), ".config", "hev", "foh-prompt.md") }

func readBounded(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxPromptPart))
	return strings.TrimSpace(string(b)), err
}

// Parts returns the prompt sections in order. A missing optional file is
// skipped; an unreadable --prompt file is an error.
func (p PromptComposer) Parts() ([]PromptPart, error) {
	parts := []PromptPart{{Label: "base", Source: "built-in", Text: BasePrompt}}
	if p.Cwd != "" {
		for _, name := range []string{"AGENTS.md", "MISSION.md"} {
			if text, err := readBounded(filepath.Join(p.Cwd, name)); err == nil && text != "" {
				parts = append(parts, PromptPart{Label: "workspace", Source: name, Text: text})
			}
		}
	}
	user := p.UserPath
	if user == "" {
		user = UserPromptPath()
	}
	if text, err := readBounded(user); err == nil && text != "" {
		parts = append(parts, PromptPart{Label: "user", Source: "foh-prompt.md", Text: text})
	}
	if cp := strings.TrimSpace(p.ConfigPrompt); cp != "" {
		if text, err := readBounded(expandHome(cp)); err == nil {
			if text != "" {
				parts = append(parts, PromptPart{Label: "config", Source: "[foh].system_prompt file", Text: text})
			}
		} else if errors.Is(err, os.ErrNotExist) && !looksLikePath(cp) {
			parts = append(parts, PromptPart{Label: "config", Source: "[foh].system_prompt", Text: cp})
		}
	}
	if p.ExtraPath != "" {
		text, err := readBounded(expandHome(p.ExtraPath))
		if err != nil {
			// Drop the path from the message; keep the cause.
			cause := errors.Unwrap(err)
			if cause == nil {
				cause = err
			}
			return nil, fmt.Errorf("read --prompt file: %w", cause)
		}
		parts = append(parts, PromptPart{Label: "extra", Source: "--prompt", Text: text})
	}
	if p.Header != nil {
		if h := strings.TrimSpace(p.Header()); h != "" {
			parts = append(parts, PromptPart{Label: "live", Source: "workspace", Text: h})
		}
	}
	return parts, nil
}

// Compose joins Parts into one system prompt.
func (p PromptComposer) Compose() (string, error) {
	parts, err := p.Parts()
	if err != nil {
		return "", err
	}
	return JoinParts(parts), nil
}

// JoinParts renders parts with a heading per non-base section.
func JoinParts(parts []PromptPart) string {
	var b strings.Builder
	for i, part := range parts {
		if i > 0 {
			fmt.Fprintf(&b, "\n\n## %s (%s)\n\n", part.Label, part.Source)
		}
		b.WriteString(part.Text)
	}
	return b.String()
}

func looksLikePath(s string) bool {
	return !strings.ContainsAny(s, "\n ") && (strings.HasPrefix(s, "~") || strings.ContainsAny(s, `/\`)) && strings.HasSuffix(strings.ToLower(s), ".md")
}

func expandHome(p string) string {
	if p == "~" {
		return homeDir()
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		return filepath.Join(homeDir(), p[2:])
	}
	return p
}
