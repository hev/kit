package trace

import (
	"encoding/json"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// SkillLoad is transcript evidence of a skill invocation or file load. Unknown
// paths and historical commits stay empty: today's checkout is not evidence of
// the version read by an old session. TurnUUID, Seq and TS identify the load.
type SkillLoad struct {
	Name   string `json:"name"`
	Path   string `json:"path,omitempty"`
	Source string `json:"source,omitempty"`
	Commit string `json:"commit,omitempty"`
	// Via is how the skill was loaded: "tool" (a Skill tool call), "read"
	// (a read of .../skills/NAME/SKILL.md by a read tool or a shell command)
	// or "command" (a Claude Code slash command).
	Via      string `json:"via,omitempty"`
	TurnUUID string `json:"turn_uuid"`
	Seq      int64  `json:"seq"`
	TS       string `json:"ts"`
}

type SkillLoads []SkillLoad

func (s *SkillLoads) UnmarshalJSON(raw []byte) error {
	b, err := unquoted(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, (*[]SkillLoad)(s))
}

// pinnedRelease matches a path inside a pinned skill source release,
// ~/.local/opt/hev-skills-SOURCE-COMMIT (RFC 0012; the same pattern as
// skillsources.PinnedCommit). That directory name is the version evidence
// skill capture otherwise lacks.
var pinnedRelease = regexp.MustCompile(`[/\\]\.local[/\\]opt[/\\]hev-skills-[0-9a-f]{16}-([0-9a-f]{40})([/\\]|$)`)

// pinnedCommit is the commit of the pinned release a loaded SKILL.md path is
// in: named in the path itself, or reached through a link that still
// resolves into one, as a shelf skill a dispatch run linked into its
// worktree does while the run is live. Any other path has no commit.
func pinnedCommit(p string) string {
	if m := pinnedRelease.FindStringSubmatch(p); m != nil {
		return m[1]
	}
	if !filepath.IsAbs(p) {
		return ""
	}
	if real, err := filepath.EvalSymlinks(p); err == nil {
		if m := pinnedRelease.FindStringSubmatch(real); m != nil {
			return m[1]
		}
	}
	return ""
}

var slashSkill = regexp.MustCompile(`<command-name>/([^<\s]+)</command-name>`)

var skillField = regexp.MustCompile(`"(?:skill|name|id)"\s*:\s*"([^"]+)"`)

var skillLine = regexp.MustCompile(`(?m)^(?:skill|name|id): *(\S+)`)

var skillNameChars = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// NormalizeSkillName reduces a recorded skill reference to the skill's own
// name, so a Skill-tool call and a SKILL.md read of the same skill count
// once: surrounding quotes, slashes and punctuation go, a JSON wrapper is
// unwrapped, and a plugin prefix ("plugin:skill", "plugin:plugin") is
// dropped because a SKILL.md read never carries one. A reference that is
// still not a plain name after that is not recorded.
func NormalizeSkillName(name string) string {
	name = strings.TrimSpace(name)
	if strings.HasPrefix(name, "{") {
		if m := skillField.FindStringSubmatch(name); m != nil {
			name = m[1]
		}
	}
	name = strings.Trim(name, " \t\r\n\"'`,;{}()[]")
	name = strings.TrimPrefix(name, "/")
	if i := strings.LastIndex(name, ":"); i >= 0 {
		name = name[i+1:]
	}
	if !skillNameChars.MatchString(name) {
		return ""
	}
	return name
}

// CaptureSkills is idempotent and shared by all parsers and chunk reassembly.
// Only read tool inputs and executed read commands count; prompt catalogues,
// tool output and ordinary markdown mentions do not.
func CaptureSkills(turns []Turn) []Turn {
	for ti := range turns {
		t := &turns[ti]
		for bi := range t.Blocks {
			b := &t.Blocks[bi]
			b.SkillLoads = nil
			add := func(name, p string) {
				via := "tool"
				if p != "" {
					via = "read"
				}
				if b.Type != "tool_use" {
					via = "command"
				}
				source := ""
				if p != "" {
					i := strings.LastIndex(p, "/SKILL.md")
					dir := p[:i]
					name = path.Base(dir)
					source = path.Dir(dir)
					if i := strings.LastIndex(p, "/skills/"); i >= 0 {
						source = p[:i+len("/skills")]
					} else if strings.HasPrefix(p, "skills/") {
						source = "skills"
					}
				}
				if name = NormalizeSkillName(name); name != "" {
					b.SkillLoads = append(b.SkillLoads, SkillLoad{Name: name, Path: p, Source: source, Via: via, Commit: pinnedCommit(p), TurnUUID: t.TurnUUID, Seq: t.Seq, TS: t.TS})
				}
			}
			if b.Type == "tool_use" {
				name := strings.TrimPrefix(strings.ToLower(b.ToolName), "functions.")
				if name == "skill" || (t.Harness == "gemini" && name == "activate_skill") {
					input := strings.TrimSpace(strings.TrimPrefix(b.Text, b.ToolName))
					var obj map[string]any
					if json.Unmarshal([]byte(input), &obj) == nil {
						s, _ := obj["skill"].(string)
						if s == "" {
							s, _ = obj["name"].(string) // OpenCode 1.x's skill tool
						}
						if s == "" {
							s, _ = obj["id"].(string) // OpenCode 2.x's skill tool
						}
						p := ""
						for _, key := range []string{"path", "file_path"} {
							if value, ok := obj[key].(string); ok {
								if paths := skillPaths(value); len(paths) > 0 {
									p = paths[0]
									break
								}
							}
						}
						add(s, p)
					} else if m := skillLine.FindStringSubmatch(input); m != nil {
						// Codex renders a call's arguments as "key: value" lines.
						add(m[1], "")
					} else if m := skillField.FindStringSubmatch(input); m != nil {
						// A truncated or otherwise invalid JSON input still
						// names its skill; never record the wrapper itself.
						add(m[1], "")
					} else if fields := strings.Fields(input); len(fields) > 0 {
						add(fields[0], "")
					}
				} else if name == "read" || name == "read_file" || name == "readfile" || strings.HasSuffix(name, "__read_file") {
					for _, p := range readToolSkillPaths(strings.TrimSpace(strings.TrimPrefix(b.Text, b.ToolName))) {
						add("", p)
					}
				} else if name == "bash" || name == "exec" || name == "exec_command" || name == "run_command" || name == "shell" || name == "shell_command" || strings.HasSuffix(name, "__exec_command") {
					// The same rendered-input rule as command facts: a JSON
					// object input is decoded, a script is read as a script.
					for _, p := range shellSkillPaths(shellToolCommand(b.ToolName, b.Text)) {
						add("", p)
					}
				}
			} else if t.Harness == "claude_code" && t.Role == "user" && b.Type == "text" {
				for _, m := range slashSkill.FindAllStringSubmatch(b.Text, -1) {
					switch m[1] {
					case "clear", "help", "compact", "model", "cost", "status", "exit", "resume":
						continue
					}
					add(m[1], "")
				}
			}
		}
	}
	return CaptureToolOutcomes(turns)
}

func commandStrings(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []any:
		var out []string
		for _, v := range x {
			out = append(out, commandStrings(v))
		}
		return strings.Join(out, "\n")
	case map[string]any:
		var out []string
		for _, key := range []string{"cmd", "command", "commands", "CommandLine", "script", "code"} {
			if v, ok := x[key]; ok {
				out = append(out, commandStrings(v))
			}
		}
		return strings.Join(out, "\n")
	}
	return ""
}

func skillNames(turns []Turn) StringList {
	names := map[string]bool{}
	for _, t := range turns {
		for _, b := range t.Blocks {
			for _, s := range b.SkillLoads {
				names[s.Name] = true
			}
		}
	}
	out := StringList{}
	for name := range names {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// skillPaths receives a file operand, not arbitrary transcript text.
func skillPaths(text string) []string {
	text = strings.TrimSpace(text)
	if strings.HasSuffix(text, "/SKILL.md") && (strings.HasPrefix(text, "skills/") || strings.Contains(text, "/skills/")) {
		return []string{text}
	}
	return nil
}

func readToolSkillPaths(input string) []string {
	var obj map[string]any
	if json.Unmarshal([]byte(input), &obj) == nil {
		var out []string
		for _, key := range []string{"path", "Path", "file_path", "filePath", "file"} {
			if value, ok := obj[key].(string); ok {
				out = append(out, skillPaths(value)...)
			}
		}
		return out
	}
	for _, key := range []string{"path: ", "file_path: ", "file: "} {
		input = strings.TrimPrefix(input, key)
	}
	// dot appends result metadata, and Codex can append other argument fields.
	input = strings.SplitN(input, "\n", 2)[0]
	return skillPaths(input)
}
