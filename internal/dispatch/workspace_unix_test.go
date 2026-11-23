//go:build !windows

package dispatch

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/license"
)

func TestWorkspaceEditionAndLifecycle(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("HEV_ORDERS_DIR", filepath.Join(home, "orders"))
	t.Setenv("HEV_DISPATCH_CONFIG", filepath.Join(home, "dispatch.toml"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "gitconfig"))
	t.Setenv("CARGO_TARGET_DIR", "")
	if err := os.Unsetenv("CARGO_TARGET_DIR"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_AUTHOR_NAME", "Workspace Test")
	t.Setenv("GIT_COMMITTER_NAME", "Workspace Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "workspace@example.invalid")
	t.Setenv("GIT_COMMITTER_EMAIL", "workspace@example.invalid")

	write := func(path, contents string, mode os.FileMode) {
		t.Helper()
		if err := os.WriteFile(path, []byte(contents), mode); err != nil {
			t.Fatal(err)
		}
	}
	mkdir := func(path string) {
		t.Helper()
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	git := func(dir string, args ...string) string {
		t.Helper()
		value, err := command(dir, append([]string{"git"}, args...)...)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	bin := filepath.Join(home, "bin")
	mkdir(bin)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	write(filepath.Join(bin, "claude"), "#!/bin/sh\nprintf '%s' \"$CARGO_TARGET_DIR\" > ../cargo-env\nprintf 'done\\n' > ../status.md\n", 0700)
	write(filepath.Join(bin, "gh"), "#!/bin/sh\nexit 1\n", 0700)
	write(filepath.Join(bin, "tmux"), "#!/bin/sh\nexit 0\n", 0700)
	cfg := os.Getenv("HEV_DISPATCH_CONFIG")
	setMode := func(mode string) {
		t.Helper()
		write(cfg, "cap = 2\n[workspace]\nmode = "+fmt.Sprintf("%q", mode)+"\ndriver = \"git\"\n", 0600)
	}
	setMode("auto")
	repo, origin := filepath.Join(home, "workspace", "app"), filepath.Join(home, "origin.git")
	mkdir(repo)
	git(repo, "init", "--initial-branch=main")
	git(home, "init", "--bare", origin)
	git(repo, "remote", "add", "origin", origin)
	write(filepath.Join(repo, "Cargo.toml"), "[workspace]\n", 0600)
	write(filepath.Join(repo, ".gitignore"), "private-local\n", 0600)
	git(repo, "add", ".")
	git(repo, "commit", "-m", "base")
	git(repo, "push", "origin", "main")
	base := git(repo, "rev-parse", "HEAD")
	previous := runLauncher
	t.Cleanup(func() { runLauncher = previous })
	runLauncher = func(string, string) error { return nil }
	meta := func(name string) Meta {
		t.Helper()
		var m Meta
		if err := readJSON(filepath.Join(Root(), name, "meta.json"), &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	start := func(name string) error {
		t.Helper()
		order := fmt.Sprintf("---\nname: %s\nrepo: app\nbase: main\nhost: here\nharness: claude\n---\nBuild it.\n\n## Acceptance\n- Builds.\n", name)
		o, err := Parse(order)
		if err != nil {
			t.Fatal(err)
		}
		// Workspace feature fields carried by callers are ignored.
		_, err = Handle(Request{Action: "start", Name: name, Order: order, Meta: Meta{Order: o, ConfigPath: cfg, WorkspaceCap: 25, Compacted: true}})
		return err
	}
	handle := func(action, name, note string) error {
		t.Helper()
		_, err := Handle(Request{Action: action, Name: name, Note: note})
		return err
	}
	requireError := func(err error, want string) {
		t.Helper()
		if err == nil || !strings.Contains(err.Error(), want) || strings.Contains(err.Error(), home) {
			t.Fatalf("wanted path-free %q error, got %v", want, err)
		}
	}
	read := func(dir, file string) string {
		t.Helper()
		value, err := os.ReadFile(filepath.Join(dir, file))
		if err != nil {
			t.Fatal(err)
		}
		return string(value)
	}
	if err := start("oss"); err != nil {
		t.Fatal(err)
	}
	if err := superviseRound("oss"); err != nil {
		t.Fatal(err)
	}
	oss := filepath.Join(Root(), "oss")
	if meta("oss").WorkspaceCap != 0 || read(oss, "cargo-env") != "" {
		t.Fatal("OSS launch gained slot cache configuration")
	}
	requireError(handle("compact", "oss", ""), "active Kit Pro license")
	if err := handle("close", "oss", "finished"); err != nil || !exists(filepath.Join(oss, "wt", ".git")) {
		t.Fatalf("OSS close did not keep the checkout: %v", err)
	}

	setMode("pro")
	requireError(start("unlicensed"), "active Kit Pro license")
	mkdir(filepath.Join(home, ".hev"))
	write(filepath.Join(home, ".hev", "workspace-license.json"),
		fmt.Sprintf(`{"Pro":true,"Until":%q}`, time.Now().Add(time.Hour).Format(time.RFC3339Nano)), 0600)
	requireError(start("layer-only"), "active Kit Pro license")

	now := time.Now()
	projectKit := func(exp time.Time, tier string) {
		t.Helper()
		data, err := json.Marshal(license.Status{
			Valid: true, Expires: &exp, Tier: tier,
			CheckedAt: now, Until: now.Add(time.Hour),
		})
		if err != nil {
			t.Fatal(err)
		}
		write(filepath.Join(home, ".hev", "license-status.json"), string(data), 0600)
	}
	projectKit(now.Add(-time.Minute), "trial")
	requireError(start("expired"), "active Kit Pro license")
	projectKit(now.Add(-time.Minute), "pro")
	if !workspaceEntitled(time.Now()) {
		t.Fatal("Kit grace should preserve Pro workspace access")
	}
	projectKit(now.Add(time.Hour), "pro")
	setMode("oss")
	if err := start("opt-out"); err != nil || meta("opt-out").WorkspaceCap != 0 {
		t.Fatalf("licensed OSS opt-out did not use plain worktree: %v", err)
	}
	requireError(handle("compact", "opt-out", ""), "active Kit Pro license")
	setMode("pro")
	if err := start("pro"); err != nil {
		t.Fatal(err)
	}
	if err := superviseRound("pro"); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(Root(), "pro")
	wt := filepath.Join(dir, "wt")
	if m := meta("pro"); m.WorkspaceCap != 2 || m.WorkspaceBase != base || m.Compacted {
		t.Fatalf("unexpected Pro metadata: %+v", m)
	}
	if cargo := read(dir, "cargo-env"); !strings.HasSuffix(cargo, "/slot-0/cargo-target") || !exists(cargo) {
		t.Fatalf("expected reusable cargo cache, got %q", cargo)
	}
	given := []string{"CARGO_TARGET_DIR=/caller/target"}
	if got := cargoWorkspaceEnvironment(given, wt, "slot"); !slices.Equal(got, given) {
		t.Fatalf("overrode explicit Cargo cache: %v", got)
	}

	pointer := read(wt, ".git")
	write(filepath.Join(wt, ".git"), "gitdir: /Volumes/offline/secret-path\n", 0600)
	requireError(handle("compact", "pro", ""), "inside HOME")
	write(filepath.Join(wt, ".git"), pointer, 0600)
	write(filepath.Join(wt, "private-local"), "ignored source data", 0600)
	requireError(handle("compact", "pro", ""), "untracked source")
	if err := os.Remove(filepath.Join(wt, "private-local")); err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(wt, "Cargo.toml"), "# tracked change\n", 0600)
	requireError(handle("compact", "pro", ""), "uncommitted source")
	git(wt, "add", "Cargo.toml")
	git(wt, "commit", "-m", "new head")
	head := git(wt, "rev-parse", "HEAD")
	requireError(handle("compact", "pro", ""), "unpushed")
	if git(wt, "rev-parse", "HEAD") != head {
		t.Fatal("refusal changed worktree")
	}
	git(wt, "push", "origin", "HEAD:refs/heads/d/pro")
	mkdir(filepath.Join(wt, "target"))
	write(filepath.Join(wt, "target", "artifact"), "generated", 0600)
	write(filepath.Join(dir, "report.md"), "report\n", 0600)
	write(filepath.Join(dir, "review-notes.json"), "{}\n", 0600)
	saved := map[string]string{}
	for _, file := range []string{"report.md", "result.json", "order.md", "review-notes.json"} {
		saved[file] = read(dir, file)
	}
	if err := handle("compact", "pro", ""); err != nil {
		t.Fatal(err)
	}
	if exists(wt) || !meta("pro").Compacted || meta("pro").HeadSHA != head {
		t.Fatal("compaction failed to remove wt and pin HEAD")
	}
	for file, want := range saved {
		if read(dir, file) != want {
			t.Fatalf("compaction changed %s", file)
		}
	}
	for _, action := range []string{"retry", "rework", "amend"} {
		note := "Restore after compaction"
		if action == "amend" {
			note = read(dir, "order.md") + "\nA revised order.\n"
			if _, err := Handle(Request{Action: action, Name: "pro", Order: note}); err != nil {
				t.Fatal(err)
			}
		} else if err := handle(action, "pro", note); err != nil {
			t.Fatal(err)
		}
		if git(wt, "rev-parse", "HEAD") != head || git(wt, "branch", "--show-current") != "d/pro" || meta("pro").Compacted {
			t.Fatalf("%s did not restore the exact branch and commit", action)
		}
		if err := handle("close", "pro", "finished"); err != nil || exists(wt) {
			t.Fatalf("Pro close did not compact clean checkout: %v", err)
		}
	}

	write(cfg, "cap = 2\n[workspace]\nmode = \"pro\"\ndriver = \"git\"\nauto_compact_done = true\n", 0600)
	if err := start("auto-done"); err != nil {
		t.Fatal(err)
	}
	// The unchanged base remains recoverable even with origin offline.
	git(repo, "remote", "set-url", "origin", filepath.Join(home, "unreachable.git"))
	if err := Supervise("auto-done"); err != nil {
		t.Fatal(err)
	}
	if m := meta("auto-done"); !m.Compacted || m.HeadSHA != base || exists(filepath.Join(Root(), "auto-done", "wt")) {
		t.Fatal("successful supervisor did not compact the unchanged base")
	}
}

func TestWorkspaceSlotCrashHelper(t *testing.T) {
	pool := os.Getenv("DISPATCH_SLOT_CRASH_POOL")
	if pool == "" {
		return
	}
	if _, lock, _, err := leaseWorkspaceSlot(pool, 1); err != nil {
		t.Fatal(err)
	} else {
		defer lock.Close()
	}
	fmt.Println("leased")
	<-time.After(30 * time.Second)
}

func TestWorkspaceSlotsBoundedExclusiveReusableAndCollected(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	pool := workspaceStatePath("workspace-slots/repo")
	first, _, releaseFirst, err := leaseWorkspaceSlot(pool, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if releaseFirst != nil {
			releaseFirst()
		}
	}()
	second, _, releaseSecond, err := leaseWorkspaceSlot(pool, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseSecond()
	if first == second {
		t.Fatal("attempts shared a slot")
	}
	if _, _, _, err := leaseWorkspaceSlot(pool, 2); err == nil {
		t.Fatal("allocated above cap")
	}
	old := time.Now().Add(-8 * 24 * time.Hour)
	if err := os.Chtimes(first, old, old); err != nil {
		t.Fatal(err)
	}
	if removed, err := gcWorkspaceSlots(time.Now()); err != nil || removed != 0 {
		t.Fatalf("GC touched active slot: %d, %v", removed, err)
	}
	releaseFirst()
	releaseFirst = nil
	again, _, releaseAgain, err := leaseWorkspaceSlot(pool, 2)
	if err != nil || again != first {
		t.Fatalf("did not reuse warm slot: %v", err)
	}
	releaseAgain()
	if err := os.Chtimes(first, old, old); err != nil {
		t.Fatal(err)
	}
	if removed, err := gcWorkspaceSlots(time.Now()); err != nil || removed != 1 || exists(first) || !exists(second) {
		t.Fatalf("GC did not evict exactly the idle slot: %d, %v", removed, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWorkspaceSlotCrashHelper$")
	child.Env = append(os.Environ(), "ORCH_PROCESS_HELPER=1", "DISPATCH_SLOT_CRASH_POOL="+pool)
	stdout, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	line, readErr := bufio.NewReader(stdout).ReadString('\n')
	if err := child.Process.Kill(); err != nil {
		t.Error(err)
	}
	_ = child.Wait()
	if readErr != nil || line != "leased\n" {
		t.Fatalf("helper did not acquire lease: %q, %v", line, readErr)
	}
	again, _, releaseAgain, err = leaseWorkspaceSlot(pool, 2)
	if err != nil || again != first {
		t.Fatalf("crashed process left a lease: %v", err)
	}
	releaseAgain()
}

func TestWorkspaceCheckoutKeepsGitSemanticsAndFallsBack(t *testing.T) {
	repo, dirs := t.TempDir(), t.TempDir()
	git := func(dir string, args ...string) string {
		t.Helper()
		out, err := command(dir, append([]string{"git"}, args...)...)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	git(repo, "init", "--initial-branch=main")
	if err := os.WriteFile(filepath.Join(repo, "source"), []byte("tracked\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git(repo, "add", "source")
	git(repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "base")
	head := git(repo, "rev-parse", "HEAD")
	warm := t.TempDir()
	if err := os.WriteFile(filepath.Join(warm, "source"), []byte("tracked\n"), 0600); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name  string
		clone workspaceCloneDriver
	}{
		{"supported", func(src, dst string) bool {
			b, err := os.ReadFile(filepath.Join(src, "source"))
			return err == nil && os.WriteFile(filepath.Join(dst, "source"), b, 0600) == nil
		}},
		{"unsupported", func(_, _ string) bool { return false }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wt := filepath.Join(dirs, tc.name)
			args := []string{"-C", repo, "worktree", "add", "-b", "d/" + tc.name, wt, head}
			used, err := seedDispatchWorkspace(args, wt, head, warm, tc.clone)
			if err != nil || used != (tc.name == "supported") {
				t.Fatalf("checkout driver selection: used=%v, err=%v", used, err)
			}
			if !used {
				if exists(wt) {
					t.Fatal("failed probe left a partial worktree")
				}
				if err := addDispatchWorkspace(args, head, false); err != nil {
					t.Fatal(err)
				}
			}
			if git(wt, "rev-parse", "HEAD") != head || git(wt, "branch", "--show-current") != "d/"+tc.name || git(wt, "status", "--porcelain") != "" {
				t.Fatal("checkout diverged from Git branch, HEAD or index semantics")
			}
		})
	}
}
