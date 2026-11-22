package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hev/kit/internal/score"
	"github.com/hev/kit/internal/testenv"
	"github.com/spf13/cobra"
)

func execScoreCLI(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	root := &cobra.Command{Use: "hev", SilenceErrors: true, SilenceUsage: true}
	root.AddCommand(newScoreCommand())
	var outBuf, errBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	root.SetArgs(append([]string{"score"}, args...))
	err = root.Execute()
	return outBuf.String(), errBuf.String(), err
}

func writeCLIScorerScript(t *testing.T, dir, name, unixBody, winBody string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		p := filepath.Join(dir, name+".cmd")
		body := "@echo off\r\nif \"%~1\"==\"" + testenv.PrimeArg + "\" exit /b 0\r\n" + winBody + "\r\n"
		if err := os.WriteFile(p, []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
		testenv.PrimeExec(t, nil, p)
		return p
	}
	p := filepath.Join(dir, name+".sh")
	body := "#!/bin/sh\nif [ \"${1:-}\" = \"" + testenv.PrimeArg + "\" ]; then exit 0; fi\n" + unixBody + "\n"
	if err := os.WriteFile(p, []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
	testenv.PrimeExec(t, nil, p)
	return p
}

func gitCLI(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@example.test",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@example.test",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestScoreCLIPinVerifyAndRunCommitEndToEnd(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HEV_CONFIG", filepath.Join(home, ".hev", "config.toml"))
	t.Setenv("HEV_SCORE_PINS", filepath.Join(home, ".config", "hev", "scorers", "pins.toml"))

	// Create temporary git repository with protected fixture, code, and ordinary test.
	repo := t.TempDir()
	gitCLI(t, repo, "init", "-q", "-b", "main")
	writeRepoFile := func(rel, content string) {
		p := filepath.Join(repo, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeRepoFile("score_val.txt", "0.625\n")
	writeRepoFile("internal/search/search.go", "package search\n")
	writeRepoFile("internal/search/search_test.go", "package search\n")
	writeRepoFile("testdata/eval/queries.jsonl", "{\"id\":\"1\"}\n")
	gitCLI(t, repo, "add", "-A")
	gitCLI(t, repo, "commit", "-q", "-m", "base")
	baseSHA := gitCLI(t, repo, "rev-parse", "HEAD")

	// Candidate 1: edits code and neighbouring test (allowed).
	writeRepoFile("internal/search/search.go", "package search\n// improved\n")
	writeRepoFile("internal/search/search_test.go", "package search\n// added test\n")
	writeRepoFile("score_val.txt", "0.840\n")
	gitCLI(t, repo, "add", "-A")
	gitCLI(t, repo, "commit", "-q", "-m", "improve search")
	goodSHA := gitCLI(t, repo, "rev-parse", "HEAD")

	// Candidate 2: renames protected fixture (disqualified).
	gitCLI(t, repo, "checkout", "-q", "-b", "bad-rename", baseSHA)
	gitCLI(t, repo, "mv", "testdata/eval/queries.jsonl", "testdata/moved.jsonl")
	gitCLI(t, repo, "commit", "-q", "-m", "rename fixture")
	disqualifiedSHA := gitCLI(t, repo, "rev-parse", "HEAD")
	gitCLI(t, repo, "checkout", "-q", "main")

	// Host scorer under ~/.config/hev/scorers/search-recall/scorer.md.
	scorerDir := filepath.Join(home, ".config", "hev", "scorers", "search-recall")
	if err := os.MkdirAll(scorerDir, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "ran.marker")
	cmdPath := writeCLIScorerScript(t, scorerDir, "eval",
		`echo ran > "`+marker+`"
v=$(cat score_val.txt | tr -d ' \r\n')
printf '{"value": %s, "n": 50}\n' "$v"`,
		`echo ran > "`+marker+`"
set /p v=<score_val.txt
echo {"value": %v%, "n": 50}`,
	)
	dataPath := filepath.Join(scorerDir, "gold.jsonl")
	if err := os.WriteFile(dataPath, []byte("{\"gold\":true}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	specPath := filepath.Join(scorerDir, "scorer.md")
	specBody := `---
description: Search recall metric
kind: metric
subject: commit
command: ` + filepath.Base(cmdPath) + `
direction: max
unit: ratio
scale: 1000
timeout: 10s
data:
  - gold.jsonl
protect:
  - testdata/eval/**
---
`
	if err := os.WriteFile(specPath, []byte(specBody), 0600); err != nil {
		t.Fatal(err)
	}

	// 1. Before pinning: verify and run must refuse.
	if _, _, err := execScoreCLI(t, "verify", specPath); err == nil {
		t.Fatalf("verify on unpinned scorer succeeded unexpectedly")
	}
	if _, _, err := execScoreCLI(t, "run", specPath, "--commit", goodSHA, "--repo", repo); err == nil {
		t.Fatalf("run on unpinned scorer succeeded unexpectedly")
	}

	// 2. Pin and verify (--json and human).
	pinOut, _, err := execScoreCLI(t, "pin", specPath, "--json")
	if err != nil {
		t.Fatalf("hev score pin: %v", err)
	}
	var pinned score.PinSet
	if err := json.Unmarshal([]byte(pinOut), &pinned); err != nil {
		t.Fatalf("unmarshal pin JSON: %v (%s)", err, pinOut)
	}
	if pinned.Scorer != "search-recall" || !strings.HasPrefix(pinned.Pin, "sha256:") {
		t.Fatalf("unexpected pin output: %+v", pinned)
	}

	verifyOut, _, err := execScoreCLI(t, "verify", specPath)
	if err != nil || !strings.Contains(verifyOut, "verified search-recall") {
		t.Fatalf("hev score verify failed: err=%v out=%q", err, verifyOut)
	}

	// 3. Tamper data -> verify fails; restore -> verify succeeds.
	if err := os.WriteFile(dataPath, []byte("{\"gold\":false}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	tamperOut, _, err := execScoreCLI(t, "verify", specPath, "--json")
	if err == nil {
		t.Fatalf("verify succeeded on tampered data file")
	}
	if !strings.Contains(tamperOut, `"status":"stale"`) {
		t.Fatalf("expected stale JSON status, got %s", tamperOut)
	}
	if err := os.WriteFile(dataPath, []byte("{\"gold\":true}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := execScoreCLI(t, "verify", specPath); err != nil {
		t.Fatalf("verify after restoring data: %v", err)
	}

	// 4. Run passing candidate (--json and human, plus REPO@SHA + --scorer NAME shorthand).
	runJSON, _, err := execScoreCLI(t, "run", specPath, "--commit", goodSHA, "--parent", baseSHA, "--repo", repo, "--json")
	if err != nil {
		t.Fatalf("hev score run good candidate: %v (out=%s)", err, runJSON)
	}
	var goodRes score.Result
	if err := json.Unmarshal([]byte(runJSON), &goodRes); err != nil {
		t.Fatalf("decode run JSON: %v (%s)", err, runJSON)
	}
	if goodRes.Status != score.StatusScored || goodRes.Mark == nil || *goodRes.Mark != 840 {
		t.Fatalf("unexpected good result: %+v", goodRes)
	}

	humanOut, _, err := execScoreCLI(t, "run", "--scorer", "search-recall", "--commit", repo+"@"+baseSHA)
	if err != nil || !strings.Contains(humanOut, "mark=625") {
		t.Fatalf("shorthand run failed: err=%v out=%q", err, humanOut)
	}

	// 5. Run disqualified candidate: exits non-zero, never invokes command, emits structured disqualified JSON.
	_ = os.Remove(marker)
	disqJSON, _, err := execScoreCLI(t, "run", specPath, "--commit", disqualifiedSHA, "--parent", baseSHA, "--repo", repo, "--json")
	if err == nil {
		t.Fatalf("expected non-zero exit on disqualified candidate")
	}
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Fatalf("disqualified commit invoked the scorer command!")
	}
	var disqRes score.Result
	if err := json.Unmarshal([]byte(disqJSON), &disqRes); err != nil {
		t.Fatalf("decode disqualified JSON: %v (%s)", err, disqJSON)
	}
	if disqRes.Status != score.StatusDisqualified || disqRes.Value != nil || disqRes.Mark != nil {
		t.Fatalf("invalid disqualified result: %+v", disqRes)
	}
	if len(disqRes.ProtectedPaths) != 1 || disqRes.ProtectedPaths[0] != "testdata/eval/queries.jsonl" {
		t.Fatalf("unexpected protected_paths: %v", disqRes.ProtectedPaths)
	}

	// 6. Timeout refusal and error output redaction in CLI --json and stderr.
	if runtime.GOOS != "windows" {
		slowDir := filepath.Join(t.TempDir(), "slow-metric")
		if err := os.MkdirAll(slowDir, 0700); err != nil {
			t.Fatal(err)
		}
		slowCmd := writeCLIScorerScript(t, slowDir, "slow",
			`sleep 20 &
wait`, "")
		slowSpec := filepath.Join(slowDir, "scorer.md")
		if err := os.WriteFile(slowSpec, []byte("---\nkind: metric\ncommand: "+filepath.Base(slowCmd)+"\ndirection: max\ntimeout: 120ms\n---\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := execScoreCLI(t, "pin", slowSpec); err != nil {
			t.Fatal(err)
		}
		slowOut, _, slowErr := execScoreCLI(t, "run", slowSpec, "--commit", goodSHA, "--repo", repo, "--json")
		if slowErr == nil || !strings.Contains(slowOut, "timed out") {
			t.Fatalf("expected CLI timeout refusal, err=%v out=%q", slowErr, slowOut)
		}

		secretVal := "sk-live-SecretCredentialForCLITest123456"
		t.Setenv("CLI_TEST_SECRET_KEY", secretVal)
		errDir := filepath.Join(t.TempDir(), "failing-metric")
		if err := os.MkdirAll(errDir, 0700); err != nil {
			t.Fatal(err)
		}
		errCmd := writeCLIScorerScript(t, errDir, "fail",
			`echo "auth failed with token `+secretVal+` at $(pwd)" >&2
exit 1`, "")
		errSpec := filepath.Join(errDir, "scorer.md")
		if err := os.WriteFile(errSpec, []byte("---\nkind: metric\ncommand: "+filepath.Base(errCmd)+"\ndirection: max\ntimeout: 5s\n---\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := execScoreCLI(t, "pin", errSpec); err != nil {
			t.Fatal(err)
		}
		errOut, errStderr, runErr := execScoreCLI(t, "run", errSpec, "--commit", goodSHA, "--repo", repo, "--json")
		if runErr == nil {
			t.Fatalf("expected error from failing scorer")
		}
		if strings.Contains(errOut, secretVal) || strings.Contains(errStderr, secretVal) {
			t.Fatalf("secret leaked in CLI output: stdout=%q stderr=%q", errOut, errStderr)
		}
		if strings.Contains(errOut, repo) || strings.Contains(errStderr, repo) {
			t.Fatalf("local repo path leaked in CLI error output: stdout=%q stderr=%q", errOut, errStderr)
		}
	}
}
