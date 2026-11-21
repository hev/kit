package layer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/hev/kit/internal/board/settings"
	"github.com/hev/kit/internal/layerenv"
)

func TestKeyResolutionAndTransportReuse(t *testing.T) {
	old := settings.Current
	t.Cleanup(func() { settings.Current = old })
	settings.Current = settings.Config{Endpoint: "https://layer.example", Namespace: "test"}
	for _, k := range append([]string{"FACTORY_BOARD_LAYER_API_KEY"}, layerenv.Names()...) {
		t.Setenv(k, "")
	}
	t.Setenv("HEVLAYER_API_KEY", "generic-key")
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
	t.Setenv("HEVLAYER_API_KEY", "")
	t.Setenv("PATH", t.TempDir())
	if _, err = Load(); err == nil {
		t.Fatal("missing key accepted")
	}
}
func TestReferenceResolverRejectsEmptyKey(t *testing.T) {
	old := settings.Current
	t.Cleanup(func() { settings.Current = old })
	settings.Current = settings.Config{Endpoint: "https://layer.example", Namespace: "test"}
	for _, k := range append([]string{"FACTORY_BOARD_LAYER_API_KEY"}, layerenv.Names()...) {
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
	t.Setenv("HEV_SECRETS_READ_LOG", "off")
	t.Setenv("HEVLAYER_API_KEY_OP_REF", "op://example/store/key")
	if _, err := Load(); err == nil {
		t.Fatal("empty credential accepted")
	}
}

func TestPinUsesOneAtomicStoreTransition(t *testing.T) {
	var mu sync.Mutex
	var pinned any
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Rows      []Row           `json:"patch_rows"`
			Condition json.RawMessage `json:"patch_condition"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Rows) != 1 || req.Rows[0]["id"] != "t:42" {
			t.Errorf("unexpected pin patch: %+v (%v)", req, err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		on := req.Rows[0]["pinned"]
		want := `["pinned","Eq",true]`
		if on == true {
			want = `["Or",[["pinned","Eq",false],["pinned","Eq",null]]]`
		}
		if string(req.Condition) != want {
			t.Errorf("pin condition = %s, want %s", req.Condition, want)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		affected := 0
		if on != pinned && (on == true || pinned == true) {
			affected = 1
			pinned = on
		}
		_ = json.NewEncoder(w).Encode(map[string]int{"rows_affected": affected})
	}))
	defer api.Close()
	result := make(chan bool, 2)
	for range 2 {
		client := New(api.URL, "test", "test", "")
		go func() {
			changed, err := client.SetPinned("t:42", true)
			if err != nil {
				t.Error(err)
			}
			result <- changed
		}()
	}
	first, second := <-result, <-result
	if first == second {
		t.Fatalf("concurrent pins should have one winner: %t, %t", first, second)
	}
	client := New(api.URL, "test", "test", "")
	for _, tc := range []struct{ pinned, changed bool }{{false, true}, {false, false}, {true, true}} {
		changed, err := client.SetPinned("t:42", tc.pinned)
		if err != nil || changed != tc.changed {
			t.Fatalf("set pinned %t: changed=%t (%v)", tc.pinned, changed, err)
		}
	}
}
