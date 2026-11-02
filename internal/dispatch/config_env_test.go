package dispatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHostEnvShape(t *testing.T) {
	for _, tc := range []struct {
		text  string
		valid bool
	}{
		{"env = [\"CUSTOM_KEY\"]\n", true},
		{"[hosts.local.env]\nCUSTOM_KEY = \"ignored\"\n", false},
	} {
		p := filepath.Join(t.TempDir(), "dispatch.toml")
		if err := os.WriteFile(p, []byte("[hosts.local]\nlocal = true\ncap = 1\n"+tc.text), 0600); err != nil {
			t.Fatal(err)
		}
		c, err := Load(p)
		if tc.valid {
			if err != nil {
				t.Fatal(err)
			}
			if len(c.Hosts["local"].Env) != 1 {
				t.Fatal(c)
			}
		} else if err == nil || !strings.Contains(err.Error(), "list of environment names") {
			t.Fatalf("%v", err)
		}
		_, targetErr := loadTarget(p, "local")
		if tc.valid && targetErr != nil {
			t.Fatal(targetErr)
		}
		if !tc.valid && (targetErr == nil || !strings.Contains(targetErr.Error(), "list of environment names")) {
			t.Fatalf("target: %v", targetErr)
		}
	}
}
