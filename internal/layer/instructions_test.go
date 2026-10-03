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
				queries := []any{body}
				if q, ok := body["queries"].([]any); ok {
					queries = q
				}
				for _, query := range queries {
					q := query.(map[string]any)
					if _, present := q["include_attributes"]; present {
						t.Error("include_attributes conflicts with exclude_attributes")
						http.Error(w, "conflicting projection", http.StatusBadRequest)
						return
					}
					excluded, _ := json.Marshal(q["exclude_attributes"])
					if string(excluded) != `["vector"]` {
						t.Errorf("projection: %s", excluded)
					}
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
			phrasings := []string{"policy"}
			if kind == StoreTurbopuffer {
				phrasings = append(phrasings, "guidance")
			}
			hits, err := cl.SearchPhrasings(phrasings, 5, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(hits) != 2 || hits[0].Path != "/fixture/AGENTS.md" || hits[0].Host != "fixture-host" || hits[0].Project != "/fixture" || hits[0].VersionID != "version" || hits[1].SessionID != "session" || hits[1].TurnUUID != "turn" {
				t.Fatalf("hits=%+v", hits)
			}
		})
	}
}
