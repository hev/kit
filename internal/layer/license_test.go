package layer

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func licenseServer(t *testing.T, status int, body string) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/license" {
			t.Errorf("path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer k" {
			t.Errorf("authorization %q", got)
		}
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	t.Cleanup(server.Close)
	return New(server.URL, "k", "ns", "")
}

func TestLicenseEditions(t *testing.T) {
	for _, tc := range []struct {
		name, body, edition, warning string
		status                       int
		community                    bool
	}{
		{
			name:      "community",
			status:    200,
			body:      `{"valid":false,"state":"floor","reason":"open_gateway","gateway":{"state":"floor"}}`,
			edition:   "hev layer community",
			community: true,
		},
		{
			name:      "gateway older than the route",
			status:    404,
			edition:   "hev layer community",
			community: true,
		},
		{
			name:    "pro without a key",
			status:  200,
			body:    `{"valid":false,"state":"floor","reason":"missing","gateway":{"state":"floor"}}`,
			edition: "hev layer pro · no valid license (missing)",
		},
		{
			name:    "licensed",
			status:  200,
			body:    `{"valid":true,"tier":"starter","exp":"2026-12-14T00:00:00Z","gateway":{"state":"licensed","seconds_to_deadline":8000000}}`,
			edition: "hev layer pro · starter · through 2026-12-14",
		},
		{
			name:    "trial ending",
			status:  200,
			body:    `{"valid":true,"tier":"trial","exp":"2026-10-01T00:00:00Z","gateway":{"state":"licensed","seconds_to_deadline":200000}}`,
			edition: "hev layer pro · trial · through 2026-10-01",
			warning: "the hev layer trial ends in 3 days",
		},
		{
			name:    "grace",
			status:  200,
			body:    `{"valid":true,"tier":"starter","gateway":{"state":"grace","grace_seconds_remaining":86400}}`,
			edition: "hev layer pro · starter · expired, grace ends in 1 day",
			warning: "licensed features stop in 1 day",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lic, err := licenseServer(t, tc.status, tc.body).License()
			if err != nil {
				t.Fatal(err)
			}
			if got := lic.Edition(); got != tc.edition {
				t.Errorf("edition %q, want %q", got, tc.edition)
			}
			if lic.Community() != tc.community {
				t.Errorf("community %v", lic.Community())
			}
			if w := lic.Warning(); (tc.warning == "") != (w == "") || !strings.Contains(w, tc.warning) {
				t.Errorf("warning %q, want %q", w, tc.warning)
			}
		})
	}
}

func TestLicenseErrorStatus(t *testing.T) {
	if _, err := licenseServer(t, 500, "").License(); err == nil {
		t.Fatal("a 500 is not an edition")
	}
}

func TestLicenseRequired(t *testing.T) {
	c := licenseServerAt(t, 402, `{"error":"license_required","feature":"agents","renewal_url":"https://hevlayer.com/contact"}`)
	err := c.do("GET", "/v2/agents/x/query", nil, nil)
	if feature, ok := LicenseRequired(err); !ok || feature != "agents" {
		t.Fatalf("LicenseRequired = %q, %v", feature, ok)
	}
	other := licenseServerAt(t, 402, `{"error":"quota"}`).do("GET", "/x", nil, nil)
	if _, ok := LicenseRequired(other); ok {
		t.Fatal("a 402 that is not license_required")
	}
	if _, ok := LicenseRequired(fmt.Errorf("plain")); ok {
		t.Fatal("a non-HTTP error")
	}
}

func licenseServerAt(t *testing.T, status int, body string) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	t.Cleanup(server.Close)
	return New(server.URL, "k", "ns", "")
}
