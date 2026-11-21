//go:build !windows

package dispatch

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// TestCompactEndedRunsSafetyBothSides verifies Rule 1 for dispatch compaction:
// active running, queued, recent (< compact_after), dirty, untracked, unpushed,
// out-of-home .git, and non-terminal runs are KEPT; old clean done/failed/closed
// runs are COMPACTED; and dry-run mutates zero files.
func TestCompactEndedRunsSafetyBothSides(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	ordersDir := filepath.Join(home, "orders")
	t.Setenv("HEV_ORDERS_DIR", ordersDir)
	t.Setenv("HEV_DISPATCH_CONFIG", filepath.Join(home, "dispatch.toml"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "gitconfig"))
	t.Setenv("GIT_AUTHOR_NAME", "Hygiene Test")
	t.Setenv("GIT_COMMITTER_NAME", "Hygiene Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "hygiene@example.invalid")
	t.Setenv("GIT_COMMITTER_EMAIL", "hygiene@example.invalid")

	write := func(path, contents string, mode os.FileMode) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), mode); err != nil {
			t.Fatal(err)
		}
	}
	git := func(dir string, args ...string) string {
		t.Helper()
		out, err := command(dir, append([]string{"git"}, args...)...)
		if err != nil {
			t.Fatalf("git %v in %s: %v (%s)", args, dir, err, out)
		}
		return out
	}

	repo := filepath.Join(home, "workspace", "app")
	origin := filepath.Join(home, "origin.git")
	if err := os.MkdirAll(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	git(repo, "init", "--initial-branch=main")
	git(home, "init", "--bare", origin)
	git(repo, "remote", "add", "origin", origin)
	write(filepath.Join(repo, "README.md"), "hello\n", 0o600)
	git(repo, "add", ".")
	git(repo, "commit", "-m", "initial")
	git(repo, "push", "origin", "main")
	baseSHA := git(repo, "rev-parse", "HEAD")

	now := time.Date(2026, 11, 21, 18, 0, 0, 0, time.UTC)
	oldEnd := now.Add(-48 * time.Hour)
	recentEnd := now.Add(-2 * time.Hour)

	createRun := func(name string, res *Result, pushBranch bool, commitExtra bool) (string, string) {
		t.Helper()
		dir := filepath.Join(ordersDir, name)
		wt := filepath.Join(dir, "wt")
		branch := "d/" + name
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		git(repo, "worktree", "add", "-b", branch, wt, "main")
		if commitExtra {
			write(filepath.Join(wt, "work.txt"), fmt.Sprintf("work for %s\n", name), 0o600)
			git(wt, "add", "work.txt")
			git(wt, "commit", "-m", "work "+name)
			if pushBranch {
				git(wt, "push", "origin", branch)
			}
		}
		m := Meta{
			Order:         Order{Name: name, Repo: "app", Base: "main", Host: "here", Harness: "claude"},
			Branch:        branch,
			Start:         oldEnd.Add(-time.Hour),
			ResolvedRepo:  repo,
			WorkspaceBase: baseSHA,
		}
		if err := WriteJSON(filepath.Join(dir, "meta.json"), m); err != nil {
			t.Fatal(err)
		}
		write(filepath.Join(dir, "order.md"), "test order\n", 0o600)
		if res != nil {
			res.Branch = branch
			if err := WriteJSON(filepath.Join(dir, "result.json"), res); err != nil {
				t.Fatal(err)
			}
		}
		return dir, wt
	}

	// 1. Eligible runs that MUST be compacted:
	oldDoneDir, oldDoneWT := createRun("old-done", &Result{ExitCode: 0, Outcome: "done", Start: oldEnd.Add(-time.Hour), End: oldEnd}, true, true)
	oldFailedDir, oldFailedWT := createRun("old-failed", &Result{ExitCode: 1, Start: oldEnd.Add(-time.Hour), End: oldEnd}, true, true)
	oldClosedDir, oldClosedWT := createRun("old-closed", &Result{ExitCode: 0, Start: oldEnd.Add(-time.Hour), End: oldEnd, ClosedAt: oldEnd, ClosedBy: "hev"}, true, true)

	// 2. Protected runs that MUST be kept:
	// 2a. Recent done (< compact_after)
	_, recentWT := createRun("recent-done", &Result{ExitCode: 0, Outcome: "done", Start: recentEnd.Add(-time.Hour), End: recentEnd}, true, true)

	// 2b. Dirty tracked file
	_, dirtyWT := createRun("old-dirty", &Result{ExitCode: 0, Outcome: "done", Start: oldEnd.Add(-time.Hour), End: oldEnd}, true, true)
	write(filepath.Join(dirtyWT, "README.md"), "modified uncommitted\n", 0o600)

	// 2c. Untracked source file
	_, untrackedWT := createRun("old-untracked", &Result{ExitCode: 0, Outcome: "done", Start: oldEnd.Add(-time.Hour), End: oldEnd}, true, true)
	write(filepath.Join(untrackedWT, "scratch.go"), "package main\n", 0o600)

	// 2d. Unpushed commit
	_, unpushedWT := createRun("old-unpushed", &Result{ExitCode: 0, Outcome: "done", Start: oldEnd.Add(-time.Hour), End: oldEnd}, false, true)

	// 2e. Non-terminal state (blocked)
	_, blockedWT := createRun("old-blocked", &Result{ExitCode: 0, Outcome: OutcomeBlocked, Start: oldEnd.Add(-time.Hour), End: oldEnd}, true, true)

	// 2f. Active running run (live current PID in harness.json)
	activeDir, activeWT := createRun("active-running", nil, false, false)
	if err := writeProcessRecord(filepath.Join(activeDir, "harness.json"), os.Getpid()); err != nil {
		t.Fatal(err)
	}

	// 2g. Queued run (entry in .queue/)
	_, queuedWT := createRun("queued-run", &Result{ExitCode: 0, Outcome: "done", Start: oldEnd.Add(-time.Hour), End: oldEnd}, true, true)
	if err := os.MkdirAll(filepath.Join(ordersDir, ".queue"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := WriteJSON(queuePath("queued-run"), queueEntry{
		Request:  Request{Name: "queued-run"},
		Enqueued: oldEnd,
	}); err != nil {
		t.Fatal(err)
	}

	// 2h. External volume .git mount outside HOME
	extRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	extRepo := filepath.Join(extRoot, "ext-repo")
	if err := os.MkdirAll(extRepo, 0o700); err != nil {
		t.Fatal(err)
	}
	git(extRepo, "init", "--initial-branch=main")
	write(filepath.Join(extRepo, "README.md"), "ext\n", 0o600)
	git(extRepo, "add", ".")
	git(extRepo, "commit", "-m", "ext init")
	extBase := git(extRepo, "rev-parse", "HEAD")
	extDir := filepath.Join(ordersDir, "old-external")
	extWT := filepath.Join(extDir, "wt")
	if err := os.MkdirAll(extDir, 0o700); err != nil {
		t.Fatal(err)
	}
	git(extRepo, "worktree", "add", "-b", "d/old-external", extWT, "main")
	if err := WriteJSON(filepath.Join(extDir, "meta.json"), Meta{
		Order:         Order{Name: "old-external", Repo: "ext-repo", Base: "main", Host: "here", Harness: "claude"},
		Branch:        "d/old-external",
		Start:         oldEnd.Add(-time.Hour),
		ResolvedRepo:  extRepo,
		WorkspaceBase: extBase,
	}); err != nil {
		t.Fatal(err)
	}
	if err := WriteJSON(filepath.Join(extDir, "result.json"), Result{
		ExitCode: 0, Outcome: "done", Start: oldEnd.Add(-time.Hour), End: oldEnd, Branch: "d/old-external",
	}); err != nil {
		t.Fatal(err)
	}

	// --- Step 1: Dry-run must identify exactly the 3 eligible runs and mutate ZERO files ---
	dryRep, err := CompactEndedRuns(CompactOptions{
		OlderThan: 24 * time.Hour,
		Now:       now,
		DryRun:    true,
	})
	if err != nil {
		t.Fatalf("dry-run CompactEndedRuns: %v", err)
	}
	wantCompacted := []string{"old-closed", "old-done", "old-failed"}
	slices.Sort(dryRep.Compacted)
	if !slices.Equal(dryRep.Compacted, wantCompacted) {
		t.Fatalf("dry-run compacted = %v, want %v (kept=%+v)", dryRep.Compacted, wantCompacted, dryRep.Kept)
	}
	if dryRep.BytesFreed <= 0 {
		t.Fatalf("expected positive estimated bytes freed in dry-run, got %d", dryRep.BytesFreed)
	}
	// Verify zero files mutated during dry-run.
	for _, wt := range []string{oldDoneWT, oldFailedWT, oldClosedWT, recentWT, dirtyWT, untrackedWT, unpushedWT, blockedWT, activeWT, queuedWT, extWT} {
		if _, err := os.Stat(wt); err != nil {
			t.Fatalf("dry-run removed worktree %s: %v", wt, err)
		}
	}
	var mCheck Meta
	if err := readJSON(filepath.Join(oldDoneDir, "meta.json"), &mCheck); err != nil || mCheck.Compacted {
		t.Fatalf("dry-run mutated meta.json: compacted=%t err=%v", mCheck.Compacted, err)
	}

	// --- Step 2: Real run compacts the 3 eligible runs and keeps all 8 protected runs ---
	realRep, err := CompactEndedRuns(CompactOptions{
		OlderThan: 24 * time.Hour,
		Now:       now,
		DryRun:    false,
	})
	if err != nil {
		t.Fatalf("real CompactEndedRuns: %v", err)
	}
	slices.Sort(realRep.Compacted)
	if !slices.Equal(realRep.Compacted, wantCompacted) {
		t.Fatalf("real compacted = %v, want %v (kept=%+v)", realRep.Compacted, wantCompacted, realRep.Kept)
	}

	// Eligible worktrees are removed; metadata & order.md are preserved.
	for _, tc := range []struct {
		dir string
		wt  string
	}{
		{oldDoneDir, oldDoneWT},
		{oldFailedDir, oldFailedWT},
		{oldClosedDir, oldClosedWT},
	} {
		if _, err := os.Lstat(tc.wt); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be compacted, stat err=%v", tc.wt, err)
		}
		var m Meta
		if err := readJSON(filepath.Join(tc.dir, "meta.json"), &m); err != nil || !m.Compacted || m.HeadSHA == "" {
			t.Fatalf("expected compacted meta.json in %s, got %+v err=%v", tc.dir, m, err)
		}
		if _, err := os.Stat(filepath.Join(tc.dir, "order.md")); err != nil {
			t.Fatalf("expected order.md to remain in %s: %v", tc.dir, err)
		}
	}

	// All protected worktrees MUST still exist untouched.
	for _, wt := range []string{recentWT, dirtyWT, untrackedWT, unpushedWT, blockedWT, activeWT, queuedWT, extWT} {
		if _, err := os.Stat(wt); err != nil {
			t.Fatalf("protected worktree %s was unexpectedly removed: %v", wt, err)
		}
	}
}
