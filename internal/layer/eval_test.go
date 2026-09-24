package layer

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/hev/kit/internal/trace"
)

func TestEvalSchemaAndReplay(t *testing.T) {
	stored := map[string]map[string]any{}
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v2/namespaces/ns-evals" {
			t.Errorf("wrong namespace: %s", r.URL.Path)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		schema := body["schema"].(map[string]any)
		text := schema["text"].(map[string]any)
		if text["embed"] == nil || text["full_text_search"] != true || body["distance_metric"] != "cosine_distance" {
			t.Errorf("missing hybrid schema: %#v", body)
		}
		if schema["mark_custom"].(map[string]any)["type"] != "int" {
			t.Error("dynamic mark not integer")
		}
		for _, name := range []string{"marks", "evidence", "findings"} {
			if schema[name].(map[string]any)["filterable"] != false {
				t.Error("JSON should be unfilterable")
			}
		}
		batch := body["upsert_rows"].([]any)
		if len(batch) > 30 {
			t.Error("embedded write exceeds 30 rows")
		}
		for _, raw := range batch {
			row := raw.(map[string]any)
			if row["mark_custom"] != float64(4) || !strings.Contains(row["text"].(string), "proof") || !strings.Contains(row["text"].(string), "finding") {
				t.Errorf("bad flattened row: %#v", row)
			}
			stored[row["id"].(string)] = row
		}
		fmt.Fprintf(w, `{"status":"OK","rows_upserted":%d}`, len(batch))
	}))
	defer srv.Close()
	c := New(srv.URL, "key", "ns", "")
	rows := []trace.Eval{}
	for i := 0; i < 31; i++ {
		rows = append(rows, trace.Eval{Session: fmt.Sprintf("s%d", i), TS: "2026-09-07T12:00:00Z", Marks: map[string]int{"custom": 4}, Evidence: map[string]string{"custom": "proof"}, Findings: []string{"finding"}})
	}
	for i := 0; i < 2; i++ {
		if _, err := c.WriteEvals(rows); err != nil {
			t.Fatal(err)
		}
	}
	if len(stored) != 31 || calls != 4 {
		t.Fatalf("replay or batching: %d rows, %d calls", len(stored), calls)
	}
	rows[0].TS = "2026-09-07T06:00:00-06:00"
	if _, err := c.WriteEvals(rows[:1]); err != nil {
		t.Fatal(err)
	}
	if len(stored) != 31 {
		t.Fatal("equivalent timestamps changed identity")
	}
	for _, row := range []trace.Eval{{Session: "", TS: "2026-09-07T12:00:00Z", Marks: map[string]int{}}, {Session: "s", TS: "invalid", Marks: map[string]int{}}, {Session: "s", TS: "2026-09-07T12:00:00Z", Marks: map[string]int{"bad-name": 1}}} {
		if _, err := c.WriteEvals([]trace.Eval{row}); err == nil {
			t.Fatal("invalid eval accepted")
		}
	}
	t.Log("31 evals written in <=30-row batches; replay and equivalent UTC timestamp keep 31 IDs")
}
func TestEvalNamespaceAbsentAndErrors(t *testing.T) {
	status := 404
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "store error", status) }))
	defer srv.Close()
	c := New(srv.URL, "key", "ns", "")
	if rows, err := c.ListEvalRows(nil); err != nil || len(rows) != 0 {
		t.Fatalf("absent eval namespace: %v %v", rows, err)
	}
	if _, err := c.SearchEvals("q", 5, nil); err != nil {
		t.Fatal(err)
	}
	status = 403
	if _, err := c.ListEvalRows(nil); err == nil {
		t.Fatal("authorization error swallowed")
	}
	if _, err := c.SearchEvals("q", 5, nil); err == nil {
		t.Fatal("search error swallowed")
	}
}
func TestReadSidePaginationPastTenThousand(t *testing.T) {
	for _, kind := range []string{"sessions", "evals"} {
		t.Run(kind, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				if body["top_k"] != float64(10000) || !reflect.DeepEqual(body["rank_by"], []any{"id", "asc"}) {
					t.Errorf("bad page request: %v", body)
				}
				if calls > 2 {
					http.Error(w, "too many pages", 500)
					return
				}
				if calls == 2 {
					if !reflect.DeepEqual(body["filters"], []any{"id", "Gt", "09999"}) {
						t.Errorf("cursor=%#v", body["filters"])
					}
					fmt.Fprint(w, `{"rows":[{"id":"10000","session_id":"last"}]}`)
					return
				}
				rows := make([]map[string]any, 10000)
				for i := range rows {
					rows[i] = map[string]any{"id": fmt.Sprintf("%05d", i), "session_id": fmt.Sprint(i)}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"rows": rows})
			}))
			defer srv.Close()
			c := New(srv.URL, "key", "ns", "")
			n := 0
			var err error
			if kind == "sessions" {
				var rows []trace.SessionRow
				rows, err = c.ListSessionRows(-1, nil)
				n = len(rows)
			} else {
				var rows []EvalRow
				rows, err = c.ListEvalRows(nil)
				n = len(rows)
			}
			if err != nil || n != 10001 || calls != 2 {
				t.Fatalf("rows=%d calls=%d err=%v", n, calls, err)
			}
			t.Log("10001 rows returned in 2 pages without truncation")
		})
	}
}
func TestSessionArraysSchema(t *testing.T) {
	for key, want := range map[string]string{"prompt_ts": "[]uint", "tool_names": "[]string", "total_tokens": "int"} {
		if sessionSchema()[key].(map[string]any)["type"] != want {
			t.Errorf("%s schema", key)
		}
	}
}
