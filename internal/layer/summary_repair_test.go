package layer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hev/kit/internal/trace"
)

func TestSummaryRepairConditionalReadback(t *testing.T) {
	for _, mode := range []string{"summary", "reason", "conflict", "lost write", "changed source"} {
		t.Run(mode, func(t *testing.T) {
			stored := map[string]any{"id": "s", "session_id": "sid", "harness": "codex", "summary": "", "end": 20, "first_prompt": "fix it", "cost": 42}
			if mode == "changed source" {
				stored["first_prompt"] = "new work"
			}
			writes := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/query") {
					json.NewEncoder(w).Encode(map[string]any{"rows": []any{stored}})
					return
				}
				var body map[string]json.RawMessage
				json.NewDecoder(r.Body).Decode(&body)
				if !strings.Contains(string(body["patch_condition"]), `"end"`) || !strings.Contains(string(body["patch_condition"]), `"summary_backfilled"`) {
					t.Error("missing source/summary guard")
				}
				var rows []map[string]any
				json.Unmarshal(body["patch_rows"], &rows)
				if len(rows) != 1 || len(rows[0]) != 5 || rows[0]["first_prompt_short"] != "fix it" {
					t.Fatalf("patch touched unrelated data: %v", rows)
				}
				writes++
				if mode == "conflict" {
					fmt.Fprint(w, `{"status":"OK","rows_affected":0}`)
					return
				}
				if mode != "lost write" {
					for k, v := range rows[0] {
						stored[k] = v
					}
				}
				fmt.Fprint(w, `{"status":"OK","rows_affected":1}`)
			}))
			defer srv.Close()
			before := trace.SessionRow{ID: "s", SessionID: "sid", Harness: "codex", End: 20, FirstPrompt: "fix it"}
			after := before
			after.Summary = "Fixed it"
			if mode == "reason" {
				after.Summary = ""
				after.SummaryReason = "No archived prompt available for summarization"
			}
			err := New(srv.URL, "key", "ns", "").PatchSummaryRepair(context.Background(), before, after)
			wantErr := mode == "conflict" || mode == "lost write" || mode == "changed source"
			if (err != nil) != wantErr {
				t.Fatalf("err=%v wantErr=%v", err, wantErr)
			}
			if stored["cost"] != 42 {
				t.Fatal("unrelated field changed")
			}
			if mode == "changed source" && writes != 0 {
				t.Fatal("changed source patched")
			}
		})
	}
}

func TestPatchSummariesSkipsEmpty(t *testing.T) {
	writes := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { writes++; fmt.Fprint(w, `{"status":"OK"}`) }))
	defer srv.Close()
	result, err := New(srv.URL, "key", "ns", "").PatchSessionSummaries([]trace.SessionRow{{ID: "s", Summary: " \n "}})
	if err != nil || writes != 0 || result.RowsUpserted != 0 {
		t.Fatalf("empty patch: %+v %v writes=%d", result, err, writes)
	}
}
