package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/reviewnotes"
)

const (
	testHead = "1111111111111111111111111111111111111111"
	testBase = "2222222222222222222222222222222222222222"
)

type fakeReview struct {
	files    map[string]map[string]string // run -> files
	state    map[string]string
	hunk     bool
	steer    bool
	ran      [][]string
	stdin    []string
	requests []dispatch.Request
	session  string
	comments string
}

func (f *fakeReview) deps(t *testing.T) reviewDeps {
	cache := t.TempDir()
	row := func(name, base, branch string) dispatch.Row {
		r := dispatch.Row{Kind: "run", State: f.state[name]}
		r.Name, r.Base, r.Branch, r.Repo = name, base, branch, "kit"
		return r
	}
	return reviewDeps{
		load: func() (dispatch.Config, error) {
			return dispatch.Config{Hosts: map[string]dispatch.Host{"oven": {SSH: "oven"}, "mini": {SSH: "mini"}}}, nil
		},
		inventory: func(dispatch.Config, string) map[string]dispatch.Response {
			return map[string]dispatch.Response{
				"oven": {Rows: []dispatch.Row{
					row("feat", "v0.7", "d/feat"),
					row("feat-review", "d/feat", "d/feat-review"),
					row("other", "v0.7", "d/other"),
				}},
				"mini": {Rows: []dispatch.Row{
					row("kit-review-feat", "d/feat", "d/kit-review-feat"),
					row("followup-feat", "d/feat", "d/followup-feat"),
				}},
			}
		},
		rpc: func(_ context.Context, h dispatch.Host, q dispatch.Request) (dispatch.Response, error) {
			f.requests = append(f.requests, q)
			switch q.Action {
			case "review-fetch":
				return dispatch.Response{Review: &dispatch.ReviewFetch{Branch: "d/" + q.Name, Base: "v0.7", Head: testHead, MergeBase: testBase}}, nil
			case "show":
				return dispatch.Response{Files: f.files[q.Name]}, nil
			}
			return dispatch.Response{}, errors.New("unexpected " + q.Action)
		},
		lookPath: func(string) (string, error) {
			if f.hunk {
				return "/x/hunk", nil
			}
			return "", errors.New("not found")
		},
		run: func(dir string, stdin io.Reader, _, _ io.Writer, name string, args ...string) error {
			f.ran = append(f.ran, append([]string{dir, name}, args...))
			b, _ := io.ReadAll(stdin)
			f.stdin = append(f.stdin, string(b))
			return nil
		},
		output: func(dir, name string, args ...string) ([]byte, error) {
			cmd := strings.Join(append([]string{name}, args...), " ")
			switch {
			case strings.HasPrefix(cmd, "git rev-parse --show-toplevel"):
				return []byte(dir + "\n"), nil
			case strings.HasPrefix(cmd, "git cat-file"), strings.HasPrefix(cmd, "git update-ref"):
				return nil, nil
			case strings.HasPrefix(cmd, "git diff --stat"):
				return []byte(" a.go | 2 +-\n"), nil
			case cmd == "hunk session list --json":
				return []byte(f.session), nil
			case strings.HasPrefix(cmd, "hunk session comment list"):
				f.ran = append(f.ran, append([]string{dir, name}, args...))
				return []byte(f.comments), nil
			}
			return nil, errors.New("unexpected command " + cmd)
		},
		cacheDir: func() (string, error) { return cache, nil },
		getwd:    func() (string, error) { return "/work/kit", nil },
		hasSteer: func() bool { return f.steer },
		self:     func() (string, error) { return "/bin/hev", nil },
	}
}

func notesJSON(t *testing.T, n reviewnotes.Notes) string {
	b, err := reviewnotes.Marshal(n)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func newFakeReview(t *testing.T) *fakeReview {
	build := reviewnotes.Notes{Summary: "Adds feat.", Head: testHead, Files: []reviewnotes.File{{Path: "a.go", Summary: "feat", Annotations: []reviewnotes.Annotation{{Summary: "why", NewRange: reviewnotes.Range{1, 2}, Author: "feat opus", Source: "feat"}}}}}
	review := reviewnotes.Notes{Files: []reviewnotes.File{{Path: "a.go", Annotations: []reviewnotes.Annotation{{Summary: "bug", NewRange: reviewnotes.Range{2, 2}, Tags: []string{"finding", "fail"}, Confidence: "high", Author: "feat-review sonnet", Source: "feat-review"}}}}}
	stale := reviewnotes.Notes{Head: "3333333", Files: []reviewnotes.File{{Path: "a.go", Annotations: []reviewnotes.Annotation{{Summary: "stale finding"}}}}}
	return &fakeReview{
		state: map[string]string{"feat": "running"},
		files: map[string]map[string]string{
			"feat":            {"report.md": "PR #1 done", reviewnotes.FileName: notesJSON(t, build)},
			"feat-review":     {"report.md": "VERDICT: FAIL head=" + testHead[:12] + "\nfindings", reviewnotes.FileName: notesJSON(t, review)},
			"kit-review-feat": {"report.md": "VERDICT: PASS head=3333333", reviewnotes.FileName: notesJSON(t, stale)},
		},
	}
}

func execReview(t *testing.T, f *fakeReview, args ...string) (string, string, error) {
	d := f.deps(t)
	c := newReviewCommand(&d)
	var out, errout bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&errout)
	c.SetIn(strings.NewReader(""))
	c.SetArgs(args)
	err := c.Execute()
	return out.String(), errout.String(), err
}

func TestReviewOpensHunkWithMergedNotes(t *testing.T) {
	f := newFakeReview(t)
	f.hunk = true
	_, errout, err := execReview(t, f, "feat", "--web")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.ran) != 1 {
		t.Fatalf("ran %v", f.ran)
	}
	run := f.ran[0]
	notesPath := run[5]
	want := []string{"/work/kit", "hunk", "diff", testBase + "..." + testHead, "--agent-context", notesPath, "--agent-notes", "--web"}
	if !reflect.DeepEqual(run, want) || filepath.Base(notesPath) != "merged.json" {
		t.Fatalf("hunk run %v, want %v", run, want)
	}
	merged, err := reviewnotes.Load(notesPath)
	if err != nil {
		t.Fatal(err)
	}
	a := merged.Files[0].Annotations
	if merged.Head != testHead || len(a) != 2 || a[0].Summary != "why" || a[1].Summary != "bug" || a[1].Author != "feat-review sonnet" {
		t.Fatalf("merged: %+v", merged)
	}
	for _, want := range []string{"dropped review notes from kit-review-feat", "reviews: feat-review, kit-review-feat", "feat-review: VERDICT: FAIL"} {
		if !strings.Contains(errout, want) {
			t.Errorf("stderr lacks %q:\n%s", want, errout)
		}
	}
	if strings.Contains(errout, "followup-feat") {
		t.Error("a non-review run on the branch was treated as a review")
	}
	// The local checkout already had both commits: no bundle was requested.
	for _, q := range f.requests {
		if q.Bundle {
			t.Fatalf("unneeded bundle request: %+v", q)
		}
	}
}

func TestReviewWithoutHunkPrintsVerdictStatAndHint(t *testing.T) {
	f := newFakeReview(t)
	out, _, err := execReview(t, f, "feat", "--with", "feat-review")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.ran) != 0 {
		t.Fatalf("ran %v without hunk", f.ran)
	}
	for _, want := range []string{"feat-review: VERDICT: FAIL head=" + testHead[:12], "a.go | 2 +-", "curl -fsSL https://hunk.dev/install.sh | sh"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "kit-review-feat") {
		t.Error("--with did not replace discovery")
	}
}

func TestReviewPrintNotes(t *testing.T) {
	f := newFakeReview(t)
	out, _, err := execReview(t, f, "feat", "--print-notes")
	if err != nil {
		t.Fatal(err)
	}
	n, err := reviewnotes.Parse([]byte(out))
	if err != nil || n.Summary != "Adds feat." || len(n.Files[0].Annotations) != 2 {
		t.Fatalf("printed notes %v:\n%s", err, out)
	}
	if len(f.ran) != 0 {
		t.Fatal("print-notes launched hunk")
	}
}

func TestReviewUnknownRun(t *testing.T) {
	f := newFakeReview(t)
	if _, _, err := execReview(t, f, "nope"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatal(err)
	}
	if _, _, err := execReview(t, f, "feat", "--host", "nowhere"); err == nil || !strings.Contains(err.Error(), "unknown host") {
		t.Fatal(err)
	}
}

const testSessions = `{"sessions":[{"sessionId":"s-other","repoRoot":"/x","title":"x","review":{"head":"9999999999"}},{"sessionId":"s-feat","repoRoot":"/work/kit","title":"kit 2222...1111","review":{"head":"` + testHead + `"}}]}`

const testComments = `{"comments":[
 {"noteId":"b1","source":"ai","filePath":"a.go","newRange":[2,2],"body":"bug","author":"feat-review sonnet"},
 {"noteId":"mcp:1","source":"agent","filePath":"a.go","newRange":[5,5],"body":"from a shell"},
 {"noteId":"user:2","parentId":"b1","source":"user","filePath":"a.go","newRange":[2,2],"body":"agree, fix it","author":"user"},
 {"noteId":"user:1","source":"user","filePath":"a.go","newRange":[1,1],"body":"rename this","author":"user"}
]}`

func TestReviewSendDryRun(t *testing.T) {
	f := newFakeReview(t)
	f.hunk, f.session, f.comments = true, testSessions, testComments
	out, _, err := execReview(t, f, "send", "feat", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Review notes for feat at 111111111111 from hunk (2):",
		"1. a.go:1: rename this",
		`2. a.go:2 (reply to feat-review sonnet note "bug"): agree, fix it`,
		"dry run: would send with hev dispatch say",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "from a shell") {
		t.Error("live agent comment sent without --include-live")
	}
	if len(f.ran) != 1 || f.ran[0][5] != "s-feat" {
		t.Fatalf("comment list did not target the run's session: %v", f.ran)
	}
}

func TestReviewSendDelivers(t *testing.T) {
	for _, c := range []struct {
		steer bool
		state string
		want  []string
	}{
		{false, "running", []string{"", "/bin/hev", "dispatch", "say", "feat", "-", "--host", "oven"}},
		{true, "running", []string{"", "/bin/hev", "steer", "feat", "-", "--level", "urgent", "--host", "oven"}},
		{true, "done", []string{"", "/bin/hev", "dispatch", "say", "feat", "-", "--host", "oven"}},
	} {
		f := newFakeReview(t)
		f.hunk, f.session, f.comments, f.steer = true, testSessions, testComments, c.steer
		f.state["feat"] = c.state
		out, _, err := execReview(t, f, "send", "feat", "--include-live")
		if err != nil {
			t.Fatal(err)
		}
		last := f.ran[len(f.ran)-1]
		if !reflect.DeepEqual(last, c.want) {
			t.Fatalf("steer=%v state=%s: ran %v, want %v", c.steer, c.state, last, c.want)
		}
		msg := f.stdin[len(f.stdin)-1]
		if !strings.Contains(msg, "a.go:5: from a shell") || !strings.Contains(msg, "a.go:1: rename this") || !strings.Contains(out, "Sending with") {
			t.Fatalf("message:\n%s", msg)
		}
	}
}

func TestReviewSendNothingToSend(t *testing.T) {
	f := newFakeReview(t)
	f.hunk, f.session, f.comments = true, testSessions, `{"comments":[]}`
	out, _, err := execReview(t, f, "send", "feat")
	if err != nil || !strings.Contains(out, "nothing to send") || len(f.ran) != 1 {
		t.Fatalf("%v %q %v", err, out, f.ran)
	}
}

func TestPickHunkSession(t *testing.T) {
	if sel, err := pickHunkSession([]byte(testSessions), testHead[:9], "", ""); err != nil || !reflect.DeepEqual(sel, []string{"s-feat"}) {
		t.Fatal(sel, err)
	}
	if sel, _ := pickHunkSession(nil, testHead, "explicit", ""); !reflect.DeepEqual(sel, []string{"explicit"}) {
		t.Fatal(sel)
	}
	if sel, _ := pickHunkSession([]byte(testSessions), "abcdef1234", "", "/r"); !reflect.DeepEqual(sel, []string{"--repo", "/r"}) {
		t.Fatal(sel)
	}
	if _, err := pickHunkSession([]byte(`{"sessions":[]}`), testHead, "", ""); err == nil || !strings.Contains(err.Error(), "no live hunk session") {
		t.Fatal(err)
	}
	dup := `{"sessions":[{"sessionId":"a","review":{"head":"` + testHead + `"}},{"sessionId":"b","review":{"head":"` + testHead + `"}}]}`
	if _, err := pickHunkSession([]byte(dup), testHead, "", ""); err == nil || !strings.Contains(err.Error(), "--session") {
		t.Fatal(err)
	}
}

func TestReviewCommandRegistered(t *testing.T) {
	c, _, err := rootCmd.Find([]string{"review", "send"})
	if err != nil || c.Name() != "send" {
		t.Fatal(c, err)
	}
}
