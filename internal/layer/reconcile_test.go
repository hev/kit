package layer

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
)

// Provider semantics from https://turbopuffer.com/docs/write#updating-attributes:
// FTS disables implicit filtering; enabling it is online, with 409 until ready.
// Unlike the old pinned JSON tests this server actually evaluates Eq filters.
func TestReconcileExistingQueryFilters(t *testing.T) {
	for _, namespace := range []string{"hev-traces", "kit-example-traces"} {
		t.Run(namespace, func(t *testing.T) {
			recorded, err := os.ReadFile("testdata/lyr244-provider-schema.json")
			if err != nil {
				t.Fatal(err)
			}
			var schema map[string]map[string]any
			if err := json.Unmarshal(recorded, &schema); err != nil {
				t.Fatal(err)
			}
			// Add preservation and repair cases beyond the live recorded baseline.
			schema["plan"]["filterable"] = false
			schema["text"] = map[string]any{"type": "string", "embed": map[string]any{"model": "existing-model", "attribute": "embed_text"}}

			originalText, _ := json.Marshal(schema["text"])
			rows := []map[string]any{{"id": "a", "workdir": "/lyr", "harness": "codex", "plan": "p"}, {"id": "b", "workdir": "/other", "harness": "claude_code", "plan": "q"}}
			writes, failures, indexing := 0, 1, false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == "GET" && r.URL.Path == "/v1/namespaces/"+namespace+"/schema":
					json.NewEncoder(w).Encode(schema)
				case r.Method == "POST" && r.URL.Path == "/v2/namespaces/"+namespace:
					if failures > 0 {
						failures--
						http.Error(w, "temporary failure", 503)
						return
					}
					var body struct {
						Schema map[string]map[string]any `json:"schema"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					changes := body.Schema
					if len(changes) != 2 {
						t.Errorf("unexpected changes: %v", changes)
					}
					for k, v := range changes {
						schema[k] = v
					}
					writes++
					indexing = true
					fmt.Fprint(w, `{}`)
				case r.Method == "POST" && r.URL.Path == "/v2/namespaces/"+namespace+"/query":
					var body struct {
						Filters []any `json:"filters"`
					}
					json.NewDecoder(r.Body).Decode(&body)
					field := body.Filters[0].(string)
					attr := schema[field]
					enabled, explicit := attr["filterable"].(bool)
					if !explicit {
						enabled = attr["full_text_search"] == nil
					}
					if !enabled {
						http.Error(w, "attribute isn't filterable", 400)
						return
					}
					if indexing {
						http.Error(w, "index building", 409)
						return
					}
					var hits []map[string]any
					for _, row := range rows {
						if reflect.DeepEqual(row[field], body.Filters[2]) {
							hits = append(hits, row)
						}
					}
					json.NewEncoder(w).Encode(map[string]any{"rows": hits})
				default:
					t.Errorf("unexpected operation %s %s", r.Method, r.URL.Path)
					http.Error(w, "unexpected", 500)
				}
			}))
			defer server.Close()
			c := New(server.URL, "test", namespace, "")
			query := func(field, value string) (int, error) {
				var result struct {
					Rows []map[string]any `json:"rows"`
				}
				err := c.do("POST", "/v2/namespaces/"+namespace+"/query", map[string]any{"filters": []any{field, "Eq", value}, "rank_by": []any{"id", "asc"}, "top_k": 10}, &result)
				return len(result.Rows), err
			}
			if _, err := query("workdir", "/lyr"); !reconcileStatus(err, 400) {
				t.Fatalf("old schema should reject Eq: %v", err)
			}
			if err := c.ReconcileQueryFilters(); err == nil {
				t.Fatal("lost failure")
			}
			if err := c.ReconcileQueryFilters(); err != nil {
				t.Fatal(err)
			}
			if _, err := query("workdir", "/lyr"); !reconcileStatus(err, 409) {
				t.Fatalf("index readiness: %v", err)
			}
			indexing = false
			for field, value := range map[string]string{"workdir": "/lyr", "harness": "codex", "plan": "p"} {
				if n, err := query(field, value); err != nil || n != 1 {
					t.Fatalf("Eq %s: n=%d err=%v", field, n, err)
				}
			}
			if err := c.ReconcileQueryFilters(); err != nil || writes != 1 {
				t.Fatalf("not idempotent: %d %v", writes, err)
			}
			text, _ := json.Marshal(schema["text"])
			if string(text) != string(originalText) || len(rows) != 2 {
				t.Fatal("embedding or rows changed")
			}
			fts := schema["workdir"]["full_text_search"].(map[string]any)
			if fts["tokenizer"] != "word_v4" || fts["stemming"] != false {
				t.Fatal("FTS settings changed")
			}
		})
	}
}

func TestReconcileMissingAndIncompatibleNamespaces(t *testing.T) {
	for _, response := range []string{"missing", `{"workdir":{"type":"[]string","filterable":false}}`, `{"workdir":{"type":"string","filterable":"bad"}}`} {
		t.Run(response, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					t.Error("unexpected mutation")
				}
				if response == "missing" {
					http.Error(w, "missing", 404)
					return
				}
				fmt.Fprint(w, response)
			}))
			defer server.Close()
			err := New(server.URL, "test", "ns", "").ReconcileQueryFilters()
			if (err == nil) != (response == "missing") {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func reconcileStatus(err error, status int) bool {
	var h *HTTPError
	return errors.As(err, &h) && h.Status == status
}

func TestReconcileRetriesSchemaReadFailure(t *testing.T) {
	reads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Error("already filterable schema must not be written")
		}
		reads++
		if reads == 1 {
			http.Error(w, "unavailable", 503)
			return
		}
		fmt.Fprint(w, `{"workdir":{"type":"string","filterable":true,"full_text_search":true},"harness":{"type":"string"},"plan":{"type":"string"}}`)
	}))
	defer server.Close()
	c := New(server.URL, "test", "hev-traces", "")
	if err := c.ReconcileQueryFilters(); !reconcileStatus(err, 503) {
		t.Fatalf("lost read failure: %v", err)
	}
	if err := c.ReconcileQueryFilters(); err != nil {
		t.Fatal(err)
	}
	if reads != 2 {
		t.Fatalf("reads=%d", reads)
	}
}
