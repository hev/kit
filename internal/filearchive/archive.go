// Package filearchive keeps the trace archive in files on the host, for a
// host with no Layer API key. It writes what the indexer would send Layer, in
// the same row shapes, after the same redaction, and reads it back for
// `hev traces`, `hev browse` and `hev serve`.
//
// Layout under the archive directory (~/.hev/archive/<namespace>/):
//
//	.lock                         held across every read and write
//	meta.json                     the archive's identity and redaction baseline
//	sessions/<shard>/<key>.json   one session row, patched in place by outcomes
//	blocks/<shard>/<key>.jsonl    a session's read-side block rows
//	chunks/<shard>/<key>.jsonl    a session's searchable chunk rows
//	instructions.jsonl            instruction file versions
//	index/                        the lexical search index (see lexical.go)
//
// Every read and write takes the lock and each file is replaced atomically,
// the pattern the board's filesystem store uses, so a reader never sees half a
// file and any number of processes can share the directory. Nothing is embedded and nothing is
// pruned: files grow until the operator removes them.
package filearchive

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/platformops"
	"github.com/hev/kit/internal/trace"
)

// Store is one namespace's archive directory.
type Store struct {
	dir       string
	namespace string
	id        string
}

// DirEnv overrides the archive root (the parent of the namespace directories).
const DirEnv = "HEV_ARCHIVE_DIR"

// DefaultRoot is ~/.hev/archive.
func DefaultRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".hev", "archive")
	}
	return filepath.Join(home, ".hev", "archive")
}

// Root resolves the archive root: the environment override, then the
// configured value, then the default.
func Root(configured string) string {
	if v := strings.TrimSpace(os.Getenv(DirEnv)); v != "" {
		return v
	}
	if strings.TrimSpace(configured) != "" {
		return configured
	}
	return DefaultRoot()
}

var safeNamespace = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// Open returns the archive for namespace under root, creating it if needed.
// It never opens a network connection.
func Open(root, namespace string) (*Store, error) {
	if namespace == "" {
		namespace = "hev-traces"
	}
	if !safeNamespace.MatchString(namespace) {
		return nil, fmt.Errorf("archive namespace %q must be letters, digits, '.', '_' or '-'", namespace)
	}
	if root == "" {
		root = DefaultRoot()
	}
	abs, err := filepath.Abs(filepath.Join(root, namespace))
	if err != nil {
		return nil, err
	}
	for _, sub := range []string{"sessions", "blocks", "chunks", "index"} {
		if err := platformops.MkdirAll(filepath.Join(abs, sub), 0o700); err != nil {
			return nil, fmt.Errorf("archive directory: %w", err)
		}
	}
	s := &Store{dir: abs, namespace: namespace}
	if err := s.ensureID(); err != nil {
		return nil, fmt.Errorf("archive identity: %w", err)
	}
	return s, nil
}

// Dir is the namespace directory.
func (s *Store) Dir() string { return s.dir }

// Namespace is the archive's namespace name.
func (s *Store) Namespace() string { return s.namespace }

// ArchiveID scopes the indexer's unchanged-unit cache to this archive: its
// directory and the identity it was created with.
func (s *Store) ArchiveID() string { return "file\x00" + s.dir + "\x00" + s.id }

// ChunkRunes is the chunk ceiling. Nothing is embedded, so there is no model
// limit to respect; the parser's ceiling keeps chunks the size Layer's are.
func (s *Store) ChunkRunes() int { return trace.MaxRunes }

func (s *Store) locked(fn func() error) error {
	unlock, err := platformops.LockPath(filepath.Join(s.dir, ".lock"), false)
	if err != nil {
		return fmt.Errorf("lock trace archive: %w", err)
	}
	defer unlock()
	return fn()
}

// keyOf names a session's files. Lowercase ids that are already safe file
// names are used as they are, so `hev traces show <prefix>` can find them by
// name; anything else is hashed (and the id is read from the file).
var plainKey = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,127}$`)

const instructionsKey = "~instructions"

func keyOf(sessionID string) string {
	if sessionID == "" {
		return instructionsKey
	}
	if plainKey.MatchString(sessionID) {
		return sessionID
	}
	sum := sha256.Sum256([]byte(sessionID))
	return "~" + hex.EncodeToString(sum[:16])
}

func shard(key string) string {
	if strings.HasPrefix(key, "~") {
		return "~" + key[1:3]
	}
	return key[:2]
}

func (s *Store) path(kind, key string) string {
	ext := ".jsonl"
	if kind == "sessions" {
		ext = ".json"
	}
	return filepath.Join(s.dir, kind, shard(key), key+ext)
}

func (s *Store) write(path string, data []byte) error {
	if err := platformops.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return platformops.WriteFileAtomic(path, data, 0o600)
}

func remove(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// readLines reads one JSON object per line. A missing file is empty.
func readLines(path string) ([][]byte, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out [][]byte
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 0, 1<<20), 1<<30)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		out = append(out, append([]byte(nil), line...))
	}
	return out, sc.Err()
}

// idOf reads the "id" attribute of a stored row.
func idOf(line []byte) string {
	var v struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(line, &v)
	return v.ID
}

// upsertLines merges rows into a session's JSONL file by id, keeping the file
// sorted by id so the same rows always produce the same bytes.
func (s *Store) upsertLines(path string, rows map[string][]byte) (bool, error) {
	existing, err := readLines(path)
	if err != nil {
		return false, err
	}
	byID := make(map[string][]byte, len(existing)+len(rows))
	for _, line := range existing {
		byID[idOf(line)] = line
	}
	changed := false
	for id, line := range rows {
		if old, ok := byID[id]; !ok || !bytes.Equal(old, line) {
			changed = true
		}
		byID[id] = line
	}
	if !changed {
		return false, nil
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var buf bytes.Buffer
	for _, id := range ids {
		buf.Write(byID[id])
		buf.WriteByte('\n')
	}
	return true, s.write(path, buf.Bytes())
}

// Write upserts chunk rows: the searchable text. The lexical index learns
// about them the next time it is refreshed.
func (s *Store) Write(rows []layer.Row) (layer.WriteResult, error) {
	if len(rows) == 0 {
		return layer.WriteResult{}, nil
	}
	grouped := map[string]map[string][]byte{}
	for _, row := range rows {
		if row.ID == "" {
			return layer.WriteResult{}, fmt.Errorf("chunk row without id")
		}
		b, err := json.Marshal(row)
		if err != nil {
			return layer.WriteResult{}, err
		}
		k := keyOf(row.SessionID)
		if grouped[k] == nil {
			grouped[k] = map[string][]byte{}
		}
		grouped[k][row.ID] = b
	}
	err := s.locked(func() error {
		var dirty []string
		for k, group := range grouped {
			changed, err := s.upsertLines(s.path("chunks", k), group)
			if err != nil {
				return err
			}
			if changed {
				dirty = append(dirty, k)
			}
		}
		return s.markDirty(dirty)
	})
	if err != nil {
		return layer.WriteResult{}, err
	}
	return layer.WriteResult{RowsUpserted: len(rows)}, nil
}

// WriteBlocks upserts the lossless read-side blocks the dashboard renders.
func (s *Store) WriteBlocks(rows []trace.BlockRow) (layer.WriteResult, error) {
	if len(rows) == 0 {
		return layer.WriteResult{}, nil
	}
	grouped := map[string]map[string][]byte{}
	for _, row := range rows {
		if row.ID == "" {
			return layer.WriteResult{}, fmt.Errorf("block row without id")
		}
		b, err := json.Marshal(row)
		if err != nil {
			return layer.WriteResult{}, err
		}
		k := keyOf(row.SessionID)
		if grouped[k] == nil {
			grouped[k] = map[string][]byte{}
		}
		grouped[k][row.ID] = b
	}
	err := s.locked(func() error {
		for k, group := range grouped {
			if _, err := s.upsertLines(s.path("blocks", k), group); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return layer.WriteResult{}, err
	}
	return layer.WriteResult{RowsUpserted: len(rows)}, nil
}

// readSession returns the stored session object, or nil.
func (s *Store) readSession(key string) (map[string]json.RawMessage, error) {
	b, err := os.ReadFile(s.path("sessions", key))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(b, &obj); err != nil {
		return nil, fmt.Errorf("archive session %s: %w", key, err)
	}
	return obj, nil
}

func (s *Store) writeSession(key string, obj map[string]json.RawMessage) error {
	b, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	return s.write(s.path("sessions", key), append(b, '\n'))
}

func objectOf(v any) (map[string]json.RawMessage, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var obj map[string]json.RawMessage
	return obj, json.Unmarshal(b, &obj)
}

// outcomeKeys are the attributes the outcome sweep owns. Capture never
// overwrites them, as on Layer.
func outcomeKeys(row trace.SessionRow) map[string]bool {
	keys := map[string]bool{"outcome_details": true}
	if obj, err := objectOf(row.SessionOutcomes); err == nil {
		for k := range obj {
			keys[k] = true
		}
	}
	return keys
}

func isOutcomeKey(keys map[string]bool, k string) bool {
	return keys[k] || strings.HasPrefix(k, "ci_workflow_") || strings.HasPrefix(k, "commit_outcome_")
}

// WriteSessions advances session rows the way the Layer client does: an
// older source never rolls a newer row back, populated linkage and commits
// are kept and merged, and summaries and outcomes belong to their own
// writers.
func (s *Store) WriteSessions(rows []trace.SessionRow) (layer.WriteResult, error) {
	seen := map[string]bool{}
	for _, row := range rows {
		if row.ID == "" || seen[row.ID] {
			return layer.WriteResult{}, fmt.Errorf("missing or duplicate session ID")
		}
		seen[row.ID] = true
	}
	written := 0
	err := s.locked(func() error {
		for _, input := range rows {
			key := keyOf(input.ID)
			current, err := s.readSession(key)
			if err != nil {
				return err
			}
			row := input
			row.Commits = append(trace.StringList(nil), input.Commits...)
			if current != nil {
				var old trace.SessionRow
				linkage := map[string]json.RawMessage{}
				for _, k := range []string{"id", "session_id", "end", "commits", "pr", "workdir", "repo_url", "branch"} {
					if raw, ok := current[k]; ok {
						linkage[k] = raw
					}
				}
				b, _ := json.Marshal(linkage)
				if err := json.Unmarshal(b, &old); err != nil {
					return err
				}
				if old.End > input.End {
					continue
				}
				if old.SessionID != "" && input.SessionID != "" && old.SessionID != input.SessionID {
					return fmt.Errorf("session identity conflict: %s", input.ID)
				}
				if row.SessionID == "" {
					row.SessionID = old.SessionID
				}
				for _, pair := range []struct {
					dst *string
					src string
				}{{&row.PR, old.PR}, {&row.Workdir, old.Workdir}, {&row.RepoURL, old.RepoURL}, {&row.Branch, old.Branch}} {
					if pair.src != "" {
						*pair.dst = pair.src
					}
				}
				row.Commits = append(row.Commits, old.Commits...)
			}
			row.Commits = uniqueSorted(row.Commits)
			obj, err := objectOf(row)
			if err != nil {
				return err
			}
			if current != nil {
				delete(obj, "summary")
				delete(obj, "summary_reason")
				delete(obj, "summary_backfilled")
				owned := outcomeKeys(row)
				for k := range obj {
					if isOutcomeKey(owned, k) {
						delete(obj, k)
					}
				}
				for k, v := range obj {
					current[k] = v
				}
				obj = current
			}
			if err := s.writeSession(key, obj); err != nil {
				return err
			}
			written++
		}
		return nil
	})
	return layer.WriteResult{RowsUpserted: written}, err
}

func uniqueSorted(in trace.StringList) trace.StringList {
	sort.Strings(in)
	out := trace.StringList{}
	for _, v := range in {
		if len(out) == 0 || out[len(out)-1] != v {
			out = append(out, v)
		}
	}
	return out
}

// SummariesServed is always nil: files take patches.
func (s *Store) SummariesServed() error { return nil }

// PatchSessionSummaries sets only the summary on existing session rows.
func (s *Store) PatchSessionSummaries(rows []trace.SessionRow) (layer.WriteResult, error) {
	n := 0
	err := s.locked(func() error {
		for _, row := range rows {
			if strings.TrimSpace(row.Summary) == "" {
				continue
			}
			key := keyOf(row.ID)
			current, err := s.readSession(key)
			if err != nil {
				return err
			}
			if current == nil {
				continue
			}
			current["summary"], _ = json.Marshal(row.Summary)
			current["summary_reason"], _ = json.Marshal("")
			current["summary_backfilled"], _ = json.Marshal(true)
			if err := s.writeSession(key, current); err != nil {
				return err
			}
			n++
		}
		return nil
	})
	return layer.WriteResult{RowsUpserted: n}, err
}

// OutcomePage is a bounded id-cursor scan over sessions that ended at or
// after since, as on Layer.
func (s *Store) OutcomePage(ctx context.Context, after string, since int64, limit int) ([]trace.SessionRow, error) {
	if limit < 1 || limit > 1000 {
		return nil, fmt.Errorf("outcome page limit must be 1..1000")
	}
	filter := []any{"And", []any{[]any{"end", "Gte", since}, []any{"id", "Gt", after}}}
	rows, err := s.ListSessionRows(-1, filter)
	if err != nil {
		return nil, err
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	if len(rows) > limit {
		rows = rows[:limit]
	}
	for i := range rows {
		rows[i].FirstPrompt = ""
	}
	return rows, ctx.Err()
}

// PatchOutcomes writes only enrichment: outcomes, the PR and merged commits.
func (s *Store) PatchOutcomes(ctx context.Context, row trace.SessionRow) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.locked(func() error {
		key := keyOf(row.ID)
		current, err := s.readSession(key)
		if err != nil {
			return err
		}
		if current == nil {
			return fmt.Errorf("session %s is not in the archive", row.ID)
		}
		var stored trace.SessionRow
		b, _ := json.Marshal(current)
		if err := json.Unmarshal(b, &stored); err != nil {
			return err
		}
		row.Commits = uniqueSorted(append(append(trace.StringList{}, row.Commits...), stored.Commits...))
		if row.PR == "" {
			row.PR = stored.PR
		}
		patch, err := objectOf(row.SessionOutcomes)
		if err != nil {
			return err
		}
		if len(row.WorkflowAttributes) > 0 {
			details, err := json.Marshal(row.WorkflowAttributes)
			if err != nil {
				return err
			}
			patch["outcome_details"], _ = json.Marshal(string(details))
		}
		patch["pr"], _ = json.Marshal(row.PR)
		patch["commits"], _ = json.Marshal(row.Commits)
		for k, v := range patch {
			current[k] = v
		}
		return s.writeSession(key, current)
	})
}

// sessionFiles lists every session file key on disk.
func (s *Store) keys(kind string) ([]string, error) {
	shards, err := os.ReadDir(filepath.Join(s.dir, kind))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, sh := range shards {
		if !sh.IsDir() {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(s.dir, kind, sh.Name()))
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || strings.HasPrefix(name, ".") {
				continue // temporary files from an atomic write
			}
			ext := filepath.Ext(name)
			if ext != ".json" && ext != ".jsonl" {
				continue
			}
			out = append(out, strings.TrimSuffix(name, ext))
		}
	}
	sort.Strings(out)
	return out, nil
}

// sessionObjects reads every session row in parallel. Files that vanish
// mid-scan (a concurrent delete) are skipped.
func (s *Store) sessionObjects() ([][]byte, error) {
	keys, err := s.keys("sessions")
	if err != nil {
		return nil, err
	}
	out := make([][]byte, len(keys))
	errs := make([]error, len(keys))
	var wg sync.WaitGroup
	next := make(chan int)
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				b, err := os.ReadFile(s.path("sessions", keys[i]))
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				out[i], errs[i] = b, err
			}
		}()
	}
	for i := range keys {
		next <- i
	}
	close(next)
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	kept := out[:0]
	for _, b := range out {
		if len(b) > 0 {
			kept = append(kept, b)
		}
	}
	return kept, nil
}

// ListSessionRows reads session rows that match filter, newest start first.
// A positive topK keeps that many.
func (s *Store) ListSessionRows(topK int, filter any) ([]trace.SessionRow, error) {
	match, err := compileFilter(filter)
	if err != nil {
		return nil, err
	}
	var objects [][]byte
	if err := s.locked(func() (err error) { objects, err = s.sessionObjects(); return err }); err != nil {
		return nil, err
	}
	// Decode in parallel; a filter needs the generic form as well.
	type decoded struct {
		row  trace.SessionRow
		keep bool
		err  error
	}
	out := make([]decoded, len(objects))
	var wg sync.WaitGroup
	next := make(chan int)
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				d := &out[i]
				if filter != nil {
					m, err := decodeRow(objects[i])
					if err != nil {
						d.err = err
						continue
					}
					if !match(m) {
						continue
					}
				}
				d.err = json.Unmarshal(objects[i], &d.row)
				d.keep = d.err == nil
			}
		}()
	}
	for i := range objects {
		next <- i
	}
	close(next)
	wg.Wait()
	rows := make([]trace.SessionRow, 0, len(objects))
	for _, d := range out {
		if d.err != nil {
			return nil, fmt.Errorf("archive session: %w", d.err)
		}
		if d.keep {
			rows = append(rows, d.row)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Start != rows[j].Start {
			return rows[i].Start > rows[j].Start
		}
		return rows[i].ID < rows[j].ID
	})
	if topK > 0 && len(rows) > topK {
		rows = rows[:topK]
	}
	return rows, nil
}

// ListBlockRows reads one session's block rows in transcript order.
func (s *Store) ListBlockRows(sessionID string) ([]trace.BlockRow, error) {
	var lines [][]byte
	if err := s.locked(func() (err error) { lines, err = readLines(s.path("blocks", keyOf(sessionID))); return err }); err != nil {
		return nil, err
	}
	rows := make([]trace.BlockRow, 0, len(lines))
	for _, line := range lines {
		var row trace.BlockRow
		if err := json.Unmarshal(line, &row); err != nil {
			return nil, fmt.Errorf("archive block: %w", err)
		}
		if row.SessionID == sessionID {
			rows = append(rows, row)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Seq < rows[j].Seq })
	return rows, nil
}

// chunkRows reads one key's chunk rows.
func (s *Store) chunkRows(key string) ([]layer.Row, error) {
	lines, err := readLines(s.path("chunks", key))
	if err != nil {
		return nil, err
	}
	rows := make([]layer.Row, 0, len(lines))
	for _, line := range lines {
		var row layer.Row
		if err := json.Unmarshal(line, &row); err != nil {
			return nil, fmt.Errorf("archive chunk: %w", err)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// SessionRows returns the chunks of every session whose id starts with
// prefix, in transcript order: what `hev traces show` reassembles.
func (s *Store) SessionRows(prefix string) (rows []layer.Row, err error) {
	if prefix == "" {
		return nil, nil
	}
	err = s.locked(func() error { rows, err = s.sessionRows(prefix); return err })
	return rows, err
}

func (s *Store) sessionRows(prefix string) ([]layer.Row, error) {
	keys, err := s.keys("chunks")
	if err != nil {
		return nil, err
	}
	var rows []layer.Row
	for _, k := range keys {
		if k == instructionsKey {
			continue
		}
		if !strings.HasPrefix(k, "~") && !strings.HasPrefix(k, prefix) && !strings.HasPrefix(prefix, k) {
			continue
		}
		chunks, err := s.chunkRows(k)
		if err != nil {
			return nil, err
		}
		for _, r := range chunks {
			if strings.HasPrefix(r.SessionID, prefix) {
				rows = append(rows, r)
			}
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].SessionID != rows[j].SessionID {
			return rows[i].SessionID < rows[j].SessionID
		}
		if rows[i].Seq != rows[j].Seq {
			return rows[i].Seq < rows[j].Seq
		}
		return rows[i].Part < rows[j].Part
	})
	return rows, nil
}

// ArchiveIdentityRows lists ownership coordinates for the redaction
// migration: chunks (suffix ""), blocks ("-blocks") or sessions ("-sessions").
func (s *Store) ArchiveIdentityRows(suffix string) (out []layer.Row, err error) {
	err = s.locked(func() error { out, err = s.archiveIdentityRows(suffix); return err })
	return out, err
}

func (s *Store) archiveIdentityRows(suffix string) ([]layer.Row, error) {
	var out []layer.Row
	switch suffix {
	case "-sessions":
		objects, err := s.sessionObjects()
		if err != nil {
			return nil, err
		}
		for _, b := range objects {
			var r layer.Row
			if err := json.Unmarshal(b, &r); err != nil {
				return nil, err
			}
			out = append(out, layer.Row{ID: r.ID, SessionID: r.SessionID, Host: r.Host, Harness: r.Harness})
		}
	case "", "-blocks":
		kind := "chunks"
		if suffix == "-blocks" {
			kind = "blocks"
		}
		keys, err := s.keys(kind)
		if err != nil {
			return nil, err
		}
		for _, k := range keys {
			lines, err := readLines(s.path(kind, k))
			if err != nil {
				return nil, err
			}
			for _, line := range lines {
				var r layer.Row
				if err := json.Unmarshal(line, &r); err != nil {
					return nil, err
				}
				id := layer.Row{ID: r.ID, SessionID: r.SessionID}
				if kind == "chunks" {
					id.SourcePath, id.Host, id.Harness = r.SourcePath, r.Host, r.Harness
				}
				out = append(out, id)
			}
		}
	default:
		return nil, fmt.Errorf("unknown archive namespace suffix %q", suffix)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// DeleteSessionArchive removes a session's chunks, blocks and row.
func (s *Store) DeleteSessionArchive(id string) error {
	key := keyOf(id)
	return s.locked(func() error {
		for _, kind := range []string{"chunks", "blocks", "sessions"} {
			if err := remove(s.path(kind, key)); err != nil {
				return err
			}
		}
		return s.markDirty([]string{key})
	})
}

func (s *Store) instructionsPath() string { return filepath.Join(s.dir, "instructions.jsonl") }

// InstructionVersions lists the versions recorded for one file on one host.
func (s *Store) InstructionVersions(path, host string) ([]layer.InstructionVersion, error) {
	var lines [][]byte
	if err := s.locked(func() (err error) { lines, err = readLines(s.instructionsPath()); return err }); err != nil {
		return nil, err
	}
	var out []layer.InstructionVersion
	for _, line := range lines {
		var v layer.InstructionVersion
		if err := json.Unmarshal(line, &v); err != nil {
			return nil, err
		}
		if v.Path == path && v.Host == host {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// WriteInstructionVersions upserts version rows by id.
func (s *Store) WriteInstructionVersions(rows []layer.InstructionVersion) error {
	group := map[string][]byte{}
	for _, r := range rows {
		b, err := json.Marshal(r)
		if err != nil {
			return err
		}
		group[r.ID] = b
	}
	return s.locked(func() error {
		_, err := s.upsertLines(s.instructionsPath(), group)
		return err
	})
}
