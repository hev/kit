package trace

import (
	"bufio"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/hev/kit/internal/harness"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// AgySource reads Antigravity CLI (agy) session transcripts from
// ~/.gemini/antigravity-cli/brain/<conversation-id>/.system_generated/logs/
// (preferring transcript_full.jsonl and falling back to transcript.jsonl).
// Optional session metadata (workspace path, title, parent subagent link) is
// read from conversation_summaries.db when present.
type AgySource struct{ Root string }

// DefaultAgyRoot returns the Antigravity CLI data directory: $HEV_AGY_DATA_DIR,
// else ~/.gemini/antigravity-cli.
func DefaultAgyRoot() string {
	return defaultRoot(harness.Agy)
}

func (s *AgySource) Describe() string { return "Antigravity (agy) sessions in " + s.Root }

type agyMeta struct {
	Title        string
	Workdir      string
	ParentID     string
	NestingDepth int
	AgentName    string
}

func (m agyMeta) isSidechain() bool {
	return m.ParentID != "" || m.NestingDepth > 0
}

func (m agyMeta) sigSuffix() string {
	if m.Title == "" && m.Workdir == "" && m.ParentID == "" && m.NestingDepth == 0 {
		return ""
	}
	h := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%s\x00%d", m.Title, m.Workdir, m.ParentID, m.NestingDepth)))
	return ":" + hex.EncodeToString(h[:6])
}

// loadAgyMeta reads optional conversation metadata from conversation_summaries.db.
// Missing or unreadable databases are ignored so transcript capture still works.
func (s *AgySource) loadAgyMeta() map[string]agyMeta {
	dbPath := filepath.Join(s.Root, "conversation_summaries.db")
	if _, err := os.Stat(dbPath); err != nil {
		return nil
	}
	out := map[string]agyMeta{}
	_ = openCodeRead(dbPath, func(tx *sql.Tx) error {
		out = map[string]agyMeta{}
		rows, err := tx.Query(`SELECT conversation_id, title, workspace_uris, parent_conversation_id, nesting_depth, agent_name FROM conversation_summaries`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id, title, uris, parentID, agentName string
			var depth int
			if err := rows.Scan(&id, &title, &uris, &parentID, &depth, &agentName); err != nil {
				continue
			}
			out[id] = agyMeta{
				Title:        strings.TrimSpace(title),
				Workdir:      agyWorkspacePath(uris),
				ParentID:     strings.TrimSpace(parentID),
				NestingDepth: depth,
				AgentName:    strings.TrimSpace(agentName),
			}
		}
		// Fill any parent links present only in conversation_parents.
		prows, err := tx.Query(`SELECT conversation_id, parent_id FROM conversation_parents`)
		if err == nil {
			defer prows.Close()
			for prows.Next() {
				var id, parentID string
				if err := prows.Scan(&id, &parentID); err != nil {
					continue
				}
				parentID = strings.TrimSpace(parentID)
				if parentID == "" {
					continue
				}
				m := out[id]
				if m.ParentID == "" {
					m.ParentID = parentID
					out[id] = m
				}
			}
		}
		return nil
	})
	return out
}

// agyWorkspacePath decodes the first file:// URI from conversation_summaries.workspace_uris
// (a JSON array such as ["file:///Users/hev/workspace/lyr/kit"]).
func agyWorkspacePath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var uris []string
	if json.Unmarshal([]byte(raw), &uris) != nil {
		return ""
	}
	for _, u := range uris {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		parsed, err := url.Parse(u)
		if err != nil || parsed.Scheme != "file" {
			continue
		}
		p := parsed.Path
		if parsed.Host != "" && parsed.Host != "localhost" {
			// UNC path: file://server/share/path -> \\server\share\path
			return `\\` + parsed.Host + strings.ReplaceAll(p, "/", `\`)
		}
		// Windows drive URI: file:///C:/foo -> C:\foo
		if len(p) >= 3 && p[0] == '/' && ((p[1] >= 'A' && p[1] <= 'Z') || (p[1] >= 'a' && p[1] <= 'z')) && p[2] == ':' {
			return strings.ReplaceAll(p[1:], "/", `\`)
		}
		return p
	}
	return ""
}

func (s *AgySource) Units() ([]Unit, error) {
	brainDir := filepath.Join(s.Root, "brain")
	entries, err := os.ReadDir(brainDir)
	if os.IsNotExist(err) {
		return nil, nil // agy is not installed or has no sessions here
	}
	if err != nil {
		return nil, err
	}
	metaByID := s.loadAgyMeta()
	var units []Unit
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		cid := e.Name()
		logsDir := filepath.Join(brainDir, cid, ".system_generated", "logs")
		transcript := filepath.Join(logsDir, "transcript_full.jsonl")
		info, err := os.Stat(transcript)
		if err != nil {
			transcript = filepath.Join(logsDir, "transcript.jsonl")
			info, err = os.Stat(transcript)
			if err != nil {
				continue
			}
		}
		if info.IsDir() || info.Size() == 0 {
			continue
		}
		meta := metaByID[cid]
		sig := fmt.Sprintf("%d:%d%s", info.Size(), info.ModTime().UnixNano(), meta.sigSuffix())
		units = append(units, Unit{
			Key:         transcript,
			Signature:   sig,
			IsSidechain: meta.isSidechain(),
		})
	}
	sort.Slice(units, func(i, j int) bool { return units[i].Key < units[j].Key })
	return units, nil
}

type agyToolCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

type agyMedia struct {
	MimeType string `json:"mime_type"`
	URI      string `json:"uri"`
}

type agyStep struct {
	StepIndex       int           `json:"step_index"`
	Source          string        `json:"source"`
	Type            string        `json:"type"`
	Status          string        `json:"status"`
	Error           string        `json:"error"`
	CreatedAt       string        `json:"created_at"`
	Content         string        `json:"content"`
	Thinking        string        `json:"thinking"`
	Model           string        `json:"model"`
	InputTokens     int64         `json:"input_tokens"`
	OutputTokens    int64         `json:"output_tokens"`
	CacheReadTokens int64         `json:"cache_read_tokens"`
	ToolCalls       []agyToolCall `json:"tool_calls"`
	Media           []agyMedia    `json:"media"`
}

var (
	agyUserRequestRe  = regexp.MustCompile(`(?s)<USER_REQUEST>\s*(.*?)\s*</USER_REQUEST>`)
	agyMetaEnvelopeRe = regexp.MustCompile(`(?s)<(?:ADDITIONAL_METADATA|USER_SETTINGS_CHANGE)>.*?</(?:ADDITIONAL_METADATA|USER_SETTINGS_CHANGE)>`)
)

// agyUserText extracts the user's actual request from agy's USER_INPUT framing
// (<USER_REQUEST>...</USER_REQUEST> plus <ADDITIONAL_METADATA> and
// <USER_SETTINGS_CHANGE> blocks). Plain prompts without tags pass through.
func agyUserText(content string) string {
	if m := agyUserRequestRe.FindStringSubmatch(content); m != nil {
		return strings.TrimSpace(m[1])
	}
	cleaned := agyMetaEnvelopeRe.ReplaceAllString(content, "")
	return strings.TrimSpace(cleaned)
}

func agyStepInProgress(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "RUNNING", "PENDING", "IN_PROGRESS", "STREAMING":
		return true
	}
	return false
}

func agyMediaBlocks(media []agyMedia) []Block {
	var out []Block
	for _, m := range media {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(m.MimeType)), "image/") {
			out = append(out, imageBlock(dataURLImage(0, m.URI)))
		}
	}
	return out
}

func agySessionIDFromPath(p string) string {
	// .../brain/<conversation-id>/.system_generated/logs/transcript[_full].jsonl
	logsDir := filepath.Dir(p)
	sysGen := filepath.Dir(logsDir)
	convDir := filepath.Dir(sysGen)
	return filepath.Base(convDir)
}

func (s *AgySource) Read(u Unit) ([]Turn, error) {
	f, err := os.Open(u.Key)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sessionID := agySessionIDFromPath(u.Key)
	meta := s.loadAgyMeta()[sessionID]
	sidechain := meta.isSidechain()
	agentID := ""
	parentAgentID := ""
	if sidechain {
		agentID = sessionID
		parentAgentID = meta.ParentID
	}

	type pendingCall struct {
		id   string
		name string
	}
	var pending []pendingCall
	var turns []Turn

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	line := 0
	for sc.Scan() {
		line++
		raw := strings.TrimSpace(sc.Text())
		if raw == "" {
			continue
		}
		var step agyStep
		if err := json.Unmarshal([]byte(raw), &step); err != nil {
			return nil, fmt.Errorf("agy line %d: invalid JSON: %w", line, err)
		}
		if agyStepInProgress(step.Status) {
			continue
		}

		turnUUID := fmt.Sprintf("%s:step:%d", sessionID, step.StepIndex)
		t := Turn{
			SessionID:     sessionID,
			TurnUUID:      turnUUID,
			Seq:           int64(len(turns)),
			TS:            step.CreatedAt,
			Workdir:       meta.Workdir,
			Harness:       "agy",
			SourcePath:    u.Key,
			IsSidechain:   sidechain,
			AgentID:       agentID,
			ParentAgentID: parentAgentID,
			Summary:       meta.Title,
		}

		switch strings.ToUpper(step.Type) {
		case "USER_INPUT":
			t.Role = "user"
			if text := agyUserText(step.Content); text != "" {
				t.Blocks = append(t.Blocks, Block{Type: "text", Text: text})
			}
			t.Blocks = append(t.Blocks, agyMediaBlocks(step.Media)...)
			pending = nil

		case "PLANNER_RESPONSE":
			t.Role = "assistant"
			t.Model = step.Model
			t.RequestID = turnUUID
			t.Usage = Usage{
				Input:     step.InputTokens,
				Output:    step.OutputTokens,
				CacheRead: step.CacheReadTokens,
			}
			if strings.TrimSpace(step.Thinking) != "" {
				t.Blocks = append(t.Blocks, Block{Type: "thinking", Text: step.Thinking})
			}
			if strings.TrimSpace(step.Content) != "" {
				t.Blocks = append(t.Blocks, Block{Type: "text", Text: step.Content})
			}
			for i, tc := range step.ToolCalls {
				callID := fmt.Sprintf("%s:call:%d", turnUUID, i)
				args := strings.TrimSpace(string(tc.Args))
				text := tc.Name
				if args != "" && args != "null" {
					text = tc.Name + " " + args
				}
				t.Blocks = append(t.Blocks, Block{
					Type:      "tool_use",
					Text:      text,
					ToolName:  tc.Name,
					ToolUseID: callID,
				})
				pending = append(pending, pendingCall{id: callID, name: tc.Name})
			}
			t.Blocks = append(t.Blocks, agyMediaBlocks(step.Media)...)

		default:
			if len(pending) > 0 {
				call := pending[0]
				pending = pending[1:]
				t.Role = "tool"
				isErr := strings.EqualFold(step.Status, "ERROR") || step.Error != ""
				text := step.Content
				if text == "" && step.Error != "" {
					text = step.Error
				}
				t.Blocks = append(t.Blocks, Block{
					Type:      "tool_result",
					Text:      text,
					ToolName:  call.name,
					ToolUseID: call.id,
					IsError:   isErr,
				})
				t.Blocks = append(t.Blocks, agyMediaBlocks(step.Media)...)
			} else {
				t.Role = "system"
				if strings.EqualFold(step.Source, "USER_EXPLICIT") {
					t.Role = "user"
				}
				text := step.Content
				if text == "" && step.Error != "" {
					text = step.Error
				}
				if strings.TrimSpace(text) != "" {
					t.Blocks = append(t.Blocks, Block{Type: "text", Text: text})
				}
				t.Blocks = append(t.Blocks, agyMediaBlocks(step.Media)...)
			}
		}

		if len(t.Blocks) == 0 && t.Usage == (Usage{}) && t.Model == "" {
			continue
		}
		turns = append(turns, t)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("agy read %s: %w", u.Key, err)
	}
	return CaptureSkills(selectParsedRequests(turns)), nil
}
