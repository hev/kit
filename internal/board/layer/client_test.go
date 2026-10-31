package layer

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/hev/kit/internal/board/settings"
)

func TestKeyResolutionAndTransportReuse(t *testing.T) {
	old := settings.Current
	t.Cleanup(func() { settings.Current = old })
	settings.Current = settings.Config{Endpoint: "https://layer.example", Namespace: "test"}
	for _, k := range []string{"FACTORY_BOARD_LAYER_API_KEY", "LAYER_API_KEY", "TURBOPUFFER_API_KEY", "TPUF_API_KEY"} {
		t.Setenv(k, "")
	}
	t.Setenv("LAYER_API_KEY", "generic-key")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.APIKey != "generic-key" || c.HTTP != c.transport.HTTP {
		t.Fatal("shared resolver or transport not used")
	}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"rows":[]}`))
	}))
	t.Cleanup(api.Close)
	c = New(api.URL, "test", "test", "")
	transport := c.transport
	for i := 0; i < 2; i++ {
		if _, err := c.Query(map[string]any{"top_k": 1}); err != nil {
			t.Fatal(err)
		}
	}
	if transport != c.transport {
		t.Fatal("transport changed across requests")
	}
	t.Setenv("FACTORY_BOARD_LAYER_API_KEY", "legacy-key")
	c, err = Load()
	if err != nil || c.APIKey != "legacy-key" {
		t.Fatalf("legacy key: %v", err)
	}
	t.Setenv("FACTORY_BOARD_LAYER_API_KEY", "")
	t.Setenv("LAYER_API_KEY", "")
	t.Setenv("PATH", t.TempDir())
	if _, err = Load(); err == nil {
		t.Fatal("missing key accepted")
	}
}
func TestReferenceResolverRejectsEmptyKey(t *testing.T) {
	old := settings.Current
	t.Cleanup(func() { settings.Current = old })
	settings.Current = settings.Config{Endpoint: "https://layer.example", Namespace: "test"}
	for _, k := range []string{"FACTORY_BOARD_LAYER_API_KEY", "LAYER_API_KEY", "TURBOPUFFER_API_KEY", "TPUF_API_KEY"} {
		t.Setenv(k, "")
	}
	if os.PathSeparator == '\\' {
		t.Skip("shell fixture")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "op")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("LAYER_API_KEY_OP_REF", "op://example/store/key")
	if _, err := Load(); err == nil {
		t.Fatal("empty credential accepted")
	}
}
