package index

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/trace"
)

func TestGitWindowAndBranch(t *testing.T) {
	dir := t.TempDir()
	stamp := "2026-01-01T11:00:00Z"
	run := func(args ...string) string {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = dir
		c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Example", "GIT_AUTHOR_EMAIL=example@example.com", "GIT_COMMITTER_NAME=Example", "GIT_COMMITTER_EMAIL=example@example.com", "GIT_AUTHOR_DATE="+stamp, "GIT_COMMITTER_DATE="+stamp)
		out, e := c.CombinedOutput()
		if e != nil {
			t.Fatalf("git %v: %s %v", args, out, e)
		}
		return strings.TrimSpace(string(out))
	}
	run("init", "-b", "main")
	run("commit", "--allow-empty", "-m", "base")
	stamp = "2026-01-01T12:00:00Z"
	run("checkout", "-b", "session")
	run("commit", "--allow-empty", "-m", "owned")
	owned := run("rev-parse", "HEAD")
	run("checkout", "-b", "other", "main")
	run("commit", "--allow-empty", "-m", "other")
	ms := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC).UnixMilli()
	row := trace.SessionRow{Workdir: dir, Branch: "session", Start: ms, End: ms + 1000}
	// Older ancestry and another branch's same-time commit are excluded,
	// even though that other branch is the current HEAD.
	NewGitEnricher().Enrich(context.Background(), &row, nil)
	if len(row.Commits) != 1 || !strings.Contains(strings.Join(row.Commits, " "), owned) {
		t.Fatal(row.Commits)
	}
	row.Branch = "deleted"
	row.Commits = nil
	NewGitEnricher().Enrich(context.Background(), &row, nil)
	if len(row.Commits) != 0 {
		t.Fatal(row.Commits)
	}
	row.Branch = "session"
	row.Start = ms + 2000
	row.End = ms + 3000
	NewGitEnricher().Enrich(context.Background(), &row, nil)
	if len(row.Commits) != 0 {
		t.Fatal(row.Commits)
	}
}

func TestTranscriptEvidenceAndPR(t *testing.T) {
	sha := strings.Repeat("a", 40)
	for _, tc := range []struct {
		name    string
		output  string
		failed  bool
		resolve bool
		want    bool
	}{
		{"abbreviation", "[topic aaaaaaa] change", false, true, true},
		{"full missing repo", "[topic " + sha + "] change", false, false, true},
		{"JSON runner", fmt.Sprintf(`{"exit_code":0,"output":"[topic %s] change\n"}`, sha), false, false, true},
		{"ambiguous", "[topic aaaaaaa] change", false, false, false},
		{"failed tool", "[topic " + sha + "] change", true, false, false},
		{"failed shell", "[topic " + sha + "] change\nProcess exited with code 1", false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row := trace.SessionRow{SessionID: "s", Workdir: "/nonexistent", RepoURL: "https://github.com/example/project", Branch: "topic"}
			turns := []trace.Turn{{SessionID: "s", Blocks: []trace.Block{{Type: "tool_use", ToolUseID: "c", Text: `git commit -m change`}, {Type: "tool_result", ToolUseID: "c", Text: tc.output, IsError: tc.failed}, {Type: "tool_result", ToolUseID: "unmatched", Text: "[topic " + strings.Repeat("b", 40) + "] fabricated"}}}}
			g := &GitEnricher{Run: func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
				if _, ok := ctx.Deadline(); !ok {
					t.Fatal("unbounded call")
				}
				if name == "git" && tc.resolve {
					return []byte(sha + "\n"), nil
				}
				if name == "gh" {
					if dir != "" {
						t.Fatal("gh depends on missing workdir")
					}
					if !strings.Contains(strings.Join(args, " "), "--state all") {
						t.Fatal(args)
					}
					return []byte(fmt.Sprintf(`[{"number":42,"commits":[{"oid":%q}]}]`, sha)), nil
				}
				return nil, errors.New("missing or ambiguous")
			}}
			g.Enrich(context.Background(), &row, turns)
			if tc.want {
				if !reflect.DeepEqual(row.Commits, trace.StringList{sha}) || row.PR != "42" {
					t.Fatalf("%+v", row)
				}
			} else if len(row.Commits) != 0 || row.PR != "" {
				t.Fatalf("invented %+v", row)
			}
		})
	}
}

func TestPRReuseAndUnavailable(t *testing.T) {
	sha := strings.Repeat("a", 40)
	for _, response := range []string{`[]`, `invalid`, fmt.Sprintf(`[{"number":1,"commits":[{"oid":%q}]},{"number":2,"commits":[{"oid":%q}]}]`, sha, sha)} {
		row := trace.SessionRow{RepoURL: "example/project", Branch: "topic", Commits: trace.StringList{sha}}
		g := &GitEnricher{Run: func(context.Context, string, string, ...string) ([]byte, error) { return []byte(response), nil }}
		g.Enrich(context.Background(), &row, nil)
		if row.PR != "" || len(row.Commits) != 1 {
			t.Fatal(row)
		}
	}
	row := trace.SessionRow{RepoURL: "example/project", Branch: "topic", PR: "42", Commits: trace.StringList{sha}}
	g := &GitEnricher{Run: func(context.Context, string, string, ...string) ([]byte, error) {
		t.Fatal("existing PR should not require gh")
		return nil, nil
	}}
	g.Enrich(context.Background(), &row, nil)
}

func TestGitUnavailableAndDeadline(t *testing.T) {
	for _, failure := range []string{"no gh", "no auth", "network unavailable"} {
		row := trace.SessionRow{RepoURL: "example/project", Branch: "topic"}
		g := &GitEnricher{Run: func(context.Context, string, string, ...string) ([]byte, error) { return nil, errors.New(failure) }}
		g.Enrich(context.Background(), &row, nil)
		if len(row.Commits) != 0 || row.PR != "" {
			t.Fatal(row)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	row := trace.SessionRow{RepoURL: "example/project", Branch: "topic"}
	g := &GitEnricher{Run: func(ctx context.Context, _ string, _ string, _ ...string) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	start := time.Now()
	g.Enrich(ctx, &row, nil)
	if time.Since(start) > time.Second {
		t.Fatal("did not honor caller deadline")
	}
}

func TestGitOriginFormats(t *testing.T) {
	for _, remote := range []string{"git@github.com:example/project.git", "https://github.com/example/project.git", "ssh://git@github.com/example/project.git", "https://user:secret@github.com/example/project.git"} {
		if got := githubRepo(remote); got != "github.com/example/project" {
			t.Fatalf("%q", got)
		}
	}
}

func TestPRTruncatedHistoryIsUnknown(t *testing.T) {
	sha := strings.Repeat("a", 40)
	rows := make([]string, 20)
	for i := range rows {
		rows[i] = fmt.Sprintf(`{"number":%d,"commits":[{"oid":%q}]}`, i+1, strings.Repeat("b", 40))
	}
	rows[0] = fmt.Sprintf(`{"number":42,"commits":[{"oid":%q}]}`, sha)
	row := trace.SessionRow{RepoURL: "example/project", Branch: "topic", Commits: trace.StringList{sha}}
	g := &GitEnricher{Run: func(context.Context, string, string, ...string) ([]byte, error) {
		return []byte("[" + strings.Join(rows, ",") + "]"), nil
	}}
	g.Enrich(context.Background(), &row, nil)
	if row.PR != "" {
		t.Fatal("guessed from truncated history", row.PR)
	}
}
