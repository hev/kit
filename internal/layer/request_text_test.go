package layer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/trace"
)

func TestWriteBlocksLargeRequestText(t *testing.T) {
	text := strings.Repeat("x", 26<<10)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Schema map[string]struct {
				Filterable *bool `json:"filterable"`
			} `json:"schema"`
			Rows []trace.BlockRow `json:"upsert_rows"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		flag := body.Schema["request_text"].Filterable
		if flag == nil || *flag || len(body.Rows) != 1 || body.Rows[0].RequestText != text {
			t.Error("large request missing or implicitly filterable")
		}
		fmt.Fprint(w, `{"status":"OK","rows_upserted":1}`)
	}))
	defer srv.Close()
	if _, err := New(srv.URL, "", "scratch", "").WriteBlocks([]trace.BlockRow{{ID: "b", RequestText: text}}); err != nil {
		t.Fatal(err)
	}
}

func TestRateLimitBackoffReplaysWrite(t *testing.T) {
	calls := 0
	var first time.Time
	var original string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		calls++
		if calls == 1 {
			first = time.Now()
			original = string(raw)
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(429)
			return
		}
		if time.Since(first) < time.Second || string(raw) != original {
			t.Error("retry did not back off or changed body")
		}
		fmt.Fprint(w, `{"status":"OK","rows_upserted":1}`)
	}))
	defer srv.Close()
	c := New(srv.URL, "", "scratch", "")
	if _, err := c.WriteBlocks([]trace.BlockRow{{ID: "b", Text: "hello"}}); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestRateLimitBackoffHonorsCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Header().Set("Retry-After", "60"); w.WriteHeader(429) }))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := New(srv.URL, "", "scratch", "").WithContext(ctx).Request("POST", "/v2/namespaces/scratch", map[string]any{"schema": blockSchema()}, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%v", err)
	}
}

// Fail when a new string field silently falls back to the provider's default
// filterable index. Query-required fields remain governed by IndexedAttributes.
func TestTraceStringAttributesExplicitlyDeclared(t *testing.T) {
	var check func(reflect.Type, map[string]any)
	check = func(typ reflect.Type, schema map[string]any) {
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			if f.Anonymous {
				check(f.Type, schema)
				continue
			}
			key := strings.Split(f.Tag.Get("json"), ",")[0]
			if key == "id" || key == "-" || key == "" {
				continue
			}
			if f.Type.Kind() != reflect.String && !(f.Type.Kind() == reflect.Slice && f.Type.Elem().Kind() == reflect.String) {
				continue
			}
			raw, ok := schema[key]
			if !ok {
				t.Errorf("%s.%s has no schema", typ.Name(), key)
				continue
			}
			if _, explicit := raw.(map[string]any)["filterable"].(bool); !explicit {
				t.Errorf("%s.%s has implicit filterability", typ.Name(), key)
			}
		}
	}
	check(reflect.TypeOf(trace.BlockRow{}), blockSchema())
	check(reflect.TypeOf(trace.SessionRow{}), sessionSchema(true))
	// path/version_id are written only for instruction-derived chunk rows and
	// explicitly declared by Write's VersionID branch.
	chunks := New("", "", "", "").schema()
	for _, key := range []string{"path", "version_id"} {
		chunks[key] = map[string]any{"type": "string", "filterable": false}
	}
	check(reflect.TypeOf(Row{}), chunks)
}
