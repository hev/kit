package index

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
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

func TestGeminiStartContextDiscovery(t *testing.T) {
	home := t.TempDir()
	repo := filepath.Join(home, "repo")
	extensions := filepath.Join(home, ".gemini", "extensions", "fixture", "skills")
	if err := os.MkdirAll(extensions, 0700); err != nil {
		t.Fatal(err)
	}
	want := []string{extensions, filepath.Join(home, ".gemini", "skills"), filepath.Join(home, ".agents", "skills"), filepath.Join(repo, ".gemini", "skills"), filepath.Join(repo, ".agents", "skills")}
	if got := skillRoots("gemini", repo, home); !slices.Equal(got, want) {
		t.Fatalf("skill roots %v, want %v", got, want)
	}
	for i, root := range want {
		skill := filepath.Join(root, fmt.Sprintf("fixture-%d", i))
		if err := os.MkdirAll(skill, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("---\nname: fixture\ndescription: A fixture skill\n---\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if got := InstalledSkills(skillRoots("gemini", repo, home)); len(got) != len(want) {
		t.Fatalf("skills %+v", got)
	}
	instructions := []string{filepath.Join(home, ".gemini", "GEMINI.md"), filepath.Join(repo, "GEMINI.md")}
	if got := instructionCandidates("gemini", repo, repo, home); !slices.Equal(got, instructions) {
		t.Fatalf("instruction paths %v", got)
	}
	if got := skillRoots("agy", repo, home); len(got) != 0 {
		t.Fatalf("agy claimed Gemini skills: %v", got)
	}
}
