package trace

import (
	"encoding/json"
	"path"
	"regexp"
	"sort"
	"strings"
)

// SkillLoad is transcript evidence of a skill invocation or file load. Unknown
// paths and historical commits stay empty: today's checkout is not evidence of
// the version read by an old session. TurnUUID, Seq and TS identify the load.
type SkillLoad struct {
	Name     string `json:"name"`
	Path     string `json:"path,omitempty"`
	Source   string `json:"source,omitempty"`
	Commit   string `json:"commit,omitempty"`
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

var skillPath = regexp.MustCompile(`(?:[~/.$A-Za-z0-9_-]+/)?skills/(?:[.A-Za-z0-9_-]+/)+SKILL\.md`)
var readCommand = regexp.MustCompile(`(?:^|[\s;|(&"'])(?:cat|sed|head|tail|less|more|bat)\s+([^;|&\n]+)`)
var slashSkill = regexp.MustCompile(`<command-name>/([^<\s]+)</command-name>`)

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
				if name != "" {
					b.SkillLoads = append(b.SkillLoads, SkillLoad{Name: name, Path: p, Source: source, TurnUUID: t.TurnUUID, Seq: t.Seq, TS: t.TS})
				}
			}
			if b.Type == "tool_use" {
				name := strings.TrimPrefix(strings.ToLower(b.ToolName), "functions.")
				if name == "skill" {
					input := strings.TrimSpace(strings.TrimPrefix(b.Text, b.ToolName))
					var obj map[string]any
					if json.Unmarshal([]byte(input), &obj) == nil {
						s, _ := obj["skill"].(string)
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
					} else if fields := strings.Fields(input); len(fields) > 0 {
						add(fields[0], "")
					}
				} else if name == "read" || name == "read_file" || name == "readfile" || strings.HasSuffix(name, "__read_file") {
					for _, p := range skillPaths(b.Text) {
						add("", p)
					}
				} else if name == "bash" || name == "exec" || name == "exec_command" || name == "shell" || name == "shell_command" || strings.HasSuffix(name, "__exec_command") {
					text := b.Text
					// Decode JSON escapes in command objects before recognizing shell reads.
					if i := strings.Index(text, "{"); i >= 0 {
						var obj any
						if json.Unmarshal([]byte(text[i:]), &obj) == nil {
							text = commandStrings(obj)
						}
					}
					text = strings.ReplaceAll(text, `\n`, "\n")
					for _, command := range readCommand.FindAllStringSubmatch(text, -1) {
						// Quoted echo payloads are not executed reads.
						prefix := text[:strings.Index(text, command[0])]
						prefix = prefix[strings.LastIndexAny(prefix, ";\n|&")+1:]
						if strings.Contains(prefix, "echo ") || strings.Contains(prefix, "printf ") {
							continue
						}
						for _, p := range skillPaths(command[1]) {
							add("", p)
						}
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
	return turns
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
		for _, key := range []string{"cmd", "command", "commands", "script", "code"} {
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

func skillPaths(text string) []string {
	var out []string
	for _, span := range skillPath.FindAllStringIndex(text, -1) {
		if span[0] > 0 {
			c := text[span[0]-1]
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-' {
				continue
			}
		}
		if span[1] < len(text) {
			c := text[span[1]]
			if c != ' ' && c != '\t' && c != '\n' && c != '\r' && c != '"' && c != '\'' && c != ';' && c != ')' && c != '}' && c != ',' && c != '\\' {
				continue
			}
		}
		out = append(out, text[span[0]:span[1]])
	}
	return out
}
