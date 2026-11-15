package kitapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDeltaRedactionMarkerCredential(t *testing.T) {
	for _, key := range []string{"[redacted]", "redacted", "[", "x"} {
		t.Run(fmt.Sprintf("%x", key), func(t *testing.T) {
			for _, status := range []int{400, 401, 402, 403, 429, 503} {
				s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Retry-After", "2")
					w.WriteHeader(status)
					fmt.Fprintf(w, `{"error":%q}`, "server echoed "+r.Header.Get("Authorization"))
				}))
				c := New(s.URL, key)
				c.Retries = 0
				_, e := c.Whoami(context.Background())
				s.Close()
				if e == nil {
					t.Fatal("expected error")
				}
				if strings.Contains(e.Error(), key) {
					t.Errorf("credential appears in error for status %d", status)
				}
				if (status == 401) != errors.Is(e, ErrUnauthorized) {
					t.Error("unauthorized classification changed")
				}
				if (status == 402) != ReadOnly(e) {
					t.Error("billing classification changed")
				}
				if (status == 429 || status == 503) != Retryable(e) {
					t.Error("retry classification changed")
				}
				var api *Error
				if errors.As(e, &api) && status != 401 && api.RetryAfter != 2*time.Second {
					t.Error("retry time lost")
				}
			}
		})
	}
}
func TestDeltaWriteBillingGuidance(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(402) }))
	defer s.Close()
	c := New(s.URL, "synthetic-secret")
	c.Retries = 0
	_, e := c.PostBatch(context.Background(), "/v1/traces/chunks", []byte("{}"))
	if e == nil || !strings.Contains(e.Error(), "kept locally") || !strings.Contains(e.Error(), "uploads resume") {
		t.Fatal("missing capture payment guidance")
	}
}
