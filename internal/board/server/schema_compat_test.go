package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/hev/kit/internal/board/layer"
	"github.com/hev/kit/internal/board/store"
)

func TestBoardReadsWithOrWithoutImageAttributes(t *testing.T) {
	fixture, err := os.ReadFile("../testdata/row.json")
	if err != nil {
		t.Fatal(err)
	}
	image := store.Image{
		SHA256: strings.Repeat("a", 64), Name: "capture.png", Type: "image/png",
		Size: 123, Width: 20, Height: 30,
	}
	imagesJSON, err := json.Marshal([]store.Image{image})
	if err != nil {
		t.Fatal(err)
	}
	image.URL = store.ImagePath(image.SHA256)

	for _, tc := range []struct {
		name   string
		images []store.Image
	}{
		{name: "legacy schema"},
		{name: "image schema", images: []store.Image{image}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var row layer.Row
			if err := json.Unmarshal(fixture, &row); err != nil {
				t.Fatal(err)
			}
			row["vector"] = []float64{0.1, 0.2}
			if len(tc.images) > 0 {
				row["image_count"] = len(tc.images)
				row["images"] = string(imagesJSON)
			}
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				serveSchemaQuery(t, w, r, row)
			}))
			t.Cleanup(backend.Close)
			s, err := New(store.New(layer.New(backend.URL, "test-key", "schema-test", "")))
			if err != nil {
				t.Fatal(err)
			}
			// This fixture serves reads only. A view counter write must not add
			// schema fields before the legacy namespace has been exercised.
			s.readOnly = true
			get := func(path string, out any) string {
				t.Helper()
				w := httptest.NewRecorder()
				s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
				if w.Code != http.StatusOK {
					t.Fatalf("GET %s returned %d: %s", path, w.Code, w.Body.String())
				}
				if out != nil {
					if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
						t.Fatalf("decode GET %s: %v", path, err)
					}
				}
				return w.Body.String()
			}
			checkThread := func(thread store.Thread) {
				t.Helper()
				if thread.ID != 7 || thread.ImageCount != len(tc.images) {
					t.Fatalf("thread ID/count = %d/%d, want 7/%d", thread.ID, thread.ImageCount, len(tc.images))
				}
			}
			checkPost := func(post store.Post) {
				t.Helper()
				if post.N != 1 || !reflect.DeepEqual(post.Images, tc.images) {
					t.Fatalf("post number/images = %d/%+v, want 1/%+v", post.N, post.Images, tc.images)
				}
			}
			for _, path := range []string{"/", "/t/7"} {
				page := get(path, nil)
				if !strings.Contains(page, "A reusable finding") {
					t.Fatalf("GET %s did not render the thread", path)
				}
				if path == "/t/7" && strings.Contains(page, image.URL) != (len(tc.images) > 0) {
					t.Fatalf("GET %s rendered incorrect attachment state", path)
				}
			}
			var list []store.Thread
			get("/api/threads", &list)
			if len(list) != 1 {
				t.Fatalf("list returned %d threads, want 1", len(list))
			}
			checkThread(list[0])
			var detail struct {
				Thread store.Thread
				Posts  []store.Post
			}
			get("/api/threads/7", &detail)
			checkThread(detail.Thread)
			if len(detail.Posts) != 1 {
				t.Fatalf("detail returned %d posts, want 1", len(detail.Posts))
			}
			checkPost(detail.Posts[0])
			var hits []store.Hit
			get("/api/search?q=configuration", &hits)
			if len(hits) != 1 {
				t.Fatalf("search returned %d hits, want 1", len(hits))
			}
			checkThread(hits[0].Thread)
			checkPost(hits[0].Post)
		})
	}
}

type schemaProjection struct {
	Include []string `json:"include_attributes"`
	Exclude []string `json:"exclude_attributes"`
}

func serveSchemaQuery(t *testing.T, w http.ResponseWriter, r *http.Request, row layer.Row) {
	t.Helper()
	if r.Method != http.MethodPost || r.URL.Path != "/v2/namespaces/schema-test/query" {
		t.Errorf("unexpected Layer request %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
		return
	}
	var query struct {
		schemaProjection
		Queries []schemaProjection `json:"queries"`
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
		t.Errorf("decode Layer query: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	projections := query.Queries
	if len(projections) == 0 {
		projections = []schemaProjection{query.schemaProjection}
	}
	results := make([]map[string][]layer.Row, 0, len(projections))
	for _, p := range projections {
		if p.Include != nil && p.Exclude != nil {
			t.Error("include_attributes and exclude_attributes cannot be combined")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		for _, attr := range p.Include {
			// The fixture omits these older fields on the row but they exist in the schema.
			if _, ok := row[attr]; !ok && attr != "instance" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"status": "error",
					"error":  fmt.Sprintf("attribute %q not found in schema, cannot be part of include_attributes", attr),
				})
				return
			}
		}
		projected := layer.Row{"id": row["id"]}
		for attr, value := range row {
			if (p.Include == nil || slices.Contains(p.Include, attr)) && !slices.Contains(p.Exclude, attr) {
				projected[attr] = value
			}
		}
		if _, ok := projected["vector"]; ok {
			t.Error("board query fetched the vector")
		}
		results = append(results, map[string][]layer.Row{"rows": {projected}})
	}
	if len(query.Queries) == 0 {
		_ = json.NewEncoder(w).Encode(results[0])
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"results": results[:1]})
}
