package daemon

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These tests change process stderr and must not run in parallel.
func captureProjectWarnings(t *testing.T, load func() error) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = original; r.Close(); w.Close() }()
	if err := load(); err != nil {
		t.Fatal(err)
	}
	w.Close()
	output, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(output)
}

func TestProjectsFiltersIgnoredWithWarning(t *testing.T) {
	path := sandboxConfig(t)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("[projects]\nallow = [\"~/workspace/**\"]\ndeny = [\"/private/**\"]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	output := captureProjectWarnings(t, func() error { _, err := LoadConfig(); return err })
	want := "warning: [projects] is ignored (it never filtered capture); remove it\n"
	if output != want {
		t.Fatalf("warning = %q, want %q", output, want)
	}
}

func TestProjectsEnvironmentIgnoredWithWarning(t *testing.T) {
	for _, key := range []string{"HEVD_PROJECT_ALLOW", "HEVD_PROJECT_DENY"} {
		t.Run(key, func(t *testing.T) {
			sandboxConfig(t)
			t.Setenv(key, "/work/**")
			output := captureProjectWarnings(t, func() error { _, err := LoadConfig(); return err })
			want := "warning: " + key + " is ignored (it never filtered capture); remove it\n"
			if strings.Count(output, want) != 1 {
				t.Fatalf("warning = %q, want one %q", output, want)
			}
		})
	}
}
