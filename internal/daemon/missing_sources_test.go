package daemon

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/hev/kit/internal/index"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
)

// A pre-LYR-224 archive holds sessions whose transcripts are gone. The daemon
// must not treat that as license to run the historical migration, which
// deletes those sessions: it reports migration_pending and issues no delete.
func TestDaemonCycleNeverDeletesArchiveRowsWithMissingSources(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	config := filepath.Join(home, ".hev", "config.toml")
	t.Setenv("HEV_CONFIG", config)
	os.MkdirAll(filepath.Dir(config), 0700)
	os.WriteFile(config, []byte("[capture]\nredact_salt='"+strings.Repeat("ab", 32)+"'\n"), 0600)
	root := trace.DefaultClaudeRoot()
	os.MkdirAll(filepath.Join(root, "project"), 0700)
	os.WriteFile(filepath.Join(root, "project", "live.jsonl"), []byte("{\"type\":\"user\",\"uuid\":\"u\",\"sessionId\":\"live\",\"message\":{\"role\":\"user\",\"content\":\"hello\"}}\n"), 0600)
	gone := filepath.Join(root, "project", "gone.jsonl")

	var mu sync.Mutex
	var mutations []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == "GET":
			io.WriteString(w, `{}`)
		case strings.HasSuffix(r.URL.Path, "/query"):
			rows := []any{}
			if !strings.HasSuffix(r.URL.Path, "-blocks") && !strings.HasSuffix(r.URL.Path, "-sessions") {
				rows = append(rows, map[string]any{"id": "c1", "attributes": map[string]any{"session_id": "gone", "source_path": gone, "host": layer.Hostname(), "harness": "claude_code"}})
			}
			json.NewEncoder(w).Encode(map[string]any{"rows": rows})
		default:
			mutations = append(mutations, r.Method+" "+r.URL.Path)
			io.WriteString(w, `{"rows_affected":1}`)
		}
	}))
	defer srv.Close()

	// Pre-LYR-224 state: unscoped signatures, no migration journal.
	units, _ := (&trace.ClaudeSource{Root: root}).Units()
	state := &index.State{Units: map[string]string{}}
	for _, u := range units {
		state.Units[u.Key] = u.Signature
	}
	status := runIndexCycle(layer.New(srv.URL, "key", "ns", ""), state, slog.New(slog.NewTextHandler(io.Discard, nil)))
	for _, m := range mutations {
		if strings.Contains(m, "DELETE") || strings.HasSuffix(m, "/v2/namespaces/ns") || strings.HasSuffix(m, "-blocks") || strings.HasSuffix(m, "-sessions") {
			t.Errorf("daemon mutated archive: %s", m)
		}
	}
	if len(mutations) != 0 {
		t.Errorf("mutations = %v", mutations)
	}
	if !status.MigrationPending {
		t.Errorf("status = %+v, want migration_pending", status)
	}
}
