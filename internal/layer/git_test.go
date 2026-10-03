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

func TestRescanPreservesGitEvidence(t *testing.T) {
	sha, fresh := strings.Repeat("a", 40), strings.Repeat("b", 40)
	for _, summary := range []string{"", "new title"} {
		t.Run(summary, func(t *testing.T) {
			store := &sessionSourceStore{rows: map[string]map[string]any{"s": {"id": "s", "summary": "stored", "commits": []any{sha}, "pr": "42", "workdir": "/example"}}}
			cl := sourceTestClient(t, store)
			_, err := cl.WriteSessions([]trace.SessionRow{{ID: "s", Summary: summary, Commits: trace.StringList{fresh, sha}}})
			if err != nil {
				t.Fatal(err)
			}
			row := store.rows["s"]
			if row["pr"] != "42" || row["workdir"] != "/example" || row["summary"] != "stored" || !reflect.DeepEqual(row["commits"], []any{sha, fresh}) {
				t.Fatal(row)
			}
		})
	}
}

func TestReconcileSessionGitFiltersPreservesTypes(t *testing.T) {
	for _, typ := range []string{"string", "[]string"} {
		t.Run(typ, func(t *testing.T) {
			writes := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					if strings.Contains(r.URL.Path, "-sessions/") {
						fmt.Fprintf(w, `{"commits":{"type":%q,"filterable":false},"pr":{"type":"string","filterable":false},"summary":{"type":"string","filterable":false}}`, typ)
					} else {
						fmt.Fprint(w, `{}`)
					}
					return
				}
				var body map[string]json.RawMessage
				json.NewDecoder(r.Body).Decode(&body)
				if len(body) != 1 || body["schema"] == nil {
					t.Errorf("row/embedding mutation: %v", body)
				}
				var schema map[string]map[string]any
				json.Unmarshal(body["schema"], &schema)
				if len(schema) != 2 || schema["commits"]["type"] != typ || schema["commits"]["filterable"] != true || schema["pr"]["filterable"] != true {
					t.Error(schema)
				}
				writes++
				fmt.Fprint(w, `{}`)
			}))
			defer srv.Close()
			if err := New(srv.URL, "", "ns", "").ReconcileQueryFilters(); err != nil || writes != 1 {
				t.Fatalf("writes=%d err=%v", writes, err)
			}
		})
	}
}

// Enforce provider type semantics, rather than just inspecting a canned body.
// Existing commits:string must remain string even when the store supports arrays.
func TestWriteSessionsUsesExistingCommitType(t *testing.T) {
	for _, typ := range []string{"string", "[]string", "int", "missing", "failure"} {
		t.Run(typ, func(t *testing.T) {
			sha, newSHA := strings.Repeat("a", 40), strings.Repeat("b", 40)
			store := &sessionSourceStore{rows: map[string]map[string]any{}, schema: map[string]any{"tool_names": map[string]any{"type": "[]string", "filterable": false}}}
			if typ != "missing" {
				store.schema["commits"] = map[string]any{"type": typ, "filterable": true}
			}
			if typ == "string" {
				store.rows["s"] = map[string]any{"id": "s", "commits": `["` + sha + `"]`}
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if typ == "failure" && r.Method == "GET" {
					http.Error(w, "unavailable", 503)
					return
				}
				store.handle(w, r)
			}))
			defer srv.Close()
			_, err := New(srv.URL, "", "ns", "").WriteSessions([]trace.SessionRow{{ID: "s", Commits: trace.StringList{newSHA}}})
			if typ == "int" || typ == "failure" {
				if err == nil || len(store.writes) != 0 {
					t.Fatal(err, store.writes)
				}
				return
			}
			if err != nil || len(store.writes) != 1 {
				t.Fatal(err, store.writes)
			}
			row := store.rows["s"]
			raw, _ := json.Marshal(row["commits"])
			var got trace.StringList
			json.Unmarshal(raw, &got)
			want := trace.StringList{newSHA}
			if typ == "string" {
				want = trace.StringList{sha, newSHA}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatal(got)
			}
			schema := store.writes[0]["schema"].(map[string]any)
			if !reflect.DeepEqual(schema["tool_names"], store.schema["tool_names"]) {
				t.Fatal("changed existing schema", schema)
			}
			wantType := typ
			if typ == "missing" {
				wantType = "[]string"
			}
			if schema["commits"].(map[string]any)["type"] != wantType {
				t.Fatal(schema)
			}
		})
	}
}
