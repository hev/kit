package layer

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hev/kit/internal/trace"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBackfillUnknownBatchConditionsAndReadback(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(fmt.Sprint(conflict), func(t *testing.T) {
			var patches []map[string]any
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					fmt.Fprint(w, `{"commits":{"type":"string"},"ci_conclusions":{"type":"string"},"ci_runs":{"type":"string"},"revert_commits":{"type":"string"}}`)
					return
				}
				if strings.HasSuffix(r.URL.Path, "/query") {
					json.NewEncoder(w).Encode(map[string]any{"rows": patches})
					return
				}
				var body map[string]json.RawMessage
				json.NewDecoder(r.Body).Decode(&body)
				condition := string(body["patch_condition"])
				if !strings.Contains(condition, `["pr","Eq",null]`) || !strings.Contains(condition, `["commits","Eq","[]"]`) || !strings.Contains(condition, `"$ref_new":"outcome_checked"`) || !strings.Contains(condition, `"Lte"`) {
					t.Errorf("missing concurrent linkage/observation guards: %s", condition)
				}
				json.Unmarshal(body["patch_rows"], &patches)
				for _, p := range patches {
					for _, k := range []string{"summary", "repo_url", "session_id", "commits", "pr", "end"} {
						if _, ok := p[k]; ok {
							t.Errorf("unexpected source/linkage overwrite: %s", k)
						}
					}
				}
				if conflict {
					fmt.Fprint(w, `{"rows_affected":0,"patched_ids":[]}`)
				} else {
					fmt.Fprint(w, `{"rows_affected":2,"patched_ids":["a","b"]}`)
				}
			}))
			defer srv.Close()
			outcomes := trace.SessionOutcomes{PRState: "unknown", CIState: "unknown", Reverted: "unknown", OutcomeChecked: 123, CIConclusions: trace.StringList{}, CIRuns: trace.StringList{}, RevertCommits: trace.StringList{}}
			rows := []trace.SessionRow{{ID: "a", SessionOutcomes: outcomes}, {ID: "b", SessionOutcomes: outcomes}}
			err := New(srv.URL, "", "ns", "").PatchUnknownBackfill(context.Background(), rows)
			if (err != nil) != conflict {
				t.Fatal(err)
			}
		})
	}
}
