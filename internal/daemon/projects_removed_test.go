package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectsFiltersRejected(t *testing.T) {
	path := sandboxConfig(t)
	os.MkdirAll(filepath.Dir(path), 0700)
	if err := os.WriteFile(path, []byte("[projects]\nallow = [\"/work/**\"]\ndeny = [\"/private/**\"]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadConfig()
	if err == nil || !strings.Contains(err.Error(), "never restricted capture") {
		t.Fatalf("%v", err)
	}
}

func TestProjectsEnvironmentRejected(t *testing.T) {
	for _, key := range []string{"HEVD_PROJECT_ALLOW", "HEVD_PROJECT_DENY"} {
		t.Run(key, func(t *testing.T) {
			t.Setenv(key, "/work/**")
			err := applyEnvOverrides(&Config{})
			if err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("%v", err)
			}
		})
	}
}
