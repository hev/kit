//go:build !windows

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/reviewnotes"
)

func gitT(t *testing.T, dir string, args ...string) string {
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

// A run whose repository lives on Azure Repos goes through the whole agent
// path (find, fetch at the exact head, merge, record, send) over the dispatch
// path, with no gh on PATH.
func TestReviewAgentPathAzureReposWithoutGh(t *testing.T) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not installed")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HEV_ORCHESTRATOR", "")
	t.Setenv("PEERS_SELF", "test-host")
	bin := t.TempDir()
	if err := os.Symlink(gitPath, filepath.Join(bin, "git")); err != nil {
		t.Fatal(err)
	}
	// The run is live: say reaches its inbox.
	if err := os.WriteFile(filepath.Join(bin, "tmux"), []byte("#!/bin/sh\nfor a; do [ \"$a\" = list-sessions ] && echo d-ado-fix; done\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	// The run's repository: origin is Azure Repos; origin/main is the base.
	repo := t.TempDir()
	gitT(t, repo, "init", "-q", "-b", "main")
	gitT(t, repo, "remote", "add", "origin", "https://dev.azure.com/ORG/PROJECT/_git/REPO")
	os.WriteFile(filepath.Join(repo, "a.cs"), []byte("one\n"), 0o600)
	gitT(t, repo, "add", ".")
	gitT(t, repo, "commit", "-qm", "base")
	base := gitT(t, repo, "rev-parse", "HEAD")
	gitT(t, repo, "update-ref", "refs/remotes/origin/main", base)
	gitT(t, repo, "checkout", "-qb", "d/ado-fix")
	os.WriteFile(filepath.Join(repo, "a.cs"), []byte("one\ntwo\n"), 0o600)
	gitT(t, repo, "commit", "-qam", "fix")
	head := gitT(t, repo, "rev-parse", "HEAD")
	gitT(t, repo, "checkout", "-q", "main")
	if url := gitT(t, repo, "remote", "get-url", "origin"); url != "https://dev.azure.com/ORG/PROJECT/_git/REPO" {
		t.Fatal(url)
	}

	dir := filepath.Join(dispatch.Root(), "ado-fix")
	if err := os.MkdirAll(filepath.Join(dir, "wt"), 0o700); err != nil {
		t.Fatal(err)
	}
	m := dispatch.Meta{Order: dispatch.Order{Name: "ado-fix", Repo: "app", Base: "main", Model: "opus"}, Branch: "d/ado-fix", ResolvedRepo: repo}
	if err := dispatch.WriteJSON(filepath.Join(dir, "meta.json"), m); err != nil {
		t.Fatal(err)
	}
	worker := `{"version":1,"head":"` + head + `","summary":"Fixes two.","files":[{"path":"a.cs","summary":"adds two","annotations":[{"summary":"why two","newRange":[2,2],"author":"ado-fix opus","source":"ado-fix"}]}]}`
	if err := os.WriteFile(filepath.Join(dir, reviewnotes.FileName), []byte(worker), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", bin)
	if _, err := exec.LookPath("gh"); err == nil {
		t.Fatal("gh is on PATH")
	}

	cache, wd := t.TempDir(), t.TempDir()
	d := defaultReviewDeps(new(string))
	d.load = func() (dispatch.Config, error) {
		return dispatch.Config{Hosts: map[string]dispatch.Host{"win-oven": {Local: true}}}, nil
	}
	d.cacheDir = func() (string, error) { return cache, nil }
	d.getwd = func() (string, error) { return wd, nil }
	run := func(args ...string) string {
		t.Helper()
		c := newReviewCommand(&d)
		var out, errout strings.Builder
		c.SetOut(&out)
		c.SetErr(&errout)
		c.SetIn(strings.NewReader(""))
		c.SetArgs(args)
		if err := c.Execute(); err != nil {
			t.Fatalf("hev review %v: %v\n%s", args, err, errout.String())
		}
		return out.String()
	}

	// A prior review's findings for this head, and one for the base.
	findings := filepath.Join(t.TempDir(), "findings.json")
	os.WriteFile(findings, []byte(`{"files":[{"path":"a.cs","annotations":[{"summary":"two needs a test","newRange":[2,2],"tags":["finding","fail"],"confidence":"high"}]}]}`), 0o600)
	if out := run("record", "ado-fix", "--head", head, "--file", findings, "--reviewer", "o-bcc"); !strings.Contains(out, "fail, 1 findings") {
		t.Fatal(out)
	}
	run("record", "ado-fix", "--head", base, "--file", findings, "--reviewer", "o-bcc")

	var ac struct {
		reviewnotes.Notes
		Ref, Repo string
		MergeBase string `json:"merge_base"`
		Dropped   []reviewnotes.Dropped
		Reviews   []reviewnotes.Record
	}
	out := run("ado-fix", "--json")
	if err := json.Unmarshal([]byte(out), &ac); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if ac.Head != head || len(ac.Files) != 1 || len(ac.Files[0].Annotations) != 2 || ac.Files[0].Annotations[1].Summary != "two needs a test" || len(ac.Dropped) != 1 || ac.Dropped[0].Head != base || len(ac.Reviews) != 2 {
		t.Fatalf("agent context:\n%s", out)
	}
	// The branch was fetched into the cache clone at its exact head.
	if got := gitT(t, ac.Repo, "rev-parse", ac.Ref); got != head || !strings.HasPrefix(ac.Repo, cache) || ac.MergeBase != base {
		t.Fatalf("fetched %s in %s, want %s", got, ac.Repo, head)
	}

	out = run("send", "ado-fix", "--recorded")
	inbox, _ := os.ReadFile(filepath.Join(dir, "inbox.md"))
	if !strings.Contains(string(inbox), "1. a.cs:2: [fail, high confidence] two needs a test") || !strings.Contains(out, "sent ado-fix") {
		t.Fatalf("send: %s\ninbox:\n%s", out, inbox)
	}
	if out = run("send", "ado-fix", "--recorded"); !strings.Contains(out, "nothing to send") {
		t.Fatal(out)
	}
}
