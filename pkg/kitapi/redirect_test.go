package kitapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hev/kit/pkg/kitapi"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// The client never follows a redirect, so the kit key goes only to the
// configured API host: not to a subdomain (where Go would forward the
// Authorization header), not to another host, and not even back to the same
// host on another path. Each refusal is a clear error.
func TestClientRefusesRedirects(t *testing.T) {
	for _, tc := range []struct{ name, location string }{
		{"subdomain", "https://child.api.example.test/collect"},
		{"other host", "https://elsewhere.example.net/collect"},
		{"same host", "https://api.example.test/v2/whoami"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls int
			var followed []string
			rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				w := httptest.NewRecorder()
				if calls == 1 {
					w.Header().Set("Location", tc.location)
					w.WriteHeader(http.StatusTemporaryRedirect)
					return w.Result(), nil
				}
				followed = append(followed, r.URL.String()+" auth="+r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"key_id":"ok"}`))
				return w.Result(), nil
			})
			c := kitapi.New("https://api.example.test", "hk_synthetic")
			c.HTTP.Transport = rt
			c.Retries = 0
			_, err := c.Whoami(context.Background())
			if len(followed) != 0 {
				t.Fatalf("redirect was followed: %v", followed)
			}
			if err == nil || !strings.Contains(err.Error(), "redirect to "+tc.location+" refused") {
				t.Fatalf("err = %v, want a refused-redirect error", err)
			}
			if kitapi.Retryable(err) {
				t.Fatalf("a redirect refusal should not be retried: %v", err)
			}
		})
	}
}

// A caller-supplied http.Client keeps its own redirect policy for its own
// use; the kit client overrides it only for its requests.
func TestClientRedirectPolicyIgnoresCallerClient(t *testing.T) {
	var leaked bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked = r.Header.Get("Authorization") != ""
		w.Write([]byte(`{}`))
	}))
	defer target.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/v1/whoami", http.StatusPermanentRedirect)
	}))
	defer api.Close()
	c := kitapi.New(api.URL, "hk_synthetic")
	c.HTTP = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return nil }}
	c.Retries = 0
	if _, err := c.Whoami(context.Background()); err == nil {
		t.Fatal("redirect accepted")
	}
	if leaked {
		t.Fatal("key reached the redirect target")
	}
}

func TestKitClientHidesKeysInErrorText(t *testing.T) {
	const key = "hk_synthetic_test_key_for_error"
	for _, redirect := range []bool{false, true} {
		api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if redirect {
				w.Header().Set("Location", "/"+key)
				w.WriteHeader(http.StatusTemporaryRedirect)
				return
			}
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"hk_word hk_synthetic_test_key_for_error"}`))
		}))
		client := kitapi.New(api.URL, key)
		_, err := client.Whoami(context.Background())
		api.Close()
		if err == nil || strings.Contains(err.Error(), key) || !strings.Contains(err.Error(), "[redacted]") {
			t.Fatal("request did not produce a redacted error")
		}
		if !redirect && !strings.Contains(err.Error(), "hk_word") {
			t.Fatal("the short identifier was lost")
		}
	}
}

func TestKitClientRawPathCannotChangeOrigin(t *testing.T) {
	client := kitapi.New("https://api.example.test", "hk_synthetic")
	client.HTTP.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.example.test" {
			t.Fatal("request reached a different origin")
		}
		return httptest.NewRecorder().Result(), nil
	})
	if _, err := client.DoRaw(context.Background(), http.MethodGet, "@elsewhere.example.net/v1/whoami", nil, nil); err == nil {
		t.Fatal("accepted non-path input")
	}
	if _, err := client.DoRaw(context.Background(), http.MethodGet, "/v1/whoami", nil, nil); err != nil {
		t.Fatal(err)
	}
}
