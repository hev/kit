package serve

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/filearchive"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
)

// TestServeFromFileArchive is `hev serve` with no Layer key: the dashboard
// reads sessions, transcripts and search hits from the file archive through
// the Reader seam, and the page says search is lexical.
func TestServeFromFileArchive(t *testing.T) {
	store, err := filearchive.Open(t.TempDir(), "hev-traces")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	ts := now.Add(-time.Hour).UTC().Format(time.RFC3339)
	for _, s := range []struct{ id, harness, text string }{
		{"aaaa1111", "claude_code", "the preflight failed because the gateway rejected the key"},
		{"bbbb2222", "codex", "the deploy was flaky on friday"},
	} {
		if _, err := store.Write([]layer.Row{{ID: s.id + "-c0", SessionID: s.id, TS: ts, Harness: s.harness, Role: "assistant", BlockType: "text", Tier: "text", Text: s.text}}); err != nil {
			t.Fatal(err)
		}
		if _, err := store.WriteBlocks([]trace.BlockRow{{ID: s.id + "-b0", SessionID: s.id, Role: "assistant", BlockType: "text", Text: s.text, Start: now.Add(-time.Hour).UnixMilli(), End: now.Add(-time.Hour).UnixMilli(), OK: true}}); err != nil {
			t.Fatal(err)
		}
		if _, err := store.WriteSessions([]trace.SessionRow{{ID: s.id, SessionID: s.id, Harness: s.harness, Model: "claude-sonnet-4-6", Host: "laptop", Start: now.Add(-time.Hour).UnixMilli(), End: now.Add(-50 * time.Minute).UnixMilli(), FirstPrompt: s.text, PromptCount: 1, Cost: 0.25}}); err != nil {
			t.Fatal(err)
		}
	}
	h := New(store).WithCacheTTL(0).Handler()
	get := func(path string, out any) int {
		t.Helper()
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if out != nil && w.Code == 200 {
			if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
				t.Fatalf("%s: %v: %s", path, err, w.Body.String())
			}
		}
		return w.Code
	}
	var list struct {
		Sessions []map[string]any `json:"sessions"`
	}
	if code := get("/api/sessions?window=7d", &list); code != 200 || len(list.Sessions) != 2 {
		t.Fatalf("sessions %d: %+v", code, list)
	}
	var filtered struct {
		Sessions []map[string]any `json:"sessions"`
	}
	if code := get("/api/sessions?window=7d&harness=codex", &filtered); code != 200 || len(filtered.Sessions) != 1 || filtered.Sessions[0]["id"] != "bbbb2222" {
		t.Fatalf("filtered %d: %+v", code, filtered)
	}
	var one map[string]any
	if code := get("/api/session/aaaa1111", &one); code != 200 || !strings.Contains(mustJSON(t, one), "gateway rejected the key") {
		t.Fatalf("session %d: %v", code, one)
	}
	var hits struct {
		Sessions []map[string]any `json:"sessions"`
		Storage  map[string]any   `json:"storage"`
	}
	if code := get("/api/search?window=7d&q=gateway+key", &hits); code != 200 || len(hits.Sessions) != 1 || hits.Sessions[0]["id"] != "aaaa1111" || hits.Storage["search"] != "lexical" {
		t.Fatalf("search %d: %+v", code, hits)
	}
	if code := get("/api/search?window=7d&mode=phrase&q=deploy+was+flaky", &hits); code != 200 || len(hits.Sessions) != 1 || hits.Sessions[0]["id"] != "bbbb2222" {
		t.Fatalf("phrase %d: %+v", code, hits)
	}
	var stats map[string]any
	if code := get("/api/stats?window=7d", &stats); code != 200 {
		t.Fatalf("stats %d", code)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if !strings.Contains(w.Body.String(), `"search":"lexical"`) || !strings.Contains(w.Body.String(), "need a Layer API key") {
		t.Fatalf("page does not say search is lexical and grades need a key")
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
