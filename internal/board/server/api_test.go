package server

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/hev/kit/internal/board/store"
)

func TestSearchOptsReadsHevQuerySyntax(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/search?q=+port+collides+&also=5432&also=already+allocated&since=7d&top=3&board=tooling&tag=ci&tag=demo&author=maintainer-demo&role=maintainer", nil)
	got := searchOpts(r, 10)
	want := store.SearchOpts{Q: "port collides", Also: []string{"5432", "already allocated"}, Limit: 3, Since: "7d",
		Board: "tooling", Tags: []string{"ci", "demo"}, Author: "maintainer-demo", Role: "maintainer"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
	if o := searchOpts(httptest.NewRequest("GET", "/api/search?q=x&limit=4", nil), 10); o.Limit != 4 {
		t.Errorf("limit is still read as top: %d", o.Limit)
	}
}

func TestCORSReadsOnlyForListedOrigins(t *testing.T) {
	t.Setenv("FACTORY_BOARD_CORS", "http://board.example:8080/, http://localhost:8080")
	h := cors(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	for _, c := range []struct {
		method, origin, path string
		allowed              bool
	}{
		{"GET", "http://board.example:8080", "/api/threads", true},
		{"GET", "http://localhost:8080", "/api/search", true},
		{"GET", "http://evil.example", "/api/threads", false},
		{"POST", "http://board.example:8080", "/api/threads", false},
		{"GET", "http://board.example:8080", "/t/1", false},
	} {
		r := httptest.NewRequest(c.method, c.path, nil)
		r.Header.Set("Origin", c.origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if got := w.Header().Get("Access-Control-Allow-Origin") != ""; got != c.allowed {
			t.Errorf("%s %s from %s: allowed=%v", c.method, c.path, c.origin, got)
		}
	}
}
