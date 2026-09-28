package index

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
)

// sessionStore is a sessions namespace that behaves as both stores do: an
// upsert replaces the whole row, a patch changes only the attributes it names.
type sessionStore struct {
	mu      sync.Mutex
	rows    map[string]map[string]json.RawMessage
	upserts []string
}

func (s *sessionStore) serve(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if r.URL.Path == "/health" {
			io.WriteString(w, `{"status":"ok","version":"0.7.2"}`)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var body struct {
			Upserts []map[string]json.RawMessage `json:"upsert_rows"`
			Patches []map[string]json.RawMessage `json:"patch_rows"`
			Filters []json.RawMessage            `json:"filters"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Error(err)
		}
		switch r.URL.Path {
		case "/v2/namespaces/ns-sessions/query":
			var ids []string
			if len(body.Filters) == 3 {
				json.Unmarshal(body.Filters[2], &ids)
			}
			// The summary read filters by id and asks for the summary alone;
			// the listing reads whole rows.
			out := []map[string]json.RawMessage{}
			for _, id := range ids {
				if row, ok := s.rows[id]; ok {
					out = append(out, map[string]json.RawMessage{"id": row["id"], "summary": row["summary"]})
				}
			}
			if ids == nil {
				for _, row := range s.rows {
					out = append(out, row)
				}
			}
			json.NewEncoder(w).Encode(map[string]any{"rows": out})
		case "/v2/namespaces/ns-sessions":
			for _, row := range body.Upserts {
				var id string
				json.Unmarshal(row["id"], &id)
				s.rows[id] = row
				s.upserts = append(s.upserts, string(raw))
			}
			for _, patch := range body.Patches {
				var id string
				json.Unmarshal(patch["id"], &id)
				for k, v := range patch {
					if s.rows[id] != nil {
						s.rows[id][k] = v
					}
				}
			}
			io.WriteString(w, `{"status":"OK","rows_upserted":1,"rows_affected":1}`)
		default:
			io.WriteString(w, `{"status":"OK","rows_upserted":1}`)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (s *sessionStore) summary(t *testing.T, id string) string {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	var summary string
	json.Unmarshal(s.rows[id]["summary"], &summary)
	return summary
}

// A rescan after the transcript grows rewrites the session row, and must keep
// the summary `hev index --summarize` patched in; only a new harness title
// replaces it. The same on both lanes.
func TestRescanKeepsAGeneratedSummary(t *testing.T) {
	for _, kind := range []string{layer.StoreTurbopuffer, layer.StorePgvector} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "session.jsonl")
			turn := func(uuid, text string) string {
				return fmt.Sprintf(`{"type":"user","uuid":%q,"sessionId":"s1","timestamp":"2026-09-05T10:00:00Z","message":{"role":"user","content":%q}}`+"\n", uuid, text)
			}
			grow := func(line string) {
				f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
				if err != nil {
					t.Fatal(err)
				}
				defer f.Close()
				io.WriteString(f, line)
			}
			grow(turn("u1", strings.Repeat("a long first prompt ", 50)))

			store := &sessionStore{rows: map[string]map[string]json.RawMessage{}}
			cl, err := layer.New(store.serve(t).URL, "key", "ns", "").WithStore(kind)
			if err != nil {
				t.Fatal(err)
			}
			st := &State{Units: map[string]string{}}
			src := &trace.ClaudeSource{Root: root}
			scan := func() {
				t.Helper()
				if rep, err := Run(src, cl, st, Options{ReadSide: true}); err != nil || len(rep.Errors) != 0 || rep.UnitsIndexed != 1 {
					t.Fatalf("scan: %+v %v", rep, err)
				}
			}
			scan()
			rows, err := cl.ListSessionRows(-1, nil)
			if err != nil || len(rows) != 1 || rows[0].Summary != "" {
				t.Fatalf("first scan rows %+v %v", rows, err)
			}
			id := rows[0].ID

			// hev index --summarize
			rows[0].Summary = "Generated title"
			if _, err := cl.PatchSessionSummaries(rows); err != nil {
				t.Fatal(err)
			}

			grow(turn("u2", "and one more thing"))
			before := len(store.upserts)
			scan()
			if got := store.summary(t, id); got != "Generated title" {
				t.Fatalf("summary after rescan = %q", got)
			}
			var prompts int
			store.mu.Lock()
			json.Unmarshal(store.rows[id]["prompt_count"], &prompts)
			store.mu.Unlock()
			if prompts != 2 {
				t.Fatalf("rescan did not rewrite the row: prompt_count %d", prompts)
			}
			// One upsert per rescan, as before: preserving the summary is a
			// read, never a second write of the row.
			if n := len(store.upserts) - before; n != 1 {
				t.Fatalf("%d session upserts on rescan", n)
			}

			grow(`{"type":"ai-title","sessionId":"s1","aiTitle":"Harness title"}` + "\n")
			scan()
			if got := store.summary(t, id); got != "Harness title" {
				t.Fatalf("summary after a harness title = %q", got)
			}
		})
	}
}
