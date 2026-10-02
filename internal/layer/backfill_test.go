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

func TestBackfillReadbackAndLinkage(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(fmt.Sprint(conflict), func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			stored := map[string]any{"id": "s", "repo_url": "preserved", "summary": "keep", "commits": "[]"}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					fmt.Fprint(w, `{"commits":{"type":"string"},"workdir":{"type":"string","full_text_search":true}}`)
					return
				}
				var body map[string]json.RawMessage
				json.NewDecoder(r.Body).Decode(&body)
				if strings.HasSuffix(r.URL.Path, "/query") {
					json.NewEncoder(w).Encode(map[string]any{"rows": []any{stored}})
					return
				}
				var schema map[string]map[string]any
				json.Unmarshal(body["schema"], &schema)
				if schema["workdir"]["full_text_search"] != true {
					t.Error("lost existing schema settings")
				}
				var patches []map[string]any
				json.Unmarshal(body["patch_rows"], &patches)
				if patches[0]["repo_url"] != nil || patches[0]["summary"] != nil {
					t.Error("overwrote preserved metadata")
				}
				for k, v := range patches[0] {
					stored[k] = v
				}
				if conflict {
					stored["pr"] = "conflicting"
				}
				fmt.Fprint(w, `{"rows_affected":1}`)
			}))
			defer srv.Close()
			row := trace.SessionRow{ID: "s", RepoURL: "replacement", Workdir: "source", PR: "7", Commits: trace.StringList{strings.Repeat("a", 40)}}
			err := New(srv.URL, "", "ns", "").PatchBackfill(context.Background(), row)
			if (err != nil) != conflict {
				t.Fatalf("conflict=%v err=%v", conflict, err)
			}
			if stored["summary"] != "keep" || stored["repo_url"] != "preserved" {
				t.Fatal(stored)
			}
		})
	}
}
