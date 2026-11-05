package main

import (
	"net/http/httptest"
	"testing"
)

func TestLoginCallbackRejectsWrongState(t *testing.T) {
	codes := make(chan string, 1)
	h := loginCallback("expected", codes)
	for _, path := range []string{"/callback?state=wrong&code=secret", "/callback?state=expected", "/else?state=expected&code=secret"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 400 {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
	select {
	case <-codes:
		t.Fatal("invalid callback delivered code")
	default:
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/callback?state=expected&code=valid", nil))
	if w.Code != 200 || <-codes != "valid" {
		t.Fatal("valid callback rejected")
	}
}
func TestTeamServiceURLBoundary(t *testing.T) {
	for _, raw := range []string{"http://example.com", "https://u:p@example.com", "https://example.com?key=x", "file:///tmp/x"} {
		if validTeamURL(raw) {
			t.Fatalf("accepted %s", raw)
		}
	}
	for _, raw := range []string{"https://example.com", "http://127.0.0.1:8787"} {
		if !validTeamURL(raw) {
			t.Fatalf("rejected %s", raw)
		}
	}
}
