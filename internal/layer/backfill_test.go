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
)

func TestBackfillReadbackAndLinkage(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(fmt.Sprint(conflict), func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			stored := map[string]any{"id": "s", "session_id": "session", "harness": "harness", "host": "host", "repo_url": "preserved", "start": float64(100), "end": float64(200), "tool_count": float64(2), "summary": "keep", "commits": "[]"}
			protected := map[string]any{}
			for _, field := range []string{"id", "session_id", "harness", "host", "repo_url", "start", "end", "tool_count"} {
				protected[field] = stored[field]
			}
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
			for field, want := range protected {
				if !reflect.DeepEqual(stored[field], want) {
					t.Errorf("protected analyzer field changed: %s %v != %v", field, stored[field], want)
				}
			}
			if stored["summary"] != "keep" || stored["repo_url"] != "preserved" {
				t.Fatal(stored)
			}
		})
	}
}

func TestBackfillReadbackDoesNotFenceLaterLegacyUpsert(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	stored := map[string]any{"id": "s", "end": 200, "summary": "keep", "commits": "[]"}
	legacy := map[string]any{"id": "s", "end": 200, "summary": "keep", "commits": "[]", "pr": ""}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			fmt.Fprint(w, `{"commits":{"type":"string"}}`)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/query") {
			json.NewEncoder(w).Encode(map[string]any{"rows": []any{stored}})
			return
		}
		var body struct {
			Patches []map[string]any `json:"patch_rows"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		for k, v := range body.Patches[0] {
			stored[k] = v
		}
		fmt.Fprint(w, `{"rows_affected":1}`)
	}))
	defer srv.Close()
	err := New(srv.URL, "", "ns", "").PatchBackfill(context.Background(), trace.SessionRow{ID: "s", PR: "7", Commits: trace.StringList{strings.Repeat("a", 40)}})
	if err != nil {
		t.Fatal(err)
	}
	// Provider upsert replaces the entire document. This models an older writer
	// publishing an already-read row after our successful patch and readback.
	stored = legacy
	if stored["end"] != 200 || stored["pr"] != "" || stored["commits"] != "[]" {
		t.Fatal("fixture failed to model legacy overwrite")
	}
}

func TestBackfillZeroAffectedRowsIsNotAcknowledged(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	reads := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			fmt.Fprint(w, `{"commits":{"type":"string"}}`)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/query") {
			reads++
			fmt.Fprint(w, `{"rows":[{"id":"s","commits":"[]"}]}`)
			return
		}
		fmt.Fprint(w, `{"rows_affected":0}`)
	}))
	defer srv.Close()
	err := New(srv.URL, "", "ns", "").PatchBackfill(context.Background(), trace.SessionRow{ID: "s", PR: "7"})
	if err == nil || !strings.Contains(err.Error(), "expected one") || reads != 1 {
		t.Fatal("zero write acknowledged", err, reads)
	}
}
