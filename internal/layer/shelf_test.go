package layer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hev/kit/internal/skillsources"
)

func TestSkillsCollectionWireAndKeys(t *testing.T) {
	sha := strings.Repeat("a", 40)
	var wrote, queried map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch r.URL.Path {
		case "/v2/namespaces/ns-skills":
			wrote = body
			w.Write([]byte(`{"status":"OK","rows_upserted":1}`))
		case "/v2/namespaces/ns-skills/query":
			queried = body
			w.Write([]byte(`{"results":[{"rows":[{"id":"x","text":"t","pack":"p","skill":"s","commit":"` + sha + `","license":"MIT","author":"o","path":"skills/s/SKILL.md"}]}]}`))
		default:
			t.Errorf("unexpected %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	c := New(srv.URL, "k", "ns", "")
	chunk := skillsources.Chunk{ID: skillsources.ChunkKey("src", sha, "skills/s/SKILL.md", 0), Text: "t", Pack: "p", Skill: "s", Kind: "tool", Source: "src", Commit: sha, Path: "skills/s/SKILL.md", License: "MIT", Author: "o", Homepage: "https://github.com/o/r"}
	if _, err := c.WriteSkillChunks([]skillsources.Chunk{chunk}); err != nil {
		t.Fatal(err)
	}
	schema := wrote["schema"].(map[string]any)
	assertIndexPolicy(t, schema, "skills")
	if wrote["distance_metric"] != "cosine_distance" || schema["text"].(map[string]any)["full_text_search"] != true {
		t.Fatalf("write body %#v", wrote)
	}
	row := wrote["upsert_rows"].([]any)[0].(map[string]any)
	for _, k := range []string{"id", "commit", "path", "source", "license", "author", "homepage", "pack"} {
		if row[k] == "" || row[k] == nil {
			t.Fatalf("row lacks %s: %v", k, row)
		}
	}
	hits, err := c.SearchSkills("tests first", 3, []any{"commit", "In", []string{sha}})
	if err != nil || len(hits) != 1 || hits[0].Commit != sha || hits[0].License != "MIT" || hits[0].Pack != "p" {
		t.Fatalf("hits %+v %v", hits, err)
	}
	b, _ := json.Marshal(queried)
	if !strings.Contains(string(b), `"commit","In"`) {
		t.Fatalf("query not scoped to pins: %s", b)
	}
}
