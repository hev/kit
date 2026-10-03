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
	"testing"

	"github.com/hev/kit/internal/index"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
)

func TestLegacyStartupDefersHistoricalRebuildAndInstructionDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	config := filepath.Join(home, ".hev", "config.toml")
	t.Setenv("HEV_CONFIG", config)
	os.MkdirAll(filepath.Dir(config), 0700)
	original := "[capture]\nredact_salt='" + strings.Repeat("ab", 32) + "'\n"
	os.WriteFile(config, []byte(original), 0600)
	root := trace.DefaultClaudeRoot()
	os.MkdirAll(filepath.Join(root, "project"), 0700)
	transcript := filepath.Join(root, "project", "session.jsonl")
	os.WriteFile(transcript, []byte("{\"type\":\"user\",\"uuid\":\"u\",\"sessionId\":\"s\",\"message\":{\"role\":\"user\",\"content\":\"historical text\"}}\n"), 0600)
	os.WriteFile(filepath.Join(home, ".claude", "CLAUDE.md"), []byte("instructions must not be backfilled"), 0600)
	units, err := (&trace.ClaudeSource{Root: root}).Units()
	if err != nil || len(units) != 1 {
		t.Fatalf("units=%v err=%v", units, err)
	}
	files, err := (trace.InstructionSource{Home: home}).Files()
	if err != nil || len(files) != 1 {
		t.Fatalf("instruction fixture not discoverable: %v %v", files, err)
	}
	state := &index.State{Units: map[string]string{units[0].Key: units[0].Signature}}
	protected := map[string]any{"id": "s", "summary": "independent summary", "merged": "true", "pr": "30", "commits": []any{"full-sha"}}
	writes := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			io.WriteString(w, `{}`)
			return
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if strings.HasSuffix(r.URL.Path, "/query") {
			json.NewEncoder(w).Encode(map[string]any{"rows": []any{protected}})
			return
		}
		writes++
		t.Errorf("unexpected historical write/delete/reembed: %s %v", r.URL.Path, body)
		io.WriteString(w, `{"rows_affected":1}`)
	}))
	defer srv.Close()
	status := runIndexCycle(layer.New(srv.URL, "key", "ns", ""), state, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if status.LastError != "" || !status.MigrationPending || status.UnitsIndexed != 0 || writes != 0 {
		t.Fatalf("status=%+v writes=%d", status, writes)
	}
	journals, _ := filepath.Glob(filepath.Join(home, ".hev", "archive-redaction-*.json"))
	if len(journals) != 0 {
		t.Fatalf("created false journal: %v", journals)
	}
	after, _ := os.ReadFile(config)
	if string(after) != original {
		t.Fatal("startup changed config")
	}
	if protected["summary"] != "independent summary" || protected["merged"] != "true" || protected["pr"] != "30" {
		t.Fatal("protected enrichment changed")
	}
}
