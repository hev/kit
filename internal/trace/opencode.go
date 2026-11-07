package trace

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// OpenCodeSource reads OpenCode's SQLite store (opencode.db, OpenCode 1.2+;
// verified against 1.19.5). Sessions live in the v1 `session`, `message` and
// `part` tables; message and part rows hold the session-v1 JSON shapes from
// packages/schema/src/v1/session.ts. Each session is one unit keyed
// "<db>#<session id>", so a session that gains turns is re-read whole and its
// turn IDs (OpenCode message and part IDs) stay stable.
//
// OpenCode writes the database in WAL mode while it runs. Every scan opens a
// fresh read-only connection, reads inside one deferred read transaction (a
// consistent snapshot that never blocks OpenCode's writer) and closes it before
// returning, so no lock or snapshot outlives a Units or Read call. See
// openCodeRead for when the scan may touch OpenCode's -wal and -shm files.
//
// Assistant messages are archived only once OpenCode marks them complete:
// chunk IDs hash their text, so a mid-stream read would leave a truncated
// chunk behind for good (and a half-streamed secret can slip past redaction).
type OpenCodeSource struct{ Root string }

// DefaultOpenCodeRoot mirrors OpenCode's data directory: xdg-basedir's
// $XDG_DATA_HOME, else ~/.local/share, on every OS (including macOS and
// Windows), joined with "opencode".
func DefaultOpenCodeRoot() string {
	if root := os.Getenv("HEV_OPENCODE_DATA_DIR"); root != "" {
		return root
	}
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "opencode")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "opencode")
}

func (s *OpenCodeSource) Describe() string { return "OpenCode sessions in " + s.Root }

// openCodeDatabases lists opencode.db and per-channel opencode-<channel>.db.
func (s *OpenCodeSource) openCodeDatabases() ([]string, error) {
	entries, err := os.ReadDir(s.Root)
	if os.IsNotExist(err) {
		return nil, nil // OpenCode is not installed here
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".db") || (name != "opencode.db" && !strings.HasPrefix(name, "opencode-")) {
			continue
		}
		out = append(out, filepath.Join(s.Root, name))
	}
	sort.Strings(out)
	return out, nil
}

// openCodeSettle is how long a session must be idle before an assistant
// message OpenCode never completed (it was killed mid-stream; aborts and errors
// do complete the message) is archived as it stands.
const openCodeSettle = time.Hour

// openCodeDSN opens read-only through a URI so SQLite never rewrites the
// database. immutable additionally keeps SQLite from creating -wal and -shm
// files, and is only safe while nothing writes the database.
func openCodeDSN(path string, immutable bool) string {
	p := filepath.ToSlash(path)
	if strings.HasPrefix(path, `\\`) {
		p = strings.ReplaceAll(path, `\`, "/") // file:////server/share/...
	} else if WindowsPath(path) {
		p = "/" + strings.ReplaceAll(path, `\`, "/") // file:///C:/...
	}
	q := "mode=ro&_pragma=busy_timeout(5000)&_pragma=query_only(1)"
	if immutable {
		q = "mode=ro&immutable=1&_pragma=query_only(1)"
	}
	return (&url.URL{Scheme: "file", Path: p, RawQuery: q}).String()
}

// openCodeRead runs fn inside one read-only snapshot of the database.
//
// With no -wal file, OpenCode is closed (it deletes the WAL on a clean exit)
// and a plain read-only open would create -wal and -shm files of our own, or
// fail in a read-only directory. That case is read immutable, and the read is
// kept only if the file did not change underneath it and no WAL appeared;
// otherwise OpenCode started writing, and the read is repeated as an ordinary
// WAL reader sharing OpenCode's own -shm. fn must reset what it collects.
func openCodeRead(path string, fn func(*sql.Tx) error) error {
	before, err := os.Stat(path)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path + "-wal"); os.IsNotExist(err) {
		err := openCodeReadDSN(openCodeDSN(path, true), fn)
		after, statErr := os.Stat(path)
		_, walErr := os.Stat(path + "-wal")
		if statErr == nil && os.IsNotExist(walErr) && after.Size() == before.Size() && after.ModTime().Equal(before.ModTime()) {
			return err
		}
	}
	return openCodeReadDSN(openCodeDSN(path, false), fn)
}

func openCodeReadDSN(dsn string, fn func(*sql.Tx) error) error {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	return fn(tx)
}

// Units lists one unit per session. A database that cannot be read becomes a
// single unit whose Read reports why, so it is logged and retried each cycle
// without hiding the other databases; one with no session table yet (a
// channel database OpenCode created but never migrated) has nothing to read.
func (s *OpenCodeSource) Units() ([]Unit, error) {
	dbs, err := s.openCodeDatabases()
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	var units []Unit
	for _, path := range dbs {
		var found []Unit
		err := openCodeRead(path, func(tx *sql.Tx) error {
			found = nil
			// The change stamp covers rows OpenCode updates in place (streaming
			// parts, tool state transitions, titles, totals). The sum still moves
			// if a clock step makes an update older than the newest row. Read
			// holds back an unfinished assistant message until the session
			// settles, so settling must move the signature too.
			rows, err := tx.Query(`SELECT s.id, s.parent_id IS NOT NULL, s.time_updated,
				(SELECT count(*) || '-' || coalesce(max(time_updated), 0) || '-' || coalesce(sum(time_updated), 0) FROM message m WHERE m.session_id = s.id),
				(SELECT count(*) || '-' || coalesce(max(time_updated), 0) || '-' || coalesce(sum(time_updated), 0) FROM part p WHERE p.session_id = s.id),
				max(s.time_updated, coalesce((SELECT max(time_updated) FROM message m WHERE m.session_id = s.id), 0), coalesce((SELECT max(time_updated) FROM part p WHERE p.session_id = s.id), 0)),
				EXISTS (SELECT 1 FROM message m WHERE m.session_id = s.id AND json_valid(m.data) AND json_extract(m.data, '$.role') = 'assistant' AND coalesce(json_extract(m.data, '$.time.completed'), 0) = 0)
				FROM session s ORDER BY s.id`)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var id, messages, parts string
				var child, open bool
				var updated, last int64
				if err := rows.Scan(&id, &child, &updated, &messages, &parts, &last, &open); err != nil {
					return err
				}
				sig := fmt.Sprintf("%d-%s-%s", updated, messages, parts)
				if open && now-last < openCodeSettle.Milliseconds() {
					sig += "-streaming"
				}
				found = append(found, Unit{Key: path + "#" + id, Signature: sig, IsSidechain: child})
			}
			return rows.Err()
		})
		switch {
		case err == nil:
			units = append(units, found...)
		case strings.Contains(err.Error(), "no such table: session"):
		default:
			units = append(units, Unit{Key: path + "#", Signature: "unreadable"})
		}
	}
	return units, nil
}

type openCodeTokens struct {
	Input     int64 `json:"input"`
	Output    int64 `json:"output"`
	Reasoning int64 `json:"reasoning"`
	Cache     struct {
		Read  int64 `json:"read"`
		Write int64 `json:"write"`
	} `json:"cache"`
}

type openCodeMessage struct {
	Role     string `json:"role"`
	ParentID string `json:"parentID"`
	ModelID  string `json:"modelID"`
	Model    struct {
		ModelID string `json:"modelID"`
		Variant string `json:"variant"`
	} `json:"model"`
	Variant string `json:"variant"`
	Time    struct {
		Completed int64 `json:"completed"`
	} `json:"time"`
	Path struct {
		Cwd string `json:"cwd"`
	} `json:"path"`
	Cost   float64        `json:"cost"`
	Tokens openCodeTokens `json:"tokens"`
	Error  *struct {
		Name string `json:"name"`
		Data struct {
			Message string `json:"message"`
		} `json:"data"`
	} `json:"error"`
}

type openCodePart struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	Synthetic   bool   `json:"synthetic"`
	Ignored     bool   `json:"ignored"`
	Tool        string `json:"tool"`
	CallID      string `json:"callID"`
	Mime        string `json:"mime"`
	Filename    string `json:"filename"`
	Name        string `json:"name"`
	Prompt      string `json:"prompt"`
	Description string `json:"description"`
	Agent       string `json:"agent"`
	State       struct {
		Status string          `json:"status"`
		Input  json.RawMessage `json:"input"`
		Output string          `json:"output"`
		Error  string          `json:"error"`
		Time   struct {
			Start int64 `json:"start"`
			End   int64 `json:"end"`
		} `json:"time"`
	} `json:"state"`
}

func openCodeTS(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return time.UnixMilli(ms).UTC().Format(time.RFC3339Nano)
}

// openCodeDir reverses OpenCode's storage encoding, which writes Windows
// paths with forward slashes ("C:/Users/..."); see core/src/database/path.ts.
func openCodeDir(p string) string {
	if (len(p) >= 3 && p[1] == ':' && p[2] == '/' && ((p[0] >= 'a' && p[0] <= 'z') || (p[0] >= 'A' && p[0] <= 'Z'))) || strings.HasPrefix(p, "//") {
		return strings.ReplaceAll(p, "/", `\`)
	}
	return p
}

func (s *OpenCodeSource) Read(u Unit) ([]Turn, error) {
	i := strings.LastIndex(u.Key, "#")
	if i < 0 {
		return nil, fmt.Errorf("opencode unit %q: missing session id", u.Key)
	}
	path, sessionID := u.Key[:i], u.Key[i+1:]
	if sessionID == "" {
		// Units could not read this database; report why.
		err := openCodeRead(path, func(tx *sql.Tx) error {
			var n int
			return tx.QueryRow(`SELECT count(*) FROM session`).Scan(&n)
		})
		if err == nil {
			err = fmt.Errorf("unreadable while listing sessions; retrying next scan")
		}
		return nil, fmt.Errorf("opencode database %s: %w", path, err)
	}
	type row struct {
		id, data         string
		created, updated int64
	}
	var (
		dir, title, parent string
		messages           []row
		parts              map[string][]row
		v2Messages         int
		last               int64
	)
	err := openCodeRead(path, func(tx *sql.Tx) error {
		messages, parts, v2Messages = nil, map[string][]row{}, 0
		var parentID sql.NullString
		if err := tx.QueryRow(`SELECT directory, title, parent_id, time_updated FROM session WHERE id = ?`, sessionID).Scan(&dir, &title, &parentID, &last); err != nil {
			return err
		}
		parent = parentID.String
		rows, err := tx.Query(`SELECT id, time_created, time_updated, data FROM message WHERE session_id = ? ORDER BY time_created, id`, sessionID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.id, &r.created, &r.updated, &r.data); err != nil {
				rows.Close()
				return err
			}
			last = max(last, r.updated)
			messages = append(messages, r)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
		rows, err = tx.Query(`SELECT message_id, id, time_created, time_updated, data FROM part WHERE session_id = ? ORDER BY message_id, id`, sessionID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var messageID string
			var r row
			if err := rows.Scan(&messageID, &r.id, &r.created, &r.updated, &r.data); err != nil {
				return err
			}
			last = max(last, r.updated)
			parts[messageID] = append(parts[messageID], r)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(messages) == 0 {
			// OpenCode's v2 session engine writes session_message instead. Its
			// shape is not archived yet; fail loudly rather than record nothing.
			if err := tx.QueryRow(`SELECT count(*) FROM session_message WHERE session_id = ?`, sessionID).Scan(&v2Messages); err != nil && !strings.Contains(err.Error(), "no such table") {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("opencode session %s: %w", sessionID, err)
	}
	if v2Messages > 0 {
		return nil, fmt.Errorf("opencode session %s: v2 session_message storage is not supported", sessionID)
	}
	workdir := openCodeDir(dir)
	var turns []Turn
	add := func(t Turn) {
		t.Seq = int64(len(turns))
		t.SessionID = sessionID
		t.Harness = "opencode"
		t.SourcePath = path
		t.Summary = title
		if parent != "" {
			t.IsSidechain = true
			t.AgentID = sessionID
			t.ParentAgentID = parent
		}
		if t.Workdir == "" {
			t.Workdir = workdir
		}
		turns = append(turns, t)
	}
	// A user message's variant applies to the replies that answer it.
	variants := map[string]string{}
	settled := time.Now().UnixMilli()-last >= openCodeSettle.Milliseconds()
	for _, mr := range messages {
		var m openCodeMessage
		if err := json.Unmarshal([]byte(mr.data), &m); err != nil {
			return nil, fmt.Errorf("opencode message %s: invalid JSON", mr.id)
		}
		if m.Role == "assistant" && m.Time.Completed == 0 && !settled {
			// Still streaming: stop here so turn order stays a stable prefix.
			break
		}
		model, effort := m.ModelID, m.Variant
		if m.Role == "user" {
			model = m.Model.ModelID
			if effort == "" {
				effort = m.Model.Variant
			}
			variants[mr.id] = effort
		} else if effort == "" {
			effort = variants[m.ParentID]
		}
		if effort == "default" {
			effort = ""
		}
		t := Turn{TurnUUID: mr.id, ParentUUID: m.ParentID, TS: openCodeTS(mr.created), Workdir: openCodeDir(m.Path.Cwd), Model: model, Effort: effort}
		switch m.Role {
		case "user", "assistant":
			t.Role = m.Role
		default:
			return nil, fmt.Errorf("opencode message %s: unsupported role %q", mr.id, m.Role)
		}
		var after []Turn
		var files []Block
		for _, pr := range parts[mr.id] {
			var p openCodePart
			if err := json.Unmarshal([]byte(pr.data), &p); err != nil {
				return nil, fmt.Errorf("opencode part %s: invalid JSON", pr.id)
			}
			switch p.Type {
			case "text":
				if p.Ignored || p.Text == "" {
					continue
				}
				if p.Synthetic && t.Role == "user" {
					// Injected context such as @file contents is not the user's request.
					after = append(after, Turn{TurnUUID: pr.id, ParentUUID: mr.id, TS: t.TS, Role: "system", Workdir: t.Workdir, Model: model, Blocks: []Block{{Type: "text", Text: p.Text}}})
					continue
				}
				t.Blocks = append(t.Blocks, Block{Type: "text", Text: p.Text})
			case "reasoning":
				if p.Text != "" {
					t.Blocks = append(t.Blocks, Block{Type: "thinking", Text: p.Text})
				}
			case "tool":
				if p.State.Status == "pending" {
					continue // the model is still writing the call's input
				}
				input := strings.TrimSpace(string(p.State.Input))
				if input == "" {
					input = "{}"
				}
				t.Blocks = append(t.Blocks, Block{Type: "tool_use", Text: p.Tool + " " + input, ToolName: p.Tool, ToolUseID: p.CallID})
				var out string
				switch p.State.Status {
				case "completed":
					out = p.State.Output
				case "error":
					out = p.State.Error
				default: // running: the call has no result yet
					continue
				}
				ts := openCodeTS(p.State.Time.End)
				if ts == "" {
					ts = openCodeTS(pr.created)
				}
				after = append(after, Turn{TurnUUID: pr.id, ParentUUID: mr.id, TS: ts, Role: "tool", Workdir: t.Workdir, Model: model,
					Blocks: []Block{{Type: "tool_result", Text: out, ToolName: p.Tool, ToolUseID: p.CallID, IsError: p.State.Status == "error"}}})
			case "file":
				// File parts carry data: URLs with whole attachments; keep the name
				// only, after the prompt text so it never becomes the session's request.
				name := p.Filename
				if name == "" {
					name = p.Mime
				}
				files = append(files, Block{Type: "text", Text: "[file omitted: " + name + "]"})
			case "agent":
				t.Blocks = append(t.Blocks, Block{Type: "text", Text: "@" + p.Name})
			case "subtask":
				t.Blocks = append(t.Blocks, Block{Type: "text", Text: strings.TrimSpace("[subtask " + p.Agent + "] " + p.Description + "\n" + p.Prompt)})
			case "step-start", "step-finish", "snapshot", "patch", "compaction", "retry":
			default:
				return nil, fmt.Errorf("opencode part %s: unsupported type %q", pr.id, p.Type)
			}
		}
		t.Blocks = append(t.Blocks, files...)
		if m.Error != nil && m.Error.Name != "" {
			t.Blocks = append(t.Blocks, Block{Type: "text", Text: strings.TrimSpace("[" + m.Error.Name + "] " + m.Error.Data.Message)})
		}
		if t.Role == "assistant" {
			t.RequestID = mr.id
			// OpenCode reports reasoning tokens separately; they bill as output.
			t.Usage = Usage{Input: m.Tokens.Input, Output: m.Tokens.Output + m.Tokens.Reasoning, CacheRead: m.Tokens.Cache.Read, CacheCreation: m.Tokens.Cache.Write}
			t.Cost = m.Cost
		}
		add(t)
		for _, r := range after {
			add(r)
		}
	}
	return CaptureSkills(selectParsedRequests(turns)), nil
}
