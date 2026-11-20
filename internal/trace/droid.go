package trace

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hev/kit/internal/harness"
)

// DroidSource reads Factory Droid's v2 project-partitioned session JSONL.
type DroidSource struct{ Root string }

func (s *DroidSource) Describe() string { return "Factory Droid sessions in " + s.Root }

func (s *DroidSource) Units() ([]Unit, error) {
	projects, err := os.ReadDir(s.Root)
	if s.Root == "" || os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var units []Unit
	for _, project := range projects {
		if !project.IsDir() {
			continue
		}
		dir := filepath.Join(s.Root, project.Name())
		files, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, file := range files {
			if !file.Type().IsRegular() || !strings.HasSuffix(file.Name(), ".jsonl") {
				continue
			}
			info, err := file.Info()
			if err == nil && info.Size() > 0 {
				units = append(units, Unit{
					Key:       filepath.Join(dir, file.Name()),
					Signature: fmt.Sprintf("%d:%d", info.Size(), info.ModTime().UnixNano()),
				})
			}
		}
	}
	return units, nil
}

type droidRecord struct {
	Type, ID, Title, Cwd, Timestamp string
	Version                         int
	Seq                             int64
	Message                         struct {
		Role, ParentID, Visibility, HookEventName string
		Content                                   []droidPart
	}
}

type droidPart struct {
	Type, Text, Thinking, ID, Name, ToolUseID string
	Input, Content                            json.RawMessage
	IsError                                   bool
}

func (s *DroidSource) Read(u Unit) ([]Turn, error) {
	f, err := os.Open(u.Key)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	var header droidRecord
	var turns []Turn
	for line := 1; sc.Scan(); line++ {
		var record droidRecord
		if err := json.Unmarshal(sc.Bytes(), &record); err != nil {
			// A session may be read while Droid is appending its final record.
			continue
		}
		if line == 1 {
			if record.Type != "session_start" || record.Version != 2 || record.ID == "" {
				return nil, fmt.Errorf("unsupported Droid session header")
			}
			header = record
			continue
		}
		m := record.Message
		if header.ID == "" || record.Type != "message" || record.ID == "" ||
			m.HookEventName != "" || m.Visibility == "user_only" {
			continue
		}
		switch m.Role {
		case "user", "assistant", "tool":
		default:
			continue
		}
		var blocks []Block
		for _, p := range m.Content {
			switch p.Type {
			case "text":
				blocks = append(blocks, Block{Type: "text", Text: p.Text})
			case "thinking":
				blocks = append(blocks, Block{Type: "thinking", Text: p.Thinking})
			case "tool_use":
				blocks = append(blocks, Block{Type: "tool_use", Text: p.Name + " " + string(p.Input), ToolName: p.Name, ToolUseID: p.ID})
			case "tool_result":
				blocks = append(blocks, Block{Type: "tool_result", Text: droidResultText(p.Content), ToolUseID: p.ToolUseID, IsError: p.IsError})
			}
		}
		if len(blocks) > 0 {
			turns = append(turns, Turn{
				SessionID: header.ID, Summary: header.Title, Workdir: header.Cwd,
				SourcePath: u.Key, Harness: harness.Droid, TurnUUID: record.ID,
				ParentUUID: m.ParentID, Seq: record.Seq, TS: record.Timestamp,
				Role: m.Role, Blocks: blocks,
			})
		}
	}
	return turns, sc.Err()
}

func droidResultText(raw json.RawMessage) string {
	var plain string
	if json.Unmarshal(raw, &plain) == nil {
		return plain
	}
	var parts []droidPart
	if json.Unmarshal(raw, &parts) != nil {
		return ""
	}
	var text []string
	for _, p := range parts {
		if p.Type == "text" {
			text = append(text, p.Text)
		}
	}
	return strings.Join(text, "\n")
}
