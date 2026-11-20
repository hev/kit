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
	Gemini   = "gemini"
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
			return filepath.Join(dir, filepath.Base(h.Config))
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
	PreviewTail               bool
	Path, Override, XDGSubdir string
	RequireHome               bool
}

func (t Trace) Root() string {
	if t.Override != "" {
		if dir := os.Getenv(t.Override); dir != "" {
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
	User, Project                    []string
	UserInstructions, UserExtensions string
	ProjectInstructions              []string
	InstallTarget, InstallMarker     string
	CoveredBy                        string
}

var geminiHookEvents = map[string]string{
	"session-start": "SessionStart",
	"prompt-submit": "BeforeAgent",
	"pre-tool":      "BeforeTool",
	"post-tool":     "AfterTool",
	"stop":          "AfterAgent",
	"pre-compact":   "PreCompress",
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
		Trace:  &Trace{Path: ".gemini/antigravity-cli", Override: "HEV_AGY_DATA_DIR", PreviewTail: true},
		Skills: &Skills{UserInstructions: ".gemini/GEMINI.md", ProjectInstructions: []string{"GEMINI.md", "AGENTS.md"}}},
	{Name: Gemini, Binary: Gemini, TraceName: Gemini,
		Dispatch: &Dispatch{ModelFlag: "--model", PromptPrefix: "--prompt=",
			Args:          []string{"--output-format", "stream-json", "--approval-mode", "yolo"},
			ProjectSkills: ".gemini/skills", SessionEnv: []string{"GEMINI_SESSION_ID"}},
		Hooks: &Hooks{Events: geminiHookEvents, Config: ".gemini/settings.json"},
		Peers: &Peers{MatchProcess: matchGemini, Headless: geminiHeadless},
		Trace: &Trace{Path: ".gemini/tmp", RequireHome: true},
		Skills: &Skills{User: []string{".gemini/skills", ".agents/skills"},
			UserExtensions: ".gemini/extensions",
			Project:        []string{".gemini/skills", ".agents/skills"}, UserInstructions: ".gemini/GEMINI.md",
			ProjectInstructions: []string{"GEMINI.md"}, InstallTarget: ".gemini", InstallMarker: ".gemini", CoveredBy: "agents"}},
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

// PeerDetectionOrder preserves the engine's matching precedence.
func PeerDetectionOrder() []string { return []string{Claude, Codex, Gemini, OpenCode, Pi} }

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

func CompatDetectionOrder() []string { return []string{Pi, Claude, Codex, Gemini} }

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

var geminiCLI = regexp.MustCompile(`(?:^|/)@google/gemini-cli/(?:bundle/gemini\.js|dist/index\.js)$`)

func matchGemini(_ []string, exe, entry, base string) bool {
	return exe == Gemini || (exe == "node" && (base == Gemini || geminiCLI.MatchString(filepath.ToSlash(entry))))
}

func geminiHeadless(words []string) bool {
	start := 1
	if len(words) > 0 && filepath.Base(words[0]) == "node" {
		start = 2
	}
	for _, arg := range words[min(start, len(words)):] {
		if arg == "--" {
			break
		}
		if arg == "-p" || arg == "--prompt" || strings.HasPrefix(arg, "--prompt=") || strings.HasPrefix(arg, "-p=") {
			return true
		}
	}
	return false
}
