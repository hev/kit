// Package harness describes the harness capabilities shared by kit subsystems.
package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

const (
	Claude   = "claude"
	Codex    = "codex"
	Pi       = "pi"
	OpenCode = "opencode"
	Agy      = "agy"
	Dot      = "dot"
	Grok     = "grok"
	// FOH is hev foh, kit's optional native front-of-house harness (RFC 0029).
	FOH = "hev"
)

type Descriptor struct {
	Name, Binary, TraceName string
	Aliases                 []string
	Dispatch                *Dispatch
	Hooks                   *Hooks
	Peers                   *Peers
	Trace                   *Trace
	Skills                  *Skills
}

type Dispatch struct {
	ModelFlag, PromptPrefix string
	Env                     []string
	Args, OutsideRepo       []string
	EffortArgs              func(string) []string
	ProjectSkills           string
	SessionEnv              []string
	PresenceEnv             string
}

func (d Dispatch) Argv(model, effort, prompt string, outsideRepo bool) []string {
	args := append([]string(nil), d.Args...)
	if model != "" {
		args = append(args, d.ModelFlag, model)
	}
	if effort != "" && d.EffortArgs != nil {
		args = append(args, d.EffortArgs(effort)...)
	}
	if outsideRepo {
		args = append(args, d.OutsideRepo...)
	}
	return append(args, d.PromptPrefix+prompt)
}

type Hooks struct {
	Events          map[string]string
	Config, HomeEnv string
	// Invocation selects how session-scoped hook settings are supplied.
	Invocation string
	// CodexResponse selects Codex's block and context response shapes.
	CodexResponse bool
	TrustRequired bool
}

func (h Hooks) SettingsPath(home string) string {
	if h.HomeEnv != "" {
		if dir := os.Getenv(h.HomeEnv); dir != "" {
			_, relative, _ := strings.Cut(h.Config, "/")
			return filepath.Join(dir, filepath.FromSlash(relative))
		}
	}
	return filepath.Join(home, filepath.FromSlash(h.Config))
}

var hookEvents = map[string]string{
	"session-start": "SessionStart",
	"prompt-submit": "UserPromptSubmit",
	"pre-tool":      "PreToolUse",
	"post-tool":     "PostToolUse",
	"stop":          "Stop",
	"pre-compact":   "PreCompact",
}

// NativeHookEvent is the event vocabulary shared by the current hook adapters.
func NativeHookEvent(event string) string { return hookEvents[event] }

func (h Hooks) NativeEvent(event string) string { return h.Events[event] }

type Peers struct {
	Paste, InboxOnly, ExecutablePathMatch bool
	MatchProcess                          func(words []string, exe, entry, entryBase string) bool
	Headless                              func(words []string) bool
}

type Trace struct {
	// PreviewTail preserves the ordering of exported skill preview blocks.
	PreviewTail                               bool
	Path, Override, OverrideSubdir, XDGSubdir string
	RequireHome                               bool
}

func (t Trace) Root() string {
	if t.Override != "" {
		if dir := os.Getenv(t.Override); dir != "" {
			if t.OverrideSubdir != "" {
				return filepath.Join(dir, t.OverrideSubdir)
			}
			return dir
		}
	}
	if t.XDGSubdir != "" {
		if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
			return filepath.Join(dir, t.XDGSubdir)
		}
	}
	home, err := os.UserHomeDir()
	if err != nil && t.RequireHome {
		return ""
	}
	return filepath.Join(home, filepath.FromSlash(t.Path))
}

type Skills struct {
	User, Project                []string
	UserInstructions             string
	ProjectInstructions          []string
	InstallTarget, InstallMarker string
	CoveredBy                    string
}

var registry = []Descriptor{
	{Name: Claude, Binary: Claude, TraceName: "claude_code", Aliases: []string{"claude_code"},
		Dispatch: &Dispatch{ModelFlag: "--model", Args: []string{"-p", "--verbose", "--output-format", "stream-json", "--dangerously-skip-permissions"},
			EffortArgs: func(effort string) []string { return []string{"--effort", effort} }, ProjectSkills: ".claude/skills", SessionEnv: []string{"CLAUDE_CODE_SESSION_ID", "CLAUDE_SESSION_ID"}, PresenceEnv: "CLAUDECODE"},
		Hooks: &Hooks{Events: hookEvents, Config: ".claude/settings.json", Invocation: "settings"},
		Peers: &Peers{Paste: true, MatchProcess: matchClaude, Headless: func(words []string) bool {
			return slices.Contains(words, "-p") || slices.Contains(words, "--print")
		}},
		Trace: &Trace{Path: ".claude/projects", RequireHome: true},
		Skills: &Skills{User: []string{".claude/skills"}, Project: []string{".claude/skills"},
			UserInstructions: ".claude/CLAUDE.md", ProjectInstructions: []string{"CLAUDE.md", ".claude/CLAUDE.md", "CLAUDE.local.md"},
			InstallTarget: ".claude", InstallMarker: ".claude"}},
	{Name: Codex, Binary: Codex, TraceName: Codex, Aliases: []string{"codex_cli"},
		Dispatch: &Dispatch{ModelFlag: "--model", Args: []string{"exec", "--json", "--dangerously-bypass-approvals-and-sandbox"},
			EffortArgs:  func(effort string) []string { return []string{"-c", fmt.Sprintf("model_reasoning_effort=%q", effort)} },
			OutsideRepo: []string{"--skip-git-repo-check"}, ProjectSkills: ".agents/skills", SessionEnv: []string{"CODEX_THREAD_ID"}},
		Hooks: &Hooks{Events: hookEvents, Config: ".codex/hooks.json", HomeEnv: "CODEX_HOME", Invocation: "config", CodexResponse: true, TrustRequired: true},
		Peers: &Peers{Paste: true, ExecutablePathMatch: true, MatchProcess: matchCodex, Headless: func(words []string) bool {
			return len(words) > 1 && slices.Contains(words[1:min(len(words), 3)], "exec")
		}},
		Trace: &Trace{Path: ".codex/sessions", RequireHome: true},
		Skills: &Skills{User: []string{".codex/skills", ".agents/skills"}, Project: []string{".agents/skills"},
			UserInstructions: ".codex/AGENTS.md", ProjectInstructions: []string{"AGENTS.md"},
			InstallTarget: ".codex", InstallMarker: ".codex", CoveredBy: "agents"}},
	{Name: Dot, Binary: Dot, TraceName: Dot, Trace: &Trace{Path: ".dot/traces", RequireHome: true, PreviewTail: true}},
	// hev kit (alias: hev foh) writes its own JSONL sessions and is matched as a
	// peer only when invoked as `hev kit` or `hev foh` (never for ordinary hev subcommands).
	{Name: FOH, Binary: FOH, TraceName: "hev_foh", Aliases: []string{"hev_foh", "foh", "kit"},
		Peers: &Peers{Paste: true, MatchProcess: matchHevKit, Headless: hevKitHeadless},
		Trace: &Trace{Path: ".hev/foh/sessions", Override: "HEV_FOH_SESSIONS_DIR", RequireHome: true}},
	{Name: Pi, Binary: Pi, TraceName: Pi,
		Peers: &Peers{InboxOnly: true, MatchProcess: matchPi},
		Trace: &Trace{Path: ".pi/agent/sessions", Override: "HEV_PI_SESSIONS_DIR"},
		Skills: &Skills{User: []string{".pi/agent/skills", ".agents/skills"}, Project: []string{".pi/skills", ".agents/skills"},
			UserInstructions: ".pi/agent/AGENTS.md", ProjectInstructions: []string{"AGENTS.md", "CLAUDE.md"},
			InstallTarget: ".agents", InstallMarker: ".pi"}},
	{Name: OpenCode, Binary: OpenCode, TraceName: OpenCode,
		Peers: &Peers{},
		Trace: &Trace{Path: ".local/share/opencode", Override: "HEV_OPENCODE_DATA_DIR", XDGSubdir: "opencode"},
		Skills: &Skills{UserInstructions: ".config/opencode/AGENTS.md", ProjectInstructions: []string{"AGENTS.md"},
			InstallTarget: ".agents", InstallMarker: ".config/opencode"}},
	{Name: Agy, Binary: Agy, TraceName: Agy,
		// Keep the prompt attached to --print so it cannot consume another flag.
		Dispatch: &Dispatch{ModelFlag: "--model", PromptPrefix: "--print=",
			Args:          []string{"--dangerously-skip-permissions", "--output-format", "stream-json"},
			EffortArgs:    func(effort string) []string { return []string{"--effort", effort} },
			ProjectSkills: ".agents/skills", SessionEnv: []string{"ANTIGRAVITY_CONVERSATION_ID"}},
		Peers: &Peers{Paste: true, MatchProcess: func(_ []string, exe, _, _ string) bool {
			return exe == Agy
		}, Headless: func(words []string) bool {
			for i := 1; i < len(words); i++ {
				word := words[i]
				if word == "--" {
					break
				}
				flag, _, _ := strings.Cut(word, "=")
				if flag == "-p" || flag == "--print" || flag == "--prompt" {
					return true
				}
			}
			return false
		}},
		Trace:  &Trace{Path: ".gemini/antigravity-cli", Override: "HEV_AGY_DATA_DIR", PreviewTail: true},
		Skills: &Skills{UserInstructions: ".gemini/GEMINI.md", ProjectInstructions: []string{"GEMINI.md", "AGENTS.md"}}},
	{Name: Grok, Binary: Grok, TraceName: Grok,
		Dispatch: &Dispatch{ModelFlag: "--model", PromptPrefix: "--single=",
			Args:          []string{"--output-format", "streaming-json", "--no-auto-update"},
			EffortArgs:    func(effort string) []string { return []string{"--reasoning-effort", effort} },
			ProjectSkills: ".grok/skills", SessionEnv: []string{"GROK_SESSION_ID"}},
		Hooks: &Hooks{Events: hookEvents, Config: ".grok/hooks/hev-kit.json", HomeEnv: "GROK_HOME"},
		Peers: &Peers{MatchProcess: matchGrok, Headless: grokHeadless},
		Trace: &Trace{Path: ".grok/sessions", Override: "GROK_HOME", OverrideSubdir: "sessions", RequireHome: true},
		Skills: &Skills{User: []string{".grok/skills", ".agents/skills", ".claude/skills"},
			Project:          []string{".grok/skills", ".claude/skills"},
			UserInstructions: ".claude/CLAUDE.md", ProjectInstructions: []string{"AGENTS.md", "Agents.md", "AGENT.md", "CLAUDE.md", "Claude.md", "CLAUDE.local.md", ".claude/CLAUDE.md", ".claude/CLAUDE.local.md"},
			InstallTarget: ".grok", InstallMarker: ".grok"}},
}

// All returns descriptors in trace discovery order. Treat capabilities as read-only.
func All() []Descriptor { return append([]Descriptor(nil), registry...) }

// Lookup resolves a canonical name or alias. Treat capabilities as read-only.
func Lookup(name string) (Descriptor, bool) {
	for _, d := range registry {
		if d.Name == name {
			return d, true
		}
		for _, alias := range d.Aliases {
			if alias == name {
				return d, true
			}
		}
	}
	return Descriptor{}, false
}

// DispatchFor preserves the dispatch order vocabulary, which uses canonical names.
func DispatchFor(name string) (Descriptor, bool) {
	d, ok := Lookup(name)
	return d, ok && name == d.Name && d.Dispatch != nil
}

// HooksFor preserves the hook CLI vocabulary, which uses canonical names.
func HooksFor(name string) *Hooks {
	d, _ := Lookup(name)
	if name == d.Name {
		return d.Hooks
	}
	return nil
}

// PeerDetectionOrder preserves the engine's matching precedence. Gemini is
// observation-only legacy support, with no registered kit capabilities.
func PeerDetectionOrder() []string { return []string{Claude, Codex, "gemini", OpenCode, Pi, Grok} }

func CanPaste(name string) bool {
	d, _ := Lookup(name)
	return name == d.Name && d.Peers != nil && d.Peers.Paste
}

func InboxOnly(name string) bool {
	d, _ := Lookup(name)
	return name == d.Name && d.Peers != nil && d.Peers.InboxOnly
}

// CallerIdentity follows the precedence used when harness environments nest.
func CallerIdentity() (name, session string) {
	for _, d := range registry {
		if d.Dispatch == nil {
			continue
		}
		for _, key := range d.Dispatch.SessionEnv {
			if value := os.Getenv(key); value != "" {
				return d.Name, value
			}
		}
	}
	for _, d := range registry {
		if d.Dispatch != nil && d.Dispatch.PresenceEnv != "" && os.Getenv(d.Dispatch.PresenceEnv) != "" {
			return d.Name, ""
		}
	}
	return "", ""
}

// SkillTarget identifies a supported metadata installation target.
type SkillTarget struct {
	Name, Dir string
	CoveredBy string
}

func SkillTargets() []SkillTarget {
	var targets []SkillTarget
	seen := make(map[string]bool)
	for _, d := range registry {
		if d.Skills == nil || d.Skills.InstallTarget == "" {
			continue
		}
		s := d.Skills
		name := strings.TrimPrefix(s.InstallTarget, ".")
		if !seen[name] {
			targets = append(targets, SkillTarget{Name: name, Dir: filepath.FromSlash(s.InstallTarget), CoveredBy: s.CoveredBy})
			seen[name] = true
		}
	}
	return targets
}

func CompatDetectionOrder() []string { return []string{FOH, Agy, Pi, Claude, Codex, Grok} }

func matchHevKit(words []string, exe, _, _ string) bool {
	return exe == FOH && len(words) >= 2 && (words[1] == "kit" || words[1] == "foh")
}

func hevKitHeadless(words []string) bool {
	for i := 2; i < len(words); i++ {
		if words[i] == "--" {
			break
		}
		flag, _, _ := strings.Cut(words[i], "=")
		if flag == "--once" || flag == "--show-mascot" {
			return true
		}
	}
	return false
}

func matchClaude(_ []string, exe, entry, base string) bool {
	return exe == Claude || strings.Contains(entry, "/@anthropic-ai/claude-code/") || base == Claude
}

func matchCodex(_ []string, exe, _, base string) bool {
	return exe == Codex || base == Codex || base == "codex.js"
}

var piCLI = regexp.MustCompile(`(?:^|/)@earendil-works/pi-coding-agent/dist/(?:bundle/)?cli\.js$`)

func matchPi(words []string, exe, entry, _ string) bool {
	return (len(words) == 1 && words[0] == Pi) || (exe == "node" && piCLI.MatchString(entry))
}

var grokPackage = regexp.MustCompile(`(?:^|/)@xai-official/grok(?:-(?:linux|darwin|win32)-(?:x64|arm64))?/bin/grok(?:\.exe)?$`)

func matchGrok(words []string, exe, entry, _ string) bool {
	if grokPackage.MatchString(filepath.ToSlash(words[0])) || (exe == "node" && grokPackage.MatchString(filepath.ToSlash(entry))) {
		return true
	}
	if exe != Grok && exe != "grok.exe" {
		return false
	}
	for i, word := range words[1:] {
		if word == "--output-format=streaming-json" || (word == "--output-format" && i+2 < len(words) && words[i+2] == "streaming-json") {
			return grokHeadless(words)
		}
	}
	return false
}

func grokHeadless(words []string) bool {
	for _, word := range words[1:] {
		switch strings.SplitN(word, "=", 2)[0] {
		case "-p", "--single", "--prompt-file", "--prompt-json":
			return true
		}
	}
	return false
}
