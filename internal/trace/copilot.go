package trace

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hev/kit/internal/harness"
)

// CopilotSource reads the local CLI event log from each session directory.
type CopilotSource struct{ Root string }

func (s *CopilotSource) Describe() string { return "GitHub Copilot CLI sessions in " + s.Root }

func (s *CopilotSource) Units() ([]Unit, error) {
	if s.Root == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(s.Root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot list Copilot session directory")
	}
	var units []Unit
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(s.Root, entry.Name(), "events.jsonl")
		info, err := os.Lstat(path)
		if err == nil && info.Mode().IsRegular() {
			units = append(units, Unit{
				Key: path, Signature: fmt.Sprintf("%d-%d", info.Size(), info.ModTime().UnixNano()),
			})
		}
	}
	return units, nil
}

type copilotEvent struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Timestamp string          `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
}

func (s *CopilotSource) Read(u Unit) ([]Turn, error) {
	f, err := os.Open(u.Key)
	if err != nil {
		return nil, fmt.Errorf("cannot open Copilot event log")
	}
	defer f.Close()
	var session struct {
		ID      string `json:"sessionId"`
		Context struct {
			Cwd    string `json:"cwd"`
			Branch string `json:"branch"`
		} `json:"context"`
	}
	var turns []Turn
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64<<10), 16<<20)
	for line := 1; scanner.Scan(); line++ {
		var event copilotEvent
		if json.Unmarshal(scanner.Bytes(), &event) != nil {
			return nil, fmt.Errorf("Copilot event log line %d: invalid JSON", line)
		}
		if line == 1 {
			if event.Type != "session.start" || json.Unmarshal(event.Data, &session) != nil || session.ID == "" {
				return nil, fmt.Errorf("Copilot event log: missing session header")
			}
			continue
		}
		var role string
		switch event.Type {
		case "user.message":
			role = "user"
		case "assistant.message":
			role = "assistant"
		default:
			continue
		}
		var message struct {
			Content string `json:"content"`
		}
		if json.Unmarshal(event.Data, &message) != nil || event.ID == "" || event.Timestamp == "" {
			return nil, fmt.Errorf("Copilot event log line %d: invalid message", line)
		}
		if message.Content == "" {
			continue
		}
		turn := Turn{
			SessionID: session.ID, TurnUUID: event.ID, Seq: int64(line),
			TS: event.Timestamp, Role: role, Blocks: textBlock(message.Content),
			Workdir: session.Context.Cwd, Branch: session.Context.Branch,
			Harness: harness.Copilot, SourcePath: u.Key,
		}
		if len(turns) > 0 {
			turn.ParentUUID = turns[len(turns)-1].TurnUUID
		}
		turns = append(turns, turn)
	}
	if scanner.Err() != nil {
		return nil, fmt.Errorf("cannot scan Copilot event log")
	}
	if session.ID == "" {
		return nil, fmt.Errorf("Copilot event log: missing session header")
	}
	return turns, nil
}
