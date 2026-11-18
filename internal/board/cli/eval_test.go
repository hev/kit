package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/hev/kit/internal/board/settings"
)

func TestEvalSyntheticEndpoint(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/search" || r.URL.Query().Get("top") != "2" || r.URL.Query().Get("board") != "synthetic" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
		hits := `[]`
		switch r.URL.Query().Get("q") {
		case "alpha":
			hits = `[{"thread":{"id":101,"title":"Imaginary alpha"},"snippet":"Hello, world!","score":0.8}]`
		case "beta":
			hits = `[{"thread":{"id":102,"title":"Imaginary beta"},"snippet":"one two","score":0.2},{"thread":{"id":101},"snippet":"three","score":0.1}]`
		case "neighbor":
			hits = `[{"thread":{"id":103},"snippet":"nearby","score":0.1}]`
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(hits))
	}))
	defer api.Close()
	old := settings.Current.URL
	settings.Current.URL = api.URL
	t.Cleanup(func() { settings.Current.URL = old })
	dir := t.TempDir()
	fixture := filepath.Join(dir, "fixture.json")
	output := filepath.Join(dir, "result.json")
	data := `{"version":"synthetic-v1","triage":{"keep_rate":0.5},"items":[
 {"id":1,"type":"fact","provenance":"external","question":"alpha","sources":[101],"judgment":"right"},
 {"id":2,"type":"current","provenance":"host-state","question":"beta","sources":[101],"superseded":[102],"judgment":"stale"},
 {"id":3,"type":"fact","provenance":"external","question":"alpha","sources":[999],"judgment":"right"},
 {"id":4,"type":"abstention","provenance":"external","question":"empty","sources":[],"judgment":"right"},
 {"id":5,"type":"abstention","provenance":"external","question":"neighbor","sources":[],"judgment":"right"}]}`
	if err := os.WriteFile(fixture, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runEval([]string{fixture, "--k", "2", "--board", "synthetic", "--out", output}); err != nil {
		t.Fatal(err)
	}
	read := func() evalResult {
		t.Helper()
		b, e := os.ReadFile(output)
		if e != nil {
			t.Fatal(e)
		}
		var r evalResult
		if e = json.Unmarshal(b, &r); e != nil {
			t.Fatal(e)
		}
		return r
	}
	r := read()
	if !r.Items[0].Hit || *r.Items[0].Rank != 1 || r.Items[0].Tokens != 4 || r.Items[2].Hit || r.Items[2].Rank != nil || !r.Items[3].Hit || r.Items[4].Hit {
		t.Fatalf("rows: %+v", r.Items)
	}
	if !r.Items[1].SupersededSurfaced || !r.Items[1].StaleAsCurrent || *r.Items[1].Rank != 2 {
		t.Fatal(r.Items[1])
	}
	a := r.Aggregates
	if a.Recall.Hits != 2 || a.Recall.Total != 3 || a.ByType["fact"].Rate != 0.5 || a.ByProvenance["host-state"].Hits != 1 || a.Abstention.Rate != 0.5 || a.Stale != 1 || a.Superseded != 1 || a.Correct.Hits != 2 || a.MeanTokens != 2.4 || a.P90Tokens != 4 {
		t.Fatalf("aggregates: %+v", a)
	}
	if r.BoardURL != api.URL || r.KitVersion == "" || r.Time.IsZero() || string(r.FixtureVersion) != `"synthetic-v1"` || string(r.Triage) == "" {
		t.Fatal(r)
	}
	if err := runEval([]string{"--json", fixture, "--k", "2", "--board", "synthetic", "--out", output, "--threshold", "0.5", "--only", "4,5"}); err != nil {
		t.Fatal(err)
	}
	r = read()
	if len(r.Items) != 2 || !r.Items[1].Hit || *r.Items[1].AbstentionRaw || !*r.Items[1].AbstentionThreshold || r.Aggregates.Abstention.Rate != 1 {
		t.Fatal(r)
	}
}

func TestEvalMissingScoreAndValidation(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"thread":{"id":100},"snippet":"Synthetic."}]`))
	}))
	defer api.Close()
	old := settings.Current.URL
	settings.Current.URL = api.URL
	t.Cleanup(func() { settings.Current.URL = old })
	path := filepath.Join(t.TempDir(), "fixture.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"items":[{"id":1,"type":"abstention","question":"fictional","sources":[]}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{path, "--threshold", "0.5"}, {path, "--k", "0"}, {path, "--only", "99"}, {path, "--only", "x"}, {path, "--threshold", "NaN"}} {
		if err := runEval(args); err == nil {
			t.Fatalf("expected error for %v", args)
		}
	}
}
func TestEstimateExcerptTokens(t *testing.T) {
	for _, tc := range []struct {
		s string
		n int
	}{{"", 0}, {"one two", 2}, {"hello, world!", 4}, {"café 中文\n42", 3}, {"a-b", 3}} {
		if got := estimateExcerptTokens(tc.s); got != tc.n {
			t.Errorf("%q: %d, want %d", tc.s, got, tc.n)
		}
	}
}
