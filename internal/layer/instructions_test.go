package layer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInstructionSearchProvenanceAndTraceCoordinates(t *testing.T) {
	for _, kind := range []string{StoreTurbopuffer, StorePgvector} {
		t.Run(kind, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if servePgvectorDeclaration(w, r) {
					return
				}
				var body map[string]any
				json.NewDecoder(r.Body).Decode(&body)
				q := body
				if qs, ok := body["queries"].([]any); ok {
					q = qs[0].(map[string]any)
				}
				// The gateway 400s on a query that sets both.
				if q["include_attributes"] != nil {
					t.Errorf("include_attributes must be unset alongside exclude_attributes: %v", q["include_attributes"])
				}
				if got, _ := json.Marshal(q["exclude_attributes"]); string(got) != `["vector"]` {
					t.Errorf("optional provenance must allow old schemas: exclude_attributes=%s", got)
				}
				rows := []map[string]any{{"id": "instruction", "text": "policy", "harness": "instructions", "path": "/fixture/AGENTS.md", "project": "/fixture", "host": "fixture-host", "version_id": "version"}, {"id": "trace", "text": "trace", "session_id": "session", "turn_uuid": "turn", "harness": "codex"}}
				if kind == StorePgvector {
					json.NewEncoder(w).Encode(map[string]any{"rows": rows})
				} else {
					json.NewEncoder(w).Encode(map[string]any{"results": []any{map[string]any{"rows": rows}}})
				}
			}))
			defer srv.Close()
			cl, err := New(srv.URL, "fixture", "fixture", "").WithStore(kind)
			if err != nil {
				t.Fatal(err)
			}
			hits, err := cl.SearchPhrasings([]string{"policy"}, 5, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(hits) != 2 || hits[0].Path != "/fixture/AGENTS.md" || hits[0].Host != "fixture-host" || hits[0].Project != "/fixture" || hits[0].VersionID != "version" || hits[1].SessionID != "session" || hits[1].TurnUUID != "turn" {
				t.Fatalf("hits=%+v", hits)
			}
		})
	}
}
