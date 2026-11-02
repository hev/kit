package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/hev/kit/internal/board/layer"
	"github.com/hev/kit/internal/board/store"
)

func TestReadOnlyWalkRouter(t *testing.T) {
	s, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	s.readOnly = true
	for _, pattern := range s.patterns {
		method, path, _ := strings.Cut(pattern, " ")
		if method == "GET" {
			continue
		}
		path = strings.ReplaceAll(path, "{id}", "1")
		for _, verb := range []string{method, "PUT", "PATCH", "DELETE"} {
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, httptest.NewRequest(verb, path, strings.NewReader(`{}`)))
			if w.Code != 403 || !strings.Contains(w.Body.String(), "read-only") {
				t.Errorf("%s %s: %d %s", verb, path, w.Code, w.Body.String())
			}
		}
	}
}

func TestReadOnlyThreadReadsNeverWrite(t *testing.T) {
	row, err := os.ReadFile("../testdata/row.json")
	if err != nil {
		t.Fatal(err)
	}
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/query") {
			t.Errorf("write reached backend: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(append(append([]byte(`{"rows":[`), row...), []byte(`]}`)...))
	}))
	defer backend.Close()
	s, err := New(store.New(layer.New(backend.URL, "key", "test", "")))
	if err != nil {
		t.Fatal(err)
	}
	s.readOnly = true
	for _, path := range []string{"/api/threads/7", "/t/7"} {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 {
			t.Errorf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
}
