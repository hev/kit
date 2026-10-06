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

// DotSource reads hev dot voice-session traces. Dot writes one JSONL file per
// session under <root>/<YYYY-MM-DD>/<session>.jsonl, already redacted by dot.
type DotSource struct {
	Root string // ~/.dot/traces
}

// DefaultDotRoot is where dot keeps its date-partitioned session traces.
func DefaultDotRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".dot", "traces")
}

func (s *DotSource) Describe() string { return "dot traces in " + s.Root }

func (s *DotSource) Units() ([]Unit, error) {
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

type dotToolCall struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Args       json.RawMessage `json:"args"`
	Result     json.RawMessage `json:"result"`
	IsError    bool            `json:"is_error"`
	DurationMS *int64          `json:"duration_ms"`
}

type dotLine struct {
	SessionID string        `json:"session_id"`
	ID        string        `json:"id"`
	TS        string        `json:"ts"`
	Role      string        `json:"role"`
	Text      string        `json:"text"`
	Model     string        `json:"model"`
	ToolCalls []dotToolCall `json:"tool_calls"`
}

func (s *DotSource) Read(u Unit) ([]Turn, error) {
	f, err := os.Open(u.Key)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)

	var (
		turns    []Turn
		previous string
		lineNo   int
	)
	add := func(rec dotLine, uuid, role string, blocks []Block) {
		if len(blocks) == 0 {
			return
		}
		turns = append(turns, Turn{
			SessionID:  rec.SessionID,
			TurnUUID:   uuid,
			ParentUUID: previous,
			Seq:        int64(len(turns)),
			TS:         rec.TS,
			Role:       role,
			Blocks:     blocks,
			Workdir:    "dot",
			Harness:    "dot",
			SourcePath: u.Key,
			Model:      rec.Model,
		})
		previous = uuid
	}
	for sc.Scan() {
		lineNo++
		line := sc.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		var rec dotLine
		if err := json.Unmarshal(line, &rec); err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		if rec.SessionID == "" {
			rec.SessionID = strings.TrimSuffix(filepath.Base(u.Key), ".jsonl")
		}
		uuid := rec.ID
		if uuid == "" {
			uuid = fmt.Sprintf("%s:%d", u.Key, lineNo)
		}
		switch rec.Role {
		case "user":
			add(rec, uuid, "user", textBlock(rec.Text))
		case "assistant":
			blocks := textBlock(rec.Text)
			var results []Block
			for _, tc := range rec.ToolCalls {
				text := tc.Name
				if len(tc.Args) > 0 && string(tc.Args) != "null" {
					text += " " + codexToolInput(tc.Args)
				}
				result := codexText(tc.Result)
				// tool_result is not in the default index tiers, so the
				// latency and result size ride on the tool_use text too.
				meta := fmt.Sprintf("[result_bytes: %d", len(result))
				if tc.DurationMS != nil {
					meta += fmt.Sprintf(", duration_ms: %d", *tc.DurationMS)
				}
				text += "\n" + meta + "]"
				blocks = append(blocks, Block{Type: "tool_use", Text: text, ToolName: tc.Name, ToolUseID: tc.ID})
				if tc.DurationMS != nil {
					result = strings.TrimRight(result, "\n") + fmt.Sprintf("\n[duration_ms: %d]", *tc.DurationMS)
				}
				if strings.TrimSpace(result) != "" {
					results = append(results, Block{Type: "tool_result", Text: result, ToolUseID: tc.ID, IsError: tc.IsError})
				}
			}
			add(rec, uuid, "assistant", blocks)
			add(rec, uuid+":result", "tool", results)
		case "event":
			add(rec, uuid, "system", textBlock(rec.Text))
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return turns, nil
}
