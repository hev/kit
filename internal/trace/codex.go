package trace

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CodexSource reads Codex CLI rollout transcripts.
type CodexSource struct {
	Root string // ~/.codex/sessions
}

// DefaultCodexRoot is where Codex keeps its date-partitioned rollout JSONL.
func DefaultCodexRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".codex", "sessions")
}

func (s *CodexSource) Describe() string { return "codex rollouts in " + s.Root }

func (s *CodexSource) Units() ([]Unit, error) {
	var units []Unit
	err := filepath.WalkDir(s.Root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		units = append(units, Unit{
			Key:       p,
			Signature: fmt.Sprintf("%d-%d", info.Size(), info.ModTime().UnixNano()),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(units, func(i, j int) bool { return units[i].Key < units[j].Key })
	return units, nil
}

type codexLine struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type codexTokens struct {
	Input         int64 `json:"input_tokens"`
	Output        int64 `json:"output_tokens"`
	CacheRead     int64 `json:"cached_input_tokens"`
	CacheCreation int64 `json:"cache_write_input_tokens"`
}

type codexPayload struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	CallID    string          `json:"call_id"`
	TurnID    string          `json:"turn_id"`
	Role      string          `json:"role"`
	Message   string          `json:"message"`
	Text      string          `json:"text"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	Input     json.RawMessage `json:"input"`
	Output    json.RawMessage `json:"output"`
	Content   json.RawMessage `json:"content"`
	Summary   json.RawMessage `json:"summary"`

	SessionID  string `json:"session_id"`
	CWD        string `json:"cwd"`
	CLIVersion string `json:"cli_version"`
	Model      string `json:"model"`
	Effort     string `json:"effort"`
	Info       *struct {
		LastTokenUsage  *codexTokens `json:"last_token_usage"`
		TotalTokenUsage *codexTokens `json:"total_token_usage"`
	} `json:"info"`
}

func (s *CodexSource) Read(u Unit) ([]Turn, error) {
	sourcePath := u.Key
	f, err := os.Open(u.Key)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)

	var (
		turns      []Turn
		sessionID  string
		workdir    string
		previous   string
		model      string
		effort     string
		requestID  string
		lineNo     int
		pending    int
		cumulative codexTokens
	)
	for sc.Scan() {
		lineNo++
		line := sc.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		var rec codexLine
		if err := json.Unmarshal(line, &rec); err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		var p codexPayload
		if len(rec.Payload) > 0 {
			if err := json.Unmarshal(rec.Payload, &p); err != nil {
				return nil, fmt.Errorf("line %d payload: %w", lineNo, err)
			}
		}
		if rec.Type == "session_meta" {
			sessionID = p.SessionID
			if sessionID == "" {
				sessionID = p.ID
			}
			workdir = p.CWD
			model = p.Model
			continue
		}
		if rec.Type == "turn_context" {
			if p.CWD != "" {
				workdir = p.CWD
			}
			if p.Model != "" {
				model = p.Model
			}
			effort = p.Effort
			requestID = p.TurnID
			continue
		}
		if rec.Type == "event_msg" && p.Type == "token_count" {
			if p.Info != nil {
				u := p.Info.LastTokenUsage
				if total := p.Info.TotalTokenUsage; total != nil {
					delta := codexTokens{Input: total.Input - cumulative.Input, Output: total.Output - cumulative.Output, CacheRead: total.CacheRead - cumulative.CacheRead, CacheCreation: total.CacheCreation - cumulative.CacheCreation}
					if delta.Input < 0 || delta.Output < 0 || delta.CacheRead < 0 || delta.CacheCreation < 0 {
						// A counter reset, not corruption: a resumed Codex
						// session restarts its running total from zero, so the
						// new total is all usage since the reset. Treat it the
						// way Prometheus treats a counter reset. Rejecting it
						// dropped the whole session from the index.
						delta = *total
					}
					cumulative = *total
					if delta == (codexTokens{}) {
						continue
					}
					u = &delta
				}
				if u != nil {
					if u.Input < u.CacheRead {
						return nil, fmt.Errorf("line %d: cached input exceeds input", lineNo)
					}
					normalized := Usage{Input: u.Input - u.CacheRead, Output: u.Output, CacheRead: u.CacheRead, CacheCreation: u.CacheCreation}
					rid := fmt.Sprintf("%s:usage:%d", sessionID, lineNo)
					if p.Info.TotalTokenUsage == nil && requestID != "" {
						rid = requestID
					}
					found := false
					for i := pending; i < len(turns); i++ {
						if turns[i].Role == "assistant" {
							turns[i].Usage = normalized
							turns[i].RequestID = rid
							found = true
						}
					}
					if !found {
						turns = append(turns, Turn{SessionID: sessionID, TurnUUID: rid, Seq: int64(len(turns)), TS: rec.Timestamp, Role: "assistant", Harness: "codex", Workdir: workdir, SourcePath: sourcePath, Model: model, RequestID: rid, Usage: normalized})
					}
					pending = len(turns)
				}
			}
			continue
		}

		role, uuid, blocks := codexBlocks(rec.Type, p)
		if len(blocks) == 0 {
			continue
		}
		if uuid == "" {
			uuid = p.TurnID
		}
		if uuid == "" {
			uuid = fmt.Sprintf("%s:%d", u.Key, lineNo)
		}
		turns = append(turns, Turn{
			SessionID:  sessionID,
			TurnUUID:   uuid,
			ParentUUID: previous,
			Seq:        int64(len(turns)),
			TS:         rec.Timestamp,
			Role:       role,
			Blocks:     blocks,
			Workdir:    workdir,
			Harness:    "codex",
			SourcePath: u.Key,
			Model:      model,
			Effort:     effort,
			RequestID:  requestID,
		})
		previous = uuid
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return turns, nil
}

func codexBlocks(recordType string, p codexPayload) (string, string, []Block) {
	switch recordType {
	case "event_msg":
		switch p.Type {
		case "user_message":
			return "user", p.TurnID, textBlock(p.Message)
		case "agent_message":
			return "assistant", p.TurnID, textBlock(p.Message)
		case "agent_reasoning":
			return "assistant", p.TurnID, typedTextBlock("thinking", p.Text)
		}
	case "response_item":
		switch p.Type {
		case "message":
			if p.Role != "user" && p.Role != "assistant" {
				return "", "", nil
			}
			return p.Role, p.ID, codexMessageBlocks(p.Content)
		case "reasoning":
			return "assistant", p.ID, codexReasoningBlocks(p.Summary)
		case "function_call", "custom_tool_call":
			input := p.Arguments
			if len(input) == 0 {
				input = p.Input
			}
			text := p.Name
			if len(input) > 0 {
				text += " " + compactJSON(input)
			}
			return "assistant", p.ID, []Block{{Type: "tool_use", Text: text, ToolName: p.Name, ToolUseID: p.CallID}}
		case "function_call_output", "custom_tool_call_output":
			text := codexText(p.Output)
			return "tool", p.ID, typedToolResult(text, p.CallID)
		}
	}
	return "", "", nil
}

func textBlock(s string) []Block { return typedTextBlock("text", s) }

func typedTextBlock(typ, s string) []Block {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return []Block{{Type: typ, Text: s}}
}

func typedToolResult(s, id string) []Block {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return []Block{{Type: "tool_result", Text: s, ToolUseID: id}}
}

func codexMessageBlocks(raw json.RawMessage) []Block {
	var items []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &items) != nil {
		return textBlock(codexText(raw))
	}
	var blocks []Block
	for _, item := range items {
		if item.Type == "input_text" || item.Type == "output_text" || item.Type == "text" {
			blocks = append(blocks, textBlock(item.Text)...)
		}
	}
	return blocks
}

func codexReasoningBlocks(raw json.RawMessage) []Block {
	var items []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &items) != nil {
		return typedTextBlock("thinking", codexText(raw))
	}
	var blocks []Block
	for _, item := range items {
		blocks = append(blocks, typedTextBlock("thinking", item.Text)...)
	}
	return blocks
}

func codexText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var items []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &items) == nil {
		var parts []string
		for _, item := range items {
			if item.Text != "" {
				parts = append(parts, item.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return compactJSON(raw)
}

func compactJSON(raw json.RawMessage) string {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return string(raw)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return string(raw)
	}
	return string(b)
}
