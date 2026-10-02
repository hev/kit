package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstructionSourceConfiguration(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, "config.toml")
	t.Setenv("HEV_CONFIG", path)
	for _, tc := range []struct {
		text    string
		enabled bool
	}{{"[capture]\n", true}, {"[capture]\ninstructions=false\n", false}, {"[capture]\ninstructions=true\n", true}} {
		if err := os.WriteFile(path, []byte(tc.text), 0600); err != nil {
			t.Fatal(err)
		}
		cfg, err := LoadConfig()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.CaptureInstructions != tc.enabled {
			t.Fatalf("enabled=%v want=%v", cfg.CaptureInstructions, tc.enabled)
		}
	}
}
