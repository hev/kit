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
			if err == nil || !strings.Contains(err.Error(), "redirect refused") || strings.Contains(err.Error(), tc.location) {
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
