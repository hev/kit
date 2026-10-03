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

func TestPhraseWirePaginationBudgetAndFilters(t *testing.T) {
	for _, kind := range []string{StoreTurbopuffer, StorePgvector} {
		t.Run(kind, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Header.Get("Authorization") != "Bearer own-key" || r.URL.Path != "/v2/namespaces/own/query" {
					t.Errorf("identity changed: %s", r.URL.Path)
				}
				var body struct {
					Rank    []any    `json:"rank_by"`
					Filters []any    `json:"filters"`
					Top     int      `json:"top_k"`
					Attrs   []string `json:"include_attributes"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(body.Rank, []any{"id", "asc"}) || body.Top > 1000 || body.Top < 1 {
					t.Errorf("not a bounded ordered scan: %+v", body)
				}
				want := []any{"session_id", "In", []any{"own-session"}}
				cursor := ""
				if body.Filters[0] == "And" {
					fs := body.Filters[1].([]any)
					if !reflect.DeepEqual(fs[0], want) {
						t.Errorf("lost filter: %v", body.Filters)
					}
					gt := fs[1].([]any)
					if gt[0] != "id" || gt[1] != "Gt" {
						t.Errorf("wrong cursor: %v", gt)
					}
					cursor = gt[2].(string)
				} else if !reflect.DeepEqual(body.Filters, want) {
					t.Errorf("lost filter: %v", body.Filters)
				}
				rows := []Hit{}
				for i := 0; i < 1002; i++ {
					id := fmt.Sprintf("%04d", i)
					if id <= cursor {
						continue
					}
					text := "amber quartz reversed"
					if i == 1000 || i == 1001 {
						text = "QUARTZ\t\nAMBER"
					}
					rows = append(rows, Hit{ID: id, SessionID: "own-session", Text: text, Dist: 99})
					if len(rows) == body.Top {
						break
					}
				}
				json.NewEncoder(w).Encode(map[string]any{"rows": rows})
			}))
			defer server.Close()
			c := New(server.URL, "own-key", "own", "")
			c.Caps, _ = StaticCapabilities(kind)
			c.Caps.Features = append(c.Caps.Features, FeatureCoverage{ID: FeatureOrderedScan, Support: Supported})
			for _, tc := range []struct {
				limit, want int
				truncated   bool
				calls       int
			}{{1000, 0, true, 2}, {1001, 1, true, 2}, {1002, 2, false, 2}} {
				calls = 0
				hits, truncated, err := c.PhraseHits("quartz amber", tc.limit, []any{"session_id", "In", []string{"own-session"}})
				if err != nil || len(hits) != tc.want || truncated != tc.truncated || calls != tc.calls {
					t.Fatalf("limit %d: hits=%v truncated=%v calls=%d err=%v", tc.limit, hits, truncated, calls, err)
				}
				for _, hit := range hits {
					if hit.Dist != 0 {
						t.Fatal("scan fabricated relevance", hit)
					}
				}
			}
		})
	}
}

func TestPhraseCapabilitiesAndUpstreamFailureNeverFallback(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; http.Error(w, "unsupported ordered scan", 422) }))
	defer server.Close()
	c := New(server.URL, "key", "own", "")
	for _, support := range []Support{Unsupported, Undeclared, Approximate} {
		c.Caps = Capabilities{Declared: true, Features: []FeatureCoverage{{ID: FeatureOrderedScan, Support: support}}}
		if _, _, err := c.PhraseHits("quartz amber", 10, nil); err == nil || !strings.Contains(err.Error(), "ordered_scan") {
			t.Fatal(err)
		}
	}
	if calls != 0 {
		t.Fatal("unsupported capability made a request")
	}
	c.Caps, _ = StaticCapabilities(StoreTurbopuffer)
	if _, _, err := c.PhraseHits("quartz amber", 10, nil); err == nil {
		t.Fatal("upstream refusal lost")
	}
	if calls != 1 {
		t.Fatal("phrase search fell back", calls)
	}
}

func TestPhraseEvalWireAndMissingNamespace(t *testing.T) {
	missing := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/namespaces/own-evals/query" {
			t.Error(r.URL.Path)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if !reflect.DeepEqual(body["filters"], []any{"id", "In", []any{"eval"}}) || !reflect.DeepEqual(body["rank_by"], []any{"id", "asc"}) {
			t.Error(body)
		}
		if missing {
			http.Error(w, "namespace not found", 404)
			return
		}
		fmt.Fprint(w, `{"rows":[{"id":"eval","session_id":"own-session","text":"QUARTZ\nAMBER"}]}`)
	}))
	defer server.Close()
	c := New(server.URL, "key", "own", "")
	hits, truncated, err := c.PhraseEvals("quartz amber", 10, []any{"id", "In", []string{"eval"}})
	if err != nil || truncated || len(hits) != 1 {
		t.Fatal(hits, truncated, err)
	}
	missing = true
	hits, truncated, err = c.PhraseEvals("quartz amber", 10, []any{"id", "In", []string{"eval"}})
	if err != nil || truncated || len(hits) != 0 {
		t.Fatal(hits, truncated, err)
	}
}
