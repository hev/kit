package index

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
)

// This fixture models persistent namespaces, exact deletion, read-back and
// whole-row upserts. Assertions inspect retained rows, not just sent requests.
type migrationStore struct {
	mu                sync.Mutex
	rows              map[string]map[string]map[string]any
	failSuffix        string
	failDelete        bool
	ignoreDelete      bool
	writes            int
	deletes           int
	afterSessionWrite func()
}

func (s *migrationStore) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var body map[string]any
	json.NewDecoder(r.Body).Decode(&body)
	ns := strings.TrimPrefix(r.URL.Path, "/v2/namespaces/")
	query := strings.HasSuffix(ns, "/query")
	ns = strings.TrimSuffix(ns, "/query")
	if s.rows[ns] == nil {
		s.rows[ns] = map[string]map[string]any{}
	}
	if query {
		var rows []map[string]any
		for _, row := range s.rows[ns] {
			if fixtureFilter(row, body["filters"]) {
				rows = append(rows, row)
			}
		}
		sort.Slice(rows, func(i, j int) bool { return fmt.Sprint(rows[i]["id"]) < fmt.Sprint(rows[j]["id"]) })
		if k, ok := body["top_k"].(float64); ok && len(rows) > int(k) {
			rows = rows[:int(k)]
		}
		json.NewEncoder(w).Encode(map[string]any{"rows": rows})
		return
	}
	if filter, ok := body["delete_by_filter"]; ok {
		s.deletes++
		if s.failDelete {
			http.Error(w, "injected cleanup failure", 500)
			return
		}
		if !s.ignoreDelete {
			for id, row := range s.rows[ns] {
				if fixtureFilter(row, filter) {
					delete(s.rows[ns], id)
				}
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"status": "OK"})
		return
	}
	if rows, ok := body["upsert_rows"].([]any); ok {
		s.writes++
		if s.failSuffix != "" && strings.HasSuffix(ns, s.failSuffix) {
			http.Error(w, "injected write failure", 400)
			return
		}
		for _, raw := range rows {
			row := raw.(map[string]any)
			s.rows[ns][row["id"].(string)] = row
		}
		if strings.HasSuffix(ns, "-sessions") && s.afterSessionWrite != nil {
			s.afterSessionWrite()
		}
	}
	json.NewEncoder(w).Encode(map[string]any{"status": "OK", "rows_upserted": 1})
}
func fixtureFilter(row map[string]any, raw any) bool {
	if raw == nil {
		return true
	}
	f := raw.([]any)
	if f[0] == "And" {
		for _, child := range f[1].([]any) {
			if !fixtureFilter(row, child) {
				return false
			}
		}
		return true
	}
	value := row[f[0].(string)]
	switch f[1] {
	case "Eq":
		return value == f[2]
	case "Gt":
		return fmt.Sprint(value) > fmt.Sprint(f[2])
	case "In":
		for _, v := range f[2].([]any) {
			if value == v {
				return true
			}
		}
		return false
	}
	panic("unsupported fixture filter")
}
func migrationFixture(t *testing.T, kind string) (*trace.ClaudeSource, *layer.Client, *migrationStore) {
	t.Helper()
	t.Setenv("HEV_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	root := t.TempDir()
	path := filepath.Join(root, "session.jsonl")
	line := map[string]any{"type": "user", "uuid": "u1", "sessionId": "s1", "timestamp": "2026-10-01T10:00:00Z", "message": map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": fixtureText()}, map[string]any{"type": "tool_result", "text": fixtureText()}}}}
	raw, _ := json.Marshal(line)
	if err := os.WriteFile(path, append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	store := &migrationStore{rows: map[string]map[string]map[string]any{}}
	for _, suffix := range []string{"", "-blocks", "-sessions"} {
		store.rows["archive"+suffix] = map[string]map[string]any{
			"old":       {"id": "old", "session_id": "s1", "text": fixtureText(), "summary": fixtureText(), "first_prompt": fixtureText(), "source_path": path, "host": layer.Hostname(), "harness": "claude_code"},
			"gone":      {"id": "gone", "session_id": "gone", "text": fixtureText(), "source_path": filepath.Join(root, "missing.jsonl"), "host": layer.Hostname(), "harness": "claude_code"},
			"unrelated": {"id": "unrelated", "session_id": "other", "text": "unrelated archive", "source_path": "/other/root/transcript.jsonl", "host": layer.Hostname(), "harness": "claude_code"},
		}
	}
	srv := httptest.NewServer(store)
	t.Cleanup(srv.Close)
	cl, err := layer.New(srv.URL, "key", "archive", "").WithStore(kind)
	if err != nil {
		t.Fatal(err)
	}
	return &trace.ClaudeSource{Root: root}, cl, store
}
func assertMigrationComplete(t *testing.T, want bool) {
	t.Helper()
	config := os.Getenv("HEV_CONFIG")
	paths, _ := filepath.Glob(filepath.Join(filepath.Dir(config), "archive-redaction-*.json"))
	complete := false
	for _, p := range paths {
		raw, _ := os.ReadFile(p)
		var st migrationState
		if err := json.Unmarshal(raw, &st); err != nil {
			t.Fatal(err)
		}
		complete = complete || st.Complete
	}
	if complete != want {
		t.Fatalf("completion = %v, want %v", complete, want)
	}
}
func assertMigratedRows(t *testing.T, s *migrationStore) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	for ns, rows := range s.rows {
		if !strings.HasPrefix(ns, "archive") {
			continue
		}
		if rows["unrelated"] == nil {
			t.Fatalf("unrelated archive deleted in %s", ns)
		}
		if rows["old"] != nil || rows["gone"] != nil {
			t.Fatalf("legacy row retained in %s", ns)
		}
		raw, _ := json.Marshal(rows)
		assertClean(t, string(raw))
	}
	if len(s.rows["archive"]) < 2 || len(s.rows["archive-blocks"]) < 2 || len(s.rows["archive-sessions"]) < 2 {
		t.Fatal("scrubbed rebuild missing")
	}
}
func TestMigrationFirstUpgradeAllLanesAndPaths(t *testing.T) {
	for _, kind := range []string{"pgvector", "turbopuffer"} {
		for _, mode := range []string{"normal", "read-side", "summary"} {
			t.Run(kind+"/"+mode, func(t *testing.T) {
				src, cl, store := migrationFixture(t, kind)
				units, _ := src.Units()
				st := &State{Units: map[string]string{units[0].Key: units[0].Signature}}
				var err error
				if mode == "summary" {
					_, err = Summarize(src, cl, 200, nil, nil, func(trace.SessionRow) (string, error) { return fixtureText(), nil }, nil)
				} else {
					_, err = Run(src, cl, st, Options{ReadSide: mode == "read-side", Limit: 1})
					if mode == "normal" && err != nil {
						_, err = Run(src, cl, st, Options{Limit: 1})
					}
				}
				if err != nil {
					t.Fatal(err)
				}
				assertMigratedRows(t, store)
				assertMigrationComplete(t, true)
				if mode != "summary" {
					writes := store.writes
					rep, err := Run(src, cl, st, Options{})
					if err != nil || rep.UnitsSkipped != 1 || store.writes != writes {
						t.Fatalf("unchanged retry: %+v %v writes %d/%d", rep, err, store.writes, writes)
					}
				}
			})
		}
	}
}
func TestMigrationFailuresAndInterruptedRetries(t *testing.T) {
	for _, failure := range []string{"delete", "retained", "blocks", "sessions"} {
		t.Run(failure, func(t *testing.T) {
			src, cl, store := migrationFixture(t, "pgvector")
			switch failure {
			case "delete":
				store.failDelete = true
			case "retained":
				store.ignoreDelete = true
			default:
				store.failSuffix = "-" + failure
			}
			_, err := Run(src, cl, &State{Units: map[string]string{}}, Options{})
			if err == nil {
				t.Fatal("failure accepted")
			}
			assertMigrationComplete(t, false)
			// Simulate a new process: no unit state. Some raw ownership rows have
			// already disappeared; the saved journal must still remove all old rows.
			store.failDelete = false
			store.ignoreDelete = false
			store.failSuffix = ""
			if _, err := Run(src, cl, &State{Units: map[string]string{}}, Options{}); err != nil {
				t.Fatal(err)
			}
			assertMigratedRows(t, store)
			assertMigrationComplete(t, true)
		})
	}
}
func TestMigrationMissingRootAndUnattributableOrphans(t *testing.T) {
	t.Run("missing-root", func(t *testing.T) {
		src, cl, store := migrationFixture(t, "turbopuffer")
		os.RemoveAll(src.Root)
		if _, err := Run(src, cl, &State{Units: map[string]string{}}, Options{}); err != nil {
			t.Fatal(err)
		}
		for _, rows := range store.rows {
			if rows["old"] != nil || rows["gone"] != nil {
				t.Fatal("missing source retained raw rows")
			}
			if rows["unrelated"] == nil {
				t.Fatal("unrelated removed")
			}
		}
		assertMigrationComplete(t, true)
	})
	t.Run("orphan-summary", func(t *testing.T) {
		src, cl, store := migrationFixture(t, "pgvector")
		delete(store.rows["archive"], "gone")
		delete(store.rows["archive-blocks"], "gone")
		if _, err := Run(src, cl, &State{Units: map[string]string{}}, Options{}); err == nil || !strings.Contains(err.Error(), "orphan session") {
			t.Fatalf("orphan accepted: %v", err)
		}
		if store.deletes != 0 {
			t.Fatal("deleted before ownership validation")
		}
		assertMigrationComplete(t, false)
	})
}
func TestMigrationOptOutAndArchiveIdentity(t *testing.T) {
	src, cl, store := migrationFixture(t, "pgvector")
	config := os.Getenv("HEV_CONFIG")
	if err := os.WriteFile(config, []byte("[capture]\nredact = false\n"), 0600); err != nil {
		t.Fatal(err)
	}
	st := &State{Units: map[string]string{}}
	if _, err := Run(src, cl, st, Options{}); err != nil {
		t.Fatal(err)
	}
	if store.deletes != 0 || store.rows["archive"]["old"] == nil {
		t.Fatal("opt-out migrated")
	}
	assertMigrationComplete(t, false)
	os.WriteFile(config, []byte("[capture]\nredact = true\n"), 0600)
	if _, err := Run(src, cl, st, Options{}); err != nil {
		t.Fatal(err)
	}
	assertMigratedRows(t, store)
	// Another namespace, even with unchanged source signatures, gets its own
	// migration and cannot inherit this archive's completion.
	cl.Namespace = "another"
	if _, err := Run(src, cl, st, Options{}); err != nil {
		t.Fatal(err)
	}
	if len(store.rows["another"]) == 0 {
		t.Fatal("namespace inherited signatures")
	}
	cl.APIKey = "another-account"
	if _, err := Run(src, cl, st, Options{}); err != nil {
		t.Fatal(err)
	}
	paths, _ := filepath.Glob(filepath.Join(filepath.Dir(config), "archive-redaction-*.json"))
	if len(paths) != 3 {
		t.Fatalf("identity journals: %d", len(paths))
	}
}

func TestMigrationSourceReadFailureAndCorruptJournal(t *testing.T) {
	t.Run("read", func(t *testing.T) {
		src, cl, store := migrationFixture(t, "pgvector")
		// An existing transcript whose path is a dangling symlink is an unreadable
		// source, not a missing-root archive that may be discarded.
		path := filepath.Join(src.Root, "session.jsonl")
		os.Remove(path)
		os.Symlink(filepath.Join(src.Root, "absent"), path)
		if _, err := Run(src, cl, &State{Units: map[string]string{}}, Options{}); err == nil {
			t.Fatal("source failure accepted")
		}
		if store.deletes != 0 {
			t.Fatal("cleanup preceded source read")
		}
		assertMigrationComplete(t, false)
	})
	t.Run("journal", func(t *testing.T) {
		src, cl, store := migrationFixture(t, "pgvector")
		path := filepath.Join(filepath.Dir(os.Getenv("HEV_CONFIG")), "archive-redaction-"+archiveKey(cl, src.Root)+".json")
		os.WriteFile(path, []byte("{broken"), 0600)
		if _, err := Run(src, cl, &State{Units: map[string]string{}}, Options{}); err == nil {
			t.Fatal("corrupt journal accepted")
		}
		if store.deletes != 0 {
			t.Fatal("deleted after journal error")
		}
	})
}
func TestMigrationConcurrentWritersAndReenabledOptOut(t *testing.T) {
	src, cl, store := migrationFixture(t, "pgvector")
	st := &State{Units: map[string]string{}}
	errors := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { _, err := Run(src, cl, st, Options{}); errors <- err }()
	}
	for i := 0; i < 2; i++ {
		if err := <-errors; err != nil {
			t.Fatal(err)
		}
	}
	if store.deletes != 6 {
		t.Fatalf("concurrent migration deleted %d times, want 6", store.deletes)
	}
	assertMigratedRows(t, store)
	config := os.Getenv("HEV_CONFIG")
	raw, _ := os.ReadFile(config)
	disabled := strings.Replace(string(raw), "[capture]", "[capture]\nredact = false", 1)
	os.WriteFile(config, []byte(disabled), 0600)
	if _, err := Run(src, cl, st, Options{Force: true}); err != nil {
		t.Fatal(err)
	}
	assertMigrationComplete(t, false)
	os.WriteFile(config, raw, 0600)
	if _, err := Run(src, cl, st, Options{}); err != nil {
		t.Fatal(err)
	}
	assertMigratedRows(t, store)
	assertMigrationComplete(t, true)
}

func TestCodexMigrationAndDryRun(t *testing.T) {
	src, cl, store := migrationFixture(t, "turbopuffer")
	path := filepath.Join(src.Root, "session.jsonl")
	first, _ := json.Marshal(map[string]any{"timestamp": "2026-10-01T10:00:00Z", "type": "session_meta", "payload": map[string]any{"id": "s1"}})
	second, _ := json.Marshal(map[string]any{"timestamp": "2026-10-01T10:00:01Z", "type": "event_msg", "payload": map[string]any{"type": "user_message", "message": fixtureText()}})
	os.WriteFile(path, append(append(append(first, '\n'), second...), '\n'), 0600)
	for _, rows := range store.rows {
		for _, row := range rows {
			row["harness"] = "codex"
		}
	}
	codex := &trace.CodexSource{Root: src.Root}
	st := &State{Units: map[string]string{}}
	if _, err := Run(codex, cl, st, Options{DryRun: true}); err != nil {
		t.Fatal(err)
	}
	if store.deletes != 0 || store.writes != 0 {
		t.Fatal("dry run mutated archive")
	}
	assertMigrationComplete(t, false)
	if _, err := Run(codex, cl, st, Options{}); err != nil {
		t.Fatal(err)
	}
	assertMigratedRows(t, store)
	assertMigrationComplete(t, true)
}

func TestMigrationCompletionSaveFailureRetries(t *testing.T) {
	src, cl, store := migrationFixture(t, "pgvector")
	dir := filepath.Dir(os.Getenv("HEV_CONFIG"))
	store.afterSessionWrite = func() {
		if err := os.Chmod(dir, 0500); err != nil {
			t.Error(err)
		}
	}
	t.Cleanup(func() { os.Chmod(dir, 0700) })
	if _, err := Run(src, cl, &State{Units: map[string]string{}}, Options{}); err == nil {
		t.Fatal("completion save failure accepted")
	}
	assertMigrationComplete(t, false)
	paths, _ := filepath.Glob(filepath.Join(dir, "archive-redaction-*.json"))
	for _, p := range paths {
		raw, _ := os.ReadFile(p)
		assertClean(t, string(raw))
	}
	os.Chmod(dir, 0700)
	store.afterSessionWrite = nil
	if _, err := Run(src, cl, &State{Units: map[string]string{}}, Options{}); err != nil {
		t.Fatal(err)
	}
	assertMigrationComplete(t, true)
	assertMigratedRows(t, store)
}
