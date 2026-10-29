package trace

import (
	"encoding/json"
	"path"
	"regexp"
	"sort"
	"strings"
)

// SkillLoad records evidence of a load, never a guess based on installed files.
// Path/source/commit remain empty when a native invocation does not record them.
type SkillLoad struct {
	Name   string `json:"name"`
	Path   string `json:"path,omitempty"`
	Source string `json:"source,omitempty"`
	Turn   string `json:"turn"`
	Seq    int64  `json:"seq"`
	Time   string `json:"time,omitempty"`
	Commit string `json:"commit,omitempty"`
}

type SkillLoads []SkillLoad

func (s *SkillLoads) UnmarshalJSON(raw []byte) error {
	raw, err := unquoted(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, (*[]SkillLoad)(s))
}

var skillPath = regexp.MustCompile(`(?:[~$./A-Za-z0-9_@+-]+/)?skills/(?:[.A-Za-z0-9_@+-]+/)+SKILL\.md`)
var readCommand = regexp.MustCompile(`(?:^|[\s;|(&"\\])(?:cat|head|tail|sed|less|more|bat|read_file)\s`)
var slashSkill = regexp.MustCompile(`<command-name>/([^<\s]+)</command-name>`)
var slashSkillMessage = regexp.MustCompile(`<command-message>([^<]+)</command-message>`)

// captureSkills runs after parsing, while turn identity and tool input coexist.
func captureSkills(turns []Turn) []Turn {
	for ti := range turns {
		t := &turns[ti]
		for bi := range t.Blocks {
			b := &t.Blocks[bi]
			var loads SkillLoads
			add := func(name, p string) {
				if name == "" {
					return
				}
				source := ""
				if p != "" {
					source = skillSource(p)
				}
				loads = append(loads, SkillLoad{Name: name, Path: p, Source: source, Turn: t.TurnUUID, Seq: t.Seq, Time: t.TS})
			}
			if b.Type == "tool_use" {
				tool := strings.ToLower(b.ToolName)
				if tool == "skill" {
					text := strings.TrimSpace(strings.TrimPrefix(b.Text, b.ToolName))
					var input struct {
						Skill string `json:"skill"`
					}
					_ = json.Unmarshal([]byte(text), &input)
					if input.Skill != "" {
						add(input.Skill, "")
					} else if fields := strings.Fields(text); len(fields) > 0 {
						add(fields[0], "")
					}
				}
				read := tool == "read" || tool == "read_file" || tool == "readfile" || tool == "functions.read_file"
				command := tool == "bash" || tool == "exec" || tool == "exec_command" || tool == "shell" || tool == "functions.exec" || tool == "functions.exec_command"
				// Unescape JSON input so shell commands are recognizable, including nested exec calls.
				text := strings.ReplaceAll(b.Text, `\"`, `"`)
				if read || (command && readCommand.MatchString(text)) {
					seen := map[string]bool{}
					for _, p := range skillReadPaths(text, read) {
						if !seen[p] {
							add(path.Base(path.Dir(p)), p)
							seen[p] = true
						}
					}
				}
			} else if t.Harness == "claude_code" && t.Role == "user" && b.Type == "text" {
				name := slashSkill.FindStringSubmatch(b.Text)
				message := slashSkillMessage.FindStringSubmatch(b.Text)
				if len(name) > 0 && len(message) > 0 && name[1] == strings.TrimSpace(message[1]) && !builtinSlash(name[1]) {
					add(name[1], "")
				}
			}
			b.SkillLoads = loads
		}
	}
	return turns
}

func skillNames(turns []Turn) StringList {
	set := map[string]bool{}
	for _, t := range turns {
		for _, b := range t.Blocks {
			for _, s := range b.SkillLoads {
				set[s.Name] = true
			}
		}
	}
	out := StringList{}
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// Require complete path components: notskills/ and SKILL.md.bak are ordinary files.
func skillPaths(text string) []string {
	var out []string
	for _, loc := range skillPath.FindAllStringIndex(text, -1) {
		boundary := func(c byte) bool { return strings.ContainsRune(" \t\r\n\\\"'`;,()[]{}<>|&", rune(c)) }
		if loc[0] > 0 && !boundary(text[loc[0]-1]) {
			continue
		}
		if loc[1] < len(text) && !boundary(text[loc[1]]) {
			continue
		}
		out = append(out, text[loc[0]:loc[1]])
	}
	return out
}

func builtinSlash(name string) bool {
	switch name {
	case "clear", "compact", "help", "status", "model", "cost", "login", "logout", "resume", "config", "export", "permissions", "rename", "rewind", "context":
		return true
	}
	return false
}

func skillReadPaths(text string, direct bool) []string {
	if direct {
		return skillPaths(text)
	}
	var out []string
	for _, loc := range readCommand.FindAllStringIndex(text, -1) {
		rest := text[loc[1]:]
		// Keep evidence within this shell command, not later echoes or writes.
		if end := strings.IndexAny(rest, ";|&\n>"); end >= 0 {
			rest = rest[:end]
		}
		out = append(out, skillPaths(rest)...)
	}
	return out
}

func skillSource(p string) string {
	if strings.HasPrefix(p, "skills/") {
		return "skills"
	}
	if i := strings.Index(p, "/skills/"); i >= 0 {
		return p[:i+len("/skills")]
	}
	return ""
}
