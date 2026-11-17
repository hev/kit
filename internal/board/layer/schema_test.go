package layer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeSchema is a Layer namespace that keeps only its schema, the way the
// store merges a schema-only write: named attributes are replaced, others
// stay.
type fakeSchema struct {
	mu     sync.Mutex
	schema map[string]map[string]any
	writes []map[string]any
}

func (f *fakeSchema) serve(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && r.URL.Path == "/v1/namespaces/board-test/schema":
			json.NewEncoder(w).Encode(f.schema)
		case r.Method == "POST" && r.URL.Path == "/v2/namespaces/board-test":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			f.writes = append(f.writes, body)
			for k := range body {
				if k != "schema" {
					t.Errorf("schema write carried %q", k)
				}
			}
			for k, v := range body["schema"].(map[string]any) {
				f.schema[k] = v.(map[string]any)
			}
			w.Write([]byte(`{"status":"OK"}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.Error(w, "no", 400)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// The live board's schema before the change: no facets, title and body with
// no filterable setting (so the store's default, true).
func liveSchema() map[string]map[string]any {
	return map[string]map[string]any{
		"text":  {"type": "string", "full_text_search": true, "embed": map[string]any{"model": "m"}},
		"title": {"type": "string"},
		"body":  {"type": "string", "full_text_search": false},
		"tags":  {"type": "[]string", "filterable": true},
	}
}

func TestFacetSchemaWriteIsIdempotent(t *testing.T) {
	f := &fakeSchema{schema: liveSchema()}
	c := New(f.serve(t).URL, "k", "board-test", "")
	plan, err := c.PlanSchema(FacetSchema())
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 6 {
		t.Fatalf("first plan: %+v", plan)
	}
	for _, ch := range plan {
		if !strings.HasPrefix(ch.Attr, "facet") || ch.To["filterable"] != true || ch.From != nil {
			t.Fatalf("change %+v", ch)
		}
	}
	if err := c.ApplySchema(plan); err != nil {
		t.Fatal(err)
	}
	if f.schema["title"]["filterable"] != nil {
		t.Fatal("the facet write touched title")
	}
	again, err := c.PlanSchema(FacetSchema())
	if err != nil || len(again) != 0 {
		t.Fatalf("rerun plans %+v (%v)", again, err)
	}
	if err := c.ApplySchema(again); err != nil || len(f.writes) != 1 {
		t.Fatalf("an empty plan wrote: %d writes (%v)", len(f.writes), err)
	}
}

func TestTextSchemaWriteIsSeparateAndIdempotent(t *testing.T) {
	f := &fakeSchema{schema: liveSchema()}
	c := New(f.serve(t).URL, "k", "board-test", "")
	plan, err := c.PlanSchema(TextSchema())
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 2 || plan[0].Attr != "body" || plan[1].Attr != "title" {
		t.Fatalf("plan %+v", plan)
	}
	// An existing attribute keeps its other settings.
	if plan[0].To["full_text_search"] != false || plan[0].To["filterable"] != false {
		t.Fatalf("body change %+v", plan[0].To)
	}
	if err := c.ApplySchema(plan); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.schema["facet_repo"]; ok {
		t.Fatal("the text write declared facets")
	}
	if again, err := c.PlanSchema(TextSchema()); err != nil || len(again) != 0 {
		t.Fatalf("rerun plans %+v (%v)", again, err)
	}
}

func TestSchemaPlanRefusesTypeChangesAndEmbeddings(t *testing.T) {
	if _, err := PlanSchemaFrom(map[string]map[string]any{"facet_repo": {"type": "string"}}, FacetSchema()); err == nil {
		t.Fatal("type change planned")
	}
	if _, err := PlanSchemaFrom(map[string]map[string]any{"title": {"type": "string", "embed": map[string]any{}}}, TextSchema()); err == nil {
		t.Fatal("embedded attribute planned")
	}
	if plan, err := PlanSchemaFrom(map[string]map[string]any{}, FacetSchema()); err != nil || len(plan) != 6 {
		t.Fatalf("missing namespace: %+v %v", plan, err)
	}
}

func TestOrdinaryWritesDeclareFacetsButNotTitleOrBody(t *testing.T) {
	s := New("http://layer.example", "k", "ns", "").Schema()
	for _, k := range FacetAttrs {
		if d, ok := s[k].(map[string]any); !ok || d["filterable"] != true {
			t.Fatalf("%s not declared filterable: %v", k, s[k])
		}
	}
	for _, k := range []string{"title", "body"} {
		if _, ok := s[k]; ok {
			t.Fatalf("ordinary writes redeclare %s; that would undo the text schema write", k)
		}
	}
}
