package trace

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// OpenCode 2.x session_message rows hold the Session.Message.Info JSON shapes
// from packages/schema/src/session-message.ts (verified against 2.2.6). The row
// type column names the shape; assistant content parts are inline in the
// message or, when experimental.part_rows moved them, session_part rows that
// come first.

type openCodeV2Error struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type openCodeV2Message struct {
	Time struct {
		Created   int64 `json:"created"`
		Completed int64 `json:"completed"`
	} `json:"time"`
	Text  string `json:"text"`
	Files []struct {
		Name string `json:"name"`
		Mime string `json:"mime"`
		Data string `json:"data"`
		URI  string `json:"uri"`
	} `json:"files"`
	Agents []struct {
		Name string `json:"name"`
	} `json:"agents"`
	Skills []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"skills"`
	Model struct {
		ID      string `json:"id"`
		Variant string `json:"variant"`
	} `json:"model"`
	Content []json.RawMessage `json:"content"`
	Cost    float64           `json:"cost"`
	Tokens  openCodeTokens    `json:"tokens"`
	Error   *openCodeV2Error  `json:"error"`
	// skill messages
	Skill string `json:"skill"`
	Name  string `json:"name"`
	// shell messages (a command the user ran with !)
	Command string   `json:"command"`
	Status  string   `json:"status"`
	Exit    *float64 `json:"exit"`
	Output  *struct {
		Output string `json:"output"`
	} `json:"output"`
}

type openCodeV2Part struct {
	Type  string `json:"type"`
	Text  string `json:"text"`
	ID    string `json:"id"`
	Name  string `json:"name"`
	State struct {
		Status  string          `json:"status"`
		Input   json.RawMessage `json:"input"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
			Mime string `json:"mime"`
			Name string `json:"name"`
			URI  string `json:"uri"`
		} `json:"content"`
		Error *openCodeV2Error `json:"error"`
	} `json:"state"`
	Time struct {
		Completed int64 `json:"completed"`
	} `json:"time"`
}

func openCodeV2ErrorText(e *openCodeV2Error) string {
	if e == nil || (e.Type == "" && e.Message == "") {
		return ""
	}
	return strings.TrimSpace("[" + e.Type + "] " + e.Message)
}

// openCodeSkillCall records a skill OpenCode loaded into context outside a
// model tool call, in the shape of its skill tool so skill capture sees it.
func openCodeSkillCall(id, name string) Block {
	if id == "" {
		id = name
	}
	input, _ := json.Marshal(map[string]string{"id": id})
	return Block{Type: "tool_use", Text: "skill " + string(input), ToolName: "skill"}
}

// v2 reads OpenCode 2.x session_message rows (in seq order) and session_part
// rows (by message, in ordinal order).
func (b *openCodeBuilder) v2(messages []openCodeRow, parts map[string][]openCodeRow) error {
	var user string
	for _, mr := range messages {
		var m openCodeV2Message
		if err := json.Unmarshal([]byte(mr.data), &m); err != nil {
			return fmt.Errorf("opencode message %s: invalid JSON", mr.id)
		}
		ts := openCodeTS(mr.created)
		switch mr.typ {
		case "user":
			user = mr.id
			t := Turn{TurnUUID: mr.id, TS: ts, Role: "user"}
			if m.Text != "" {
				t.Blocks = append(t.Blocks, Block{Type: "text", Text: m.Text})
			}
			for _, a := range m.Agents {
				t.Blocks = append(t.Blocks, Block{Type: "text", Text: "@" + a.Name})
			}
			// Attachments carry whole files; keep the name only, after the prompt.
			for _, f := range m.Files {
				// An inline image is decoded for capture; a "media:<sha256>"
				// reference points into OpenCode's own blob table and is
				// recorded as not inline.
				if im, ok := fileImage(0, f.Mime, f.Data, f.URI); ok {
					t.Blocks = append(t.Blocks, imageBlock(im))
					continue
				}
				name := f.Name
				if name == "" {
					name = f.Mime
				}
				t.Blocks = append(t.Blocks, Block{Type: "text", Text: "[file omitted: " + name + "]"})
			}
			b.add(t)
			// A skill attached to the prompt is loaded with it.
			for _, s := range m.Skills {
				b.add(Turn{TurnUUID: mr.id + "#skill-" + s.ID, ParentUUID: mr.id, TS: ts, Role: "system", Blocks: []Block{openCodeSkillCall(s.ID, s.Name)}})
			}
		case "assistant":
			if m.Time.Completed == 0 && !b.settled {
				// Still streaming: stop here so turn order stays a stable prefix.
				b.stopped = true
				return nil
			}
			effort := m.Model.Variant
			if effort == "default" {
				effort = ""
			}
			// OpenCode reports reasoning tokens separately; they bill as output.
			t := Turn{TurnUUID: mr.id, ParentUUID: user, TS: ts, Role: "assistant", Model: m.Model.ID, Effort: effort, RequestID: mr.id, Cost: m.Cost,
				Usage: Usage{Input: m.Tokens.Input, Output: m.Tokens.Output + m.Tokens.Reasoning, CacheRead: m.Tokens.Cache.Read, CacheCreation: m.Tokens.Cache.Write}}
			content := make([]json.RawMessage, 0, len(parts[mr.id])+len(m.Content))
			for _, pr := range parts[mr.id] {
				content = append(content, json.RawMessage(pr.data))
			}
			content = append(content, m.Content...)
			var after []Turn
			for i, raw := range content {
				var p openCodeV2Part
				if err := json.Unmarshal(raw, &p); err != nil {
					return fmt.Errorf("opencode message %s part %d: invalid JSON", mr.id, i)
				}
				switch p.Type {
				case "text":
					if p.Text != "" {
						t.Blocks = append(t.Blocks, Block{Type: "text", Text: p.Text})
					}
				case "reasoning":
					if p.Text != "" {
						t.Blocks = append(t.Blocks, Block{Type: "thinking", Text: p.Text})
					}
				case "tool":
					if p.State.Status == "streaming" {
						continue // the model is still writing the call's input
					}
					input := strings.TrimSpace(string(p.State.Input))
					if input == "" || input == "null" {
						input = "{}"
					}
					t.Blocks = append(t.Blocks, Block{Type: "tool_use", Text: p.Name + " " + input, ToolName: p.Name, ToolUseID: p.ID})
					if p.State.Status != "completed" && p.State.Status != "error" {
						continue // running: the call has no result yet
					}
					var out []string
					var images []Image
					for ci, c := range p.State.Content {
						im, isImage := fileImage(ci, c.Mime, "", c.URI)
						switch {
						case c.Type == "text":
							out = append(out, c.Text)
						case c.Type == "file" && isImage:
							images = append(images, im)
							out = append(out, im.Marker())
						case c.Name != "":
							out = append(out, "[file omitted: "+c.Name+"]")
						default:
							out = append(out, "[file omitted: "+c.Mime+"]")
						}
					}
					if e := openCodeV2ErrorText(p.State.Error); e != "" {
						out = append([]string{e}, out...)
					}
					id := p.ID
					if id == "" {
						id = strconv.Itoa(i)
					}
					rts := openCodeTS(p.Time.Completed)
					if rts == "" {
						rts = ts
					}
					after = append(after, Turn{TurnUUID: mr.id + "#" + id, ParentUUID: mr.id, TS: rts, Role: "tool", Model: m.Model.ID,
						Blocks: []Block{{Type: "tool_result", Text: strings.Join(out, "\n"), ToolName: p.Name, ToolUseID: p.ID, IsError: p.State.Status == "error", Images: images}}})
				default:
					return fmt.Errorf("opencode message %s part %d: unsupported type %q", mr.id, i, p.Type)
				}
			}
			if e := openCodeV2ErrorText(m.Error); e != "" {
				t.Blocks = append(t.Blocks, Block{Type: "text", Text: e})
			}
			b.add(t)
			for _, r := range after {
				b.add(r)
			}
		case "synthetic", "system":
			// Context OpenCode injected for the model, such as an attached file's
			// contents or a tool rename notice; not the user's request.
			if m.Text != "" {
				b.add(Turn{TurnUUID: mr.id, ParentUUID: user, TS: ts, Role: "system", Blocks: []Block{{Type: "text", Text: m.Text}}})
			}
		case "skill":
			b.add(Turn{TurnUUID: mr.id, ParentUUID: user, TS: ts, Role: "system", Blocks: []Block{openCodeSkillCall(m.Skill, m.Name)}})
		case "shell":
			input, _ := json.Marshal(map[string]string{"command": m.Command})
			b.add(Turn{TurnUUID: mr.id, TS: ts, Role: "user", Blocks: []Block{{Type: "tool_use", Text: "shell " + string(input), ToolName: "shell", ToolUseID: mr.id}}})
			if m.Status == "" || m.Status == "running" {
				continue
			}
			out := ""
			if m.Output != nil {
				out = m.Output.Output
			}
			failed := m.Status != "exited" || (m.Exit != nil && *m.Exit != 0)
			rts := openCodeTS(m.Time.Completed)
			if rts == "" {
				rts = ts
			}
			b.add(Turn{TurnUUID: mr.id + "#result", ParentUUID: mr.id, TS: rts, Role: "tool",
				Blocks: []Block{{Type: "tool_result", Text: out, ToolName: "shell", ToolUseID: mr.id, IsError: failed}}})
		case "compaction", "idle", "agent-switched", "model-switched", "location-switched":
			// Compaction summaries restate earlier turns; the rest is session state.
		default:
			return fmt.Errorf("opencode message %s: unsupported type %q", mr.id, mr.typ)
		}
	}
	return nil
}
