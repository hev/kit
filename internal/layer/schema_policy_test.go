package layer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
)

func assertIndexPolicy(t *testing.T, schema map[string]any, kind string) {
	t.Helper()
	wanted := map[string]bool{}
	for _, field := range IndexedAttributes[kind] {
		wanted[field] = true
	}
	for field, raw := range schema {
		definition := raw.(map[string]any)
		enabled, explicit := definition["filterable"].(bool)
		if !explicit || enabled != wanted[field] {
			t.Errorf("%s.%s filterable=%v; expected explicit %v", kind, field, definition["filterable"], wanted[field])
		}
		if strings.HasPrefix(field, "commit_outcome_") || strings.HasPrefix(field, "ci_workflow_") || strings.HasPrefix(field, "mark_") {
			t.Errorf("dynamic column declared: %s", field)
		}
	}
}

func TestDeclaredIndexPolicy(t *testing.T) {
	var policy map[string][]string
	raw, err := os.ReadFile("../../docs/schema-cut/policy.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &policy); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(policy, IndexedAttributes) {
		t.Fatal("documented index policy differs from code")
	}
	for _, arrays := range []bool{false, true} {
		assertIndexPolicy(t, New("", "", "", "").schema(), "chunks")
		assertIndexPolicy(t, blockSchema(), "blocks")
		assertIndexPolicy(t, sessionSchema(arrays), "sessions")
		assertIndexPolicy(t, outcomeSchema(arrays), "sessions")
	}
	for kind, fields := range policy {
		ns := "hev-traces-" + kind
		if kind == "chunks" {
			ns = "hev-traces"
		}
		raw, err := os.ReadFile("../../docs/schema-cut/" + ns + ".schema.json")
		if err != nil {
			t.Fatal(err)
		}
		var schema map[string]any
		if err = json.Unmarshal(raw, &schema); err != nil {
			t.Fatal(err)
		}
		assertIndexPolicy(t, schema, kind)
		for _, field := range fields {
			if schema[field] == nil {
				t.Errorf("repair baseline missing %s.%s", kind, field)
			}
		}
	}
}

// Exercise chunk query builders so the guard checks the filters sent on the
// wire, rather than a second copy of SummaryUserChunks' field list.
func TestChunkQueryFiltersIndexed(t *testing.T) {
	indexed := map[string]bool{"id": true} // Provider built-in, outside the policy.
	for _, field := range IndexedAttributes["chunks"] {
		indexed[field] = true
	}
	var checkFilter func(any)
	checkFilter = func(raw any) {
		if raw == nil {
			return
		}
		filter := raw.([]any)
		key := filter[0].(string)
		if key == "And" || key == "Or" {
			for _, child := range filter[1].([]any) {
				checkFilter(child)
			}
			return
		}
		if !indexed[key] {
			t.Errorf("chunk query filter %q missing from IndexedAttributes[chunks]", key)
		}
	}
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/namespaces/chunks/query" {
			t.Errorf("unexpected query path: %s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		checkFilter(body["filters"])
		requests++
		fmt.Fprint(w, `{"rows":[],"aggregation_groups":[]}`)
	}))
	defer srv.Close()
	c := New(srv.URL, "", "chunks", "")
	queries := []struct {
		name string
		run  func() error
	}{
		{"SummaryUserChunks", func() error { _, err := c.SummaryUserChunks(context.Background(), []string{"s"}); return err }},
		{"ListSessions", func() error { _, err := c.ListSessions("2026-01-01", "2026-02-01"); return err }},
		{"SessionRows", func() error { _, err := c.SessionRows("s"); return err }},
		{"archiveSessionRows", func() error { _, err := c.archiveSessionRows("", "s"); return err }},
		{"ArchiveIdentityRows", func() error { _, err := c.ArchiveIdentityRows(""); return err }},
		// Search/PhraseHits forward caller filters. Enumerate the supported
		// chunk attributes as well as the shared ID pagination predicate.
		{"PhraseHits", func() error {
			var filters []any
			for _, key := range []string{"session_id", "harness", "project", "ts", "plan", "workdir", "id"} {
				filters = append(filters, []any{key, "Eq", "value"})
			}
			_, _, err := c.PhraseHits("test", 1, []any{"And", filters})
			return err
		}},
	}
	for _, query := range queries {
		t.Run(query.name, func(t *testing.T) {
			before := requests
			if err := query.run(); err != nil {
				t.Fatal(err)
			}
			if requests == before {
				t.Fatal("query did not send a request")
			}
		})
	}
}
