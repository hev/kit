package index

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hev/kit/internal/trace"
)

// With no starting context in the transcript, capture derives it from the
// working directory and home, marks it derived, relativizes paths and keeps
// sizes and hashes only.
func TestDerivedStartContext(t *testing.T) {
	home := t.TempDir()
	repo := filepath.Join(home, "src", "app")
	wd := filepath.Join(repo, "svc")
	write := func(p, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	os.MkdirAll(filepath.Join(repo, ".git"), 0o755)
	write(filepath.Join(home, ".codex", "AGENTS.md"), "global rules\n")
	write(filepath.Join(repo, "AGENTS.md"), "repo rules\n")
	write(filepath.Join(wd, "AGENTS.md"), "svc\n")
	write(filepath.Join(home, ".agents", "skills", "peers", "SKILL.md"), "---\nname: peers\ndescription: Talk to other agents.\n---\nbody that is not counted\n")
	write(filepath.Join(home, ".codex", "skills", ".system", "imagegen", "SKILL.md"), "---\nname: imagegen\ndescription: >\n  Make images\n  of things.\n---\n")

	row := trace.SessionRow{Harness: "codex", Workdir: wd, ContextDigest: "sha256:abc"}
	sessionStartContext(&row, home, nil)
	c := row.StartContext
	if c == nil || c.Source != "derived" || c.ContextDigest != "sha256:abc" {
		t.Fatalf("start context: %+v", c)
	}
	want := []trace.InstructionRef{
		{Path: "~/.codex/AGENTS.md", Bytes: 13, SHA256: trace.ShortHash([]byte("global rules\n")), Source: "derived"},
		{Path: "AGENTS.md", Bytes: 11, SHA256: trace.ShortHash([]byte("repo rules\n")), Source: "derived"},
		{Path: "svc/AGENTS.md", Bytes: 4, SHA256: trace.ShortHash([]byte("svc\n")), Source: "derived"},
	}
	if len(c.Instructions) != len(want) {
		t.Fatalf("instructions: %+v", c.Instructions)
	}
	for i := range want {
		if c.Instructions[i] != want[i] {
			t.Fatalf("instruction %d = %+v, want %+v", i, c.Instructions[i], want[i])
		}
	}
	wantSkillBytes := int64(len("peers") + len("Talk to other agents.") + len("imagegen") + len("Make images of things."))
	if c.Skills != 2 || c.SkillBytes != wantSkillBytes || c.SkillsSource != "derived" {
		t.Fatalf("skills: %d %d %q, want 2 %d", c.Skills, c.SkillBytes, c.SkillsSource, wantSkillBytes)
	}
	if row.StartContextBytes != 13+11+4+wantSkillBytes {
		t.Fatalf("bytes = %d", row.StartContextBytes)
	}

	// A transcript record is kept and only completed: its paths are made
	// relative, and the source becomes mixed when skills are derived.
	row = trace.SessionRow{Harness: "codex", Workdir: wd, StartContext: &trace.StartContext{}}
	row.StartContext.AddInstruction(filepath.Join(repo, "AGENTS.md"), "merged", "transcript", []byte("seen"))
	sessionStartContext(&row, home, nil)
	if c := row.StartContext; len(c.Instructions) != 1 || c.Instructions[0].Path != "AGENTS.md" || c.Instructions[0].Source != "transcript" || c.Source != "mixed" || c.Skills != 2 {
		t.Fatalf("completed transcript record: %+v", c)
	}

	names := []string{}
	for _, s := range InstalledSkills(HarnessSkillRoots(home)) {
		names = append(names, s.Name)
	}
	if len(names) != 2 || names[0] != "imagegen" || names[1] != "peers" {
		t.Fatalf("installed: %v", names)
	}
}

func TestGrokStartContextSkills(t *testing.T) {
	home := t.TempDir()
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct{ root, file, content string }{
		{home, ".grok/skills/personal/SKILL.md", "---\nname: personal\ndescription: Personal skill\n---\n"},
		{repo, ".grok/skills/project/SKILL.md", "---\nname: project\ndescription: Project skill\n---\n"},
		{repo, ".codex/skills/foreign/SKILL.md", "---\nname: foreign\ndescription: Foreign skill\n---\n"},
		{repo, "AGENTS.md", "Example instructions\n"},
	} {
		path := filepath.Join(entry.root, entry.file)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(entry.content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	row := trace.SessionRow{Harness: "grok", Workdir: repo}
	sessionStartContext(&row, home, nil)
	if c := row.StartContext; c == nil || c.Skills != 2 || len(c.Instructions) != 1 || c.Instructions[0].Path != "AGENTS.md" {
		t.Fatalf("Grok context: %+v", c)
	}
}
