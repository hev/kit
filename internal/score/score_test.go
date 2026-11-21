package score_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/score"
	"github.com/hev/kit/internal/testenv"
	"github.com/hev/kit/internal/trace"
)

func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HEV_CONFIG", filepath.Join(home, ".hev", "config.toml"))
	t.Setenv("HEV_SCORE_PINS", filepath.Join(home, ".config", "hev", "scorers", "pins.toml"))
	return home
}

func writeTestScript(t *testing.T, dir, baseName, unixBody, winBody string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		p := filepath.Join(dir, baseName+".cmd")
		content := "@echo off\r\nif \"%~1\"==\"" + testenv.PrimeArg + "\" exit /b 0\r\n" + winBody + "\r\n"
		if err := os.WriteFile(p, []byte(content), 0700); err != nil {
			t.Fatal(err)
		}
		testenv.PrimeExec(t, nil, p)
		return p
	}
	p := filepath.Join(dir, baseName+".sh")
	content := "#!/bin/sh\nif [ \"${1:-}\" = \"" + testenv.PrimeArg + "\" ]; then exit 0; fi\n" + unixBody + "\n"
	if err := os.WriteFile(p, []byte(content), 0700); err != nil {
		t.Fatal(err)
	}
	testenv.PrimeExec(t, nil, p)
	return p
}

func initGitRepo(t *testing.T, files map[string]string) (repo string, baseSHA string) {
	t.Helper()
	repo = t.TempDir()
	gitRun(t, repo, "init", "-q", "-b", "main")
	for rel, body := range files {
		full := filepath.Join(repo, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	gitRun(t, repo, "add", "-A")
	gitRun(t, repo, "commit", "-q", "-m", "initial")
	baseSHA = strings.TrimSpace(gitRun(t, repo, "rev-parse", "HEAD"))
	return repo, baseSHA
}

func gitRun(t *testing.T, dir string, args ...string) string {
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
	return string(out)
}

func TestParseSpecValidationRules(t *testing.T) {
	cases := []struct {
		name    string
		spec    string
		wantErr string
		check   func(*testing.T, score.Spec)
	}{
		{
			name: "valid full metric spec accepted",
			spec: `---
name: trace-recall-at-10
description: recall@10 on objective queries
kind: metric
subject: commit
command: score.sh
direction: max
unit: ratio
scale: 1000
timeout: 30m
data:
  - snapshot/archive.tar
  - queries/objective.jsonl
hidden: false
protect:
  - pkg/search/testdata/**
  - scripts/score-*.py
---
Evaluate recall@10 on the candidate commit.
`,
			check: func(t *testing.T, s score.Spec) {
				if s.Name != "trace-recall-at-10" || s.MarkName() != "trace_recall_at_10" {
					t.Fatalf("unexpected name/mark: %q / %q", s.Name, s.MarkName())
				}
				if s.Kind != "metric" || s.Subject != "commit" || s.Direction != "max" {
					t.Fatalf("unexpected core fields: %+v", s)
				}
				if s.Scale != 1000 || s.TimeoutDuration() != 30*time.Minute || s.Hidden {
					t.Fatalf("unexpected numeric/bool fields: %+v", s)
				}
				if len(s.Data) != 2 || len(s.Protect) != 2 {
					t.Fatalf("unexpected slices: %+v", s)
				}
			},
		},
		{
			name: "valid minimal spec defaults subject=commit and scale=1",
			spec: `---
kind: metric
command: run.sh
direction: min
timeout: 15s
hidden: true
---
`,
			check: func(t *testing.T, s score.Spec) {
				if s.Subject != "commit" || s.Scale != 1 || s.Direction != "min" || !s.Hidden {
					t.Fatalf("unexpected defaults: %+v", s)
				}
			},
		},
		{
			name: "valid visibility hidden sets Hidden=true",
			spec: `---
kind: metric
command: run.sh
direction: max
timeout: 1m
visibility: hidden
---
`,
			check: func(t *testing.T, s score.Spec) {
				if !s.Hidden {
					t.Fatalf("expected Hidden=true from visibility: hidden")
				}
			},
		},
		{
			name:    "missing frontmatter delimiter refused",
			spec:    "kind: metric\ncommand: run.sh\ndirection: max\ntimeout: 1m\n",
			wantErr: "must start with ---",
		},
		{
			name:    "unclosed frontmatter refused",
			spec:    "---\nkind: metric\ncommand: run.sh\ndirection: max\ntimeout: 1m\n",
			wantErr: "unclosed frontmatter",
		},
		{
			name: "unknown frontmatter key refused",
			spec: `---
kind: metric
command: run.sh
direction: max
timeout: 1m
bogus_field: 123
---
`,
			wantErr: "field bogus_field not found",
		},
		{
			name: "non-metric kind refused",
			spec: `---
kind: judge
command: run.sh
direction: max
timeout: 1m
---
`,
			wantErr: `kind must be "metric"`,
		},
		{
			name: "invalid subject refused",
			spec: `---
kind: metric
subject: session
command: run.sh
direction: max
timeout: 1m
---
`,
			wantErr: `subject must be "commit"`,
		},
		{
			name: "invalid scorer name refused",
			spec: `---
name: Bad_Name
kind: metric
command: run.sh
direction: max
timeout: 1m
---
`,
			wantErr: "invalid scorer name",
		},
		{
			name: "missing command refused",
			spec: `---
kind: metric
direction: max
timeout: 1m
---
`,
			wantErr: "command is required",
		},
		{
			name: "invalid direction refused",
			spec: `---
kind: metric
command: run.sh
direction: upward
timeout: 1m
---
`,
			wantErr: `direction must be "max" or "min"`,
		},
		{
			name: "missing direction refused",
			spec: `---
kind: metric
command: run.sh
timeout: 1m
---
`,
			wantErr: `direction must be "max" or "min"`,
		},
		{
			name: "zero scale refused",
			spec: `---
kind: metric
command: run.sh
direction: max
scale: 0
timeout: 1m
---
`,
			wantErr: "scale must be a positive integer",
		},
		{
			name: "negative scale refused",
			spec: `---
kind: metric
command: run.sh
direction: max
scale: -5
timeout: 1m
---
`,
			wantErr: "scale must be a positive integer",
		},
		{
			name: "missing timeout refused",
			spec: `---
kind: metric
command: run.sh
direction: max
---
`,
			wantErr: "timeout is required",
		},
		{
			name: "zero timeout refused",
			spec: `---
kind: metric
command: run.sh
direction: max
timeout: 0s
---
`,
			wantErr: "timeout",
		},
		{
			name: "negative timeout refused",
			spec: `---
kind: metric
command: run.sh
direction: max
timeout: -5s
---
`,
			wantErr: "timeout",
		},
		{
			name: "unbounded timeout above 24h refused",
			spec: `---
kind: metric
command: run.sh
direction: max
timeout: 48h
---
`,
			wantErr: "timeout",
		},
		{
			name: "contradictory visibility and hidden refused",
			spec: `---
kind: metric
command: run.sh
direction: max
timeout: 1m
hidden: true
visibility: visible
---
`,
			wantErr: "conflicts",
		},
		{
			name: "empty data entry refused",
			spec: `---
kind: metric
command: run.sh
direction: max
timeout: 1m
data: [""]
---
`,
			wantErr: "data[0]",
		},
		{
			name: "duplicate data entry refused",
			spec: `---
kind: metric
command: run.sh
direction: max
timeout: 1m
data: ["queries.jsonl", "queries.jsonl"]
---
`,
			wantErr: "duplicate",
		},
		{
			name: "parent traversal in protect refused",
			spec: `---
kind: metric
command: run.sh
direction: max
timeout: 1m
protect: ["../outside/**"]
---
`,
			wantErr: "protect[0]",
		},
		{
			name: "malformed glob in protect refused",
			spec: `---
kind: metric
command: run.sh
direction: max
timeout: 1m
protect: ["scripts/[unclosed"]
---
`,
			wantErr: "protect[0]",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := score.ParseSpec(tc.spec)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil (%+v)", tc.wantErr, got)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tc.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.check != nil {
				tc.check(t, got)
			}
		})
	}
}

func TestPinSetVerifyAndTamperRules(t *testing.T) {
	isolateHome(t)

	setupScorer := func(t *testing.T) (storePath, specPath, cmdPath, dataPath string) {
		t.Helper()
		root := t.TempDir()
		scorerDir := filepath.Join(root, "recall-metric")
		if err := os.MkdirAll(scorerDir, 0700); err != nil {
			t.Fatal(err)
		}
		cmdPath = writeTestScript(t, scorerDir, "score", `echo '{"value": 0.75}'`, `echo {"value": 0.75}`)
		dataPath = filepath.Join(scorerDir, "queries.jsonl")
		if err := os.WriteFile(dataPath, []byte("{\"id\":\"q1\"}\n"), 0600); err != nil {
			t.Fatal(err)
		}
		specPath = filepath.Join(scorerDir, "scorer.md")
		specText := "---\nkind: metric\nsubject: commit\ncommand: " + filepath.Base(cmdPath) + "\ndirection: max\nscale: 100\ntimeout: 5s\ndata:\n  - queries.jsonl\n---\n"
		if err := os.WriteFile(specPath, []byte(specText), 0600); err != nil {
			t.Fatal(err)
		}
		storePath = filepath.Join(t.TempDir(), "pins.toml")
		return storePath, specPath, cmdPath, dataPath
	}

	t.Run("unpinned scorer refused when store missing or entry absent", func(t *testing.T) {
		storePath, specPath, _, _ := setupScorer(t)
		_, _, err := score.Verify(storePath, specPath)
		if !errors.Is(err, score.ErrUnpinned) {
			t.Fatalf("expected ErrUnpinned, got %v", err)
		}
	})

	t.Run("matching pin set accepted and re-pin updates cleanly", func(t *testing.T) {
		storePath, specPath, _, dataPath := setupScorer(t)
		ps1, err := score.Pin(storePath, specPath)
		if err != nil {
			t.Fatalf("Pin: %v", err)
		}
		if ps1.Scorer != "recall-metric" || !strings.HasPrefix(ps1.Pin, "sha256:") || len(ps1.Data) != 1 {
			t.Fatalf("unexpected PinSet: %+v", ps1)
		}
		spec, psVerified, err := score.Verify(storePath, specPath)
		if err != nil {
			t.Fatalf("Verify matching pin: %v", err)
		}
		if spec.Name != "recall-metric" || psVerified.Pin != ps1.Pin {
			t.Fatalf("mismatch after Verify: %+v vs %+v", spec, psVerified)
		}

		// Authorized host update followed by re-pin succeeds with a new digest.
		if err := os.WriteFile(dataPath, []byte("{\"id\":\"q1\"}\n{\"id\":\"q2\"}\n"), 0600); err != nil {
			t.Fatal(err)
		}
		ps2, err := score.Pin(storePath, specPath)
		if err != nil {
			t.Fatalf("re-Pin: %v", err)
		}
		if ps2.Pin == ps1.Pin {
			t.Fatalf("expected composite pin digest to change after data edit")
		}
		if _, _, err := score.Verify(storePath, specPath); err != nil {
			t.Fatalf("Verify after re-Pin: %v", err)
		}
	})

	tamperCases := []struct {
		name     string
		tamper   func(t *testing.T, storePath, specPath, cmdPath, dataPath string)
		wantPart string
	}{
		{
			name: "tamper spec file refused as stale",
			tamper: func(t *testing.T, _, specPath, _, _ string) {
				b, err := os.ReadFile(specPath)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(specPath, append(b, []byte("\n# edited\n")...), 0600); err != nil {
					t.Fatal(err)
				}
			},
			wantPart: "spec",
		},
		{
			name: "tamper command script refused as stale",
			tamper: func(t *testing.T, _, _, cmdPath, _ string) {
				b, err := os.ReadFile(cmdPath)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(cmdPath, []byte(strings.ReplaceAll(string(b), "0.75", "1.00")), 0700); err != nil {
					t.Fatal(err)
				}
			},
			wantPart: "command",
		},
		{
			name: "tamper data file refused as stale",
			tamper: func(t *testing.T, _, _, _, dataPath string) {
				if err := os.WriteFile(dataPath, []byte("{\"id\":\"tampered\"}\n"), 0600); err != nil {
					t.Fatal(err)
				}
			},
			wantPart: "data[0]",
		},
		{
			name: "deleted data file refused as stale",
			tamper: func(t *testing.T, _, _, _, dataPath string) {
				if err := os.Remove(dataPath); err != nil {
					t.Fatal(err)
				}
			},
			wantPart: "missing file",
		},
		{
			name: "corrupted composite pin in store refused as stale",
			tamper: func(t *testing.T, storePath, _, _, _ string) {
				b, err := os.ReadFile(storePath)
				if err != nil {
					t.Fatal(err)
				}
				// Replace composite sha256:<hex> while leaving member hashes intact.
				corrupted := strings.Replace(string(b), "pin = \"sha256:", "pin = \"sha256:0000", 1)
				if err := os.WriteFile(storePath, []byte(corrupted), 0600); err != nil {
					t.Fatal(err)
				}
			},
			wantPart: "manifest digest mismatch",
		},
	}

	for _, tc := range tamperCases {
		t.Run(tc.name, func(t *testing.T) {
			storePath, specPath, cmdPath, dataPath := setupScorer(t)
			if _, err := score.Pin(storePath, specPath); err != nil {
				t.Fatal(err)
			}
			tc.tamper(t, storePath, specPath, cmdPath, dataPath)
			_, _, err := score.Verify(storePath, specPath)
			if !errors.Is(err, score.ErrStalePin) {
				t.Fatalf("expected ErrStalePin, got %v", err)
			}
			if !strings.Contains(err.Error(), tc.wantPart) {
				t.Fatalf("expected error mentioning %q, got %q", tc.wantPart, err.Error())
			}
		})
	}

	if runtime.GOOS != "windows" {
		t.Run("replacing data file with symlink refused", func(t *testing.T) {
			storePath, specPath, _, dataPath := setupScorer(t)
			if _, err := score.Pin(storePath, specPath); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(t.TempDir(), "elsewhere.jsonl")
			if err := os.WriteFile(target, []byte("{\"id\":\"q1\"}\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(dataPath); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, dataPath); err != nil {
				t.Fatal(err)
			}
			_, _, err := score.Verify(storePath, specPath)
			if !errors.Is(err, score.ErrStalePin) {
				t.Fatalf("expected ErrStalePin on symlink replacement, got %v", err)
			}
		})
	}
}

func TestProtectedPathsAndEndToEndScoring(t *testing.T) {
	isolateHome(t)

	repo, baseSHA := initGitRepo(t, map[string]string{
		"pkg/search/rank.go":                  "package search\nfunc Score() int { return 1 }\n",
		"pkg/search/rank_test.go":             "package search\n",
		"pkg/search/testdata/queries.jsonl":   "{\"id\":\"q1\",\"want\":\"s1\"}\n",
		"pkg/search/testdata/sub/extra.jsonl": "{\"id\":\"q2\"}\n",
		"scripts/score-recall.py":             "# protected scorer helper\n",
		"scripts/test-recall.py":              "# ordinary test script\n",
		"metric.txt":                          "0.851\n",
	})

	// Create distinct commits from baseSHA for table-driven check of refused vs kept diffs.
	makeCommit := func(t *testing.T, fn func(repo string)) string {
		t.Helper()
		gitRun(t, repo, "checkout", "-q", "-B", "work", baseSHA)
		fn(repo)
		gitRun(t, repo, "add", "-A")
		gitRun(t, repo, "commit", "-q", "-m", "candidate")
		return strings.TrimSpace(gitRun(t, repo, "rev-parse", "HEAD"))
	}

	goodImplSHA := makeCommit(t, func(r string) {
		_ = os.WriteFile(filepath.Join(r, "pkg/search/rank.go"), []byte("package search\nfunc Score() int { return 2 }\n"), 0600)
		_ = os.WriteFile(filepath.Join(r, "metric.txt"), []byte("0.912\n"), 0600)
	})
	goodNeighbourTestSHA := makeCommit(t, func(r string) {
		_ = os.WriteFile(filepath.Join(r, "pkg/search/rank_test.go"), []byte("package search\n// added unit test\n"), 0600)
		_ = os.WriteFile(filepath.Join(r, "scripts/test-recall.py"), []byte("# updated non-protected test\n"), 0600)
		_ = os.WriteFile(filepath.Join(r, "metric.txt"), []byte("0.875\n"), 0600)
	})
	badDirectEditSHA := makeCommit(t, func(r string) {
		_ = os.WriteFile(filepath.Join(r, "pkg/search/testdata/queries.jsonl"), []byte("{\"id\":\"easy\"}\n"), 0600)
	})
	badNestedGlobSHA := makeCommit(t, func(r string) {
		_ = os.WriteFile(filepath.Join(r, "pkg/search/testdata/sub/extra.jsonl"), []byte("{\"id\":\"easy2\"}\n"), 0600)
	})
	badRenameOutSHA := makeCommit(t, func(r string) {
		gitRun(t, r, "mv", "pkg/search/testdata/queries.jsonl", "pkg/search/moved_queries.jsonl")
	})
	badRenameIntoProtectedSHA := makeCommit(t, func(r string) {
		gitRun(t, r, "mv", "scripts/test-recall.py", "scripts/score-hijack.py")
	})

	// Leave repo HEAD at baseSHA so scoring goodImplSHA proves RunCommit checks out the candidate SHA, not repo HEAD.
	gitRun(t, repo, "checkout", "-q", baseSHA)

	scorerDir := filepath.Join(t.TempDir(), "trace-recall")
	if err := os.MkdirAll(scorerDir, 0700); err != nil {
		t.Fatal(err)
	}
	markerDir := t.TempDir()
	invokedMarker := filepath.Join(markerDir, "invoked.count")

	cmdPath := writeTestScript(t, scorerDir, "run",
		`echo x >> "`+invokedMarker+`"
if [ ! -f "$HEV_SCORE_DATA_0" ]; then
  echo "missing pinned data" >&2
  exit 1
fi
val=$(cat metric.txt | tr -d ' \r\n')
printf '{"value": %s, "n": 400, "detail": {"split": "objective"}}\n' "$val"`,
		`echo x >> "`+invokedMarker+`"
set /p val=<metric.txt
echo {"value": %val%, "n": 400, "detail": {"split": "objective"}}`,
	)

	dataFile := filepath.Join(scorerDir, "queries.jsonl")
	if err := os.WriteFile(dataFile, []byte("{\"q\":1}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	specPath := filepath.Join(scorerDir, "scorer.md")
	specContent := `---
name: trace-recall
description: Objective recall scorer
kind: metric
subject: commit
command: ` + filepath.Base(cmdPath) + `
direction: max
unit: ratio
scale: 1000
timeout: 10s
data:
  - queries.jsonl
protect:
  - pkg/search/testdata/**
  - scripts/score-*.py
---
`
	if err := os.WriteFile(specPath, []byte(specContent), 0600); err != nil {
		t.Fatal(err)
	}
	pinStore := filepath.Join(t.TempDir(), "pins.toml")
	if _, err := score.Pin(pinStore, specPath); err != nil {
		t.Fatalf("Pin: %v", err)
	}

	cases := []struct {
		name            string
		commit          string
		wantStatus      string
		wantMark        int
		wantProtected   []string
		wantCmdExecuted bool
	}{
		{
			name:            "ordinary implementation edit kept and scored",
			commit:          goodImplSHA,
			wantStatus:      score.StatusScored,
			wantMark:        912,
			wantCmdExecuted: true,
		},
		{
			name:            "ordinary neighbouring test edit kept and scored",
			commit:          goodNeighbourTestSHA,
			wantStatus:      score.StatusScored,
			wantMark:        875,
			wantCmdExecuted: true,
		},
		{
			name:            "direct edit to protected file disqualified without invoking command",
			commit:          badDirectEditSHA,
			wantStatus:      score.StatusDisqualified,
			wantProtected:   []string{"pkg/search/testdata/queries.jsonl"},
			wantCmdExecuted: false,
		},
		{
			name:            "nested glob edit under protected tree disqualified without invoking command",
			commit:          badNestedGlobSHA,
			wantStatus:      score.StatusDisqualified,
			wantProtected:   []string{"pkg/search/testdata/sub/extra.jsonl"},
			wantCmdExecuted: false,
		},
		{
			name:            "rename from protected path to unprotected path disqualified on old path",
			commit:          badRenameOutSHA,
			wantStatus:      score.StatusDisqualified,
			wantProtected:   []string{"pkg/search/testdata/queries.jsonl"},
			wantCmdExecuted: false,
		},
		{
			name:            "rename from unprotected path into protected glob disqualified on new path",
			commit:          badRenameIntoProtectedSHA,
			wantStatus:      score.StatusDisqualified,
			wantProtected:   []string{"scripts/score-hijack.py"},
			wantCmdExecuted: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_ = os.Remove(invokedMarker)
			res, err := score.RunCommit(context.Background(), score.RunOptions{
				ScorerPath:   specPath,
				PinStorePath: pinStore,
				RepoPath:     repo,
				Commit:       tc.commit,
				Parent:       baseSHA,
			})
			if err != nil {
				t.Fatalf("RunCommit unexpected error: %v", err)
			}
			_, markerErr := os.Stat(invokedMarker)
			invoked := markerErr == nil
			if invoked != tc.wantCmdExecuted {
				t.Fatalf("command executed = %v, want %v", invoked, tc.wantCmdExecuted)
			}
			if res.Status != tc.wantStatus {
				t.Fatalf("status = %q, want %q (%+v)", res.Status, tc.wantStatus, res)
			}
			if tc.wantStatus == score.StatusDisqualified {
				if res.Value != nil || res.Mark != nil || res.Marks != nil {
					t.Fatalf("disqualified result must never carry a numeric score: %+v", res)
				}
				if strings.Join(res.ProtectedPaths, ",") != strings.Join(tc.wantProtected, ",") {
					t.Fatalf("protected_paths = %v, want %v", res.ProtectedPaths, tc.wantProtected)
				}
				rawJSON, _ := json.Marshal(res)
				if strings.Contains(string(rawJSON), `"value"`) || strings.Contains(string(rawJSON), `"mark"`) {
					t.Fatalf("disqualified JSON leaked numeric keys: %s", rawJSON)
				}
				if _, evalErr := res.Eval("kit", "", "mini", time.Now()); evalErr == nil {
					t.Fatalf("expected Eval() on disqualified result to fail")
				}
				return
			}

			if res.Mark == nil || *res.Mark != tc.wantMark || res.Marks["trace_recall"] != tc.wantMark {
				t.Fatalf("mark = %v (marks=%v), want %d", res.Mark, res.Marks, tc.wantMark)
			}
			ev, err := res.Eval("kit", "", "mini", time.Date(2026, 11, 20, 20, 0, 0, 0, time.UTC))
			if err != nil {
				t.Fatalf("Eval: %v", err)
			}
			if ev.Session != "commit:kit@"+tc.commit || ev.Role != "scorer" || ev.Instance != "trace-recall" {
				t.Fatalf("unexpected eval row metadata: %+v", ev)
			}
			rows, err := layer.EvalRows([]trace.Eval{ev})
			if err != nil || len(rows) != 1 {
				t.Fatalf("layer.EvalRows compatibility check failed: %v (%v)", err, rows)
			}
		})
	}
}
