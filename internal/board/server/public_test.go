package server

import (
	"github.com/hev/kit/internal/board/settings"
	"net/http/httptest"
	"testing"
)

func TestPublicBaseTrust(t *testing.T) {
	old := settings.Current
	t.Cleanup(func() { settings.Current = old })
	settings.Current = settings.Config{}
	t.Setenv("FACTORY_BOARD_PUBLIC_URL", "")
	r := httptest.NewRequest("GET", "http://internal.example/c/1/", nil)
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("X-Forwarded-Host", "external.example")
	if got := publicBase(r); got != "http://internal.example" {
		t.Fatal(got)
	}
	settings.Current.TrustedProxy = true
	if got := publicBase(r); got != "https://external.example" {
		t.Fatal(got)
	}
	settings.Current.PublicURL = "https://configured.example/"
	if got := publicBase(r); got != "https://configured.example" {
		t.Fatal(got)
	}
}
