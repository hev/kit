package layer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestSessionEnrichmentFilterPlanIsAdditiveReadOnly(t *testing.T) {
	for _, listType := range []string{"string", "[]string"} {
		t.Run(listType, func(t *testing.T) {
			original := map[string]map[string]any{
				"commits":                         {"type": listType, "filterable": false, "glob": true},
				"ci_conclusions":                  {"type": listType, "filterable": false},
				"pr":                              {"type": "string", "filterable": false, "full_text_search": map[string]any{"language": "english"}},
				"summary":                         {"type": "string", "filterable": false, "full_text_search": true},
				"ci_workflow_existing_conclusion": {"type": "string", "filterable": false, "regex": true},
				"first_prompt":                    {"type": "string", "embed": map[string]any{"model": "preserved"}},
			}
			before, _ := json.Marshal(original)
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "GET" {
					t.Error("repair plan wrote archive", r.Method)
				}
				json.NewEncoder(w).Encode(original)
			}))
			defer srv.Close()
			plan, e := New(srv.URL, "", "ns", "").SessionEnrichmentFilterPlan(context.Background())
			if e != nil {
				t.Fatal(e)
			}
			schema := plan["schema"].(map[string]any)
			if len(plan) != 1 || schema["summary"] != nil || schema["first_prompt"] != nil {
				t.Fatal("unrelated schema or row mutation", plan)
			}
			for _, field := range []string{"commits", "ci_conclusions", "pr", "ci_workflow_existing_conclusion"} {
				got := schema[field].(map[string]any)
				want := map[string]any{}
				for k, v := range original[field] {
					want[k] = v
				}
				want["filterable"] = true
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("settings changed %s %+v != %+v", field, got, want)
				}
			}
			for _, field := range []string{"pr_merged", "pr_closed", "revert_until", "reverted", "ci_state"} {
				if schema[field] == nil {
					t.Error("missing additive field", field)
				}
			}
			after, _ := json.Marshal(original)
			if string(before) != string(after) || calls != 1 {
				t.Fatal("source changed", calls)
			}
		})
	}
}
func TestSessionEnrichmentFilterPlanRefusesUnsupportedDefinitions(t *testing.T) {
	for _, schema := range []string{`{"commits":{"type":"int"}}`, `{"pr":{"type":"string","embed":{"model":"x"}}}`} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				t.Error("mutation on failed preflight")
			}
			fmt.Fprint(w, schema)
		}))
		_, e := New(srv.URL, "", "ns", "").SessionEnrichmentFilterPlan(context.Background())
		srv.Close()
		if e == nil {
			t.Fatal("unsupported repair accepted", schema)
		}
	}
}
