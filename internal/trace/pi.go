package trace

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PiSource archives native PI v3 immutable history, not the active model context.
// Tree parents and context changes are retained as explicit pi_event blocks.
type PiSource struct{ Root string }

func DefaultPiRoot() string {
	if root := os.Getenv("HEV_PI_SESSIONS_DIR"); root != "" {
		return root
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".pi", "agent", "sessions")
}
func (s *PiSource) Describe() string       { return "native pi sessions in " + s.Root }
func (s *PiSource) Units() ([]Unit, error) { return (&DotSource{Root: s.Root}).Units() }

type piUsage struct {
	Input      int64 `json:"input"`
	Output     int64 `json:"output"`
	CacheRead  int64 `json:"cacheRead"`
	CacheWrite int64 `json:"cacheWrite"`
	Cost       struct {
		Total float64 `json:"total"`
	} `json:"cost"`
}
type piMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	Model      string          `json:"model"`
	Provider   string          `json:"provider"`
	ToolCallID string          `json:"toolCallId"`
	ToolName   string          `json:"toolName"`
	IsError    bool            `json:"isError"`
	Usage      piUsage         `json:"usage"`
	Command    string          `json:"command"`
	Output     string          `json:"output"`
}
type piEntry struct {
	Type             string          `json:"type"`
	Version          int             `json:"version,omitempty"`
	ID               string          `json:"id"`
	ParentID         string          `json:"parentId"`
	Timestamp        string          `json:"timestamp"`
	Cwd              string          `json:"cwd,omitempty"`
	ParentSession    string          `json:"parentSession,omitempty"`
	Provider         string          `json:"provider,omitempty"`
	ModelID          string          `json:"modelId,omitempty"`
	Model            string          `json:"model,omitempty"`
	Kind             string          `json:"kind,omitempty"`
	ThinkingLevel    string          `json:"thinkingLevel,omitempty"`
	CustomType       string          `json:"customType,omitempty"`
	Summary          string          `json:"summary,omitempty"`
	FirstKeptEntryID string          `json:"firstKeptEntryId,omitempty"`
	FromID           string          `json:"fromId,omitempty"`
	TargetID         string          `json:"targetId,omitempty"`
	Replacement      json.RawMessage `json:"replacement,omitempty"`
	Name             string          `json:"name,omitempty"`
	Label            string          `json:"label,omitempty"`
	Message          *piMessage      `json:"message,omitempty"`
	Content          json.RawMessage `json:"content,omitempty"`
	Usage            piUsage         `json:"usage"`
}

func piBlocks(content json.RawMessage) ([]Block, error) {
	if len(content) == 0 || string(content) == "null" {
		return nil, nil
	}
	var text string
	if json.Unmarshal(content, &text) == nil {
		return textBlock(text), nil
	}
	var parts []struct {
		Type      string          `json:"type"`
		Text      string          `json:"text"`
		Thinking  string          `json:"thinking"`
		ID        string          `json:"id"`
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(content, &parts); err != nil {
		return nil, fmt.Errorf("invalid message content")
	}
	var out []Block
	for _, p := range parts {
		switch p.Type {
		case "text":
			out = append(out, Block{Type: "text", Text: p.Text})
		case "thinking":
			out = append(out, Block{Type: "thinking", Text: p.Thinking})
		case "toolCall":
			out = append(out, Block{Type: "tool_use", Text: p.Name + " " + string(p.Arguments), ToolName: p.Name, ToolUseID: p.ID})
		case "image":
			out = append(out, Block{Type: "text", Text: "[image omitted]"})
		default:
			return nil, fmt.Errorf("unsupported content type %q", p.Type)
		}
	}
	return out, nil
}
func (s *PiSource) Read(u Unit) ([]Turn, error) {
	f, err := os.Open(u.Key)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	var header piEntry
	var turns []Turn
	entries := map[string]piEntry{}
	models := map[string]string{}
	providers := map[string]string{}
	efforts := map[string]string{}
	line := 0
	fail := func(reason string) ([]Turn, error) { return nil, fmt.Errorf("pi line %d: %s", line, reason) }
	for sc.Scan() {
		line++
		if strings.TrimSpace(sc.Text()) == "" {
			continue
		}
		var e piEntry
		if json.Unmarshal(sc.Bytes(), &e) != nil {
			return fail("invalid JSON (including incomplete final records)")
		}
		if header.ID == "" {
			if e.Type != "session" || e.Version != 3 || e.ID == "" || e.Timestamp == "" {
				return fail("expected native v3 session header")
			}
			header = e
			continue
		}
		if e.ID == "" || e.Timestamp == "" || e.Type == "session" {
			return fail("invalid entry identity")
		}
		if _, ok := entries[e.ID]; ok {
			return fail("duplicate entry ID")
		}
		if e.ParentID != "" {
			if _, ok := entries[e.ParentID]; !ok {
				return fail("unknown parent entry")
			}
		}
		model, provider, effort := models[e.ParentID], providers[e.ParentID], efforts[e.ParentID]
		if e.Type == "model_change" {
			model = e.ModelID
			provider = e.Provider
		}
		if e.Type == "thinking_level_change" {
			effort = e.ThinkingLevel
		}
		t := Turn{SessionID: header.ID, TurnUUID: e.ID, ParentUUID: e.ParentID, Seq: int64(len(turns)), TS: e.Timestamp, Role: "system", Workdir: header.Cwd, Harness: "pi", SourcePath: u.Key, Model: model, Effort: effort}
		usage := e.Usage
		switch e.Type {
		case "message":
			if e.Message == nil {
				return fail("missing message")
			}
			m := e.Message
			t.Role = m.Role
			switch m.Role {
			case "user", "assistant":
			case "toolResult":
				t.Role = "tool"
			case "bashExecution":
				t.Role = "tool"
			default:
				return fail("unsupported message role")
			}
			t.Blocks, err = piBlocks(m.Content)
			if err != nil {
				return fail(err.Error())
			}
			if m.Role == "toolResult" {
				for i := range t.Blocks {
					t.Blocks[i].Type = "tool_result"
					t.Blocks[i].ToolName = m.ToolName
					t.Blocks[i].ToolUseID = m.ToolCallID
					t.Blocks[i].IsError = m.IsError
				}
			}
			if m.Role == "bashExecution" {
				t.Blocks = []Block{{Type: "tool_use", ToolName: "bash", Text: m.Command}, {Type: "tool_result", ToolName: "bash", Text: m.Output}}
			}
			if m.Model != "" {
				t.Model = m.Model
			}
			if m.Provider != "" {
				provider = m.Provider
			}
			usage = m.Usage
		case "custom_message":
			t.Blocks, err = piBlocks(e.Content)
			if err != nil {
				return fail(err.Error())
			}
		case "usage":
			t.Model = e.Model
			provider = e.Provider
		case "compaction", "branch_summary":
			t.Blocks = textBlock(e.Summary)
		case "context_edit", "custom", "model_change", "thinking_level_change", "label", "session_info":
		default:
			return fail("unsupported entry type")
		}
		t.Usage = Usage{Input: usage.Input, Output: usage.Output, CacheRead: usage.CacheRead, CacheCreation: usage.CacheWrite}
		t.Cost = usage.Cost.Total
		if t.Role == "assistant" || e.Type == "usage" || usage.Input+usage.Output+usage.CacheRead+usage.CacheWrite > 0 {
			t.RequestID = e.ID
		}
		// An allowlisted event envelope retains branch/context provenance without
		// copying opaque extension state, image payloads, signatures or credentials.
		meta := e
		meta.Message = nil
		meta.Content = nil
		meta.Usage = piUsage{}
		envelope := struct {
			piEntry
			ProviderUsed      string `json:"providerUsed,omitempty"`
			ParentSessionPath string `json:"parentSessionPath,omitempty"`
			History           string `json:"view"`
		}{meta, provider, header.ParentSession, "immutable_history"}
		raw, _ := json.Marshal(envelope)
		t.Blocks = append(t.Blocks, Block{Type: "pi_event", Text: string(raw)})
		if e.Type == "session_info" {
			t.Summary = e.Name
		}
		entries[e.ID] = e
		models[e.ID] = model
		providers[e.ID] = provider
		efforts[e.ID] = effort
		turns = append(turns, t)
	}
	if sc.Err() != nil {
		return fail("record exceeds 16 MiB or read failed")
	}
	if header.ID == "" {
		return fail("missing session header")
	}
	return turns, nil
}
