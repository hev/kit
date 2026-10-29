package trace

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
)

// SkillLoad records transcript evidence, never the current filesystem revision.
// Empty path/source/commit means the harness did not supply that information.
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

func (s *SkillLoads) UnmarshalJSON(b []byte) error {
	b, err := unquoted(b)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, (*[]SkillLoad)(s))
}

var skillPath = regexp.MustCompile(`(?:~?/|\./)?[^\s"'` + "`" + `<>;|(){}]*skills/[^\s/"'` + "`" + `<>]+(?:/[^\s/"'` + "`" + `<>]+)*/SKILL\.md`)
var skillRead = regexp.MustCompile(`(?:^|[\s;|&"'])\b(?:cat|head|sed|less|more|read_file)\b`)
var skillCommand = regexp.MustCompile(`<command-name>/([^<\s]+)</command-name>`)

func captureSkills(turns []Turn) []Turn {
	for i := range turns {
		t := &turns[i]
		for j := range t.Blocks {
			b := &t.Blocks[j]
			var loads []SkillLoad
			add := func(name, path string) {
				source := ""
				if k := strings.LastIndex(path, "/skills/"); k >= 0 {
					source = path[:k+len("/skills")]
				} else if strings.HasPrefix(path, "skills/") {
					source = "skills"
				}
				loads = append(loads, SkillLoad{Name: name, Path: path, Source: source, TurnUUID: t.TurnUUID, Seq: t.Seq, TS: t.TS})
			}
			lower := strings.ToLower(b.ToolName)
			if b.Type == "tool_use" {
				if lower == "skill" {
					input := strings.TrimSpace(strings.TrimPrefix(b.Text, b.ToolName))
					var obj map[string]any
					if json.Unmarshal([]byte(input), &obj) == nil {
						if name, ok := obj["skill"].(string); ok && name != "" {
							add(name, "")
						}
					}
				}
				read := strings.Contains(lower, "read") || ((strings.Contains(lower, "exec") || lower == "bash" || lower == "shell") && skillRead.MatchString(b.Text))
				if read {
					seen := map[string]bool{}
					for _, path := range skillPath.FindAllString(b.Text, -1) {
						if !strings.Contains(path, "/skills/") && !strings.HasPrefix(path, "skills/") {
							continue
						}
						if seen[path] {
							continue
						}
						seen[path] = true
						parts := strings.Split(path, "/")
						add(parts[len(parts)-2], path)
					}
				}
			}
			// Claude records expanded slash commands with explicit skill provenance.
			if t.Harness == "claude_code" && t.Role == "user" && b.Type == "text" && strings.Contains(b.Text, "<command-message>") && strings.Contains(b.Text, "Base directory for this skill:") {
				if m := skillCommand.FindStringSubmatch(b.Text); len(m) > 0 {
					path := ""
					if paths := skillPath.FindAllString(b.Text, -1); len(paths) > 0 {
						path = paths[0]
					}
					add(m[1], path)
				}
			}
			b.SkillLoads = loads
		}
	}
	return turns
}

func sessionSkills(turns []Turn) []string {
	seen := map[string]bool{}
	for _, t := range turns {
		for _, b := range t.Blocks {
			for _, s := range b.SkillLoads {
				seen[s.Name] = true
			}
		}
	}
	out := []string{}
	for s := range seen {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
