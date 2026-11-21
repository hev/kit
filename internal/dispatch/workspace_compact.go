package dispatch

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func withinWorkspaceHome(home, path string) bool {
	rel, err := filepath.Rel(home, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// Inspect one component at a time so an out-of-home symlink is refused before
// even statting its destination (in particular an offline /Volumes mount).
func workspaceHomePath(home, path string) (string, error) {
	unsafePath := errors.New("workspace and Git administration must resolve inside HOME")
	path = filepath.Clean(path)
	for hops := 0; hops < 40; hops++ {
		if !withinWorkspaceHome(home, path) {
			return "", unsafePath
		}
		rel, _ := filepath.Rel(home, path)
		parts := strings.Split(rel, string(filepath.Separator))
		next := home
		redirected := false
		for i, part := range parts {
			next = filepath.Join(next, part)
			info, err := os.Lstat(next)
			if err != nil {
				return "", unsafePath
			}
			if info.Mode()&os.ModeSymlink == 0 {
				continue
			}
			link, err := os.Readlink(next)
			if err != nil {
				return "", unsafePath
			}
			if !filepath.IsAbs(link) {
				link = filepath.Join(filepath.Dir(next), link)
			}
			path = filepath.Join(append([]string{link}, parts[i+1:]...)...)
			redirected = true
			break
		}
		if !redirected {
			return path, nil
		}
	}
	return "", unsafePath
}

func linkedWorkspace(wt string) (string, error) {
	unsafePath := errors.New("worktree Git administration must resolve inside HOME")
	home, err := os.UserHomeDir()
	if err != nil || !filepath.IsAbs(home) {
		return "", unsafePath
	}
	dotgit := filepath.Join(wt, ".git")
	resolved, err := workspaceHomePath(home, dotgit)
	if err != nil || resolved != dotgit {
		return "", unsafePath
	}
	b, err := readWorkspacePointer(dotgit)
	gitdir, valid := strings.CutPrefix(b, "gitdir: ")
	if err != nil || !valid || gitdir == "" {
		return "", unsafePath
	}
	resolve := func(base, pointer string) (string, error) {
		if pointer == "" {
			return "", unsafePath
		}
		if !filepath.IsAbs(pointer) {
			pointer = filepath.Join(base, pointer)
		}
		return workspaceHomePath(home, pointer)
	}
	gitdir, err = resolve(wt, gitdir)
	if err != nil {
		return "", err
	}
	backlink, err := readWorkspacePointer(filepath.Join(gitdir, "gitdir"))
	if err != nil {
		return "", unsafePath
	}
	registered, err := resolve(gitdir, backlink)
	if err != nil || registered != dotgit {
		return "", unsafePath
	}
	commonFile, err := workspaceHomePath(home, filepath.Join(gitdir, "commondir"))
	if err != nil {
		return "", err
	}
	b, err = readWorkspacePointer(commonFile)
	if err != nil {
		return "", unsafePath
	}
	common, err := resolve(gitdir, b)
	if err != nil {
		return "", err
	}
	if filepath.Dir(gitdir) != filepath.Join(common, "worktrees") {
		return "", unsafePath
	}
	return common, nil
}

func readWorkspacePointer(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 8192 {
		return "", errors.New("invalid Git workspace pointer")
	}
	b, err := os.ReadFile(path)
	return strings.TrimRight(string(b), "\r\n"), err
}

func workspaceHead(wt string, m Meta) (string, error) {
	head, err := command(wt, "git", "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return "", errors.New("cannot read workspace HEAD")
	}
	branch, err := command(wt, "git", "symbolic-ref", "--quiet", "HEAD")
	if m.Parent != "" && err != nil && head == m.Revision {
		return head, nil
	}
	if err != nil || branch != "refs/heads/"+m.Branch {
		return "", errors.New("worktree is not on the dispatch branch")
	}
	return head, nil
}

// Callers hold the dispatch root lock and have fenced run launches.
func compactWorkspace(dir string, m Meta) error {
	wt := filepath.Join(dir, "wt")
	if m.Compacted {
		if _, err := os.Lstat(wt); os.IsNotExist(err) {
			return nil
		}
	}
	if m.Forward != nil {
		return errors.New("run moved to another host")
	}
	if processRecordLive(filepath.Join(dir, "harness.json")) {
		return errors.New("run still has an active harness")
	}
	var result Result
	if readJSON(filepath.Join(dir, "result.json"), &result) == nil {
		reconcileLaunch(dir, &result)
		if result.LaunchUnconfirmed {
			return errors.New("run launch is unconfirmed")
		}
	}
	if c := steerCounts(dir); c != nil && c.Halting {
		return errors.New("run is halting")
	}
	common, err := linkedWorkspace(wt)
	if err != nil {
		return err
	}
	head, err := workspaceHead(wt, m)
	if err != nil {
		return err
	}

	if head != m.WorkspaceBase {
		if _, err = command(wt, "git", "fetch", "--no-tags", "--prune", "--refmap=", "origin", "+refs/heads/*:refs/remotes/origin/*"); err != nil {
			return errors.New("cannot verify workspace HEAD on origin")
		}
		refs, err := command(wt, "git", "for-each-ref", "--contains="+head, "--count=1", "refs/remotes/origin/")
		if err != nil || refs == "" {
			return errors.New("workspace HEAD is unpushed")
		}
	}
	status, err := command(wt, "git", "-c", "core.fsmonitor=false", "status", "--porcelain=v1", "--untracked-files=no", "--ignore-submodules=none")
	if err != nil {
		return errors.New("cannot inspect tracked workspace changes")
	}
	if status != "" {
		return errors.New("workspace has uncommitted source changes")
	}
	flags, err := command(wt, "git", "ls-files", "-v", "-z")
	if err != nil {
		return errors.New("cannot inspect workspace index")
	}
	for _, entry := range strings.Split(flags, "\x00") {
		if entry != "" && (entry[0] == 'S' || (entry[0] >= 'a' && entry[0] <= 'z')) {
			return errors.New("workspace index hides tracked files from status")
		}
	}
	untracked, err := command(wt, "git", "ls-files", "--others", "--directory", "--no-empty-directory", "-z")
	if err != nil {
		return errors.New("cannot inspect untracked workspace files")
	}
	for _, path := range strings.Split(untracked, "\x00") {
		if path != "" && !strings.HasPrefix(path, "target/") && !strings.HasPrefix(path, "node_modules/") {
			return errors.New("workspace has untracked source files")
		}
	}

	ignore, err := os.CreateTemp(dir, "workspace-excludes-")
	if err != nil {
		return errors.New("cannot prepare workspace compaction")
	}
	defer os.Remove(ignore.Name())
	_, err = ignore.WriteString("/target/\n/node_modules/\n")
	closeErr := ignore.Close()
	if err != nil || closeErr != nil {
		return errors.New("cannot prepare workspace compaction")
	}
	if current, err := workspaceHead(wt, m); err != nil || current != head {
		return errors.New("workspace HEAD changed during compaction")
	}
	prior := m
	m.Compacted, m.HeadSHA = true, head
	if err = WriteJSON(filepath.Join(dir, "meta.json"), m); err != nil {
		return errors.New("cannot record workspace compaction")
	}
	if _, err = command(wt, "git", "--git-dir="+common, "-c", "core.excludesFile="+ignore.Name(), "worktree", "remove", "--", wt); err != nil {
		if _, statErr := os.Lstat(wt); statErr == nil {
			if rollbackErr := WriteJSON(filepath.Join(dir, "meta.json"), prior); rollbackErr != nil {
				return errors.New("cannot remove workspace or restore its metadata")
			}
		}
		return errors.New("Git refused to remove the workspace")
	}
	return nil
}

func restoreWorkspace(dir string, m *Meta) error {
	if !m.Compacted {
		return nil
	}
	wt := filepath.Join(dir, "wt")
	if _, err := os.Lstat(wt); os.IsNotExist(err) {
		repo := m.ResolvedRepo
		home, homeErr := os.UserHomeDir()
		if homeErr != nil || !filepath.IsAbs(home) {
			return errors.New("cannot restore workspace outside HOME")
		}
		if _, err := workspaceHomePath(home, repo); err != nil {
			return err
		}
		if _, err := workspaceHomePath(home, dir); err != nil {
			return err
		}
		args := []string{"worktree", "add", "--", wt, m.Branch}
		if m.Parent == "" {
			ref, err := command(repo, "git", "rev-parse", "--verify", "refs/heads/"+m.Branch+"^{commit}")
			if err != nil || ref != m.HeadSHA {
				return errors.New("dispatch branch no longer points to the recorded HEAD")
			}
		} else {
			args = []string{"worktree", "add", "--detach", "--", wt, m.HeadSHA}
		}
		if _, err := command(repo, append([]string{"git"}, args...)...); err != nil {
			return errors.New("cannot restore the recorded worktree")
		}
	}
	if head, err := workspaceHead(wt, *m); err != nil || head != m.HeadSHA {
		return errors.New("restored worktree does not match the recorded HEAD")
	}
	m.Compacted = false
	if err := WriteJSON(filepath.Join(dir, "meta.json"), m); err != nil {
		return errors.New("cannot record workspace restoration")
	}
	return nil
}

func compactDispatch(name string, all bool) (string, error) {
	unlock, err := lockRoot()
	if err != nil {
		return "", err
	}
	defer unlock()
	live, err := liveNames()
	if err == nil {
		err = addReservations(live)
	}
	if err != nil {
		return "", err
	}
	compact := func(name string) error {
		dir := filepath.Join(Root(), name)
		var m Meta
		if err := readJSON(filepath.Join(dir, "meta.json"), &m); err != nil {
			return errors.New("order not found")
		}
		if live[name] {
			return errors.New("run is running or halting")
		}
		_, pro, err := workspacePolicy()
		if err != nil {
			return err
		}
		if !pro {
			return errWorkspaceLicense
		}
		return compactWorkspace(dir, m)
	}
	if !all {
		if !cachedWorkspaceLicense(time.Now()) {
			return "", errWorkspaceLicense
		}
		if err := compact(name); err != nil {
			return "", err
		}
		return "compacted " + name, nil
	}
	if !cachedWorkspaceLicense(time.Now()) {
		return "", errWorkspaceLicense
	}
	entries, err := os.ReadDir(Root())
	if err != nil {
		return "", errors.New("cannot list runs for compaction")
	}
	var lines []string
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || !ValidName(name) || !exists(filepath.Join(Root(), name, "meta.json")) {
			continue
		}
		if err := compact(name); err != nil {
			lines = append(lines, fmt.Sprintf("kept %s: %v", name, err))
		} else {
			lines = append(lines, "compacted "+name)
		}
	}
	return strings.Join(lines, "\n"), nil
}

func autoCompactDone(dir string) {
	var m Meta
	if readJSON(filepath.Join(dir, "meta.json"), &m) != nil || m.WorkspaceCap == 0 {
		return
	}
	if cfg, pro, err := workspacePolicy(); err != nil || !pro || !cfg.WorkspacePolicy.AutoCompactDone {
		return
	}
	// Amend can hold the root lock while waiting for this supervisor to exit.
	unlock, err := tryWorkspaceDispatchLock()
	if err != nil {
		return
	}
	defer unlock()
	var result Result
	if readJSON(filepath.Join(dir, "result.json"), &result) != nil || State(&result, false) != "done" {
		return
	}
	if readJSON(filepath.Join(dir, "meta.json"), &m) != nil || exists(filepath.Join(dir, "stop")) {
		return
	}
	if cfg, pro, err := workspacePolicy(); err == nil && pro && cfg.WorkspacePolicy.AutoCompactDone {
		if err := compactWorkspace(dir, m); err != nil {
			if log, openErr := os.OpenFile(filepath.Join(dir, "log"), os.O_APPEND|os.O_WRONLY, 0600); openErr == nil {
				_, _ = fmt.Fprintln(log, "worktree kept:", err)
				_ = log.Close()
			}
		}
	}
}
