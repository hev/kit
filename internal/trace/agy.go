package trace

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// AgySource reads Google Antigravity CLI (agy) conversations. agy keeps one
// SQLite database per conversation, <root>/conversations/<id>.db, whose steps
// table holds one protobuf-encoded step per row (see agy_proto.go for the
// fields read). <root>/conversation_summaries.db holds each conversation's
// title, and <root>/brain/<id>/ holds agy's own working files: a JSONL
// rendering of the steps, full tool outputs and the agent's scratch files.
// Capture reads the steps, which are the only complete record (the JSONL
// lacks tool call IDs and exit codes).
//
// Each conversation is one unit keyed by its database path. agy writes the
// database in WAL mode while it runs; reads use the same discipline as
// OpenCode's (openCodeRead): a fresh read-only connection and one read
// transaction per call, closed before returning, so a scan never blocks or
// rewrites a running agy. Where SQLite cannot open the live file at all, the
// database and its WAL are copied and the copy is read.
//
// A step agy has not finished (a response still streaming, a command still
// running) ends what is read, so turns stay a stable prefix, until the
// conversation has been idle for agySettle.
type AgySource struct {
	Root string

	// summaries are the titles Units read, reused by the Reads that follow
	// it so a scan opens conversation_summaries.db once.
	mu        sync.Mutex
	summaries map[string]agySummary
}

// DefaultAgyRoot is agy's data directory: ~/.gemini/antigravity-cli on every
// OS (agy resolves it from the user's home directory, %USERPROFILE% on
// Windows). HEV_AGY_DATA_DIR overrides it.
func DefaultAgyRoot() string {
	if root := os.Getenv("HEV_AGY_DATA_DIR"); root != "" {
		return root
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".gemini", "antigravity-cli")
}

func (s *AgySource) Describe() string { return "Antigravity CLI (agy) conversations in " + s.Root }

const agySettle = time.Hour

type agySummary struct {
	title, parent string
}

// agySummaries reads conversation titles. It is best effort: a missing or
// unreadable summaries database only costs titles.
func (s *AgySource) agySummaries() map[string]agySummary {
	out := map[string]agySummary{}
	path := filepath.Join(s.Root, "conversation_summaries.db")
	if _, err := os.Stat(path); err != nil {
		return out
	}
	_ = agyRead(path, func(tx *sql.Tx) error {
		clear(out)
		rows, err := tx.Query(`SELECT conversation_id, title, parent_conversation_id FROM conversation_summaries`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			var title, parent sql.NullString
			if err := rows.Scan(&id, &title, &parent); err != nil {
				return err
			}
			out[id] = agySummary{title: strings.TrimSpace(title.String), parent: parent.String}
		}
		return rows.Err()
	})
	return out
}

// Units lists conversations/*.db. The signature is the database's and its
// WAL's size and modification time (agy appends to the WAL, so the database
// file alone does not move while it runs) plus the title, which agy writes
// elsewhere and later.
func (s *AgySource) Units() ([]Unit, error) {
	dir := filepath.Join(s.Root, "conversations")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil // agy is not installed here
	}
	if err != nil {
		return nil, err
	}
	summaries := s.agySummaries()
	s.mu.Lock()
	s.summaries = summaries
	s.mu.Unlock()
	now := time.Now()
	var units []Unit
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".db") {
			continue
		}
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err != nil {
			continue // removed while listing
		}
		mod := info.ModTime()
		sig := fmt.Sprintf("%d-%d", info.Size(), info.ModTime().UnixNano())
		if wal, err := os.Stat(path + "-wal"); err == nil {
			sig += fmt.Sprintf("+%d-%d", wal.Size(), wal.ModTime().UnixNano())
			if wal.ModTime().After(mod) {
				mod = wal.ModTime()
			}
		}
		sum := summaries[strings.TrimSuffix(name, ".db")]
		if sum.title != "" {
			h := sha256.Sum256([]byte(sum.title))
			sig += "-t" + hex.EncodeToString(h[:4])
		}
		// Read holds back unfinished steps until the conversation settles,
		// so settling has to move the signature too.
		if now.Sub(mod) < agySettle {
			sig += "-fresh"
		}
		units = append(units, Unit{Key: path, Signature: sig, IsSidechain: sum.parent != "", Modified: mod.UnixMilli()})
	}
	sort.Slice(units, func(i, j int) bool { return units[i].Key < units[j].Key })
	return units, nil
}

// agyReadInPlace is the in-place read; tests replace it to exercise the copy.
var agyReadInPlace = openCodeRead

// agyRead runs fn in one read-only snapshot of an agy database. It reads in
// place first (see openCodeRead); if SQLite cannot open the live file, it
// copies the database and WAL to a private directory and reads the copy,
// keeping the read only if the originals did not change during the copy. An
// error from fn's own queries is returned as it is.
func agyRead(path string, fn func(*sql.Tx) error) error {
	ran := false
	err := agyReadInPlace(path, func(tx *sql.Tx) error { ran = true; return fn(tx) })
	if err == nil || (ran && !agyCantOpen(err)) {
		return err
	}
	for range 3 {
		ok, cerr := agyReadCopy(path, fn)
		if cerr != nil {
			return fmt.Errorf("%w (reading a copy: %v)", err, cerr)
		}
		if ok {
			return nil
		}
	}
	return fmt.Errorf("%w (agy kept writing while it was copied; retrying next scan)", err)
}

// agyCantOpen reports SQLite's SQLITE_CANTOPEN, which a lazily opened
// connection can return from its first query rather than from opening.
func agyCantOpen(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "unable to open database file") || strings.Contains(msg, "SQLITE_CANTOPEN")
}

func agyStamp(path string) string {
	stamp := ""
	for _, p := range []string{path, path + "-wal"} {
		if info, err := os.Stat(p); err == nil {
			stamp += fmt.Sprintf("%d-%d;", info.Size(), info.ModTime().UnixNano())
		}
	}
	return stamp
}

func agyReadCopy(path string, fn func(*sql.Tx) error) (bool, error) {
	tmp, err := os.MkdirTemp("", "hev-agy-")
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(tmp)
	before := agyStamp(path)
	dst := filepath.Join(tmp, "conversation.db")
	if err := agyCopyFile(path, dst); err != nil {
		return false, err
	}
	if err := agyCopyFile(path+"-wal", dst+"-wal"); err != nil && !os.IsNotExist(err) {
		return false, err
	}
	if agyStamp(path) != before {
		return false, nil
	}
	// The copy is private, so SQLite may build its own -shm beside it.
	return true, openCodeReadDSN(openCodeDSN(dst, false), fn)
}

func agyCopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

type agyRow struct {
	idx     int64
	typ     uint64
	status  uint64
	payload []byte
}

type agyModel struct{ name, display string }

func (s *AgySource) Read(u Unit) ([]Turn, error) {
	path := u.Key
	id := strings.TrimSuffix(filepath.Base(path), ".db")
	var (
		rows  []agyRow
		gens  [][]byte
		meta  []byte
		mtime int64
	)
	if info, err := os.Stat(path); err == nil {
		mtime = info.ModTime().UnixMilli()
	}
	if info, err := os.Stat(path + "-wal"); err == nil {
		mtime = max(mtime, info.ModTime().UnixMilli())
	}
	err := agyRead(path, func(tx *sql.Tx) error {
		rows, gens, meta = nil, nil, nil
		tables := map[string]bool{}
		tr, err := tx.Query(`SELECT name FROM sqlite_master WHERE type = 'table'`)
		if err != nil {
			return err
		}
		for tr.Next() {
			var name string
			if err := tr.Scan(&name); err != nil {
				tr.Close()
				return err
			}
			tables[name] = true
		}
		tr.Close()
		if !tables["steps"] {
			if len(tables) == 0 {
				return nil // created, not yet migrated: nothing to read
			}
			return fmt.Errorf("no steps table; agy may have changed its storage format")
		}
		sr, err := tx.Query(`SELECT idx, step_type, status, step_payload FROM steps ORDER BY idx`)
		if err != nil {
			return err
		}
		defer sr.Close()
		for sr.Next() {
			var r agyRow
			if err := sr.Scan(&r.idx, &r.typ, &r.status, &r.payload); err != nil {
				return err
			}
			rows = append(rows, r)
		}
		if err := sr.Err(); err != nil {
			return err
		}
		if tables["gen_metadata"] {
			gr, err := tx.Query(`SELECT data FROM gen_metadata ORDER BY idx`)
			if err != nil {
				return err
			}
			defer gr.Close()
			for gr.Next() {
				var d []byte
				if err := gr.Scan(&d); err != nil {
					return err
				}
				gens = append(gens, d)
			}
			if err := gr.Err(); err != nil {
				return err
			}
		}
		if tables["trajectory_metadata_blob"] {
			err := tx.QueryRow(`SELECT data FROM trajectory_metadata_blob WHERE id = 'main'`).Scan(&meta)
			if err != nil && err != sql.ErrNoRows {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("agy conversation %s: %w", path, err)
	}
	s.mu.Lock()
	summaries := s.summaries
	s.mu.Unlock()
	if summaries == nil {
		summaries = s.agySummaries()
	}
	sum := summaries[id]
	b := &agyBuilder{session: id, path: path, title: sum.title, parent: sum.parent,
		settled: time.Now().UnixMilli()-mtime >= agySettle.Milliseconds(),
		models:  agyModels(gens), calls: map[string]string{}}
	b.trajectory(meta)
	for _, r := range rows {
		if !b.step(r) {
			break
		}
	}
	return CaptureSkills(selectParsedRequests(b.turns)), nil
}

// agyModels maps step index to the model that generated it, from the
// CortexStepGeneratorMetadata rows: step_indices (2) and chat_model (1), a
// ChatModelMetadata whose response_model (19) is the model ID and
// model_display_name (21) the name agy shows ("Gemini 4 Neon (Medium)"). The
// step's own model field is an enum placeholder.
func agyModels(gens [][]byte) map[int64]agyModel {
	out := map[int64]agyModel{}
	for _, g := range gens {
		m, err := pbParse(g)
		if err != nil {
			continue
		}
		chat := m.msg(1)
		model := agyModel{name: chat.str(19), display: chat.str(21)}
		if model.name == "" && model.display == "" {
			continue
		}
		for _, idx := range m.uints(2) {
			out[int64(idx)] = model
		}
	}
	return out
}

var agyEffort = regexp.MustCompile(`\((Minimal|Low|Medium|High|XHigh|Max)\)\s*$`)

// id returns the model ID (falling back to the display name) and the
// reasoning effort agy names in the display name.
func (m agyModel) id() (string, string) {
	name := m.name
	if name == "" {
		name = strings.TrimSpace(agyEffort.ReplaceAllString(m.display, ""))
	}
	effort := ""
	if g := agyEffort.FindStringSubmatch(m.display); g != nil {
		effort = strings.ToLower(g[1])
	}
	return name, effort
}

type agyBuilder struct {
	session, path, title, parent string
	workdir, branch, repo        string
	settled                      bool
	models                       map[int64]agyModel
	calls                        map[string]string // tool call ID -> turn that made it
	lastUser, lastTurn           string
	turns                        []Turn
}

// trajectory reads CortexTrajectoryMetadata: workspaces (1) with
// workspace_folder_absolute_uri (1), repository (3, git_origin_url 2) and
// branch_name (4); workspace_uris (7); parent_conversation_id (5), set on a
// subagent's conversation.
func (b *agyBuilder) trajectory(data []byte) {
	m, err := pbParse(data)
	if err != nil {
		return
	}
	if ws := m.msgs(1); len(ws) > 0 {
		b.workdir = agyFileURI(ws[0].str(1))
		b.branch = ws[0].str(4)
		b.repo = ws[0].msg(3).str(2)
	}
	if b.workdir == "" {
		if uris := m.strs(7); len(uris) > 0 {
			b.workdir = agyFileURI(uris[0])
		}
	}
	if p := m.str(5); p != "" && b.parent == "" {
		b.parent = p
	}
}

func (b *agyBuilder) add(t Turn) {
	t.Seq = int64(len(b.turns))
	t.SessionID = b.session
	t.Harness = "agy"
	t.SourcePath = b.path
	t.Summary = b.title
	t.Workdir, t.Branch, t.RepoURL = b.workdir, b.branch, b.repo
	if b.parent != "" {
		t.IsSidechain = true
		t.AgentID = b.session
		t.ParentAgentID = b.parent
	}
	b.lastTurn = t.TurnUUID
	b.turns = append(b.turns, t)
}

// step turns one row into turns. It returns false to stop reading: the step
// is unfinished and the conversation has not settled. A step that does not
// decode, or whose type carries nothing for the archive, is skipped.
func (b *agyBuilder) step(r agyRow) bool {
	if agyUnfinished(r.status) && !b.settled {
		return false
	}
	m, err := pbParse(r.payload)
	if err != nil {
		return true
	}
	md := m.msg(agyStepMetadata)
	uuid := b.session + ":" + strconv.FormatInt(r.idx, 10)
	ts := pbTime(md.msg(1)) // created_at
	switch r.typ {
	case agyTypeUserInput:
		blocks := agyUserBlocks(m.msg(agyStepUserInput))
		if len(blocks) == 0 {
			return true
		}
		b.add(Turn{TurnUUID: uuid, TS: ts, Role: "user", Blocks: blocks})
		b.lastUser = uuid
	case agyTypePlannerResponse:
		b.planner(r, m, md, uuid, ts)
	case agyTypeSystemMessage:
		// Notices agy injects for the model (a background task stopped, a
		// subagent's message), not the user's request.
		if text := m.msg(agyStepSystemMessage).str(1); strings.TrimSpace(text) != "" {
			b.add(Turn{TurnUUID: uuid, ParentUUID: b.lastUser, TS: ts, Role: "system", Blocks: []Block{{Type: "text", Text: text}}})
		}
	case agyTypeErrorMessage:
		if text := agyErrorText(m.msg(agyStepErrorMessage).msg(3)); text != "" {
			b.add(Turn{TurnUUID: uuid, ParentUUID: b.lastUser, TS: ts, Role: "system", Blocks: []Block{{Type: "text", Text: text}}})
		}
	default:
		// Any other step that answers a tool call (GENERIC in agy 1.5.12)
		// is that call's result; steps that answer none are agy's own
		// bookkeeping (checkpoints, ephemeral context) and are skipped.
		if call := md.msg(4); call.str(1) != "" || r.typ == agyTypeGeneric {
			b.result(r, m, call, uuid, ts)
		}
	}
	return true
}

// agyUserBlocks reads CortexStepUserInput: user_response (2) is the prompt
// as typed; items (3, text 1) and query (1) are older spellings of it.
// Images (5, ImageData) and media (9, Media) follow the text.
func agyUserBlocks(in pbMsg) []Block {
	text := in.str(2)
	if text == "" {
		var parts []string
		for _, it := range in.msgs(3) {
			if t := it.str(1); t != "" {
				parts = append(parts, t)
			}
		}
		text = strings.Join(parts, "")
	}
	if text == "" {
		text = in.str(1)
	}
	var blocks []Block
	if strings.TrimSpace(text) != "" {
		blocks = append(blocks, Block{Type: "text", Text: text})
	}
	n := 0
	for _, im := range in.msgs(5) { // base64_data 1, mime_type 2, uri 4
		if img, ok := fileImage(n, im.str(2), im.str(1), im.str(4)); ok {
			blocks = append(blocks, imageBlock(img))
			n++
		}
	}
	for _, md := range in.msgs(9) {
		if img, ok := agyMedia(n, md); ok {
			blocks = append(blocks, imageBlock(img))
			n++
		} else if name := agyMediaName(md); name != "" {
			blocks = append(blocks, Block{Type: "text", Text: "[file omitted: " + name + "]"})
		}
	}
	return blocks
}

// agyMedia reads an image from exa.codeium_common_pb.Media: mime_type (1),
// inline_data (2), uri (5).
func agyMedia(i int, md pbMsg) (Image, bool) {
	mime := md.str(1)
	if !strings.HasPrefix(strings.ToLower(mime), "image/") {
		return Image{}, false
	}
	if data := md.bytes(2); len(data) > 0 {
		return decodeImage(i, data), true
	}
	return Image{Index: i, Skipped: skipNotInline}, true
}

func agyMediaName(md pbMsg) string {
	for _, n := range []uint64{9, 5, 1} { // display_name, uri, mime_type
		if s := md.str(n); s != "" {
			return s
		}
	}
	return ""
}

// planner reads CortexStepPlannerResponse: response (1, or modified_response
// 8), thinking (3, unless thinking_redacted 5), tool_calls (7, ChatToolCall:
// id 1, name 2, arguments_json 3) and message_id (6). Usage is the step
// metadata's model_usage (9, ModelUsageStats): input_tokens 2, output_tokens
// 3 (thinking included), cache_write_tokens 4, cache_read_tokens 5,
// response_id 11.
func (b *agyBuilder) planner(r agyRow, m, md pbMsg, uuid, ts string) {
	p := m.msg(agyStepPlanner)
	var blocks []Block
	if th := p.str(3); strings.TrimSpace(th) != "" && p.uint(5) == 0 {
		blocks = append(blocks, Block{Type: "thinking", Text: th})
	}
	text := p.str(1)
	if text == "" {
		text = p.str(8)
	}
	if strings.TrimSpace(text) != "" {
		blocks = append(blocks, Block{Type: "text", Text: text})
	}
	for _, c := range p.msgs(7) {
		name, args := c.str(2), strings.TrimSpace(c.str(3))
		if args == "" {
			args = "{}"
		}
		blocks = append(blocks, Block{Type: "tool_use", Text: name + " " + args, ToolName: name, ToolUseID: c.str(1)})
		if id := c.str(1); id != "" {
			b.calls[id] = uuid
		}
	}
	for i, media := range p.msgs(20) {
		if img, ok := agyMedia(i, media); ok {
			blocks = append(blocks, imageBlock(img))
		}
	}
	if len(blocks) == 0 {
		return
	}
	usage := md.msg(9)
	rid := usage.str(11)
	if rid == "" {
		rid = p.str(6)
	}
	if rid == "" {
		rid = uuid
	}
	model, effort := b.models[r.idx].id()
	b.add(Turn{TurnUUID: uuid, ParentUUID: b.lastUser, TS: ts, Role: "assistant", Model: model, Effort: effort, RequestID: rid, Blocks: blocks,
		Usage: Usage{Input: int64(usage.uint(2)), Output: int64(usage.uint(3)), CacheCreation: int64(usage.uint(4)), CacheRead: int64(usage.uint(5))}})
}

// agyResultHeader is the timing agy puts ahead of every tool result, once or
// twice; it is not output.
var agyResultHeader = regexp.MustCompile(`\A(?:(?:Created|Completed) At: [^\n]*\n)+`)

// result records a tool result. The call is the step metadata's tool_call
// (4, ChatToolCall). A GENERIC step's result (140.2, GenericStepResult) holds
// the text the model saw in result (1) (long output is cut there and kept
// whole at full_output_uri 7, which is not read), and, for a command,
// step_render_info (3) content_blocks (10) terminal (3) exit_code (3).
func (b *agyBuilder) result(r agyRow, m, call pbMsg, uuid, ts string) {
	id, name := call.str(1), call.str(2)
	res := m.msg(agyStepGeneric).msg(2)
	text := agyResultHeader.ReplaceAllString(res.str(1), "")
	var exit *int
	for _, cb := range res.msg(3).msgs(10) {
		if term := cb.msg(3); len(term) > 0 {
			code := int(term.sint32(3)) // proto3 omits a zero exit status
			exit = &code
		}
	}
	errText := agyErrorText(m.msg(agyStepError))
	if errText != "" {
		text = strings.TrimSpace(errText + "\n" + text)
	}
	failed := r.status == agyStatusError || r.status == agyStatusCanceled || r.status == agyStatusInterrupted || errText != "" || (exit != nil && *exit != 0)
	if text == "" && !failed && exit == nil && id == "" {
		return
	}
	parent := b.calls[id]
	if parent == "" {
		parent = b.lastTurn
	}
	if name == "" {
		name = "agy_step_" + strconv.FormatUint(r.typ, 10)
	}
	b.add(Turn{TurnUUID: uuid, ParentUUID: parent, TS: ts, Role: "tool",
		Blocks: []Block{{Type: "tool_result", Text: text, ToolName: name, ToolUseID: id, IsError: failed, ExitCode: exit}}})
}

// agyErrorText reads CortexErrorDetails: user_error_message (1), short_error
// (2), model_error_message (9).
func agyErrorText(e pbMsg) string {
	for _, n := range []uint64{1, 2, 9} {
		if s := strings.TrimSpace(e.str(n)); s != "" {
			return "[error] " + s
		}
	}
	return ""
}
