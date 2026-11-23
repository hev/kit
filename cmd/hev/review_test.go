package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/reviews"
)

const reviewTestToken = "ghp_0123456789abcdefghijklmnopqrstuvwxyz"

func reviewHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HEV_CONFIG", "")
	t.Setenv(reviews.DirEnv, "")
	return home
}

func reviewCLI(stdin string, args ...string) (string, error) {
	c := newReviewCommand()
	c.SilenceUsage = true
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&out)
	c.SetIn(strings.NewReader(stdin))
	c.SetArgs(args)
	err := c.Execute()
	return strings.TrimSpace(out.String()), err
}

func mustReviewCLI(t *testing.T, stdin string, args ...string) string {
	t.Helper()
	out, err := reviewCLI(stdin, args...)
	if err != nil {
		t.Fatalf("review %s: %v", args[0], err)
	}
	return out
}

func TestReviewCLIValidationAndSearch(t *testing.T) {
	home := reviewHome(t)
	head := strings.Repeat("b", 40)
	out := mustReviewCLI(t, `[{"file":"pkg/parser.go","severity":"major","body":"retry deadlock `+reviewTestToken+`"}]`,
		"create", "--repo", "kit", "--head", head, "--order", "example", "--verdict", "rework", "-m", "Migration "+reviewTestToken)
	id, verdict, ok := strings.Cut(out, " ")
	if !ok || !strings.HasPrefix(id, "rev_") || verdict != "rework" {
		t.Fatalf("unexpected review output %q", out)
	}
	minor := mustReviewCLI(t, "", "add", id, "--file", "pkg/other.go", "--lines", "2-4", "--severity", "minor", "-m", "retry cleanup")
	mustReviewCLI(t, "", "resolve", id, minor, "--status", "waived", "-m", "confirmed "+reviewTestToken)
	file := filepath.Join(home, "findings.json")
	if err := os.WriteFile(file, []byte(`[{"file":"pkg/parser.go","severity":"major","body":"retry deadlock"}]`), 0600); err != nil {
		t.Fatal(err)
	}
	mustReviewCLI(t, "", "put", "--repo", "other", "--head", head, "--verdict", "pass", "-m", "migration", "--findings-file", file)
	for _, tc := range []struct {
		query string
		flags []string
		count int
	}{
		{"migration retry parser.go", nil, 2},
		{"migration deadlock", []string{"--repo", "kit", "--path", "parser", "--severity", "major", "--open"}, 1},
		{"migration", []string{"--repo", "missing"}, 0},
		{"deadlock", []string{"--path", "other"}, 0},
		{"deadlock", []string{"--severity", "minor"}, 0},
		{"cleanup", []string{"--repo", "kit", "--severity", "minor"}, 1},
		{"cleanup", []string{"--open"}, 0},
		{reviewTestToken, nil, 0},
	} {
		var result []reviews.Review
		args := append([]string{"search", tc.query, "--json"}, tc.flags...)
		out := mustReviewCLI(t, "", args...)
		if err := json.Unmarshal([]byte(out), &result); err != nil || len(result) != tc.count || strings.Contains(out, reviewTestToken) {
			t.Fatalf("search result count %d, want %d, decode error %v, query flags %v", len(result), tc.count, err, tc.flags)
		}
		if len(result) == 2 && result[0].Repo != "other" {
			t.Fatal("results must be newest first")
		}
	}
	out = mustReviewCLI(t, "", "ls", "--order", "example", "--head", head, "--verdict", "rework", "--open", "--json")
	if !strings.Contains(out, id) || strings.Contains(out, reviewTestToken) {
		t.Fatal("list missing the selected review or leaked its raw text")
	}
	out = mustReviewCLI(t, "", "show", id, "--json")
	var r reviews.Review
	if err := json.Unmarshal([]byte(out), &r); err != nil || r.ID != id || len(r.Findings) != 2 || strings.Contains(out, reviewTestToken) {
		t.Fatal("show did not return the scrubbed review")
	}
	if f := r.Findings[1]; f.StartLine != 2 || f.EndLine != 4 || f.Status != "waived" || f.ResolvedAt == nil {
		t.Fatal("show lost line range or resolution")
	}
	mustReviewCLI(t, "", "resolve", id, r.Findings[0].ID, "--status", "fixed")

	for _, tc := range []struct {
		stdin string
		args  []string
	}{
		{"", []string{"put", "--repo", "kit", "--head", head, "--verdict", reviewTestToken}},
		{`[{"file":"p.go","severity":"major","status":"closed","body":"issue"}]`, []string{"put", "--repo", "kit", "--head", head}},
		{"", []string{"add", id, "--file", "pkg/file.go", "--severity", reviewTestToken, "-m", "finding"}},
		{"", []string{"add", id, "--file", "../" + reviewTestToken, "--severity", "major", "-m", "finding"}},
		{"", []string{"add", id, "--file", "/tmp/file.go", "--severity", "major", "-m", "finding"}},
		{"", []string{"add", id, "--file", "pkg/file.go", "--lines", "5-2", "--severity", "major", "-m", "finding"}},
		{"", []string{"resolve", id, minor, "--status", "open"}},
		{"", []string{"resolve", id, reviewTestToken, "--status", "fixed"}},
		{"", []string{"show", "../" + reviewTestToken}},
		{"", []string{"ls", "--verdict", "accepted"}},
		{"", []string{"search", "retry", "--severity", "medium"}},
	} {
		out, err := reviewCLI(tc.stdin, tc.args...)
		if err == nil || strings.Contains(err.Error(), reviewTestToken) || strings.Contains(out, reviewTestToken) {
			t.Fatalf("review %s should reject input without echoing secrets", tc.args[0])
		}
	}
}

func TestReviewCLIStoreLocation(t *testing.T) {
	home := reviewHome(t)
	config := filepath.Join(home, ".hev", "config.toml")
	fill(t, config, 0)
	if err := os.WriteFile(config, []byte("[reviews]\ndir = \"~/custom-reviews\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	head := strings.Repeat("c", 40)
	out := mustReviewCLI(t, "", "put", "--repo", "kit", "--head", head)
	id, _, _ := strings.Cut(out, " ")
	if _, err := os.Stat(filepath.Join(home, "custom-reviews", id, "review.json")); err != nil {
		t.Fatal(err)
	}
	override := t.TempDir()
	t.Setenv(reviews.DirEnv, override)
	out = mustReviewCLI(t, "", "put", "--repo", "kit", "--head", head)
	id, _, _ = strings.Cut(out, " ")
	if _, err := os.Stat(filepath.Join(override, id, "review.json")); err != nil {
		t.Fatal(err)
	}
}

func reviewGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, "git", args...)
	c.Dir = dir
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", args[0], err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestReviewCLIRunPinsDiffAndVerdict(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell fixture for the reviewer")
	}
	reviewHome(t)
	t.Setenv("HEV_ORDERS_DIR", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	for _, role := range []string{"AUTHOR", "COMMITTER"} {
		t.Setenv("GIT_"+role+"_NAME", "Test")
		t.Setenv("GIT_"+role+"_EMAIL", "review@example.test")
	}
	dir, bin := t.TempDir(), t.TempDir()
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	reviewGit(t, dir, "init", "-qb", "main")
	reviewGit(t, dir, "remote", "add", "origin", "https://github.com/hev/kit.git")
	file := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(file, []byte("base\n"), 0600); err != nil {
		t.Fatal(err)
	}
	reviewGit(t, dir, "add", "a.txt")
	reviewGit(t, dir, "commit", "-qm", "base")
	base := reviewGit(t, dir, "rev-parse", "HEAD")
	reviewGit(t, dir, "checkout", "-qb", "feature")
	if err := os.WriteFile(file, []byte("base\nfeature change\n"), 0600); err != nil {
		t.Fatal(err)
	}
	reviewGit(t, dir, "commit", "-qam", "head")
	head := reviewGit(t, dir, "rev-parse", "HEAD")
	if err := os.WriteFile(file, []byte("uncommitted work\n"), 0600); err != nil {
		t.Fatal(err)
	}

	codex := filepath.Join(bin, "codex")
	fake := `#!/bin/sh
set -eu
[ -f schema.json ]
[ ! -e a.txt ]
case " $* " in *" --sandbox read-only "*" --output-schema schema.json "*) ;; *) exit 3 ;; esac
input=$(cat)
case "$input" in *"untrusted data"*"+feature change"*) ;; *) exit 4 ;; esac
case "$input" in *"uncommitted work"*) exit 5 ;; esac
printf '%s\n' '{"verdict":"rework","summary":"Needs a fix","findings":[{"file":"a.txt","start_line":2,"end_line":2,"severity":"major","body":"Fix this issue"}]}'
`
	if err := os.WriteFile(codex, []byte(fake), 0700); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(dispatch.Root(), "example")
	if err := os.MkdirAll(runDir, 0700); err != nil {
		t.Fatal(err)
	}
	meta := dispatch.Meta{Order: dispatch.Order{Name: "example", Repo: "kit", Base: "main"},
		Branch: "feature", ResolvedRepo: dir}
	if err := dispatch.WriteJSON(filepath.Join(runDir, "meta.json"), meta); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(t.TempDir(), "dispatch.toml")
	if err := os.WriteFile(config, []byte("[hosts.local]\nlocal = true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	empty := t.TempDir()
	reviewGit(t, empty, "init", "-q")
	for _, tc := range []struct {
		args        []string
		repo, order string
	}{
		{[]string{"run", "main..feature", "--checkout", dir}, "github.com/hev/kit", ""},
		{[]string{"run", "feature", "--checkout", dir}, "github.com/hev/kit", ""},
		{[]string{"run", "example", "--checkout", empty, "--host", "local", "--config", config}, "kit", "example"},
	} {
		out := mustReviewCLI(t, "", tc.args...)
		id, verdict, _ := strings.Cut(out, " ")
		if verdict != "rework" {
			t.Fatalf("unexpected review verdict %q", verdict)
		}
		out = mustReviewCLI(t, "", "show", id, "--json")
		var r reviews.Review
		if err := json.Unmarshal([]byte(out), &r); err != nil || r.BaseSHA != base || r.HeadSHA != head ||
			r.Repo != tc.repo || r.Order != tc.order || r.Harness != "codex" || len(r.Findings) != 1 {
			t.Fatalf("review was not bound to the selected commits: %v %s", err, out)
		}
		if f := r.Findings[0]; f.File != "a.txt" || f.StartLine != 2 || f.EndLine != 2 || f.Status != "open" {
			t.Fatal("review lost its anchored finding")
		}
	}
	if err := os.WriteFile(codex, []byte("#!/bin/sh\nprintf '%s\\n' '"+reviewTestToken+"'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	out, err := reviewCLI("", "run", "main..feature", "--checkout", dir)
	if err == nil || strings.Contains(err.Error(), reviewTestToken) || strings.Contains(out, reviewTestToken) {
		t.Fatal("malformed harness output should fail without revealing its contents")
	}
	out = mustReviewCLI(t, "", "ls", "--json")
	var all []reviews.Review
	if err := json.Unmarshal([]byte(out), &all); err != nil || len(all) != 3 {
		t.Fatal("malformed harness output must not persist a review")
	}
}
