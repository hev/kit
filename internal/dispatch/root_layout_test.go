package dispatch

import (
	"os"
	"path/filepath"
	"testing"
)

// Run records from both sides: ~/.hev/orders on a fresh HOME, the legacy
// ~/dispatch where it already exists, HEV_ORDERS_DIR over both.
func TestRootUnifiedLayoutAndLegacyFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HEV_ORDERS_DIR", "")
	if got, want := Root(), filepath.Join(home, ".hev", "orders"); got != want {
		t.Fatalf("fresh: %s, want %s", got, want)
	}
	if _, err := os.Stat(filepath.Join(home, "dispatch")); !os.IsNotExist(err) {
		t.Fatal("resolving created ~/dispatch")
	}
	legacy := filepath.Join(home, "dispatch")
	if err := os.MkdirAll(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	if got := Root(); got != legacy {
		t.Fatalf("legacy: %s, want %s", got, legacy)
	}
	explicit := filepath.Join(home, "orders-elsewhere")
	t.Setenv("HEV_ORDERS_DIR", explicit)
	if got := Root(); got != explicit {
		t.Fatalf("env: %s", got)
	}
}
