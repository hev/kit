package trace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstructionDiscovery(t *testing.T) {
	home := t.TempDir()
	repo := filepath.Join(home, "repo")
	sub := filepath.Join(repo, "sub")
	write := func(p, text string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	os.MkdirAll(filepath.Join(repo, ".git"), 0700)
	os.MkdirAll(sub, 0700)
	write(filepath.Join(repo, "CLAUDE.md"), "Use @AGENTS.md\n@AGENTS.md")
	write(filepath.Join(repo, "AGENTS.md"), "Cycle @CLAUDE.md")
	write(filepath.Join(sub, "CLAUDE.md"), "sub instructions")
	write(filepath.Join(home, ".claude", "CLAUDE.md"), "global")
	root := filepath.Join(home, ".claude", "projects", "fixture")
	write(filepath.Join(root, "memory", "MEMORY.md"), "memory")
	write(filepath.Join(root, "session.jsonl"), `{"type":"user","uuid":"u","sessionId":"s","cwd":"`+sub+`","message":{"role":"user","content":"hello"}}`)
	src := InstructionSource{Home: home, Sessions: []Source{&ClaudeSource{Root: filepath.Dir(root)}}}
	files, err := src.Files()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 5 {
		t.Fatalf("files=%+v", files)
	}
	for _, f := range files {
		if f.Hash == "" || f.Mtime == "" {
			t.Fatalf("missing metadata: %+v", f)
		}
		if strings.Contains(f.Path, "memory") && f.Project != sub {
			t.Fatalf("memory project=%q", f.Project)
		}
	}
	write(filepath.Join(repo, "AGENTS.md"), strings.Repeat("a", InstructionMaxBytes+1))
	if _, err := src.Files(); err == nil {
		t.Fatal("oversize import accepted")
	}
}
func TestInstructionImportDepthAndSymlinks(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".claude")
	os.MkdirAll(dir, 0700)
	for i := 0; i < 11; i++ {
		name := "CLAUDE.md"
		if i > 0 {
			name = strings.Repeat("x", i) + ".md"
		}
		next := strings.Repeat("x", i+1) + ".md"
		os.WriteFile(filepath.Join(dir, name), []byte("@"+next), 0600)
	}
	if _, err := (InstructionSource{Home: home}).Files(); err == nil {
		t.Fatal("unbounded imports")
	}
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("@alias.md"), 0600)
	os.Symlink(filepath.Join(dir, "CLAUDE.md"), filepath.Join(dir, "alias.md"))
	files, err := (InstructionSource{Home: home}).Files()
	if err != nil || len(files) != 1 {
		t.Fatalf("symlink cycle: %v %v", files, err)
	}
}
