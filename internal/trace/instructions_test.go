package trace

import (
	"encoding/json"
	"fmt"
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
	// Marshal the native cwd so Windows backslashes remain valid JSON and
	// discovery still exercises the real Windows directory hierarchy.
	session, err := json.Marshal(map[string]any{
		"type": "user", "uuid": "u", "sessionId": "s", "cwd": sub,
		"message": map[string]string{"role": "user", "content": "hello"},
	})
	if err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(root, "session.jsonl"), string(session))
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

type countingSource struct {
	units []Unit
	dirs  map[string]string
	reads map[string]int
}

func (s *countingSource) Describe() string       { return "counting" }
func (s *countingSource) Units() ([]Unit, error) { return s.units, nil }
func (s *countingSource) Read(u Unit) ([]Turn, error) {
	s.reads[u.Key]++
	if s.dirs[u.Key] == "" {
		return nil, fmt.Errorf("unreadable")
	}
	return []Turn{{Workdir: s.dirs[u.Key]}}, nil
}

// One unreadable session costs only its own projects, and a session is read
// again only when its signature changes.
func TestInstructionDiscoveryIsIncrementalAndSkipsBadSessions(t *testing.T) {
	home := t.TempDir()
	repo := filepath.Join(home, "repo")
	os.MkdirAll(filepath.Join(repo, ".git"), 0o700)
	os.WriteFile(filepath.Join(repo, "AGENTS.md"), []byte("agents"), 0o600)
	src := &countingSource{
		units: []Unit{{Key: "bad", Signature: "1"}, {Key: "good", Signature: "1"}},
		dirs:  map[string]string{"good": repo},
		reads: map[string]int{},
	}
	is := InstructionSource{Home: home, Sessions: []Source{src}, Workdirs: &WorkdirCache{}}
	for range 3 {
		files, err := is.Files()
		if err != nil || len(files) != 1 || files[0].Project != repo {
			t.Fatalf("files=%+v err=%v", files, err)
		}
	}
	if src.reads["good"] != 1 || src.reads["bad"] != 3 {
		t.Fatalf("reads=%v", src.reads)
	}
	src.units[1].Signature = "2"
	if _, err := is.Files(); err != nil || src.reads["good"] != 2 {
		t.Fatalf("changed session not re-read: %v %v", src.reads, err)
	}
}
