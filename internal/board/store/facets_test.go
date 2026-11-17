package store

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/hev/kit/internal/board/facets"
	"github.com/hev/kit/internal/board/layer"
)

func TestFilesFacetsWriteFilterAndKeepActivity(t *testing.T) {
	s, _ := files(t)
	ctx := context.Background()
	who := Poster{Author: "agent-a", Role: "agent"}
	a, err := s.CreateThread(ctx, NewThread{Board: "tooling", Title: "Port collides", Body: "b", Poster: who,
		Facets: facets.Set{Repo: []string{"Alpha"}, Area: []string{"ci"}}, FacetsBy: facets.ByAuthor})
	if err != nil {
		t.Fatal(err)
	}
	if a.Facets == nil || a.Facets.Repo[0] != "alpha" || a.FacetsBy != facets.ByAuthor {
		t.Fatalf("created facets: %+v %q", a.Facets, a.FacetsBy)
	}
	b, _ := s.CreateThread(ctx, NewThread{Board: "tooling", Title: "Deploy stalls", Body: "port b", Poster: who})
	if b.Facets != nil || b.FacetsBy != "" {
		t.Fatalf("thread without facets carries %+v %q", b.Facets, b.FacetsBy)
	}
	// Filters: every value must match, as tags do.
	for _, c := range []struct {
		f    facets.Set
		want []int64
	}{
		{facets.Set{Repo: []string{"alpha"}}, []int64{a.ID}},
		{facets.Set{Repo: []string{"ALPHA"}, Area: []string{"ci"}}, []int64{a.ID}},
		{facets.Set{Repo: []string{"alpha"}, Area: []string{"deploy"}}, nil},
		{facets.Set{}, []int64{b.ID, a.ID}},
	} {
		ts, err := s.Threads(ctx, ListOpts{Facets: c.f})
		if err != nil {
			t.Fatal(err)
		}
		var got []int64
		for _, th := range ts {
			got = append(got, th.ID)
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("ls %s: %v, want %v", c.f.String(), got, c.want)
		}
	}
	hits, err := s.Search(ctx, SearchOpts{Q: "port", Facets: facets.Set{Area: []string{"ci"}}})
	if err != nil || len(hits) != 1 || hits[0].Thread.ID != a.ID || hits[0].Thread.Facets == nil {
		t.Fatalf("facet search: %+v %v", hits, err)
	}
	// SetFacets never moves the thread's activity.
	before, _ := s.Thread(ctx, b.ID)
	if err := s.SetFacets(ctx, b.ID, facets.Set{Tool: []string{"docker"}}, "inventory-20990101"); err != nil {
		t.Fatal(err)
	}
	after, _ := s.Thread(ctx, b.ID)
	if !after.Updated.Equal(before.Updated) || after.Views != before.Views || after.FacetsBy != "inventory-20990101" || after.Facets.Tool[0] != "docker" {
		t.Fatalf("set facets: before %+v after %+v", before, after)
	}
	// A reply's facets are added to the thread's, by the author.
	if _, err := s.Reply(ctx, NewPost{ThreadID: b.ID, Body: "more", Poster: who, Facets: facets.Set{Repo: []string{"beta"}}}); err != nil {
		t.Fatal(err)
	}
	after, _ = s.Thread(ctx, b.ID)
	if after.Facets.String() != "repo:beta tool:docker" || after.FacetsBy != facets.ByAuthor {
		t.Fatalf("reply facets: %+v %q", after.Facets, after.FacetsBy)
	}
	// Bounds are refused, never truncated.
	many := make([]string, facets.MaxValues+1)
	for i := range many {
		many[i] = strings.Repeat("v", i+1)
	}
	if err := s.SetFacets(ctx, b.ID, facets.Set{Tool: many}, "x"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("13 values: %v", err)
	}
	if _, err := s.CreateThread(ctx, NewThread{Title: "t", Body: "b", Poster: who, Facets: facets.Set{Area: []string{strings.Repeat("a", 65)}}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("oversize value: %v", err)
	}
	// Clearing writes an empty set; the wire form goes back to no facets.
	if err := s.SetFacets(ctx, a.ID, facets.Set{}, facets.ByAuthor); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Thread(ctx, a.ID); got.Facets != nil {
		t.Fatalf("cleared facets: %+v", got.Facets)
	}
}

// layerFake records every request body and answers queries with rows.
type layerFake struct {
	mu     sync.Mutex
	bodies []map[string]any
	rows   []map[string]any
}

func (f *layerFake) store(t *testing.T) *Store {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		body["_path"] = r.URL.Path
		f.bodies = append(f.bodies, body)
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/query") {
			json.NewEncoder(w).Encode(map[string]any{"rows": f.rows})
			return
		}
		w.Write([]byte(`{"status":"OK"}`))
	}))
	t.Cleanup(srv.Close)
	return New(layer.New(srv.URL, "k", "board-test", ""))
}

func contains(v any, want string) bool {
	b, _ := json.Marshal(v)
	return strings.Contains(string(b), want)
}

func TestLayerFacetFiltersAndPatch(t *testing.T) {
	f := &layerFake{rows: []map[string]any{{
		"id": "t:7", "kind": "thread", "thread_id": 7, "board": "tooling", "title": "x", "author": "a",
		"created": "2026-01-01T00:00:00.000000Z", "updated": "2026-01-01T00:00:00.000000Z",
		"facet_repo": []any{"alpha"}, "facet_area": nil, "facets_by": "tag-map",
	}}}
	s := f.store(t)
	ctx := context.Background()
	ts, err := s.Threads(ctx, ListOpts{Facets: facets.Set{Repo: []string{"alpha"}, Host: []string{"box"}}})
	if err != nil || len(ts) != 1 {
		t.Fatal(ts, err)
	}
	if ts[0].Facets == nil || ts[0].Facets.String() != "repo:alpha" || ts[0].FacetsBy != "tag-map" {
		t.Fatalf("row facets: %+v", ts[0])
	}
	q := f.bodies[len(f.bodies)-1]
	if !contains(q["filters"], `["facet_repo","Contains","alpha"]`) || !contains(q["filters"], `["facet_host","Contains","box"]`) {
		t.Fatalf("ls filters: %v", q["filters"])
	}
	if !contains(q["include_attributes"], `"facets_by"`) {
		t.Fatalf("thread attrs: %v", q["include_attributes"])
	}
	// Search resolves facet filters to thread ids first, as tags do.
	f.bodies = nil
	if _, err := s.Search(ctx, SearchOpts{Q: "x", Facets: facets.Set{Tool: []string{"git"}}}); err != nil {
		t.Fatal(err)
	}
	if !contains(f.bodies[0]["filters"], `["facet_tool","Contains","git"]`) || !contains(f.bodies[1], `"thread_id","In"`) {
		t.Fatalf("search requests: %v", f.bodies)
	}
	// SetFacets patches the six facet attributes and nothing else.
	f.bodies = nil
	if err := s.SetFacets(ctx, 7, facets.Set{Area: []string{"ci"}}, "inventory-x"); err != nil {
		t.Fatal(err)
	}
	w := f.bodies[len(f.bodies)-1]
	rows := w["patch_rows"].([]any)
	row := rows[0].(map[string]any)
	var keys []string
	for k := range row {
		keys = append(keys, k)
	}
	want := map[string]bool{"id": true, "facets_by": true, "facet_repo": true, "facet_host": true, "facet_tool": true, "facet_area": true, "facet_client": true}
	if len(keys) != len(want) {
		t.Fatalf("patch keys %v", keys)
	}
	for _, k := range keys {
		if !want[k] {
			t.Fatalf("patch touches %s", k)
		}
	}
	// A thread created without facets writes no facet attributes at all.
	f.rows = nil
	f.bodies = nil
	s.CreateThread(ctx, NewThread{Title: "t", Body: "b", Poster: Poster{Author: "a"}})
	for _, b := range f.bodies {
		if ups, ok := b["upsert_rows"]; ok && contains(ups, "facet_") {
			t.Fatalf("facet attrs on a thread without facets: %v", ups)
		}
	}
}

// Before anything declares the facet attributes, the store refuses them in
// include_attributes and filters. Reads must keep working: facets read as
// empty and a facet filter matches nothing.
func TestLayerReadsBeforeTheFacetSchemaExists(t *testing.T) {
	var asked []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		asked = append(asked, body)
		w.Header().Set("Content-Type", "application/json")
		for _, a := range append(layer.FacetAttrs, "image_count") {
			if contains(body["include_attributes"], `"`+a+`"`) || contains(body["filters"], `"`+a+`"`) {
				w.WriteHeader(400)
				w.Write([]byte(`{"error":"💔 attribute \"` + a + `\" not found in schema, cannot be part of include_attributes","status":"error"}`))
				return
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"rows": []map[string]any{{"id": "t:1", "kind": "thread", "thread_id": 1, "title": "x"}}})
	}))
	t.Cleanup(srv.Close)
	s := New(layer.New(srv.URL, "k", "board-test", ""))
	ctx := context.Background()
	ts, err := s.Threads(ctx, ListOpts{})
	if err != nil || len(ts) != 1 || ts[0].Facets != nil {
		t.Fatalf("plain ls: %+v %v", ts, err)
	}
	if th, err := s.Thread(ctx, 1); err != nil || th.ID != 1 {
		t.Fatalf("thread: %+v %v", th, err)
	}
	// Remembered: the next read asks once, without the unseen attributes.
	asked = nil
	if _, err := s.Threads(ctx, ListOpts{}); err != nil || len(asked) != 1 {
		t.Fatalf("cached unseen attributes: %d requests (%v)", len(asked), err)
	}
	ts, err = s.Threads(ctx, ListOpts{Facets: facets.Set{Repo: []string{"alpha"}}})
	if err != nil || len(ts) != 0 {
		t.Fatalf("facet ls: %+v %v", ts, err)
	}
	hits, err := s.Search(ctx, SearchOpts{Q: "x", Facets: facets.Set{Area: []string{"ci"}}})
	if err != nil || len(hits) != 0 {
		t.Fatalf("facet search: %+v %v", hits, err)
	}
}
