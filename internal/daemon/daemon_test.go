package daemon

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/hev/kit/internal/index"
	"github.com/hev/kit/internal/layer"
)

func TestIndexCycleRetriesOfflineAndPicksUpGrowingTranscript(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, ".claude", "projects", "project")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	transcript := filepath.Join(root, "session.jsonl")
	writeTurn := func(id, text string, appendFile bool) {
		flag := os.O_CREATE | os.O_WRONLY
		if appendFile {
			flag |= os.O_APPEND
		} else {
			flag |= os.O_TRUNC
		}
		f, err := os.OpenFile(transcript, flag, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		fmt.Fprintf(f, `{"type":"user","uuid":%q,"sessionId":"s1","timestamp":"2026-09-05T10:00:00Z","cwd":"/work","message":{"role":"user","content":%q}}`+"\n", id, text)
	}
	secret := "AKIAABCDEFGHIJKLMNOP"
	writeTurn("one", "first unique phrase "+secret, false)
	t.Setenv("HEV_CONFIG", filepath.Join(home, ".hev", "config.toml"))

	offline := true
	writes, deletes := 0, 0
	var mu sync.Mutex
	namespaces := map[string]map[string]map[string]any{}
	for _, suffix := range []string{"", "-blocks", "-sessions"} {
		namespaces["archive"+suffix] = map[string]map[string]any{"legacy": {
			"id": "legacy", "session_id": "s1", "text": secret, "summary": secret,
			"first_prompt": secret, "source_path": transcript, "host": layer.Hostname(), "harness": "claude_code",
		}}
	}
	var matches func(map[string]any, any) bool
	matches = func(row map[string]any, filter any) bool {
		if filter == nil {
			return true
		}
		f := filter.([]any)
		if f[0] == "And" {
			for _, c := range f[1].([]any) {
				if !matches(row, c) {
					return false
				}
			}
			return true
		}
		switch f[1] {
		case "Eq":
			return reflect.DeepEqual(row[f[0].(string)], f[2])
		case "Gt":
			return fmt.Sprint(row[f[0].(string)]) > fmt.Sprint(f[2])
		case "In":
			for _, value := range f[2].([]any) {
				if row[f[0].(string)] == value {
					return true
				}
			}
			return false
		default:
			t.Errorf("unexpected filter: %v", filter)
			return false
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if offline {
			http.Error(w, "unreachable", http.StatusServiceUnavailable)
			return
		}
		if r.Method == "GET" && strings.HasSuffix(r.URL.Path, "-sessions/schema") {
			fmt.Fprint(w, `{}`)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			http.Error(w, "bad fixture request", 400)
			return
		}
		ns := strings.TrimPrefix(r.URL.Path, "/v2/namespaces/")
		query := strings.HasSuffix(ns, "/query")
		ns = strings.TrimSuffix(ns, "/query")
		if namespaces[ns] == nil {
			namespaces[ns] = map[string]map[string]any{}
		}
		if query {
			rows := []map[string]any{}
			for _, row := range namespaces[ns] {
				if matches(row, body["filters"]) {
					rows = append(rows, row)
				}
			}
			json.NewEncoder(w).Encode(map[string]any{"rows": rows})
			return
		}
		if filter, cleanup := body["delete_by_filter"]; cleanup {
			deletes++
			for id, row := range namespaces[ns] {
				if matches(row, filter) {
					delete(namespaces[ns], id)
				}
			}
		} else if rows, ok := body["upsert_rows"].([]any); ok {
			writes++
			for _, raw := range rows {
				row := raw.(map[string]any)
				namespaces[ns][row["id"].(string)] = row
			}
		} else if rows, ok := body["patch_rows"].([]any); ok {
			writes++
			for _, raw := range rows {
				p := raw.(map[string]any)
				current := namespaces[ns][p["id"].(string)]
				if matches(current, body["patch_condition"]) {
					for k, v := range p {
						current[k] = v
					}
				}
			}
		} else {
			t.Errorf("unexpected write: %v", body)
		}
		io.WriteString(w, `{"status":"OK","rows_upserted":1,"rows_affected":1}`)
	}))
	defer srv.Close()
	client := layer.New(srv.URL, "key", "archive", "")
	state := &index.State{Units: map[string]string{}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	failed := runIndexCycle(client, state, logger)
	if failed.LastError == "" || len(state.Units) != 0 || writes != 0 || deletes != 0 {
		t.Fatalf("offline cycle = %+v, state=%v", failed, state.Units)
	}
	offline = false
	caughtUp := runIndexCycle(client, state, logger)
	if caughtUp.LastError != "" || caughtUp.UnitsIndexed != 1 || writes != 3 || deletes != 0 {
		t.Fatalf("catch-up = %+v, writes=%d", caughtUp, writes)
	}

	// Startup leaves historical rows intact and redacts newly ingested rows.
	for ns, rows := range namespaces {
		if rows["legacy"] == nil {
			t.Fatalf("startup removed legacy row in %s", ns)
		}
		for id, row := range rows {
			if id == "legacy" {
				continue
			}
			raw, _ := json.Marshal(row)
			if strings.Contains(string(raw), secret) {
				t.Fatalf("new row contains secret in %s", ns)
			}
		}
	}
	namespaces["archive-sessions"]["s1"]["summary"] = "independent summary"
	namespaces["archive-sessions"]["s1"]["merged"] = "true"
	namespaces["archive-sessions"]["s1"]["pr"] = "30"
	// Reload persisted state as a restarted daemon would.
	state = index.LoadState()
	unchanged := runIndexCycle(client, state, logger)
	if unchanged.LastError != "" || unchanged.UnitsIndexed != 0 || writes != 3 || deletes != 0 {
		t.Fatalf("unchanged cycle = %+v, upserts=%d deletes=%d", unchanged, writes, deletes)
	}
	writeTurn("two", "second phrase added while live", true)
	// An ordinary offline retry with migration still pending must keep the last
	// successful signatures, then ingest the growing source when connectivity
	// returns, without running upgrade cleanup again.
	before, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	offline = true
	mu.Unlock()
	growthOffline := runIndexCycle(client, state, logger)
	after, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if growthOffline.LastError == "" || string(after) != string(before) || writes != 3 || deletes != 0 {
		t.Fatalf("offline growth = %+v, upserts=%d deletes=%d", growthOffline, writes, deletes)
	}
	mu.Lock()
	offline = false
	mu.Unlock()
	grown := runIndexCycle(client, state, logger)
	if grown.LastError != "" || grown.UnitsIndexed != 1 || writes != 6 || deletes != 0 {
		t.Fatalf("growth cycle = %+v, upserts=%d deletes=%d", grown, writes, deletes)
	}
	newBlocks := map[string]map[string]any{}
	for id, row := range namespaces["archive-blocks"] {
		if id != "legacy" {
			newBlocks[id] = row
		}
	}
	if row := namespaces["archive-sessions"]["s1"]; row["summary"] != "independent summary" || row["merged"] != "true" || row["pr"] != "30" {
		t.Fatalf("advancing ingestion erased enrichment: %v", row)
	}
	raw, _ := json.Marshal(newBlocks)
	if !strings.Contains(string(raw), "second phrase added while live") || strings.Contains(string(raw), secret) {
		t.Fatal("growth failed to retain the new turn with scrubbed old turns")
	}
	final := runIndexCycle(client, state, logger)
	if final.LastError != "" || final.UnitsIndexed != 0 || writes != 6 || deletes != 0 {
		t.Fatalf("growth retry duplicated writes: %+v", final)
	}
}
