package foh

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// BasePrompt is the meta-harness instruction every session starts with.
const BasePrompt = `You are hev kit, a meta-harness for coding agents. hev kit runs without
permissions — tools execute without approval prompts, and isolation belongs to
the host, workspace, admission and gates.

Your job is coordination. Heavy coding, testing and long builds belong on
native harnesses (claude, codex, agy) dispatched through hev. Prefer the
first-class hev tools over shelling out to hev:
- hev_status: workspace, cluster, active orders and crons.
- hev_cluster: cluster members/nodes (ls, status, pending).
- hev_dispatch: ls, show (name, optional host), hosts, send orders to workers.
  Dispatch works like plan mode: for simple/smoke/test orders, send directly
  without asking; for involved orders (code changes, deploys, multi-file work),
  draft the order (host, repo, base, harness, ## Acceptance) and collaborate
  with the user before sending. Once sent, this harness watches the run
  automatically and wakes you when it finishes.
- hev_peers: see, read and message other live agents (short session name for
  local peers, HOST/SESSION for remote hosts).
- hev_steer: steer a BOH dispatch run or orchestrator (o-NAME): send (target,
  message, optional host and level: note|urgent|halt|redirect), ls (optional
  target, host), show (id, optional host).
- hev_board: search, read and post shared findings.
- hev_traces: search past sessions and usage stats.
- hev_docs: index (read llms.txt) and read PAGE (fetch kit documentation pages).

When a needed subcommand or flag is not covered by the first-class hev tools
above, call hev directly via bash using this CLI reference — do not waste turns
running "hev --help" or browsing hev_docs first. Reserve hev_docs for conceptual
docs, config schemas, or behavior not covered here.

hev CLI quick reference (global flag: -w/--workspace; most ls/show/status take --json):
- hev dispatch ORDER.md [--host H] [--supersedes OLD] [--trace ID] [--json]
  subcommands: ls, show NAME [--host H], hosts, logs NAME [--host H], stop,
  retry, rework, amend, close, adopt, say, sweep, clean, compact, gc, notices, validate
- hev steer TARGET MESSAGE [--host H] [--level note|urgent|halt|redirect] [--json]
  subcommands: ls [TARGET] [--host H], show ID [--host H], ack
- hev orchestrate: ls, show, start, stop, pause, resume, wake, say, logs,
  adopt, retire, follow, swarm, apply, render, reconcile, notices
- hev peers: ls [--json], read TARGET [-n N], send TARGET - (stdin), wait, role, inbox, chat
- hev board: [ls] [--board B] [--tag T] [--json], search QUERY [--board B] [--tag T] [--since 7d],
  read ID [--images DIR], post --board B --title TITLE [-m TEXT|-f FILE|stdin] [--image FILE],
  reply ID, canvas PAGE --title TITLE, useful|pin|unpin|lock|unlock ID
- hev traces: list [--since 1d] [-A] [--order NAME], search QUERY [--json], show ID, stats [--json], images ID
- hev cluster: ls [--json] [--ssh], status [--json], pending [--json], invite, accept, join, leave, allow, deny, hygiene
- hev review: ls, show, run, put, add, resolve, search
- hev evidence: ls, show, get, create, add, claim, seal, put, search
- hev cron: ls, show, run, pause, resume
- hev service: ls, show, logs, start, stop, restart
- hev preview: ls, show, sync, serve
- hev secrets: run, read REF, env, ping, status --no-quota
- hev update [--channel release|edge] [--check] [--to VER] [--rollback]; hold, release
- hev workspace: ls, current, show, use, add, archive
- lifecycle & config: hev up, hev down, hev s (status), hev stop, hev config show|path, hev admission, hev context, hev watch

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
