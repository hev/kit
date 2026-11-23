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

// checkCompactWorkspace validates every safety guard required before removing
// a dispatch run's worktree. When dryRun is true it avoids mutating remote-tracking
// refs (skipping git fetch) so dry-run mutates zero files.
func checkCompactWorkspace(dir string, m Meta, dryRun bool) (string, string, error) {
	wt := filepath.Join(dir, "wt")
	if m.Forward != nil {
		return "", "", errors.New("run moved to another host")
	}
	if processRecordLive(filepath.Join(dir, "harness.json")) {
		return "", "", errors.New("run still has an active harness")
	}
	var result Result
	if readJSON(filepath.Join(dir, "result.json"), &result) == nil {
		reconcileLaunch(dir, &result)
		if result.LaunchUnconfirmed {
			return "", "", errors.New("run launch is unconfirmed")
		}
	}
	if c := steerCounts(dir); c != nil && c.Halting {
		return "", "", errors.New("run is halting")
	}
	common, err := linkedWorkspace(wt)
	if err != nil {
		return "", "", err
	}
	if _, err := workspaceRestoreRepo(m, common); err != nil {
		return "", "", err
	}
	head, err := workspaceHead(wt, m)
	if err != nil {
		return "", "", err
	}

	if head != m.WorkspaceBase {
		if !dryRun {
			if _, err = command(wt, "git", "fetch", "--no-tags", "--prune", "--refmap=", "origin", "+refs/heads/*:refs/remotes/origin/*"); err != nil {
				return "", "", errors.New("cannot verify workspace HEAD on origin")
			}
		}
		refs, err := command(wt, "git", "for-each-ref", "--contains="+head, "--count=1", "refs/remotes/origin/")
		if err != nil || refs == "" {
			return "", "", errors.New("workspace HEAD is unpushed")
		}
	}
	status, err := command(wt, "git", "-c", "core.fsmonitor=false", "status", "--porcelain=v1", "--untracked-files=no", "--ignore-submodules=none")
	if err != nil {
		return "", "", errors.New("cannot inspect tracked workspace changes")
	}
	if status != "" {
		return "", "", errors.New("workspace has uncommitted source changes")
	}
	flags, err := command(wt, "git", "ls-files", "-v", "-z")
	if err != nil {
		return "", "", errors.New("cannot inspect workspace index")
	}
	for _, entry := range strings.Split(flags, "\x00") {
		if entry != "" && (entry[0] == 'S' || (entry[0] >= 'a' && entry[0] <= 'z')) {
			return "", "", errors.New("workspace index hides tracked files from status")
		}
	}
	untracked, err := command(wt, "git", "ls-files", "--others", "--directory", "--no-empty-directory", "-z")
	if err != nil {
		return "", "", errors.New("cannot inspect untracked workspace files")
	}
	for _, path := range strings.Split(untracked, "\x00") {
		if path != "" && !strings.HasPrefix(path, "target/") && !strings.HasPrefix(path, "node_modules/") {
			return "", "", errors.New("workspace has untracked source files")
		}
	}
	return common, head, nil
}

// Callers hold the dispatch root lock and have fenced run launches.
func compactWorkspace(dir string, m Meta) error {
	wt := filepath.Join(dir, "wt")
	if m.Compacted {
		if _, err := os.Lstat(wt); os.IsNotExist(err) {
			return nil
		}
	}
	common, head, err := checkCompactWorkspace(dir, m, false)
	if err != nil {
		return err
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
	if _, err = command(dir, "git", "--git-dir="+common, "-c", "core.excludesFile="+ignore.Name(), "worktree", "remove", "--", wt); err != nil {
		if _, statErr := os.Lstat(wt); statErr == nil {
			if rollbackErr := WriteJSON(filepath.Join(dir, "meta.json"), prior); rollbackErr != nil {
				return errors.New("cannot remove workspace or restore its metadata")
			}
		}
		return errors.New("Git refused to remove the workspace")
	}
	return nil
}

// Validate the repository that reopen will use before dropping the checkout.
// The common directory itself is enough for a bare repository.
func workspaceRestoreRepo(m Meta, common string) (string, error) {
	refusal := errors.New("cannot resolve workspace recovery repository inside HOME")
	home, err := os.UserHomeDir()
	if err != nil || !filepath.IsAbs(home) {
		return "", refusal
	}
	repo, err := workspaceHomePath(home, m.ResolvedRepo)
	if err != nil {
		return "", refusal
	}
	if repo == common {
		return repo, nil
	}
	// A conventional clone's Git directory must stay inside HOME too.
	dotgit, err := workspaceHomePath(home, filepath.Join(repo, ".git"))
	if err != nil {
		// Explicit bare repositories have no .git entry.
		if info, statErr := os.Lstat(filepath.Join(repo, "HEAD")); statErr == nil && info.Mode().IsRegular() && common == "" {
			return repo, nil
		}
		return "", refusal
	}
	info, err := os.Lstat(dotgit)
	if err != nil {
		return "", refusal
	}
	if !info.IsDir() {
		dotgit, err = linkedWorkspace(repo)
		if err != nil {
			return "", refusal
		}
	}
	if common != "" && dotgit != common {
		return "", refusal
	}
	return repo, nil
}

func restoreWorkspace(dir string, m *Meta) error {
	if !m.Compacted {
		return nil
	}
	wt := filepath.Join(dir, "wt")
	if _, err := os.Lstat(wt); os.IsNotExist(err) {
		repo, err := workspaceRestoreRepo(*m, "")
		if err != nil {
			return err
		}
		home, _ := os.UserHomeDir()
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
		if !workspaceEntitled(time.Now()) {
			return "", errWorkspaceLicense
		}
		if err := compact(name); err != nil {
			return "", err
		}
		return "compacted " + name, nil
	}
	if !workspaceEntitled(time.Now()) {
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

// CompactOptions controls age-bounded compaction of ended dispatch runs.
type CompactOptions struct {
	OlderThan time.Duration
	Now       time.Time
	DryRun    bool
}

// CompactAction describes the outcome for one inspected dispatch run.
type CompactAction struct {
	Name       string `json:"name"`
	State      string `json:"state"`
	Action     string `json:"action"` // "compacted", "would_compact", "kept"
	Reason     string `json:"reason,omitempty"`
	BytesFreed int64  `json:"bytes_freed,omitempty"`
}

// CompactReport summarizes a CompactEndedRuns pass.
type CompactReport struct {
	Compacted  []string        `json:"compacted"`
	Kept       []CompactAction `json:"kept,omitempty"`
	Actions    []CompactAction `json:"actions,omitempty"`
	BytesFreed int64           `json:"bytes_freed"`
}

// CompactEndedRuns compacts ended dispatch runs (done, failed, closed) older
// than opts.OlderThan while strictly reusing compactWorkspace's safety guards.
// Active/running, queued, recent, dirty, unpushed, and out-of-home worktrees
// are always kept. In DryRun mode, zero files are created or modified.
func CompactEndedRuns(opts CompactOptions) (CompactReport, error) {
	rep := CompactReport{Compacted: []string{}}
	root := Root()
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return rep, nil
	} else if err != nil {
		return rep, errors.New("cannot inspect dispatch root")
	}

	if !opts.DryRun {
		unlock, err := lockRoot()
		if err != nil {
			return rep, err
		}
		defer unlock()
	}

	live, err := liveNames()
	if err == nil {
		err = addReservations(live)
	}
	if err != nil {
		return rep, err
	}

	queued := map[string]bool{}
	if qEntries, qErr := queueEntries(); qErr == nil {
		for _, qe := range qEntries {
			if qe.Request.Name != "" {
				queued[qe.Request.Name] = true
			}
		}
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return rep, errors.New("cannot list runs for compaction")
	}

	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}

	recordKept := func(name, state, reason string) {
		act := CompactAction{Name: name, State: state, Action: "kept", Reason: reason}
		rep.Kept = append(rep.Kept, act)
		rep.Actions = append(rep.Actions, act)
	}

	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || !ValidName(name) {
			continue
		}
		dir := filepath.Join(root, name)
		if !exists(filepath.Join(dir, "meta.json")) {
			continue
		}
		var m Meta
		if err := readJSON(filepath.Join(dir, "meta.json"), &m); err != nil {
			recordKept(name, "unknown", "unreadable metadata")
			continue
		}
		wt := filepath.Join(dir, "wt")
		if m.Compacted {
			if _, statErr := os.Lstat(wt); os.IsNotExist(statErr) {
				continue
			}
		}
		if live[name] {
			recordKept(name, "running", "run is active")
			continue
		}
		if queued[name] {
			recordKept(name, "queued", "run is queued")
			continue
		}

		var result Result
		if err := readJSON(filepath.Join(dir, "result.json"), &result); err != nil && !os.IsNotExist(err) {
			recordKept(name, "unknown", "unreadable result")
			continue
		}
		reconcileLaunch(dir, &result)
		st := State(&result, false)
		switch st {
		case "done", "failed", StateClosed:
		default:
			recordKept(name, st, "state is "+st)
			continue
		}

		endedAt := result.End
		if st == StateClosed && !result.ClosedAt.IsZero() && result.ClosedAt.After(endedAt) {
			endedAt = result.ClosedAt
		}
		if endedAt.IsZero() {
			recordKept(name, st, "missing end timestamp")
			continue
		}
		if now.Sub(endedAt) < opts.OlderThan {
			recordKept(name, st, "newer than compact_after")
			continue
		}
		if _, statErr := os.Lstat(wt); os.IsNotExist(statErr) {
			continue
		}

		wtBytes := dirSizeBounded(wt, 200000)

		if opts.DryRun {
			if _, _, err := checkCompactWorkspace(dir, m, true); err != nil {
				recordKept(name, st, err.Error())
				continue
			}
			act := CompactAction{Name: name, State: st, Action: "would_compact", BytesFreed: wtBytes}
			rep.Compacted = append(rep.Compacted, name)
			rep.Actions = append(rep.Actions, act)
			rep.BytesFreed += wtBytes
			continue
		}

		if err := compactWorkspace(dir, m); err != nil {
			recordKept(name, st, err.Error())
			continue
		}
		act := CompactAction{Name: name, State: st, Action: "compacted", BytesFreed: wtBytes}
		rep.Compacted = append(rep.Compacted, name)
		rep.Actions = append(rep.Actions, act)
		rep.BytesFreed += wtBytes
	}

	return rep, nil
}

// dirSizeBounded sums regular file sizes under root without following symlinks.
func dirSizeBounded(root string, maxEntries int) int64 {
	var total int64
	visited := 0
	var walk func(string)
	walk = func(dir string) {
		if visited >= maxEntries {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if visited >= maxEntries {
				return
			}
			visited++
			p := filepath.Join(dir, e.Name())
			info, err := os.Lstat(p)
			if err != nil {
				continue
			}
			if info.Mode()&os.ModeSymlink != 0 {
				continue
			}
			if info.IsDir() {
				walk(p)
			} else if info.Mode().IsRegular() {
				total += info.Size()
			}
		}
	}
	walk(root)
	return total
}
