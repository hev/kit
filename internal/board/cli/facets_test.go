package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hev/kit/internal/board/facets"
	"github.com/hev/kit/internal/board/store"
)

const cliTaxonomy = `{
  "facets": {
    "repo": {"values": ["alpha", "beta"]},
    "host": {"values": ["box-one"]},
    "tool": {"values": ["docker", "git"]},
    "area": {"values": ["ci", "deploy"]},
    "client": {"values": ["acme"]}
  },
  "tag_mapping": [{"tag": "ci", "maps_to": "area:ci"}]
}`

// An invented inventory: thread 1 and 2 get facets, 3 has a value outside
// the closed list, 4 is low-confidence, 5 has no facets, 6 is a person's
// edit, 99 is not on the board.
const cliInventory = `[
  {"id": 1, "title": "ignored", "facets": {"repo": ["alpha"], "area": ["ci"], "host": [], "tool": [], "client": []}, "confidence": "high"},
  {"id": 2, "facets": {"repo": ["beta", "alpha"], "tool": ["git"]}, "confidence": "high"},
  {"id": 3, "facets": {"tool": ["kettle"], "area": ["ci"]}, "confidence": "high"},
  {"id": 4, "facets": {"client": ["acme"], "host": ["box-one"]}, "confidence": "low"},
  {"id": 5, "facets": {"repo": [], "host": [], "tool": [], "area": [], "client": []}, "confidence": "high"},
  {"id": 6, "facets": {"repo": ["beta"]}, "confidence": "high"},
  {"id": 99, "facets": {"repo": ["alpha"]}, "confidence": "high"}
]`

func facetBoard(t *testing.T) (dir string, st *store.Store) {
	t.Helper()
	withoutKey(t)
	dir = t.TempDir()
	t.Setenv("HEV_BOARD_DIR", filepath.Join(dir, "board"))
	trap := layerTrap(t)
	t.Setenv("FACTORY_BOARD_LAYER_ENDPOINT", trap)
	os.WriteFile(filepath.Join(dir, "taxonomy.json"), []byte(cliTaxonomy), 0o600)
	os.WriteFile(filepath.Join(dir, "inventory.json"), []byte(cliInventory), 0o600)
	st, err := store.NewFiles(filepath.Join(dir, "board"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for i := 1; i <= 6; i++ {
		if _, err := st.CreateThread(ctx, store.NewThread{Board: "tooling", Title: "invented thread", Body: "b", Poster: store.Poster{Author: "agent"}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.SetFacets(ctx, 6, facets.Set{Repo: []string{"alpha"}}, facets.ByAuthor); err != nil {
		t.Fatal(err)
	}
	return dir, st
}

func TestBackfillPreviewWritesNothing(t *testing.T) {
	dir, st := facetBoard(t)
	before, _ := os.ReadFile(filepath.Join(dir, "board", "threads", "1.json"))
	out := must(t, "facets", "backfill", "--from", filepath.Join(dir, "inventory.json"), "--taxonomy", filepath.Join(dir, "taxonomy.json"), "--source", "inventory-20990101", "--json")
	var p backfillPlan
	decode(t, out, &p)
	if p.Read != 7 || p.OnBoard != 6 || !reflect.DeepEqual(p.Missing, []int64{99}) || len(p.Changes) != 3 ||
		len(p.Rejected) != 1 || p.Rejected[0].ID != 3 || !reflect.DeepEqual(p.Kept, []int64{6}) || !reflect.DeepEqual(p.Empty, []int64{5}) || p.Low != 1 {
		t.Fatalf("plan: %s", out)
	}
	if !strings.Contains(p.Rejected[0].Rejects[0].Reason, "closed tool list") {
		t.Fatalf("reject reason: %+v", p.Rejected)
	}
	cov := map[string]facetCoverage{}
	for _, c := range p.Coverage {
		cov[c.Facet] = c
	}
	if cov["repo"] != (facetCoverage{"repo", 3, 4, 2}) || cov["client"] != (facetCoverage{"client", 1, 1, 1}) {
		t.Fatalf("coverage: %+v", p.Coverage)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "board", "threads", "1.json"))
	if string(before) != string(after) {
		t.Fatal("preview wrote the thread")
	}
	if th, _ := st.Thread(context.Background(), 1); th.Facets != nil {
		t.Fatal("preview set facets")
	}
	text := must(t, "facets", "backfill", "--from", filepath.Join(dir, "inventory.json"), "--taxonomy", filepath.Join(dir, "taxonomy.json"), "--source", "inventory-20990101")
	for _, want := range []string{"preview: nothing written", "rows read            7", "would change         3", "#3  tool:kettle: not in the taxonomy's closed tool list"} {
		if !strings.Contains(text, want) {
			t.Errorf("text preview lacks %q:\n%s", want, text)
		}
	}
}

func TestBackfillApplyRerunAndRestore(t *testing.T) {
	dir, st := facetBoard(t)
	ctx := context.Background()
	before, _ := st.Thread(ctx, 1)
	undo := filepath.Join(dir, "undo.jsonl")
	args := []string{"facets", "backfill", "--from", filepath.Join(dir, "inventory.json"), "--taxonomy", filepath.Join(dir, "taxonomy.json"), "--source", "inventory-20990101"}
	out := must(t, append(args, "--apply", "--undo", undo)...)
	if !strings.Contains(out, "applied: 3 threads written") {
		t.Fatalf("apply: %s", out)
	}
	one, _ := st.Thread(ctx, 1)
	if one.Facets.String() != "repo:alpha area:ci" || one.FacetsBy != "inventory-20990101" || !one.Updated.Equal(before.Updated) {
		t.Fatalf("thread 1: %+v %q", one.Facets, one.FacetsBy)
	}
	if four, _ := st.Thread(ctx, 4); four.FacetsBy != "inventory-20990101"+facets.LowSuffix {
		t.Fatalf("low confidence: %q", four.FacetsBy)
	}
	if six, _ := st.Thread(ctx, 6); six.Facets.String() != "repo:alpha" || six.FacetsBy != facets.ByAuthor {
		t.Fatal("the backfill overwrote a person's edit")
	}
	// A rerun changes nothing.
	var p backfillPlan
	decode(t, must(t, append(args, "--json")...), &p)
	if len(p.Changes) != 0 || len(p.Unchanged) != 3 {
		t.Fatalf("rerun: %+v", p)
	}
	// A newer inventory with a newer source rewrites rows the old one wrote.
	newer := filepath.Join(dir, "newer.json")
	os.WriteFile(newer, []byte(`[{"id": 1, "facets": {"repo": ["beta"]}, "confidence": "high"}]`), 0o600)
	decode(t, must(t, "facets", "backfill", "--from", newer, "--taxonomy", filepath.Join(dir, "taxonomy.json"), "--source", "inventory-20990202", "--json"), &p)
	if len(p.Changes) != 1 || p.Changes[0].ID != 1 {
		t.Fatalf("newer inventory: %+v", p)
	}
	// A person edits thread 2 after the backfill; restore leaves it alone.
	st.SetFacets(ctx, 2, facets.Set{Tool: []string{"docker"}}, facets.ByAuthor)
	preview := must(t, "facets", "restore", "--from", undo)
	if !strings.Contains(preview, "would restore 2 threads; 1 left alone") {
		t.Fatalf("restore preview: %s", preview)
	}
	if one, _ := st.Thread(ctx, 1); one.Facets == nil {
		t.Fatal("restore preview wrote")
	}
	must(t, "facets", "restore", "--from", undo, "--apply")
	if one, _ := st.Thread(ctx, 1); one.Facets != nil || one.FacetsBy != "" {
		t.Fatalf("restored: %+v %q", one.Facets, one.FacetsBy)
	}
	if two, _ := st.Thread(ctx, 2); two.Facets.String() != "tool:docker" {
		t.Fatal("restore clobbered a person's edit")
	}
	// The undo file is never overwritten.
	if _, err := run(t, append(args, "--apply", "--undo", undo)...); err == nil {
		t.Fatal("existing undo file overwritten")
	}
}

func TestSchemaInFilesystemModeWritesNothing(t *testing.T) {
	facetBoard(t)
	for _, step := range []string{"facets", "text"} {
		if out := must(t, "facets", "schema", step, "--apply"); !strings.Contains(out, "filesystem mode") {
			t.Fatalf("%s: %s", step, out)
		}
	}
	if _, err := run(t, "facets", "schema", "everything"); err == nil {
		t.Fatal("unknown schema step accepted")
	}
}

func TestFacetFlagsOnListSearchPostAndReply(t *testing.T) {
	var got []*http.Request
	var bodies []map[string]any
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r)
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, body)
		switch {
		case r.URL.Path == "/api/threads" && r.Method == "GET":
			w.Write([]byte(`[{"id":7,"board":"tooling","title":"x","author":"a","facets":{"repo":["alpha"]}}]`))
		case r.URL.Path == "/api/search":
			w.Write([]byte(`[]`))
		case r.URL.Path == "/api/threads":
			w.Write([]byte(`{"id":8,"author":"a","facets":{"repo":["alpha"]}}`))
		default:
			w.Write([]byte(`{"id":"p:7:2","n":2,"author":"a"}`))
		}
	}))
	defer api.Close()
	t.Setenv("FACTORY_BOARD_URL", api.URL)
	oldRemote := gitRemote
	gitRemote = func(string) string { return "git@github.com:example-org/Alpha.git" }
	t.Cleanup(func() { gitRemote = oldRemote })

	out := must(t, "ls", "--repo", "alpha", "--area", "ci,deploy", "--host", "box-one")
	if q := got[0].URL.Query(); !reflect.DeepEqual(q["repo"], []string{"alpha"}) || !reflect.DeepEqual(q["area"], []string{"ci", "deploy"}) || q.Get("host") != "box-one" {
		t.Fatalf("ls query %s", got[0].URL.RawQuery)
	}
	if !strings.Contains(out, "{repo:alpha}") {
		t.Fatalf("ls line: %s", out)
	}
	must(t, "search", "cache", "--tool", "git", "--client", "acme")
	if q := got[1].URL.Query(); q.Get("tool") != "git" || q.Get("client") != "acme" || q.Get("q") != "cache" {
		t.Fatalf("search query %s", got[1].URL.RawQuery)
	}
	must(t, "post", "--title", "t", "-m", "b", "--as", "a", "--tool", "git")
	if f, _ := json.Marshal(bodies[2]["facets"]); string(f) != `{"tool":["git"]}` {
		t.Fatalf("post facets %s", f)
	}
	if f, _ := json.Marshal(bodies[2]["facets_inferred"]); string(f) != `{"repo":["alpha"]}` {
		t.Fatalf("post inferred %s", f)
	}
	must(t, "reply", "7", "-m", "b", "--as", "a", "--area", "ci")
	if f, _ := json.Marshal(bodies[3]["facets"]); string(f) != `{"area":["ci"]}` {
		t.Fatalf("reply facets %s", f)
	}
	// Without facet flags, ls sends no facet parameters.
	must(t, "ls")
	for _, n := range facets.Names {
		if got[4].URL.Query().Has(n) {
			t.Fatalf("plain ls sent %s", n)
		}
	}
}

func TestInferFacetsFromRemoteAndClientFolder(t *testing.T) {
	old := gitRemote
	t.Cleanup(func() { gitRemote = old })
	for remote, want := range map[string]string{
		"https://github.com/example-org/example-app.git": "example-app",
		"git@github.com:example-org/Example-Site":        "example-site",
		"": "",
	} {
		gitRemote = func(string) string { return remote }
		if got := repoOf("/x"); got != want {
			t.Errorf("%q: %q", remote, got)
		}
	}
	gitRemote = func(string) string { return "" }
	if got := inferFacets("/home/me/workspace/clients/Example-Client/repo/sub"); got.String() != "client:example-client" {
		t.Errorf("client folder: %q", got.String())
	}
	if got := inferFacets("/home/me/workspace/app"); !got.Empty() {
		t.Errorf("no client: %q", got.String())
	}
}

// End to end on the file-backed store: a post with no facet flags gets its
// repo from the git remote and its area from the tag map.
func TestNewPostInfersFacets(t *testing.T) {
	dir, _ := facetBoard(t)
	t.Setenv("HEV_BOARD_TAXONOMY", filepath.Join(dir, "taxonomy.json"))
	t.Setenv("FACTORY_CANVAS_DIR", filepath.Join(dir, "canvas"))
	old := gitRemote
	gitRemote = func(string) string { return "https://github.com/example-org/beta.git" }
	t.Cleanup(func() { gitRemote = old })
	startBoard(t)
	var th struct {
		ID     int64      `json:"id"`
		Facets facets.Set `json:"facets"`
	}
	decode(t, must(t, "post", "--board", "tooling", "--title", "Inferred", "--tags", "ci", "-m", "b", "--as", "agent-a", "--json"), &th)
	if th.Facets.String() != "repo:beta area:ci" {
		t.Fatalf("inferred facets: %+v", th.Facets)
	}
	out := must(t, "ls", "--repo", "beta", "--json")
	var ts []struct {
		ID int64 `json:"id"`
	}
	decode(t, out, &ts)
	if len(ts) != 1 || ts[0].ID != th.ID {
		t.Fatalf("ls --repo beta: %s", out)
	}
}
