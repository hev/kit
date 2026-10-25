package layer

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestDashboardProjectionLegacySchemas(t *testing.T) {
	for _, wrapped := range []bool{false, true} {
		t.Run(fmt.Sprint(wrapped), func(t *testing.T) {
			calls := 0
			wire := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if strings.HasSuffix(r.URL.Path, "/schema") {
					schema := map[string]any{"session_id": map[string]any{"type": "string"}, "end": map[string]any{"type": "uint"}, "cost": map[string]any{"type": "float"}, "first_prompt": map[string]any{"type": "string"}}
					if wrapped {
						_ = json.NewEncoder(w).Encode(map[string]any{"schema": schema})
					} else {
						_ = json.NewEncoder(w).Encode(schema)
					}
					return
				}
				var q map[string]any
				_ = json.NewDecoder(r.Body).Decode(&q)
				// Optional columns missing in legacy schemas must not be named upstream.
				if !reflect.DeepEqual(q["include_attributes"], []any{"session_id", "end", "cost"}) || q["filters"] != nil || q["exclude_attributes"] != nil {
					t.Errorf("projection %v", q)
				}
				fmt.Fprint(w, `{"rows":[{"id":"legacy","session_id":"s","end":1,"cost":4}],"source":{"rows":1,"pages":1,"bytes":100,"complete":true,"consistency":"observed","threads":8}}`)
			}))
			defer wire.Close()
			rows, err := New(wire.URL, "fixture", "test", "").ListDashboardSessionRows(true, true, false)
			if err != nil || len(rows) != 1 || rows[0].Cost != 4 || calls != 2 {
				t.Fatalf("rows=%v err=%v calls=%d", rows, err, calls)
			}
		})
	}
}

func TestDashboardRejectsUnusableSchema(t *testing.T) {
	calls := 0
	wire := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; fmt.Fprint(w, `{"unexpected":"schema"}`) }))
	defer wire.Close()
	_, err := New(wire.URL, "fixture", "test", "").ListDashboardSessionRows(true, true, false)
	if err == nil || calls != 1 {
		t.Fatalf("unexpected full-row fallback: calls=%d err=%v", calls, err)
	}
}

func TestDashboardSourceErrorsNeverMaskOrRepeatReads(t *testing.T) {
	for _, tc := range []struct {
		status   int
		body     string
		fallback bool
	}{
		{422, `{"error":"UnsupportedByStore","feature":"projected_source_rows"}`, true},
		{422, `{"error":"UnsupportedByStore","feature":"latest_by"}`, false},
		{422, `{"error":"bad projection"}`, false},
		{401, `{"error":"Unauthorized"}`, false}, {403, `{"error":"Forbidden"}`, false},
		{404, `{"error":"NotFound"}`, false}, {429, `{"error":"throttle"}`, false}, {500, `{"error":"internal"}`, false},
		{200, `{"rows":[{"id":"a","session_id":"s"}],"source":{"rows":1,"pages":1,"bytes":10,"complete":false,"consistency":"observed","threads":8}}`, false},
		{200, `{"rows":[{"id":"a","session_id":"s"},{"id":"a","session_id":"s"}],"source":{"rows":2,"pages":1,"bytes":10,"complete":true,"consistency":"observed","threads":8}}`, false},
	} {
		t.Run(fmt.Sprint(tc.status, tc.body), func(t *testing.T) {
			scans, queries := 0, 0
			wire := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/schema") {
					fmt.Fprint(w, `{"session_id":{},"end":{}}`)
					return
				}
				if strings.HasSuffix(r.URL.Path, "/scans/rows") {
					scans++
					w.WriteHeader(tc.status)
					fmt.Fprint(w, tc.body)
					return
				}
				queries++
				fmt.Fprint(w, `{"rows":[{"id":"a","session_id":"s","end":1}]}`)
			}))
			defer wire.Close()
			rows, err := New(wire.URL, "fixture", "test", "").ListDashboardSessionRows(true, true, false)
			if scans != 1 || (tc.fallback && (queries != 1 || err != nil || len(rows) != 1)) || (!tc.fallback && (queries != 0 || err == nil || rows != nil)) {
				t.Fatalf("rows=%v err=%v scans=%d queries=%d", rows, err, scans, queries)
			}
		})
	}
}
