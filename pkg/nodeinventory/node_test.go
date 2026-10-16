package nodeinventory

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestObserveRealRuntimeCreatesNoLocalState(t *testing.T) {
	absent := filepath.Join(t.TempDir(), "absent")
	t.Setenv("HEV_CONFIG", filepath.Join(absent, "config.toml"))
	t.Setenv("FACTORY_HOME", absent)
	n, err := Observe(context.Background())
	if err != nil || n.Platform != runtime.GOOS || n.Architecture != runtime.GOARCH || n.Cores != runtime.NumCPU() {
		t.Fatal(n, err)
	}
	for _, v := range []Value[string]{n.Health, n.Capacity, n.CurrentWork, n.Maintenance} {
		if v.State != "Unknown" || v.Value != nil {
			t.Fatal(v)
		}
	}
	if _, err := os.Stat(absent); !os.IsNotExist(err) {
		t.Fatal("observer wrote state", err)
	}
	b, err := json.Marshal(n)
	if err != nil || !json.Valid(b) {
		t.Fatal(err)
	}
}
func TestObserveCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Observe(ctx); err == nil {
		t.Fatal("ignored cancellation")
	}
}
