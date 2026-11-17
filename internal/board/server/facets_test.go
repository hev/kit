package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hev/kit/internal/board/facets"
	"github.com/hev/kit/internal/board/layer"
	"github.com/hev/kit/internal/board/settings"
	"github.com/hev/kit/internal/board/store"
)

type wireCase struct {
	Method   string `json:"method"`
	Path     string `json:"path"`
	Body     any    `json:"body"`
	Status   int    `json:"status"`
	Response any    `json:"response"`
}

func layerRowServer(t *testing.T, rowFile string) *store.Store {
	row, err := os.ReadFile(rowFile)
	if err != nil {
		t.Fatal(err)
	}
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/query") {
			w.Write(append(append([]byte(`{"rows":[`), row...), []byte(`]}`)...))
			return
		}
		w.Write([]byte(`{"status":"OK"}`))
	}))
	t.Cleanup(backend.Close)
	return store.New(layer.New(backend.URL, "test-key", "wire-test", ""))
}

// The facets fixture: a row that carries facets answers with the additive
// "facets" field; facets_by stays off the wire. Set BOARD_WIRE_UPDATE=1 to
// rewrite wire-facets.json after a deliberate change.
func TestFacetWireFixture(t *testing.T) {
	s, err := New(layerRowServer(t, "../testdata/row-facets.json"))
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{"/api/threads?limit=20&repo=example-app", "/api/threads/7?peek=1", "/api/search?q=configuration&top=10&area=ci"}
	var got []wireCase
	for _, p := range paths {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest("GET", p, nil))
		var body any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		got = append(got, wireCase{Method: "GET", Path: p, Status: w.Code, Response: body})
		if strings.Contains(w.Body.String(), "facets_by") || !strings.Contains(w.Body.String(), `"facets": {`) {
			t.Errorf("%s: %s", p, w.Body.String())
		}
	}
	const fixture = "../testdata/wire-facets.json"
	if os.Getenv("BOARD_WIRE_UPDATE") != "" {
		b, _ := json.MarshalIndent(got, "", "  ")
		os.WriteFile(fixture, append(b, '\n'), 0o644)
	}
	b, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	var want []wireCase
	json.Unmarshal(b, &want)
	gotJSON, _ := json.Marshal(got)
	var gotAny, wantAny any
	json.Unmarshal(gotJSON, &gotAny)
	json.Unmarshal(b, &wantAny)
	if !reflect.DeepEqual(gotAny, wantAny) {
		t.Fatalf("facet wire changed:\n%s", gotJSON)
	}
}

// The legacy row has no facet attributes: no "facets" key anywhere.
func TestLegacyRowHasNoFacetsField(t *testing.T) {
	s, err := New(layerRowServer(t, "../testdata/row.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"/api/threads", "/api/threads/7?peek=1", "/api/search?q=configuration"} {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest("GET", p, nil))
		if w.Code != 200 || strings.Contains(w.Body.String(), "facets") {
			t.Errorf("%s: %d %s", p, w.Code, w.Body.String())
		}
	}
}

const serverTaxonomy = `{
  "facets": {
    "repo": {"values": ["alpha", "beta"]},
    "host": {"values": ["box-one"]},
    "tool": {"values": ["docker", "git"]},
    "area": {"values": ["ci", "deploy"]},
    "client": {"values": ["acme"]}
  },
  "tag_mapping": [
    {"tag": "ci", "maps_to": "area:ci"},
    {"tag": "docker", "maps_to": "tool:docker"},
    {"tag": "alpha", "maps_to": "repo:alpha"}
  ]
}`

// facetServer is a server on a fresh filesystem board with the test taxonomy.
func facetServer(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	tax := filepath.Join(dir, "taxonomy.json")
	os.WriteFile(tax, []byte(serverTaxonomy), 0o600)
	old := settings.Current
	t.Cleanup(func() { settings.Current = old })
	settings.Current.Taxonomy = tax
	settings.Current.CanvasDir = filepath.Join(dir, "canvas")
	settings.Current.Dir = filepath.Join(dir, "board")
	st, err := store.NewFiles(filepath.Join(dir, "board"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(st)
	if err != nil {
		t.Fatal(err)
	}
	return s, st
}

func call(t *testing.T, s *Server, method, path string, body any) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest(method, path, rd))
	var out map[string]any
	json.Unmarshal(w.Body.Bytes(), &out)
	return w, out
}

func TestPostFacetsFromFlagsTagsAndInference(t *testing.T) {
	s, st := facetServer(t)
	ctx := context.Background()
	// Tags alone: the tag map fills the facets, facets_by tag-map.
	w, out := call(t, s, "POST", "/api/threads", map[string]any{"board": "tooling", "title": "t1", "body": "b", "author": "a", "tags": []string{"CI", "docker", "other"}})
	if w.Code != 201 || !reflect.DeepEqual(out["facets"], map[string]any{"tool": []any{"docker"}, "area": []any{"ci"}}) {
		t.Fatalf("tag map: %d %s", w.Code, w.Body.String())
	}
	if th, _ := st.Thread(ctx, 1); th.FacetsBy != facets.ByTagMap {
		t.Fatalf("facets_by %q", th.FacetsBy)
	}
	// The poster's values win per facet; inferred guesses fill what is left
	// and unknown guesses are dropped; the tag map fills the rest.
	w, out = call(t, s, "POST", "/api/threads", map[string]any{"board": "tooling", "title": "t2", "body": "b", "author": "a", "tags": []string{"ci", "alpha"},
		"facets": map[string]any{"tool": []string{"git"}}, "facets_inferred": map[string]any{"repo": []string{"beta"}, "client": []string{"unknown-client"}}})
	if w.Code != 201 || !reflect.DeepEqual(out["facets"], map[string]any{"repo": []any{"beta"}, "tool": []any{"git"}, "area": []any{"ci"}}) {
		t.Fatalf("author facets: %d %s", w.Code, w.Body.String())
	}
	if th, _ := st.Thread(ctx, 2); th.FacetsBy != facets.Author("a") {
		t.Fatalf("facets_by %q", th.FacetsBy)
	}
	// An explicit value outside the closed list is refused, with the reason.
	w, _ = call(t, s, "POST", "/api/threads", map[string]any{"board": "tooling", "title": "t3", "body": "b", "author": "a", "facets": map[string]any{"tool": []string{"kettle"}}})
	if w.Code != 400 || !strings.Contains(w.Body.String(), "closed tool list") {
		t.Fatalf("unknown tool: %d %s", w.Code, w.Body.String())
	}
	// No tags, no facets: the field is omitted.
	w, out = call(t, s, "POST", "/api/threads", map[string]any{"board": "tooling", "title": "t4", "body": "b", "author": "a"})
	if _, has := out["facets"]; w.Code != 201 || has {
		t.Fatalf("no facets: %d %s", w.Code, w.Body.String())
	}
	// A reply adds its facets to the thread's.
	w, _ = call(t, s, "POST", "/api/threads/3/posts", map[string]any{"body": "r", "author": "b", "facets": map[string]any{"host": []string{"box-one"}}})
	if th, _ := st.Thread(ctx, 3); w.Code != 201 || th.Facets == nil || th.Facets.String() != "host:box-one" {
		t.Fatalf("reply facets: %d %s %+v", w.Code, w.Body.String(), th)
	}
	w, _ = call(t, s, "POST", "/api/threads/3/posts", map[string]any{"body": "r", "author": "b", "facets": map[string]any{"host": []string{"box-nine"}}})
	if w.Code != 400 {
		t.Fatalf("unknown reply host: %d", w.Code)
	}
}

func ids(t *testing.T, s *Server, path string) []int {
	t.Helper()
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest("GET", path, nil))
	if w.Code != 200 {
		t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
	}
	var raw []map[string]any
	json.Unmarshal(w.Body.Bytes(), &raw)
	var out []int
	for _, r := range raw {
		if th, ok := r["thread"].(map[string]any); ok {
			r = th
		}
		out = append(out, int(r["id"].(float64)))
	}
	return out
}

func seed(t *testing.T, st *store.Store) {
	t.Helper()
	ctx := context.Background()
	for _, th := range []store.NewThread{
		{Board: "tooling", Title: "Cache port clash", Body: "the cache port clashes", Facets: facets.Set{Repo: []string{"alpha"}, Area: []string{"ci"}}, FacetsBy: "author"},
		{Board: "tooling", Title: "Deploy waits on the cache", Body: "deploy and cache", Facets: facets.Set{Repo: []string{"beta"}, Area: []string{"deploy"}, Host: []string{"box-one"}}, FacetsBy: "inventory-20990101:low"},
		{Board: "research", Title: "Cache eviction notes", Body: "cache notes", Facets: facets.Set{Tool: []string{"docker"}, Client: []string{"acme"}}, FacetsBy: "tag-map"},
		{Board: "research", Title: "Untagged cache thought", Body: "cache"},
	} {
		th.Poster = store.Poster{Author: "agent-a", Role: "agent"}
		if _, err := st.CreateThread(ctx, th); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFacetFiltersOnTheAPI(t *testing.T) {
	s, st := facetServer(t)
	seed(t, st)
	for path, want := range map[string][]int{
		"/api/threads?repo=alpha":               {1},
		"/api/threads?area=deploy&host=box-one": {2},
		"/api/threads?repo=alpha&repo=beta":     nil,
		"/api/threads?tool=docker":              {3},
		"/api/threads?client=acme":              {3},
		"/api/threads?board=tooling&area=ci":    {1},
		"/api/search?q=cache&area=ci":           {1},
		"/api/search?q=cache&host=box-one":      {2},
		"/api/search?q=cache&client=acme":       {3},
		"/api/search?q=cache&tool=docker":       {3},
		"/api/search?q=cache&repo=beta":         {2},
	} {
		if got := ids(t, s, path); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: %v, want %v", path, got, want)
		}
	}
	// Unfiltered listing and search are unchanged by facets.
	if got := ids(t, s, "/api/threads"); len(got) != 4 {
		t.Errorf("unfiltered ls: %v", got)
	}
	if got := ids(t, s, "/api/search?q=cache"); len(got) != 4 {
		t.Errorf("unfiltered search: %v", got)
	}
}

func page(t *testing.T, s *Server, path string) string {
	t.Helper()
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest("GET", path, nil))
	if w.Code != 200 {
		t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
	}
	return w.Body.String()
}

func TestFacetChipsFilterBarAndReviewPage(t *testing.T) {
	s, st := facetServer(t)
	seed(t, st)
	b := page(t, s, "/b/tooling")
	for _, want := range []string{`class="chip chip-repo" href="/b/tooling?repo=alpha"`, `class="facetbar"`, `<select name="area"`, `<option value="deploy">deploy</option>`} {
		if !strings.Contains(b, want) {
			t.Errorf("board page lacks %s", want)
		}
	}
	if filtered := page(t, s, "/b/tooling?area=deploy"); strings.Contains(filtered, "Cache port clash") || !strings.Contains(filtered, "Deploy waits") || !strings.Contains(filtered, `<option value="deploy" selected>`) {
		t.Error("board filter")
	}
	sp := page(t, s, "/search?q=cache&tool=docker")
	if !strings.Contains(sp, "Cache eviction notes") || strings.Contains(sp, "Cache port clash") || !strings.Contains(sp, `href="/search?q=cache&amp;tool=docker"`) {
		t.Errorf("search page:\n%s", sp)
	}
	tp := page(t, s, "/t/1")
	if !strings.Contains(tp, `action="/t/1/facets"`) || !strings.Contains(tp, "chip-area") {
		t.Error("thread page chips or edit form")
	}
	rv := page(t, s, "/review")
	if !strings.Contains(rv, "Untagged cache thought") || !strings.Contains(rv, "low-confidence inventory row") || strings.Contains(rv, "Cache port clash") {
		t.Errorf("review page:\n%s", rv)
	}
	if rv := page(t, s, "/review?missing=repo"); !strings.Contains(rv, "Cache eviction notes") || strings.Contains(rv, "Cache port clash") {
		t.Error("review missing=repo")
	}
}

func TestThreadPageFacetEdit(t *testing.T) {
	s, st := facetServer(t)
	seed(t, st)
	ctx := context.Background()
	before, _ := st.Thread(ctx, 2)
	post := func(form url.Values) int {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/t/2/facets", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		s.Handler().ServeHTTP(w, r)
		return w.Code
	}
	if code := post(url.Values{"op": {"add"}, "facet": {"tool"}, "value": {"git"}, "author": {"person-a"}}); code != 303 {
		t.Fatalf("add: %d", code)
	}
	if code := post(url.Values{"op": {"remove"}, "facet": {"host"}, "value": {"box-one"}}); code != 303 {
		t.Fatalf("remove: %d", code)
	}
	if code := post(url.Values{"op": {"add"}, "facet": {"tool"}, "value": {"kettle"}}); code != 400 {
		t.Fatalf("unknown value: %d", code)
	}
	if audit, aerr := os.ReadFile(st.Audit()); aerr != nil || !strings.Contains(string(audit), `"actor":"person-a"`) || !strings.Contains(string(audit), `"after_by":"author:person-a"`) || strings.Count(string(audit), "\n") != 5 {
		t.Fatalf("thread-page edits not audited (%s, %v):\n%s", st.Audit(), aerr, audit)
	}
	after, _ := st.Thread(ctx, 2)
	if after.Facets.String() != "repo:beta tool:git area:deploy" || after.FacetsBy != facets.Author("anon") || !after.Updated.Equal(before.Updated) {
		t.Fatalf("after edit: %+v %q", after.Facets, after.FacetsBy)
	}
}

// A post whose tags map to more values than a facet holds is refused with the
// reason, never written with the first twelve; naming the facet explicitly
// takes it off the tag map.
func TestPostRejectsOverCountTagMap(t *testing.T) {
	var vals, maps []string
	for i := 0; i <= facets.MaxValues; i++ {
		v := fmt.Sprintf("tool%02d", i)
		vals = append(vals, `"`+v+`"`)
		maps = append(maps, `{"tag": "many", "maps_to": "tool:`+v+`"}`)
	}
	s, st := facetServer(t)
	tax := filepath.Join(t.TempDir(), "crowded.json")
	os.WriteFile(tax, []byte(`{"facets": {"tool": {"values": [`+strings.Join(vals, ",")+`]}}, "tag_mapping": [`+strings.Join(maps, ",")+`]}`), 0o600)
	settings.Current.Taxonomy = tax
	s, err := New(st)
	if err != nil {
		t.Fatal(err)
	}
	w, _ := call(t, s, "POST", "/api/threads", map[string]any{"board": "tooling", "title": "t", "body": "b", "author": "a", "tags": []string{"many"}})
	if w.Code != 400 || !strings.Contains(w.Body.String(), "tool: 13 values, at most 12") {
		t.Fatalf("over-count tag map: %d %s", w.Code, w.Body.String())
	}
	if list, _ := st.Threads(context.Background(), store.ListOpts{}); len(list) != 0 {
		t.Fatalf("refused post written: %d threads", len(list))
	}
	w, out := call(t, s, "POST", "/api/threads", map[string]any{"board": "tooling", "title": "t", "body": "b", "author": "a", "tags": []string{"many"},
		"facets": map[string]any{"tool": []string{"tool01"}}})
	if w.Code != 201 || !reflect.DeepEqual(out["facets"], map[string]any{"tool": []any{"tool01"}}) {
		t.Fatalf("explicit tool: %d %s", w.Code, w.Body.String())
	}
}
