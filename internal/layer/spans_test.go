package layer

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hev/kit/internal/trace"
)

// The spans namespace holds scalars only, so it is writable on every store
// lane, and reads page by ID; an archive without it reads as empty.
func TestTraceSpansWriteAndPagedRead(t *testing.T) {
	var wrote map[string]any
	missing := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(strings.TrimSuffix(r.URL.Path, "/query"), "/ns-spans") {
			t.Errorf("namespace: %s", r.URL.Path)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if !strings.HasSuffix(r.URL.Path, "/query") {
			wrote = body
			json.NewEncoder(w).Encode(map[string]any{"status": "OK", "rows_upserted": 2})
			return
		}
		if missing {
			http.Error(w, "namespace not found", http.StatusNotFound)
			return
		}
		// 1500 rows, served 1000 at a time after the cursor.
		after := ""
		if f, _ := json.Marshal(body["filters"]); strings.Contains(string(f), `"Gt"`) {
			after = "s0999"
		}
		var rows []map[string]any
		for i := 0; i < 1500; i++ {
			id := fmt.Sprintf("s%04d", i)
			if id > after && len(rows) < 1000 {
				rows = append(rows, map[string]any{"id": id, "kind": "attempt", "run": "r", "run_key": "k", "start": i})
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"rows": rows})
	}))
	defer srv.Close()
	c := New(srv.URL, "", "ns", "")
	rows, err := c.ListTraceSpanRows(nil)
	if err != nil || len(rows) != 0 {
		t.Fatalf("missing namespace: %v %v", rows, err)
	}
	if _, err := c.WriteTraceSpans([]trace.SpanRow{{ID: "a", Kind: "ask", Run: "r", RunKey: "k"}, {ID: "b", Kind: "attempt", Run: "r", RunKey: "k", Attempt: 1}}); err != nil {
		t.Fatal(err)
	}
	schema := wrote["schema"].(map[string]any)
	for k, v := range schema {
		def := v.(map[string]any)
		if typ := def["type"]; typ != "string" && typ != "int" {
			t.Errorf("%s: type %v", k, typ)
		}
		want := k == "trace_id" || k == "run" || k == "start"
		if def["filterable"] != want {
			t.Errorf("%s filterable=%v", k, def["filterable"])
		}
	}
	missing = false
	rows, err = c.ListTraceSpanRows([]any{"trace_id", "Eq", "x"})
	if err != nil || len(rows) != 1500 || rows[1499].ID != "s1499" {
		t.Fatalf("paged read: %d %v", len(rows), err)
	}
}
