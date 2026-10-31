package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/hev/kit/internal/board/layer"
	"github.com/hev/kit/internal/board/store"
)

// These responses were recorded from the legacy server with the same fixed
// Layer row. They protect the API independently of the new client types.
func TestLegacyServerGoldenWire(t *testing.T) {
	row, err := os.ReadFile("../testdata/row.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Method, Path string
		Body         any
		Status       int
		Response     any
	}
	data, err := os.ReadFile("../testdata/wire.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing Layer authentication")
		}
		if !strings.HasPrefix(r.URL.Path, "/v2/namespaces/wire-test") {
			t.Errorf("namespace changed: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/query") {
			w.Write(append(append([]byte(`{"rows":[`), row...), []byte(`]}`)...))
			return
		}
		w.Write([]byte(`{"status":"OK"}`))
	}))
	defer backend.Close()
	s, err := New(store.New(layer.New(backend.URL, "test-key", "wire-test", "")))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		t.Run(c.Method+" "+c.Path, func(t *testing.T) {
			body, _ := json.Marshal(c.Body)
			r := httptest.NewRequest(c.Method, c.Path, bytes.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, r)
			var got any
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if w.Code != c.Status || !reflect.DeepEqual(got, c.Response) {
				t.Fatalf("status %d body %s; want %d %+v", w.Code, w.Body.String(), c.Status, c.Response)
			}
		})
	}
}
