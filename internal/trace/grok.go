package trace

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hev/kit/internal/harness"
)

// GrokSource reads the ACP update logs saved by Grok Build.
type GrokSource struct{ Root string }

func (s *GrokSource) Describe() string { return "Grok Build sessions in " + s.Root }

func (s *GrokSource) Units() ([]Unit, error) {
	if s.Root == "" {
		return nil, nil
	}
	paths, err := filepath.Glob(filepath.Join(s.Root, "*", "*", "updates.jsonl"))
	if err != nil {
		return nil, err
	}
	var units []Unit
	for _, path := range paths {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			continue
		}
		sig := fmt.Sprintf("%d:%d", info.Size(), info.ModTime().UnixNano())
		if summary, err := os.Stat(filepath.Join(filepath.Dir(path), "summary.json")); err == nil {
			sig += fmt.Sprintf(":%d:%d", summary.Size(), summary.ModTime().UnixNano())
		}
		units = append(units, Unit{Key: path, Signature: sig})
	}
	return units, nil
}

type grokUpdate struct {
	Kind    string          `json:"sessionUpdate"`
	CallID  string          `json:"toolCallId"`
	Name    string          `json:"name"`
	Title   string          `json:"title"`
	Status  string          `json:"status"`
	Input   json.RawMessage `json:"rawInput"`
	Output  json.RawMessage `json:"rawOutput"`
	Content json.RawMessage `json:"content"`
}

func (u grokUpdate) block() (string, Block) {
	switch u.Kind {
	case "user_message_chunk", "agent_message_chunk", "agent_thought_chunk":
		var content struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if json.Unmarshal(u.Content, &content) != nil || content.Type != "text" || content.Text == "" {
			return "", Block{}
		}
		switch u.Kind {
		case "user_message_chunk":
			return "user", Block{Type: "text", Text: content.Text}
		case "agent_thought_chunk":
			return "assistant", Block{Type: "thinking", Text: content.Text}
		default:
			return "assistant", Block{Type: "text", Text: content.Text}
		}
	case "tool_call":
		if u.Name == "" {
			u.Name = u.Title
		}
		return "assistant", Block{Type: "tool_use", ToolUseID: u.CallID, ToolName: u.Name, Text: string(u.Input)}
	case "tool_call_update":
		if u.Status == "completed" || u.Status == "failed" {
			return "tool", Block{Type: "tool_result", ToolUseID: u.CallID, Text: string(u.Output), IsError: u.Status == "failed"}
		}
	}
	return "", Block{}
}

func (s *GrokSource) Read(unit Unit) ([]Turn, error) {
	f, err := os.Open(unit.Key)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sessionDir := filepath.Dir(unit.Key)
	id := filepath.Base(sessionDir)
	var metadata struct {
		Info struct {
			Cwd string `json:"cwd"`
		} `json:"info"`
		Title string `json:"generated_title"`
	}
	if file, err := os.Open(filepath.Join(sessionDir, "summary.json")); err == nil {
		_ = json.NewDecoder(io.LimitReader(file, 1<<20)).Decode(&metadata)
		file.Close()
	}

	var turns []Turn
	var text strings.Builder
	flush := func() {
		if text.Len() != 0 {
			turns[len(turns)-1].Blocks[0].Text = text.String()
			text.Reset()
		}
	}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64<<10), 16<<20)
	for line := 1; scanner.Scan(); line++ {
		if strings.TrimSpace(scanner.Text()) == "" {
			continue
		}
		var record struct {
			Method      string `json:"method"`
			Timestamp   int64  `json:"timestamp"`
			TimestampUS int64  `json:"timestamp_us"`
			Params      struct {
				Update grokUpdate `json:"update"`
			} `json:"params"`
		}
		if json.Unmarshal(scanner.Bytes(), &record) != nil {
			return nil, fmt.Errorf("grok line %d: invalid session update JSON", line)
		}
		if record.Method != "session/update" {
			continue
		}
		role, block := record.Params.Update.block()
		if role == "" {
			continue
		}
		var timestamp string
		if record.TimestampUS != 0 {
			timestamp = time.UnixMicro(record.TimestampUS).UTC().Format(time.RFC3339Nano)
		} else if record.Timestamp != 0 {
			timestamp = time.Unix(record.Timestamp, 0).UTC().Format(time.RFC3339Nano)
		}
		// Merge adjacent streamed deltas before passing them to the chunker.
		if n := len(turns); n > 0 && (block.Type == "text" || block.Type == "thinking") {
			last := &turns[n-1]
			if last.Role == role && last.Blocks[0].Type == block.Type {
				text.WriteString(block.Text)
				continue
			}
		}
		flush()
		if block.Type == "text" || block.Type == "thinking" {
			text.WriteString(block.Text)
		}
		turns = append(turns, Turn{
			SessionID: id, TurnUUID: fmt.Sprintf("%s:update:%d", id, line),
			Seq: int64(len(turns)), TS: timestamp, Role: role, Blocks: []Block{block},
			Harness: harness.Grok, SourcePath: unit.Key, Workdir: metadata.Info.Cwd, Summary: metadata.Title,
		})
	}
	flush()
	return turns, scanner.Err()
}
