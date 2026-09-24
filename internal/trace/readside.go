package trace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"
)

// BlockRow is one whole transcript block in <namespace>-blocks. Text is never
// split here: this namespace is the lossless, ordered read side, while Chunk
// remains the retrieval shape.
type BlockRow struct {
	ID   string `json:"id"`
	Text string `json:"text"`

	SessionID     string `json:"session_id"`
	TurnUUID      string `json:"turn_uuid"`
	Seq           int64  `json:"seq"`
	Role          string `json:"role"`
	BlockType     string `json:"block_type"`
	ToolUseID     string `json:"tool_use_id,omitempty"`
	AgentID       string `json:"agent_id"`
	ParentAgentID string `json:"parent_agent_id"`

	Start    int64  `json:"start"`
	End      int64  `json:"end"`
	MS       int64  `json:"ms"`
	ToolName string `json:"tool_name,omitempty"`
	OK       bool   `json:"ok"`

	Model               string  `json:"model,omitempty"`
	Effort              string  `json:"effort,omitempty"`
	RequestID           string  `json:"request_id,omitempty"`
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	Cost                float64 `json:"cost"`
}

// ToolCounts accepts Layer's JSON string attribute and exposes an object to API clients.
type ToolCounts map[string]int

func (c *ToolCounts) UnmarshalJSON(raw []byte) error {
	if len(raw) > 0 && raw[0] == '"' {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return err
		}
		raw = []byte(text)
	}
	type counts ToolCounts
	return json.Unmarshal(raw, (*counts)(c))
}

// SessionRow is the compact trace-list and statistics shape in
// <namespace>-sessions. Summary prefers a harness title and is filled by the
// summaries pass otherwise; FirstPrompt supplies that pass when the local
// transcript has already aged out.
type SessionRow struct {
	ToolCounts       ToolCounts `json:"tool_counts"`
	ID               string     `json:"id"`
	SessionID        string     `json:"session_id"`
	Summary          string     `json:"summary"`
	FirstPrompt      string     `json:"first_prompt"`
	FirstPromptShort string     `json:"first_prompt_short"`
	Harness          string     `json:"harness"`
	Model            string     `json:"model"`
	RepoURL          string     `json:"repo_url"`
	Branch           string     `json:"branch"`
	Host             string     `json:"host"`
	Start            int64      `json:"start"`
	End              int64      `json:"end"`
	WallMS           int64      `json:"wall_ms"`
	APIMS            int64      `json:"api_ms"`
	IdleMS           int64      `json:"idle_ms"`
	PromptTS         []uint64   `json:"prompt_ts"`
	ToolNames        []string   `json:"tool_names"`
	TotalTokens      int64      `json:"total_tokens"`
	PromptCount      int64      `json:"prompt_count"`
	ToolCount        int64      `json:"tool_count"`
	RequestCount     int64      `json:"request_count"`

	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	Cost                float64 `json:"cost"`
	HasSubagents        bool    `json:"has_subagents"`
}

// Blocks converts turns to stable, globally ordered whole-block rows. A tool
// result closes the matching tool-use span without being folded into it: both
// rows remain present, so sorting by Seq reconstructs the transcript exactly.
func Blocks(turns []Turn) []BlockRow {
	var rows []BlockRow
	openTools := map[string]int{}
	var previousTS int64
	for _, turn := range turns {
		ts := timestampMS(turn.TS)
		start := ts
		if turn.Role == "assistant" && previousTS != 0 {
			start = previousTS
		}
		for blockIndex, block := range turn.Blocks {
			row := BlockRow{
				Text: block.Text, SessionID: turn.SessionID, TurnUUID: turn.TurnUUID,
				Seq: int64(len(rows)), Role: turn.Role, BlockType: block.Type,
				ToolUseID: block.ToolUseID, AgentID: turn.AgentID,
				ParentAgentID: turn.ParentAgentID, Start: start, End: ts,
				ToolName: block.ToolName, OK: !block.IsError,
				Model: turn.Model, Effort: turn.Effort, RequestID: turn.RequestID,
				InputTokens: turn.Usage.Input, OutputTokens: turn.Usage.Output,
				CacheReadTokens:     turn.Usage.CacheRead,
				CacheCreationTokens: turn.Usage.CacheCreation,
				Cost:                requestCost(turn.Model, turn.Usage),
			}
			if block.Type == "tool_use" || block.Type == "tool_result" {
				row.Start = ts
				row.End = ts
			}
			if row.End >= row.Start {
				row.MS = row.End - row.Start
			}
			row.ID = readID(turn.SessionID, turn.TurnUUID, block.Type, blockIndex, block.Text)
			rows = append(rows, row)
			if block.Type == "tool_use" && block.ToolUseID != "" {
				openTools[block.ToolUseID] = len(rows) - 1
			}
			if block.Type == "tool_result" && block.ToolUseID != "" {
				if i, ok := openTools[block.ToolUseID]; ok {
					rows[i].End = ts
					if ts >= rows[i].Start {
						rows[i].MS = ts - rows[i].Start
					}
					rows[i].OK = !block.IsError
					delete(openTools, block.ToolUseID)
				}
			}
		}
		if ts != 0 {
			previousTS = ts
		}
	}
	return rows
}

// Sessions aggregates turns by session id. repoURL is kept outside the parser
// because resolving a git remote is index-time environment work, not parsing.
func Sessions(turns []Turn, repoURL func(workdir string) string, host string) []SessionRow {
	groups := map[string][]Turn{}
	for _, turn := range turns {
		if turn.SessionID != "" {
			groups[turn.SessionID] = append(groups[turn.SessionID], turn)
		}
	}
	ids := make([]string, 0, len(groups))
	for id := range groups {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	out := make([]SessionRow, 0, len(ids))
	for _, id := range ids {
		rows := groups[id]
		s := SessionRow{ID: id, SessionID: id, Host: host, PromptTS: []uint64{}, ToolNames: []string{}}
		toolNames := map[string]bool{}
		counts := ToolCounts{}
		requests := map[string]Turn{}
		modelTokens := map[string]int64{}
		var previous int64
		for _, turn := range rows {
			if s.Summary == "" && strings.TrimSpace(turn.Summary) != "" {
				s.Summary = strings.TrimSpace(turn.Summary)
			}
			ts := timestampMS(turn.TS)
			if ts != 0 && (s.Start == 0 || ts < s.Start) {
				s.Start = ts
			}
			if ts > s.End {
				s.End = ts
			}
			if s.Harness == "" {
				s.Harness = turn.Harness
			}
			if turn.Branch != "" {
				s.Branch = turn.Branch
			}
			if s.RepoURL == "" && repoURL != nil {
				s.RepoURL = repoURL(turn.Workdir)
			}
			if turn.AgentID != "" || turn.IsSidechain {
				s.HasSubagents = true
			}

			hasPrompt := false
			for _, block := range turn.Blocks {
				switch block.Type {
				case "tool_use":
					s.ToolCount++
					if block.ToolName != "" {
						toolNames[block.ToolName] = true
						counts[block.ToolName]++
					}
				case "text":
					if turn.Role == "user" && strings.TrimSpace(block.Text) != "" {
						hasPrompt = true
						if s.FirstPrompt == "" {
							s.FirstPrompt = block.Text
							s.FirstPromptShort = ShortPrompt(block.Text)
						}
					}
				}
			}
			if hasPrompt {
				s.PromptCount++
				if ts > 0 {
					s.PromptTS = append(s.PromptTS, uint64(ts))
				}
				if previous != 0 && ts >= previous {
					s.IdleMS += ts - previous
				}
			}
			if turn.Role == "assistant" {
				rid := turn.RequestID
				if rid == "" {
					rid = turn.TurnUUID
				}
				old, found := requests[rid]
				if !found || turn.Usage.Output >= old.Usage.Output {
					requests[rid] = turn
				}
			}
			if ts != 0 {
				previous = ts
			}
		}
		for _, turn := range requests {
			s.RequestCount++
			s.InputTokens += turn.Usage.Input
			s.OutputTokens += turn.Usage.Output
			s.CacheReadTokens += turn.Usage.CacheRead
			s.CacheCreationTokens += turn.Usage.CacheCreation
			s.Cost += requestCost(turn.Model, turn.Usage)
			modelTokens[turn.Model] += turn.Usage.Input + turn.Usage.Output + turn.Usage.CacheRead + turn.Usage.CacheCreation
		}
		for model, tokens := range modelTokens {
			if s.Model == "" || tokens > modelTokens[s.Model] {
				s.Model = model
			}
		}
		blocks := Blocks(rows)
		for _, block := range blocks {
			if block.Role == "assistant" && block.MS > 0 {
				// Only one block per request contributes time.
				rid := block.RequestID
				if rid == "" {
					rid = block.TurnUUID
				}
				if _, ok := requests[rid]; ok {
					s.APIMS += block.MS
					delete(requests, rid)
				}
			}
		}
		if s.End >= s.Start {
			s.WallMS = s.End - s.Start
		}
		s.TotalTokens = s.InputTokens + s.OutputTokens + s.CacheReadTokens + s.CacheCreationTokens
		for name := range toolNames {
			s.ToolNames = append(s.ToolNames, name)
		}
		sort.Strings(s.ToolNames)
		names := append([]string{}, s.ToolNames...)
		sort.Slice(names, func(i, j int) bool {
			if counts[names[i]] == counts[names[j]] {
				return names[i] < names[j]
			}
			return counts[names[i]] > counts[names[j]]
		})
		s.ToolCounts = ToolCounts{}
		for _, name := range names[:min(6, len(names))] {
			s.ToolCounts[name] = counts[name]
		}
		sort.Slice(s.PromptTS, func(i, j int) bool { return s.PromptTS[i] < s.PromptTS[j] })
		out = append(out, s)
	}
	return out
}

func timestampMS(s string) int64 {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return 0
	}
	return t.UnixMilli()
}

func readID(parts ...any) string {
	h := sha256.New()
	for _, p := range parts {
		switch v := p.(type) {
		case string:
			h.Write([]byte(v))
		case int:
			h.Write([]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)})
		}
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:32]
}

// These are the provisional list rates used by the reviewed RFC 0004 mock.
// Unknown Claude models intentionally use the conservative Opus rate. A future
// rate table is an RFC concern; keeping the calculation here makes it explicit.
func requestCost(model string, u Usage) float64 {
	in, out, create, read := 15.0, 75.0, 30.0, 1.50
	lower := strings.ToLower(model)
	if strings.Contains(lower, "sonnet") {
		in, out, create, read = 3, 15, 6, .3
	}
	if strings.Contains(lower, "haiku") {
		in, out, create, read = 1, 5, 2, .1
	}
	return (float64(u.Input)*in + float64(u.Output)*out + float64(u.CacheCreation)*create + float64(u.CacheRead)*read) / 1_000_000
}

// ShortPrompt bounds previews by Unicode characters, never splitting UTF-8.
func ShortPrompt(s string) string {
	chars := 0
	for i := range s {
		if chars == 600 {
			return s[:i]
		}
		chars++
	}
	return s
}
