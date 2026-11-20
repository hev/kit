package trace

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hev/kit/internal/harness"
)

// GeminiSource reads Gemini CLI sessions from the project chat directories.
type GeminiSource struct{ Root string }

func (s *GeminiSource) nativeSource() (string, string) { return s.Root, harness.Gemini }
func (s *GeminiSource) Describe() string               { return "Gemini CLI sessions in " + s.Root }

func (s *GeminiSource) Units() ([]Unit, error) {
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
	registry := s.projects()
	var units []Unit
	for _, project := range projects {
		if !project.IsDir() {
			continue
		}
		dir := filepath.Join(s.Root, project.Name(), "chats")
		// Neither a project entry nor its chats child may redirect to another store.
		info, err := os.Lstat(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		workdirHash := sha256.Sum256([]byte(s.workdir(project.Name(), registry)))
		for _, entry := range entries {
			name := entry.Name()
			ext := filepath.Ext(name)
			if !entry.Type().IsRegular() || !strings.HasPrefix(name, "session-") || (ext != ".json" && ext != ".jsonl") {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			units = append(units, Unit{Key: filepath.Join(dir, name), Signature: fmt.Sprintf("%d:%d:%x", info.Size(), info.ModTime().UnixNano(), workdirHash[:8])})
		}
	}
	return units, nil
}

const geminiSessionLimit = 128 << 20

func geminiReadFile(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, errors.New("Gemini data is not a regular file within the size limit")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if int64(len(data)) > limit {
		return nil, errors.New("Gemini data exceeds size limit")
	}
	return data, err
}

func (s *GeminiSource) projects() map[string]string {
	var registry struct {
		Projects map[string]string `json:"projects"`
	}
	raw, err := geminiReadFile(filepath.Join(filepath.Dir(s.Root), "projects.json"), 4<<20)
	if err == nil {
		_ = json.Unmarshal(raw, &registry)
	}
	return registry.Projects
}

func (s *GeminiSource) workdir(project string, registry map[string]string) string {
	if raw, err := geminiReadFile(filepath.Join(s.Root, project, ".project_root"), 64<<10); err == nil {
		if path := strings.TrimSpace(string(raw)); filepath.IsAbs(path) {
			return path
		}
	}
	// A legacy SHA-256 directory can be recovered when its workspace has
	// since been registered under a slug. Without either record it is opaque.
	for path, slug := range registry {
		if filepath.IsAbs(path) && (slug == project || fmt.Sprintf("%x", sha256.Sum256([]byte(path))) == project) {
			return path
		}
	}
	return ""
}

type geminiTool struct {
	ID, Name, Status string
	Args, Result     json.RawMessage
}

type geminiMessage struct {
	ID, Type, Timestamp, Model string
	Content                    json.RawMessage
	Thoughts                   []struct{ Subject, Description string }
	ToolCalls                  []geminiTool
	Tokens                     struct{ Input, Output, Cached int64 }
}

type geminiConversation struct {
	SessionID, ProjectHash, Summary, Kind string
	Messages                              []geminiMessage
}

type geminiPatch struct {
	ID                  string
	Content             json.RawMessage
	ToolCalls           []geminiTool
	Updates             []geminiPatch
	RemoveIDs, OrderIDs []string
}

func geminiApplyMessagePatch(messages map[string]geminiMessage, patch geminiPatch) {
	msg, ok := messages[patch.ID]
	if !ok {
		return
	}
	if patch.Content != nil {
		msg.Content = patch.Content
	}
	for _, update := range patch.ToolCalls {
		if update.Result == nil {
			continue
		}
		for i := range msg.ToolCalls {
			if msg.ToolCalls[i].ID == update.ID {
				msg.ToolCalls[i].Result = update.Result
			}
		}
	}
	messages[patch.ID] = msg
}

func geminiApplyPatch(messages map[string]geminiMessage, ids []string, patch geminiPatch) []string {
	geminiApplyMessagePatch(messages, patch)
	for _, update := range patch.Updates {
		geminiApplyMessagePatch(messages, update)
	}
	for _, id := range patch.RemoveIDs {
		delete(messages, id)
	}
	reordered := make(map[string]bool)
	var tail []string
	for _, id := range patch.OrderIDs {
		if _, ok := messages[id]; ok && !reordered[id] {
			reordered[id] = true
			tail = append(tail, id)
		}
	}
	ids = slices.DeleteFunc(ids, func(id string) bool {
		_, ok := messages[id]
		return !ok || reordered[id]
	})
	return append(ids, tail...)
}

// geminiDecode handles snapshots and replays the append-only format's
// patches, checkpoints and rewinds before exposing any turns.
func geminiDecode(data []byte) (geminiConversation, error) {
	var session geminiConversation
	if json.Unmarshal(data, &session) == nil && session.SessionID != "" && session.ProjectHash != "" {
		return session, nil
	}
	session = geminiConversation{}
	messages := make(map[string]geminiMessage)
	var ids []string
	appendMessage := func(msg geminiMessage) {
		if msg.ID == "" || msg.Type == "" {
			return
		}
		if _, exists := messages[msg.ID]; !exists {
			ids = append(ids, msg.ID)
		}
		messages[msg.ID] = msg
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64<<10), geminiSessionLimit)
	for scanner.Scan() {
		var record struct {
			geminiMessage
			geminiConversation
			Set    json.RawMessage `json:"$set"`
			Patch  *geminiPatch    `json:"$patch"`
			Rewind *string         `json:"$rewindTo"`
		}
		if json.Unmarshal(scanner.Bytes(), &record) != nil {
			continue
		}
		switch {
		case record.Rewind != nil:
			at := max(0, slices.Index(ids, *record.Rewind))
			for _, id := range ids[at:] {
				delete(messages, id)
			}
			ids = ids[:at]
		case record.Patch != nil:
			ids = geminiApplyPatch(messages, ids, *record.Patch)
		case record.ID != "" && record.Type != "":
			appendMessage(record.geminiMessage)
		case record.Set != nil || (record.SessionID != "" && record.ProjectHash != ""):
			if record.Set != nil {
				// Decode into a copy to retain metadata fields omitted by this update.
				next := session
				if json.Unmarshal(record.Set, &next) != nil {
					continue
				}
				session = next
			} else {
				session = record.geminiConversation
			}
			if session.Messages != nil {
				if record.Set != nil {
					clear(messages)
					ids = ids[:0]
				}
				for _, msg := range session.Messages {
					appendMessage(msg)
				}
				session.Messages = nil
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return geminiConversation{}, errors.New("invalid Gemini JSONL record size")
	}
	if session.SessionID == "" || session.ProjectHash == "" {
		return geminiConversation{}, errors.New("missing Gemini session metadata")
	}
	for _, id := range ids {
		session.Messages = append(session.Messages, messages[id])
	}
	return session, nil
}

func geminiText(raw json.RawMessage, toolResult bool) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var parts []json.RawMessage
	if json.Unmarshal(raw, &parts) != nil {
		parts = []json.RawMessage{raw}
	}
	var out strings.Builder
	for _, part := range parts {
		var text string
		var structured struct {
			Text             string
			FunctionResponse *struct{ Response json.RawMessage }
		}
		if json.Unmarshal(part, &text) == nil {
			out.WriteString(text)
		} else if json.Unmarshal(part, &structured) == nil {
			out.WriteString(structured.Text)
			if toolResult && structured.FunctionResponse != nil {
				out.Write(structured.FunctionResponse.Response)
			}
		}
	}
	return out.String()
}

func (s *GeminiSource) Read(unit Unit) ([]Turn, error) {
	raw, err := geminiReadFile(unit.Key, geminiSessionLimit)
	if err != nil {
		return nil, errors.New("cannot read Gemini session transcript")
	}
	session, err := geminiDecode(raw)
	if err != nil {
		return nil, err
	}
	project := filepath.Base(filepath.Dir(filepath.Dir(unit.Key)))
	workdir := s.workdir(project, s.projects())
	var turns []Turn
	parent := ""
	for _, msg := range session.Messages {
		if msg.ID == "" || (msg.Type != "user" && msg.Type != "gemini") {
			continue
		}
		text := geminiText(msg.Content, false)
		if msg.Type == "user" {
			// Gemini stores generated context and tool responses as user
			// messages. The former can contain absolute workspace paths,
			// and the latter is already represented by the tool record.
			content := strings.TrimSpace(text)
			if content == "" || strings.HasPrefix(content, "<session_context>") || strings.HasPrefix(content, "<hook_context>") {
				continue
			}
		}
		turn := Turn{SessionID: session.SessionID, TurnUUID: msg.ID, ParentUUID: parent, Seq: int64(len(turns)), TS: msg.Timestamp,
			Role: "user", Harness: harness.Gemini, Workdir: workdir, SourcePath: unit.Key, Summary: session.Summary, IsSidechain: session.Kind == "subagent"}
		if msg.Type == "gemini" {
			turn.Role, turn.Model, turn.RequestID = "assistant", msg.Model, msg.ID
			turn.Usage = Usage{Input: msg.Tokens.Input, Output: msg.Tokens.Output, CacheRead: msg.Tokens.Cached}
		}
		for _, thought := range msg.Thoughts {
			if text := strings.TrimSpace(thought.Subject + "\n" + thought.Description); text != "" {
				turn.Blocks = append(turn.Blocks, Block{Type: "thinking", Text: text})
			}
		}
		if text != "" {
			turn.Blocks = append(turn.Blocks, Block{Type: "text", Text: text})
		}
		for _, tool := range msg.ToolCalls {
			turn.Blocks = append(turn.Blocks, Block{Type: "tool_use", ToolName: tool.Name, ToolUseID: tool.ID, Text: string(tool.Args)})
			if tool.Result != nil && string(tool.Result) != "null" {
				text := geminiText(tool.Result, true)
				if text == "" {
					text = string(tool.Result)
				}
				turn.Blocks = append(turn.Blocks, Block{Type: "tool_result", ToolName: tool.Name, ToolUseID: tool.ID, Text: text, IsError: tool.Status == "error"})
			}
		}
		turns = append(turns, turn)
		parent = msg.ID
	}
	return CaptureSkills(selectParsedRequests(turns)), nil
}
