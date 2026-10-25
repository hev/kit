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
				fmt.Fprint(w, `{"rows":[{"id":"legacy","session_id":"s","end":1,"cost":4}]}`)
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
