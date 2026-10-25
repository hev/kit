package layer

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestProjectedRequiredFields(t *testing.T) {
	const valid = `{"rows":[],"source":{"rows":0,"pages":1,"bytes":0,"complete":true,"consistency":"observed","threads":1}}`
	for _, field := range []string{"rows", "pages", "bytes", "complete", "consistency", "threads"} {
		for _, mode := range []string{"missing", "null", "wrong-type"} {
			t.Run(field+"/"+mode, func(t *testing.T) {
				var obj map[string]any
				_ = json.Unmarshal([]byte(valid), &obj)
				src := obj["source"].(map[string]any)
				switch mode {
				case "missing":
					delete(src, field)
				case "null":
					src[field] = nil
				case "wrong-type":
					src[field] = []any{}
				}
				raw, _ := json.Marshal(obj)
				var out projectedRowsResponse
				if json.Unmarshal(raw, &out) == nil {
					t.Fatal("accepted invalid required field")
				}
			})
		}
	}
	for _, raw := range []string{`null`, `{}`, strings.Replace(valid, `"rows":[]`, `"rows":null`, 1), strings.Replace(valid, `"rows":[],`, ``, 1), strings.Replace(valid, `"rows":[]`, `"rows":{}`, 1), strings.Replace(valid, `"rows":[]`, `"error":null,"rows":[]`, 1), strings.Replace(valid, `"rows":[]`, `"errors":[],"rows":[]`, 1)} {
		var out projectedRowsResponse
		if json.Unmarshal([]byte(raw), &out) == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	var out projectedRowsResponse
	if err := json.Unmarshal([]byte(valid), &out); err != nil {
		t.Fatal(err)
	}
}

type sourceTransport func(*http.Request) (*http.Response, error)

func (f sourceTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type trackedSourceBody struct {
	io.Reader
	closed bool
	read   int
}

func (b *trackedSourceBody) Read(p []byte) (int, error) {
	n, e := b.Reader.Read(p)
	b.read += n
	return n, e
}
func (b *trackedSourceBody) Close() error { b.closed = true; return nil }

func TestProjectedWireLimitAndGenericSemantics(t *testing.T) {
	for _, bounded := range []bool{true, false} {
		body := &trackedSourceBody{Reader: strings.NewReader(strings.Repeat(" ", 2048))}
		var request *http.Request
		c := New("http://fixture", "key", "test", "")
		c.HTTP = &http.Client{Transport: sourceTransport(func(r *http.Request) (*http.Response, error) {
			request = r
			return &http.Response{StatusCode: 200, Body: body, Header: make(http.Header)}, nil
		})}
		var err error
		if bounded {
			err = c.doContextBounded(context.Background(), "POST", "/fixture", nil, nil, 1024)
		} else {
			err = c.doContext(context.Background(), "POST", "/fixture", nil, nil)
		}
		if !body.closed {
			t.Fatal("body not closed")
		}
		if bounded {
			if err == nil || err.Error() != "projected source response exceeds wire limit of 1024 bytes" || body.read != 1025 {
				t.Fatalf("read=%d err=%v", body.read, err)
			}
			if request.Context().Err() != context.Canceled {
				t.Fatal("request not cancelled")
			}
		} else if err != nil || body.read != 2048 {
			t.Fatalf("generic behavior changed: %d %v", body.read, err)
		}
	}
}
