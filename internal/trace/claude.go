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

// ClaudeSource reads Claude Code session transcripts.
//
// This replaces reading ~/.hev/claude-code/raw-api-bodies. Those are wire
// bodies: mostly system prompt and headers, which is why a redaction pass over
// them left nothing behind (issue #8). The harness already writes a complete,
// structured transcript per session, and it is the better artifact by every
// measure — it has the turn threading, the tool results, the cwd and the git
// branch, and it does not contain the system prompt at all.
type ClaudeSource struct {
	Root string // ~/.claude/projects
}

// DefaultClaudeRoot is where Claude Code keeps transcripts, one directory per
// working directory with the path slugified into the name.
func DefaultClaudeRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "projects")
}

func (s *ClaudeSource) Describe() string { return "claude code transcripts in " + s.Root }

func (s *ClaudeSource) Units() ([]Unit, error) {
	var units []Unit
	err := filepath.WalkDir(s.Root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			// A tree we cannot read in full is still worth the part we can.
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
			Key: p,
			// Size and mtime: a transcript only ever grows, so this catches
			// every real change without hashing 110 MB on each pass.
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

// claudeLine is the subset of a transcript line this parser needs. Claude Code
// writes many line types — mode, attachment, file-history-snapshot, ai-title,
// cost-state and more — and only user and assistant carry a message. The rest
// are harness bookkeeping and are skipped rather than guessed at.
type claudeLine struct {
	Type        string          `json:"type"`
	UUID        string          `json:"uuid"`
	ParentUUID  string          `json:"parentUuid"`
	SessionID   string          `json:"sessionId"`
	Timestamp   string          `json:"timestamp"`
	CWD         string          `json:"cwd"`
	GitBranch   string          `json:"gitBranch"`
	IsSidechain bool            `json:"isSidechain"`
	RequestID   string          `json:"requestId"`
	Effort      string          `json:"effort"`
	Message     json.RawMessage `json:"message"`
	AITitle     string          `json:"aiTitle"`
}

type claudeMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
	Model   string          `json:"model"`
	Usage   struct {
		Input         int64 `json:"input_tokens"`
		Output        int64 `json:"output_tokens"`
		CacheRead     int64 `json:"cache_read_input_tokens"`
		CacheCreation int64 `json:"cache_creation_input_tokens"`
	} `json:"usage"`
}

func (s *ClaudeSource) Read(u Unit) ([]Turn, error) {
	f, err := os.Open(u.Key)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	// Tool results carry whole files and command output; the default 64 KB
	// token is far too small and would truncate a turn mid-JSON.
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)

	var (
		turns   []Turn
		seq     int64
		workdir = workdirOf(u.Key)
		titles  = map[string]string{}
	)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var cl claudeLine
		if err := json.Unmarshal(line, &cl); err != nil {
			continue // a malformed line loses itself, not the session
		}
		if cl.Type == "ai-title" && strings.TrimSpace(cl.AITitle) != "" {
			titles[cl.SessionID] = oneLine(cl.AITitle)
			continue
		}
		if cl.Type != "user" && cl.Type != "assistant" {
			continue
		}
		var msg claudeMessage
		if len(cl.Message) == 0 || json.Unmarshal(cl.Message, &msg) != nil {
			continue
		}
		blocks := normalizeBlocks(msg.Content)
		if len(blocks) == 0 {
			continue
		}
		role := msg.Role
		if role == "" {
			role = cl.Type
		}
		cwd := cl.CWD
		if cwd == "" {
			cwd = workdir
		}
		agentID := agentIDOf(u.Key)
		turns = append(turns, Turn{
			SessionID:   cl.SessionID,
			TurnUUID:    cl.UUID,
			ParentUUID:  cl.ParentUUID,
			Seq:         seq,
			TS:          cl.Timestamp,
			Role:        role,
			Blocks:      blocks,
			Workdir:     cwd,
			Branch:      cl.GitBranch,
			Harness:     "claude_code",
			SourcePath:  u.Key,
			IsSidechain: cl.IsSidechain || agentID != "",
			AgentID:     agentID,
			Model:       msg.Model,
			Effort:      cl.Effort,
			RequestID:   cl.RequestID,
			Usage: Usage{
				Input: msg.Usage.Input, Output: msg.Usage.Output,
				CacheRead: msg.Usage.CacheRead, CacheCreation: msg.Usage.CacheCreation,
			},
		})
		seq++
	}
	if err := sc.Err(); err != nil {
		return turns, err
	}
	for i := range turns {
		turns[i].Summary = titles[turns[i].SessionID]
	}
	return turns, nil
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func agentIDOf(p string) string {
	base := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
	if strings.HasPrefix(base, "agent-") {
		return strings.TrimPrefix(base, "agent-")
	}
	return ""
}

// workdirOf recovers a working directory from the transcript path for the rare
// line that records no cwd: Claude Code slugifies the path into the directory
// name under `projects`.
func workdirOf(p string) string {
	parts := strings.Split(filepath.ToSlash(p), "/")
	for i, s := range parts {
		if s == "projects" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return filepath.Base(filepath.Dir(p))
}

// normalizeBlocks flattens a message's content into typed blocks. Content is
// either a bare string (the common short user turn) or an array of typed
// blocks. Empty text is dropped: a block with nothing in it is not a chunk
// anyone can search for.
func normalizeBlocks(content json.RawMessage) []Block {
	if len(content) == 0 {
		return nil
	}
	var s string
	if json.Unmarshal(content, &s) == nil {
		if strings.TrimSpace(s) == "" {
			return nil
		}
		return []Block{{Type: "text", Text: s}}
	}

	var raw []map[string]json.RawMessage
	if json.Unmarshal(content, &raw) != nil {
		return nil
	}
	var out []Block
	for _, obj := range raw {
		var typ string
		json.Unmarshal(obj["type"], &typ)
		switch typ {
		case "text":
			if t := str(obj["text"]); strings.TrimSpace(t) != "" {
				out = append(out, Block{Type: "text", Text: t})
			}
		case "thinking":
			if t := str(obj["thinking"]); strings.TrimSpace(t) != "" {
				out = append(out, Block{Type: "thinking", Text: t})
			}
		case "tool_use":
			name := str(obj["name"])
			out = append(out, Block{
				Type:      "tool_use",
				Text:      renderToolUse(name, obj["input"]),
				ToolName:  name,
				ToolUseID: str(obj["id"]),
			})
		case "tool_result":
			t := flattenToolResult(obj["content"])
			if strings.TrimSpace(t) == "" {
				continue
			}
			out = append(out, Block{
				Type:      "tool_result",
				Text:      t,
				ToolUseID: str(obj["tool_use_id"]),
				IsError:   boolValue(obj["is_error"]),
			})
		}
	}
	return out
}

// renderToolUse keeps the part of a call that a person names when searching
// for it. Raw input JSON adds keys, punctuation, and escaped copies of prose
// to both the embedding and the terminal output; commands, paths, and queries
// carry the useful identity without that noise.
func renderToolUse(name string, input json.RawMessage) string {
	var fields map[string]json.RawMessage
	if json.Unmarshal(input, &fields) != nil {
		return name
	}

	var salient []string
	for _, key := range []string{"command", "path", "file_path", "query", "pattern"} {
		value := str(fields[key])
		if strings.TrimSpace(value) != "" && !contains(salient, value) {
			salient = append(salient, value)
		}
	}
	if len(salient) == 0 {
		return name
	}
	return name + " " + strings.Join(salient, " ")
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func boolValue(raw json.RawMessage) bool {
	var b bool
	_ = json.Unmarshal(raw, &b)
	return b
}

// flattenToolResult reduces a result's content to text. It arrives as a bare
// string, or as an array of blocks of which only the text ones carry anything
// searchable — an image block has no text and is skipped rather than rendered
// as a placeholder nobody would ever query for.
func flattenToolResult(content json.RawMessage) string {
	if len(content) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(content, &s) == nil {
		return s
	}
	var arr []map[string]json.RawMessage
	if json.Unmarshal(content, &arr) != nil {
		return ""
	}
	var parts []string
	for _, obj := range arr {
		var typ string
		json.Unmarshal(obj["type"], &typ)
		if typ == "text" {
			parts = append(parts, str(obj["text"]))
		}
	}
	return strings.Join(parts, "\n")
}

func str(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return ""
}
