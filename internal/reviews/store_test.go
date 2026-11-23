package reviews

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var testHead = strings.Repeat("a", 40)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestReviewValidation(t *testing.T) {
	s := testStore(t)
	for _, verdict := range []string{"pass", "rework", "comment", "", "accept", "PASS"} {
		_, err := s.Create(Review{Repo: "kit", HeadSHA: testHead, Verdict: verdict})
		if accepted := verdict == "" || validVerdict(verdict); accepted != (err == nil) {
			t.Errorf("verdict %q error %v", verdict, err)
		}
	}
	r, err := s.Create(Review{Repo: "kit", HeadSHA: testHead})
	if err != nil {
		t.Fatal(err)
	}
	if !validID("rev_", r.ID) {
		t.Fatalf("invalid generated ID %q", r.ID)
	}
	if _, err := s.Get(r.ID); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"../x", "/x", "rev_IIIIIIIIIIIIIIIIIIIIIIIIII", "rev_zzzzzzzzzzzzzzzzzzzzzzzzzz", strings.ToUpper(r.ID), "rev_short"} {
		if _, err := s.Get(id); err == nil {
			t.Errorf("accepted invalid ID %q", id)
		}
	}
	for _, file := range []string{"pkg/core.go", "space in.go", ".gitignore", "../core.go", "/tmp/core.go", "pkg/../core.go", "pkg/./core.go", "pkg//core.go", "C:/core.go", `pkg\core.go`, ""} {
		_, err := s.Add(r.ID, Finding{File: file, Body: "finding", Severity: "minor"})
		want := file == "pkg/core.go" || file == "space in.go" || file == ".gitignore"
		if (err == nil) != want {
			t.Errorf("file %q error %v", file, err)
		}
	}
	for _, severity := range []string{"blocker", "major", "minor", "nit", "critical", ""} {
		_, err := s.Add(r.ID, Finding{File: "core.go", Body: "finding", Severity: severity})
		if want := severity != "critical" && severity != ""; (err == nil) != want {
			t.Errorf("severity %q error %v", severity, err)
		}
	}
	for _, status := range []string{"open", "fixed", "waived", "", "closed"} {
		f, err := s.Add(r.ID, Finding{File: "core.go", Body: "finding", Severity: "minor", Status: status})
		if status == "closed" {
			if err == nil {
				t.Errorf("accepted status %q", status)
			}
			continue
		}
		if err != nil || !validID("fnd_", f.ID) {
			t.Fatalf("status %q finding %+v error %v", status, f, err)
		}
		for _, resolution := range []string{"fixed", "waived", "open", "closed"} {
			err := s.Resolve(r.ID, f.ID, resolution, "checked")
			if want := resolution == "fixed" || resolution == "waived"; (err == nil) != want {
				t.Errorf("resolve %q error %v", resolution, err)
			}
		}
	}
	if err := s.Resolve(r.ID, "../finding", "fixed", "checked"); err == nil {
		t.Fatal("accepted invalid finding ID")
	}
	if _, err := s.Create(Review{Repo: "kit", HeadSHA: testHead[:7]}); err == nil {
		t.Fatal("accepted abbreviated head")
	}
	if _, err := s.Add(r.ID, Finding{File: "core.go", StartLine: 2, EndLine: 1, Body: "finding", Severity: "minor"}); err == nil {
		t.Fatal("accepted reversed lines")
	}
}

func TestReviewRedactionAndPrivatePersistence(t *testing.T) {
	s := testStore(t)
	const key = "ghp_0123456789abcdefghijklmnopqrstuvwxyz"
	const local = "/home/private/worktree/source.go"
	r, err := s.Create(Review{Repo: "kit", HeadSHA: testHead, Reviewer: key, Summary: "migration " + key + " " + local,
		Findings: []Finding{{File: "pkg/core.go", Severity: "major", Body: "retry " + key}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Resolve(r.ID, r.Findings[0].ID, "fixed", "confirmed "+key); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(s.root, r.ID)
	filename := filepath.Join(dir, "review.json")
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), key) || strings.Contains(string(data), local) {
		t.Fatal("raw private text in stored review")
	}
	got, err := s.Get(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{got.Reviewer, got.Summary, got.Findings[0].Body, got.Findings[0].ResolutionNote} {
		if !strings.Contains(text, "[REDACTED:") {
			t.Fatal("expected scrubbed text")
		}
	}
	if runtime.GOOS != "windows" {
		for path, perm := range map[string]os.FileMode{dir: 0700, filename: 0600} {
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != perm {
				t.Fatalf("expected private storage mode %o, got %v, %v", perm, info, err)
			}
		}
	}
	for _, query := range []string{key, "migration retry"} {
		found, err := s.Search(context.Background(), query, Filter{})
		want := 0
		if query != key {
			want = 1
		}
		if err != nil || len(found) != want {
			t.Fatalf("search returned %d results, want %d, error %v", len(found), want, err)
		}
	}
	for _, err := range []error{
		func() error {
			_, err := s.Add(r.ID, Finding{File: "../" + key, Body: "finding", Severity: "major"})
			return err
		}(),
		func() error { _, err := s.Create(Review{Repo: "kit", HeadSHA: testHead, Verdict: key}); return err }(),
		s.Resolve(r.ID, r.Findings[0].ID, key, key),
	} {
		if err == nil || strings.Contains(err.Error(), key) {
			t.Fatal("expected a validation error without raw secret text")
		}
	}
}
