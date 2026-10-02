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
	sha := strings.Repeat("a", 40)
	fresh := strings.Repeat("b", 40)
	for _, summary := range []string{"", "new title"} {
		t.Run(summary, func(t *testing.T) {
			var written trace.SessionRow
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" && strings.HasSuffix(r.URL.Path, "-sessions/schema") {
					fmt.Fprint(w, `{}`)
					return
				}
				if strings.HasSuffix(r.URL.Path, "/query") {
					var query map[string]any
					json.NewDecoder(r.Body).Decode(&query)
					if query["include_attributes"] != nil {
						t.Error("new fields must not be named against legacy schema")
					}
					if query["exclude_attributes"] == nil {
						t.Error("must exclude full prompt")
					}
					json.NewEncoder(w).Encode(map[string]any{"rows": []trace.SessionRow{{ID: "s", Summary: "stored", Commits: trace.StringList{sha}, PR: "42", Workdir: "/example"}}})
					return
				}
				var body struct {
					Rows []trace.SessionRow `json:"upsert_rows"`
				}
				if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
					t.Error(e)
				}
				written = body.Rows[0]
				fmt.Fprint(w, `{"status":"OK"}`)
			}))
			defer srv.Close()
			_, err := New(srv.URL, "", "ns", "").WriteSessions([]trace.SessionRow{{ID: "s", Summary: summary, Commits: trace.StringList{fresh, sha}}})
			if err != nil {
				t.Fatal(err)
			}
			if written.PR != "42" || written.Workdir != "/example" || !reflect.DeepEqual(written.Commits, trace.StringList{sha, fresh}) {
				t.Fatal(written)
			}
			if summary == "" && written.Summary != "stored" || summary != "" && written.Summary != summary {
				t.Fatal(written.Summary)
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
			sha := strings.Repeat("a", 40)
			newSHA := strings.Repeat("b", 40)
			writes := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					if typ == "missing" {
						http.Error(w, "missing", 404)
						return
					}
					if typ == "failure" {
						http.Error(w, "unavailable", 503)
						return
					}
					fmt.Fprintf(w, `{"commits":{"type":%q,"filterable":true},"tool_names":{"type":"[]string"}}`, typ)
					return
				}
				if strings.HasSuffix(r.URL.Path, "/query") {
					// Stored string-encoded commits must be preserved alongside new IDs.
					if typ == "string" {
						fmt.Fprintf(w, `{"rows":[{"id":"s","commits":%q}]}`, `["`+sha+`"]`)
					} else {
						fmt.Fprint(w, `{"rows":[]}`)
					}
					return
				}
				writes++
				var body struct {
					Schema map[string]struct {
						Type string `json:"type"`
					} `json:"schema"`
					Rows []map[string]json.RawMessage `json:"upsert_rows"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				want := typ
				if typ == "missing" {
					want = "[]string"
				}
				if body.Schema["commits"].Type != want {
					http.Error(w, "incompatible schema change for commits", 400)
					return
				}
				raw := body.Rows[0]["commits"]
				if (raw[0] == '"') != (want == "string") {
					http.Error(w, "commits value has incompatible type", 400)
					return
				}
				var got trace.StringList
				if err := json.Unmarshal(raw, &got); err != nil {
					t.Error(err)
				}
				expected := trace.StringList{newSHA}
				if typ == "string" {
					expected = trace.StringList{sha, newSHA}
				}
				if !reflect.DeepEqual(got, expected) {
					t.Errorf("commits=%v", got)
				}
				if body.Schema["tool_names"].Type != "[]string" {
					t.Error("changed unrelated array type")
				}
				fmt.Fprint(w, `{"status":"OK"}`)
			}))
			defer srv.Close()
			cl := New(srv.URL, "", "ns", "")
			_, err := cl.WriteSessions([]trace.SessionRow{{ID: "s", Commits: trace.StringList{newSHA}}})
			if typ == "int" || typ == "failure" {
				if err == nil || writes != 0 {
					t.Fatalf("unsafe write err=%v writes=%d", err, writes)
				}
			} else if err != nil || writes != 1 {
				t.Fatalf("err=%v writes=%d", err, writes)
			}
		})
	}
}
