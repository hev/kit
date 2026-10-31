package trace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultTranscriptRootsUseCurrentUserHome(t *testing.T) {
	home := t.TempDir()
	// os.UserHomeDir uses HOME on Unix and USERPROFILE on Windows.
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for _, tc := range []struct {
		name   string
		root   string
		source Source
	}{
		{"claude", filepath.Join(home, ".claude", "projects"), &ClaudeSource{Root: DefaultClaudeRoot()}},
		{"codex", filepath.Join(home, ".codex", "sessions"), &CodexSource{Root: DefaultCodexRoot()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(tc.root, "nested", "session.jsonl")
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("{}\n"), 0600); err != nil {
				t.Fatal(err)
			}
			units, err := tc.source.Units()
			if err != nil || len(units) != 1 {
				t.Fatalf("transcript discovery: units=%v err=%v", units, err)
			}
		})
	}
}
