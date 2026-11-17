//go:build !windows

package dispatch

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/reviewnotes"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	b, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, b)
	}
	return strings.TrimSpace(string(b))
}

// reviewFixture makes a run "example" whose branch d/example is one commit
// ahead of main.
func reviewFixture(t *testing.T) (dir, repo, base, head string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	bin := t.TempDir()
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := os.WriteFile(filepath.Join(bin, "tmux"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	repo = t.TempDir()
	git(t, repo, "init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("one\n"), 0600)
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-qm", "base")
	base = git(t, repo, "rev-parse", "HEAD")
	git(t, repo, "checkout", "-qb", "d/example")
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("one\ntwo\n"), 0600)
	git(t, repo, "commit", "-qam", "change")
	head = git(t, repo, "rev-parse", "HEAD")
	git(t, repo, "checkout", "-q", "main")
	os.WriteFile(filepath.Join(repo, "b.txt"), []byte("later\n"), 0600)
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-qm", "main moves on")
	dir = filepath.Join(Root(), "example")
	if err := os.MkdirAll(filepath.Join(dir, "wt"), 0700); err != nil {
		t.Fatal(err)
	}
	m := Meta{Order: Order{Name: "example", Base: "main", Model: "opus"}, Branch: "d/example", ResolvedRepo: repo}
	if err := WriteJSON(filepath.Join(dir, "meta.json"), m); err != nil {
		t.Fatal(err)
	}
	return dir, repo, base, head
}

func TestReviewFetchBundlesTheRunBranch(t *testing.T) {
	_, _, base, head := reviewFixture(t)
	r, err := Handle(Request{Action: "review-fetch", Name: "example"})
	if err != nil {
		t.Fatal(err)
	}
	f := r.Review
	if f == nil || f.Head != head || f.MergeBase != base || f.BaseRef != "refs/heads/main" || f.Branch != "d/example" || len(f.Bundle) != 0 {
		t.Fatalf("fetch record: %+v", f)
	}

	// A full bundle lands in an empty repository.
	empty := t.TempDir()
	git(t, empty, "init", "-q")
	r, err = Handle(Request{Action: "review-fetch", Name: "example", Bundle: true})
	if err != nil || len(r.Review.Bundle) == 0 {
		t.Fatalf("full bundle: %v", err)
	}
	path := filepath.Join(t.TempDir(), "b.bundle")
	os.WriteFile(path, r.Review.Bundle, 0600)
	git(t, empty, "fetch", "-q", path, "+refs/heads/d/example:refs/hev/review/example")
	if got := git(t, empty, "rev-parse", "refs/hev/review/example"); got != head {
		t.Fatalf("fetched %s, want %s", got, head)
	}
	git(t, empty, "cat-file", "-e", base+"^{commit}")

	// A since bundle carries only commits after the merge base.
	r, err = Handle(Request{Action: "review-fetch", Name: "example", Bundle: true, Since: base})
	if err != nil || len(r.Review.Bundle) == 0 || len(r.Review.Bundle) >= len(mustRead(t, path)) {
		t.Fatalf("since bundle: %v (%d bytes)", err, len(r.Review.Bundle))
	}

	// Nothing to send when the caller already has the head.
	r, err = Handle(Request{Action: "review-fetch", Name: "example", Bundle: true, Since: head})
	if err != nil || len(r.Review.Bundle) != 0 {
		t.Fatalf("up to date: %v", err)
	}
	if _, err = Handle(Request{Action: "review-fetch", Name: "example", Bundle: true, Since: "--all"}); err == nil {
		t.Fatal("option-like since accepted")
	}
	if _, err = Handle(Request{Action: "review-fetch", Name: "missing"}); err == nil {
		t.Fatal("missing run fetched")
	}
}

func mustRead(t *testing.T, path string) []byte {
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestShowReportsReviewNotes(t *testing.T) {
	dir, _, _, head := reviewFixture(t)
	r, err := Handle(Request{Action: "show", Name: "example"})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, reviewnotes.FileName)
	if len(r.ReviewNotes) != 1 || r.ReviewNotes[0].Path != want || r.ReviewNotes[0].State != "missing" {
		t.Fatalf("missing notes: %+v", r.ReviewNotes)
	}
	os.WriteFile(want, []byte(`{"files":[{"path":"a.txt","annotations":[{"summary":"x","newRange":[2,1]}]}]}`), 0600)
	r, _ = Handle(Request{Action: "show", Name: "example"})
	if r.ReviewNotes[0].State != "invalid" || !strings.Contains(r.ReviewNotes[0].Error, "before start") {
		t.Fatalf("invalid notes: %+v", r.ReviewNotes)
	}
	doc := `{"version":1,"head":"` + head + `","files":[{"path":"a.txt","summary":"adds two","annotations":[{"summary":"x","newRange":[2,2]}]}]}`
	os.WriteFile(want, []byte(doc), 0600)
	r, _ = Handle(Request{Action: "show", Name: "example"})
	if s := r.ReviewNotes[0]; s.State != "valid" || s.Head != head || s.Annotations != 1 || r.Files[reviewnotes.FileName] != doc {
		t.Fatalf("valid notes: %+v", r.ReviewNotes)
	}
}

func TestWorkerPromptAsksForReviewNotes(t *testing.T) {
	m := Meta{Order: Order{Name: "fix-ci", Model: "opus"}}
	for _, p := range []string{workerPrompt(t.TempDir(), m), workerPrompt("", Meta{Order: Order{Name: "fix-ci", Model: "opus", Every: "1h"}}), workerPrompt("", Meta{Order: Order{Name: "fix-ci", Model: "opus"}, Parent: "p", ReviewOnly: true})} {
		for _, want := range []string{"../review-notes.json", `"author":"fix-ci opus"`, `"source":"fix-ci"`, `["finding","fail"]`, "never fail the run"} {
			if !strings.Contains(p, want) {
				t.Fatalf("prompt lacks %q:\n%s", want, p)
			}
		}
	}
	if strings.Contains(workerPrompt("", Meta{Parent: "p"}), "review-notes") {
		t.Fatal("browser validator asked for review notes")
	}
}
