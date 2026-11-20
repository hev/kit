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

// CursorSource reads exported agent transcripts below the Cursor projects root.
type CursorSource struct{ Root string }

func (s *CursorSource) Describe() string { return "Cursor transcripts in " + s.Root }

func (s *CursorSource) Units() ([]Unit, error) {
	if s.Root == "" {
		return nil, nil
	}
	projects, err := os.ReadDir(s.Root)
	if os.IsNotExist(err) {
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
		root := filepath.Join(s.Root, project.Name(), "agent-transcripts")
		found, err := (&DotSource{Root: root}).Units()
		if err != nil {
			return nil, err
		}
		for _, unit := range found {
			info, err := os.Lstat(unit.Key)
			if err == nil && info.Mode().IsRegular() {
				units = append(units, unit)
			}
		}
	}
	return units, nil
}

func (s *CursorSource) Read(u Unit) ([]Turn, error) {
	f, err := os.Open(u.Key)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	session := strings.TrimSuffix(filepath.Base(u.Key), ".jsonl")
	var turns []Turn
	var previous string
	for line := 1; sc.Scan(); line++ {
		var row struct {
			Role    string
			Message struct {
				Content []struct {
					Type, Text, Name string
					Input            json.RawMessage
				}
			}
		}
		if strings.TrimSpace(sc.Text()) == "" {
			continue
		}
		if err := json.Unmarshal(sc.Bytes(), &row); err != nil {
			return nil, fmt.Errorf("cursor transcript line %d: invalid JSON record", line)
		}
		if row.Role != "user" && row.Role != "assistant" {
			continue
		}
		var blocks []Block
		for _, item := range row.Message.Content {
			switch item.Type {
			case "text":
				blocks = append(blocks, textBlock(item.Text)...)
			case "tool_use":
				if row.Role == "assistant" && item.Name != "" {
					blocks = append(blocks, Block{Type: "tool_use", ToolName: item.Name, Text: item.Name + " " + codexToolInput(item.Input)})
				}
			}
		}
		if len(blocks) == 0 {
			continue
		}
		id := fmt.Sprintf("%s:%d", session, line)
		turns = append(turns, Turn{
			SessionID: session, TurnUUID: id, ParentUUID: previous, Seq: int64(len(turns)),
			Role: row.Role, Blocks: blocks, Harness: harness.Cursor, SourcePath: u.Key,
		})
		previous = id
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return CaptureSkills(selectParsedRequests(turns)), nil
}
