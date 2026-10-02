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
				if strings.HasSuffix(r.URL.Path, "/query") {
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
			if written.PR != "42" || written.Workdir != "/example" || !reflect.DeepEqual(written.Commits, trace.StringList{fresh, sha}) {
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
