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

// OpenCodeSource reads OpenCode's SQLite store (opencode.db). OpenCode 2.x
// (verified against 2.2.6) keeps sessions in session_v2 and session_message,
// with assistant parts inline or in session_part (see opencode_v2.go); 1.x
// (1.2 to 1.19.5) used the v1 session, message and part tables, whose rows hold
// the shapes in packages/schema/src/v1/session.ts. Both are read, and a session
// in both (upgraded) is read from its v1 rows plus the messages 2.x added
// afterwards. Each session is one unit keyed "<db>#<session id>", so a session
// that gains turns is re-read whole and its turn IDs (OpenCode message IDs, and
// part or call IDs for tool results) stay stable.
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

// DefaultOpenCodeRoot mirrors OpenCode's data directory: $XDG_DATA_HOME, else
// ~/.local/share, on every OS (including macOS and Windows), joined with
// "opencode" (1.x through xdg-basedir, 2.x in packages/util/src/global-roots.ts).
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

// openCodeTables reports which of OpenCode's session schemas a database holds.
// 1.x writes the v1 session/message/part tables. 2.x writes session_v2,
// session_message and (for assistant parts kept as rows) session_part, and on
// upgrade copies every v1 session across while leaving the v1 rows in place.
// other lists any remaining tables, so a database with neither layout can be
// told apart from one that is simply empty.
type openCodeTables struct {
	v1, v2, v2Parts bool
	other           []string
}

func openCodeSchema(tx *sql.Tx) (openCodeTables, error) {
	rows, err := tx.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite\_%' ESCAPE '\' ORDER BY name`)
	if err != nil {
		return openCodeTables{}, err
	}
	defer rows.Close()
	have := map[string]bool{}
	var other []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return openCodeTables{}, err
		}
		switch name {
		case "session", "message", "part", "session_v2", "session_message", "session_part":
			have[name] = true
		case "__drizzle_migrations":
		default:
			other = append(other, name)
		}
	}
	return openCodeTables{
		v1:      have["session"] && have["message"] && have["part"],
		v2:      have["session_v2"] && have["session_message"],
		v2Parts: have["session_part"],
		other:   other,
	}, rows.Err()
}

// openCodeLayout returns an error when a database holds tables but neither
// session layout, which is what a future OpenCode storage change looks like;
// it is reported every scan rather than skipped, so capture never stops
// silently. A database with no tables yet (a channel database OpenCode created
// but never migrated) has nothing to read. A pre-release opencode-next.db
// names its v2 table "session" and has no message table; 2.x imports it into
// opencode.db, so it is not read on its own.
func openCodeLayout(path string, t openCodeTables) error {
	if t.v1 || t.v2 || filepath.Base(path) == "opencode-next.db" {
		return nil
	}
	if len(t.other) == 0 && !t.v2Parts {
		return nil
	}
	return fmt.Errorf("no OpenCode session layout Kit recognises (1.x session/message/part or 2.x session_v2/session_message); OpenCode may have changed its storage format")
}

// Units lists one unit per session. A database that cannot be read, or whose
// layout is not recognised (see openCodeLayout), becomes a single unit whose
// Read reports why, so it is logged and retried each cycle without hiding the
// other databases.
// A session in both schemas (upgraded from 1.x) is one unit.
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
			tables, err := openCodeSchema(tx)
			if err != nil {
				return err
			}
			if err := openCodeLayout(path, tables); err != nil {
				return err
			}
			type state struct {
				sig   string
				child bool
				last  int64
				open  bool
			}
			sessions := map[string]*state{}
			// The change stamp covers rows OpenCode updates in place (streaming
			// parts, tool state transitions, titles, totals). The sum still moves
			// if a clock step makes an update older than the newest row. Read
			// holds back an unfinished assistant message until the session
			// settles, so settling must move the signature too.
			scan := func(v2 bool, query string) error {
				rows, err := tx.Query(query)
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
					st := sessions[id]
					if st == nil {
						st = &state{}
						sessions[id] = st
					}
					sig := fmt.Sprintf("%d-%s-%s", updated, messages, parts)
					if v2 {
						if st.sig != "" {
							sig = st.sig + "+" + sig
						}
					} else if st.sig != "" {
						sig += "+" + st.sig
					}
					st.sig = sig
					st.child = st.child || child
					st.last = max(st.last, last)
					st.open = st.open || open
				}
				return rows.Err()
			}
			if tables.v1 {
				if err := scan(false, `SELECT s.id, s.parent_id IS NOT NULL, s.time_updated,
					(SELECT count(*) || '-' || coalesce(max(time_updated), 0) || '-' || coalesce(sum(time_updated), 0) FROM message m WHERE m.session_id = s.id),
					(SELECT count(*) || '-' || coalesce(max(time_updated), 0) || '-' || coalesce(sum(time_updated), 0) FROM part p WHERE p.session_id = s.id),
					max(s.time_updated, coalesce((SELECT max(time_updated) FROM message m WHERE m.session_id = s.id), 0), coalesce((SELECT max(time_updated) FROM part p WHERE p.session_id = s.id), 0)),
					EXISTS (SELECT 1 FROM message m WHERE m.session_id = s.id AND json_valid(m.data) AND json_extract(m.data, '$.role') = 'assistant' AND coalesce(json_extract(m.data, '$.time.completed'), 0) = 0)
					FROM session s`); err != nil {
					return err
				}
			}
			if tables.v2 {
				parts, partsLast := `'0-0-0'`, `0`
				if tables.v2Parts {
					parts = `(SELECT count(*) || '-' || coalesce(max(time_updated), 0) || '-' || coalesce(sum(time_updated), 0) FROM session_part p WHERE p.session_id = s.id)`
					partsLast = `coalesce((SELECT max(time_updated) FROM session_part p WHERE p.session_id = s.id), 0)`
				}
				if err := scan(true, `SELECT s.id, s.parent_id IS NOT NULL, s.time_updated,
					(SELECT count(*) || '-' || coalesce(max(time_updated), 0) || '-' || coalesce(sum(time_updated), 0) FROM session_message m WHERE m.session_id = s.id),
					`+parts+`,
					max(s.time_updated, coalesce((SELECT max(time_updated) FROM session_message m WHERE m.session_id = s.id), 0), `+partsLast+`),
					EXISTS (SELECT 1 FROM session_message m WHERE m.session_id = s.id AND m.type = 'assistant' AND json_valid(m.data) AND coalesce(json_extract(m.data, '$.time.completed'), 0) = 0)
					FROM session_v2 s`); err != nil {
					return err
				}
			}
			ids := make([]string, 0, len(sessions))
			for id := range sessions {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			for _, id := range ids {
				st := sessions[id]
				sig := st.sig
				if st.open && now-st.last < openCodeSettle.Milliseconds() {
					sig += "-streaming"
				}
				found = append(found, Unit{Key: path + "#" + id, Signature: sig, IsSidechain: st.child})
			}
			return nil
		})
		if err != nil {
			units = append(units, Unit{Key: path + "#", Signature: "unreadable"})
			continue
		}
		units = append(units, found...)
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
	URL         string `json:"url"`
	Name        string `json:"name"`
	Prompt      string `json:"prompt"`
	Description string `json:"description"`
	Agent       string `json:"agent"`
	State       struct {
		Status string          `json:"status"`
		Input  json.RawMessage `json:"input"`
		Output string          `json:"output"`
		Error  string          `json:"error"`
		// Attachments are file parts a tool returned; the read tool puts an
		// image it opened here as a data: URL.
		Attachments []struct {
			Mime     string `json:"mime"`
			Filename string `json:"filename"`
			URL      string `json:"url"`
		} `json:"attachments"`
		Time struct {
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

// openCodeRow is one message or part row; typ is session_message.type (v2).
type openCodeRow struct {
	id, typ, data    string
	created, updated int64
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
			tables, err := openCodeSchema(tx)
			if err != nil {
				return err
			}
			return openCodeLayout(path, tables)
		})
		if err == nil {
			err = fmt.Errorf("unreadable while listing sessions; retrying next scan")
		}
		return nil, fmt.Errorf("opencode database %s: %w", path, err)
	}
	var (
		dir, title, parent     string
		v1Messages, v2Messages []openCodeRow
		v1Parts, v2Parts       map[string][]openCodeRow
		last                   int64
	)
	collect := func(tx *sql.Tx, query string, typed bool, add func(group string, r openCodeRow)) error {
		rows, err := tx.Query(query, sessionID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var group string
			var r openCodeRow
			dest := []any{&group, &r.id, &r.created, &r.updated, &r.data}
			if typed {
				dest = append(dest, &r.typ)
			}
			if err := rows.Scan(dest...); err != nil {
				return err
			}
			last = max(last, r.updated)
			add(group, r)
		}
		return rows.Err()
	}
	err := openCodeRead(path, func(tx *sql.Tx) error {
		dir, title, parent, last = "", "", "", 0
		v1Messages, v2Messages, v1Parts, v2Parts = nil, nil, map[string][]openCodeRow{}, map[string][]openCodeRow{}
		tables, err := openCodeSchema(tx)
		if err != nil {
			return err
		}
		found := false
		var t, p sql.NullString
		if tables.v2 {
			err := tx.QueryRow(`SELECT directory, title, parent_id, time_updated FROM session_v2 WHERE id = ?`, sessionID).Scan(&dir, &t, &p, &last)
			switch {
			case err == nil:
				found, title, parent = true, t.String, p.String
				if err := collect(tx, `SELECT session_id, id, time_created, time_updated, data, type FROM session_message WHERE session_id = ? ORDER BY seq`, true,
					func(_ string, r openCodeRow) { v2Messages = append(v2Messages, r) }); err != nil {
					return err
				}
				if tables.v2Parts {
					if err := collect(tx, `SELECT message_id, CAST(ordinal AS TEXT), time_created, time_updated, data FROM session_part WHERE session_id = ? ORDER BY message_id, ordinal`, false,
						func(m string, r openCodeRow) { v2Parts[m] = append(v2Parts[m], r) }); err != nil {
						return err
					}
				}
			case err != sql.ErrNoRows:
				return err
			}
		}
		if tables.v1 {
			var d string
			var updated int64
			err := tx.QueryRow(`SELECT directory, title, parent_id, time_updated FROM session WHERE id = ?`, sessionID).Scan(&d, &t, &p, &updated)
			switch {
			case err == nil:
				if !found {
					dir, title, parent = d, t.String, p.String
				}
				found, last = true, max(last, updated)
				if err := collect(tx, `SELECT session_id, id, time_created, time_updated, data FROM message WHERE session_id = ? ORDER BY time_created, id`, false,
					func(_ string, r openCodeRow) { v1Messages = append(v1Messages, r) }); err != nil {
					return err
				}
				if err := collect(tx, `SELECT message_id, id, time_created, time_updated, data FROM part WHERE session_id = ? ORDER BY message_id, id`, false,
					func(m string, r openCodeRow) { v1Parts[m] = append(v1Parts[m], r) }); err != nil {
					return err
				}
			case err != sql.ErrNoRows:
				return err
			}
		}
		if !found {
			return sql.ErrNoRows
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("opencode session %s: %w", sessionID, err)
	}
	b := &openCodeBuilder{session: sessionID, path: path, title: title, parent: parent, workdir: openCodeDir(dir),
		settled: time.Now().UnixMilli()-last >= openCodeSettle.Milliseconds()}
	if err := b.v1(v1Messages, v1Parts); err != nil {
		return nil, err
	}
	// 2.x copies an upgraded session's v1 messages (same IDs) and adds backdated
	// system and synthetic messages of its own; the v1 rows stay and were read
	// above, so only messages written after the last v1 message are new.
	var cutoff int64
	seen := map[string]bool{}
	for _, m := range v1Messages {
		seen[m.id] = true
		cutoff = max(cutoff, m.created)
	}
	var fresh []openCodeRow
	for _, m := range v2Messages {
		if !seen[m.id] && (len(v1Messages) == 0 || m.created > cutoff) {
			fresh = append(fresh, m)
		}
	}
	if !b.stopped {
		if err := b.v2(fresh, v2Parts); err != nil {
			return nil, err
		}
	}
	return CaptureSkills(selectParsedRequests(b.turns)), nil
}

type openCodeBuilder struct {
	session, path, title, parent, workdir string
	settled, stopped                      bool
	turns                                 []Turn
}

func (b *openCodeBuilder) add(t Turn) {
	t.Seq = int64(len(b.turns))
	t.SessionID = b.session
	t.Harness = "opencode"
	t.SourcePath = b.path
	t.Summary = b.title
	if b.parent != "" {
		t.IsSidechain = true
		t.AgentID = b.session
		t.ParentAgentID = b.parent
	}
	if t.Workdir == "" {
		t.Workdir = b.workdir
	}
	b.turns = append(b.turns, t)
}

// v1 reads OpenCode 1.x message and part rows.
func (b *openCodeBuilder) v1(messages []openCodeRow, parts map[string][]openCodeRow) error {
	// A user message's variant applies to the replies that answer it.
	variants := map[string]string{}
	for _, mr := range messages {
		var m openCodeMessage
		if err := json.Unmarshal([]byte(mr.data), &m); err != nil {
			return fmt.Errorf("opencode message %s: invalid JSON", mr.id)
		}
		if m.Role == "assistant" && m.Time.Completed == 0 && !b.settled {
			// Still streaming: stop here so turn order stays a stable prefix.
			b.stopped = true
			return nil
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
			return fmt.Errorf("opencode message %s: unsupported role %q", mr.id, m.Role)
		}
		var after []Turn
		var files []Block
		for _, pr := range parts[mr.id] {
			var p openCodePart
			if err := json.Unmarshal([]byte(pr.data), &p); err != nil {
				return fmt.Errorf("opencode part %s: invalid JSON", pr.id)
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
				var images []Image
				parts := []string{out}
				for i, a := range p.State.Attachments {
					if im, ok := fileImage(i+1, a.Mime, "", a.URL); ok {
						images = append(images, im)
						parts = append(parts, im.Marker())
					}
				}
				if len(images) > 0 {
					out = strings.Join(parts, "\n")
				}
				after = append(after, Turn{TurnUUID: pr.id, ParentUUID: mr.id, TS: ts, Role: "tool", Workdir: t.Workdir, Model: model,
					Blocks: []Block{{Type: "tool_result", Text: out, ToolName: p.Tool, ToolUseID: p.CallID, IsError: p.State.Status == "error", Images: images}}})
			case "file":
				// File parts carry data: URLs with whole attachments; keep the name
				// only, after the prompt text so it never becomes the session's request.
				// An image is decoded for capture and stands as its marker.
				if im, ok := fileImage(0, p.Mime, "", p.URL); ok {
					files = append(files, imageBlock(im))
					continue
				}
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
				return fmt.Errorf("opencode part %s: unsupported type %q", pr.id, p.Type)
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
		b.add(t)
		for _, r := range after {
			b.add(r)
		}
	}
	return nil
}
