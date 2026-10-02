package layer

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hev/kit/internal/trace"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestOutcomePatchAndRescan(t *testing.T) {
	sha := strings.Repeat("a", 40)
	other := strings.Repeat("b", 40)
	for _, typ := range []string{"string", "[]string"} {
		t.Run(typ, func(t *testing.T) {
			stored := map[string]any{"id": "s", "session_id": "s", "end": 100, "summary": "original", "commits": []string{other}, "pr": "7"}
			if typ == "string" {
				stored["commits"] = `["` + other + `"]`
			}
			writes := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					fmt.Fprintf(w, `{"commits":{"type":%q}}`, typ)
					return
				}
				var b map[string]json.RawMessage
				json.NewDecoder(r.Body).Decode(&b)
				if strings.HasSuffix(r.URL.Path, "/query") {
					json.NewEncoder(w).Encode(map[string]any{"rows": []any{stored}})
					return
				}
				writes++
				var schema map[string]map[string]any
				json.Unmarshal(b["schema"], &schema)
				if schema["commits"]["type"] != typ {
					t.Errorf("type changed: %v", schema)
				}
				if raw, ok := b["patch_rows"]; ok {
					var patches []map[string]any
					json.Unmarshal(raw, &patches)
					p := patches[0]
					if p["summary"] != nil || p["end"] != nil {
						t.Error("transcript mutation")
					}
					for k, v := range p {
						stored[k] = v
					}
					for _, k := range []string{"pr_merged", "ci_state", "ci_workflow_test_conclusion", "commit_outcome_test_reverted"} {
						if schema[k] == nil {
							t.Fatalf("missing filter schema %s", k)
						}
					}
				} else {
					var rows []map[string]any
					json.Unmarshal(b["upsert_rows"], &rows)
					stored = rows[0]
				}
				fmt.Fprint(w, `{"status":"OK","rows_affected":1}`)
			}))
			defer srv.Close()
			cl := New(srv.URL, "", "ns", "")
			row := trace.SessionRow{ID: "s", PR: "7", Commits: trace.StringList{sha}, SessionOutcomes: trace.SessionOutcomes{PRMerged: "true", CIState: "complete", Reverted: "false", WorkflowAttributes: map[string]string{"ci_workflow_test_conclusion": "failure", "commit_outcome_test_reverted": "false"}}}
			if e := cl.PatchOutcomes(context.Background(), row); e != nil {
				t.Fatal(e)
			}
			if _, e := cl.WriteSessions([]trace.SessionRow{{ID: "s", End: 101, Summary: "new title"}}); e != nil {
				t.Fatal(e)
			}
			b, _ := json.Marshal(stored)
			var result trace.SessionRow
			if e := json.Unmarshal(b, &result); e != nil {
				t.Fatal(e)
			}
			if result.PRMerged != "true" || result.CIState != "complete" || result.WorkflowAttributes["ci_workflow_test_conclusion"] != "failure" || result.Summary != "new title" || !reflect.DeepEqual(result.Commits, trace.StringList{sha, other}) || writes != 2 {
				t.Fatalf("result %+v writes %d", result, writes)
			}
		})
	}
}
func TestOutcomePageContextAndCursor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["include_attributes"] != nil || body["exclude_attributes"] == nil {
			t.Error("legacy-incompatible attributes")
		}
		if !strings.Contains(fmt.Sprint(body["filters"]), "Gt cursor") {
			t.Error(body)
		}
		fmt.Fprint(w, `{"rows":[{"id":"next"}]}`)
	}))
	defer srv.Close()
	cl := New(srv.URL, "", "ns", "")
	rows, e := cl.OutcomePage(context.Background(), "cursor", 1, 2)
	if e != nil || len(rows) != 1 || rows[0].ID != "next" {
		t.Fatalf("%v %v", rows, e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := cl.OutcomePage(ctx, "cursor", 1, 2); e == nil {
		t.Fatal("canceled page succeeded")
	}
}
func TestOutcomeWriterLockRetry(t *testing.T) {
	unlock, e := lockSessionEnrichment(false)
	if e != nil {
		t.Fatal(e)
	}
	defer unlock()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if e := New("http://invalid", "", "ns", "").PatchOutcomes(ctx, trace.SessionRow{}); e == nil || !strings.Contains(e.Error(), "writer busy") {
		t.Fatalf("%v", e)
	}
}
