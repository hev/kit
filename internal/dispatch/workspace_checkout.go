package dispatch

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/hev/kit/internal/platformops"
)

type workspaceCloneDriver func(src, dst string) bool

func cloneWorkspaceTree(src, dst string) bool {
	var args []string
	switch runtime.GOOS {
	case "darwin":
		args = []string{"-cR", "-P"}
	case "linux":
		args = []string{"-a", "--reflink=always"}
	default:
		return false
	}
	_, err := command("", append(append([]string{"cp"}, args...), src+string(filepath.Separator)+".", dst)...)
	return err == nil
}

func addDispatchWorkspace(gitArgs []string, head string, cow bool) error {
	plain := func() error {
		_, err := command("", append([]string{"git"}, gitArgs...)...)
		return err
	}
	if !cow || (runtime.GOOS != "linux" && runtime.GOOS != "darwin") {
		return plain()
	}
	repo, wt := gitArgs[1], gitArgs[len(gitArgs)-2]
	pool, err := workspacePool(repo)
	if err != nil || os.MkdirAll(pool, 0700) != nil {
		return plain()
	}
	release, err := platformops.LockPath(filepath.Join(pool, "warm.lock"), true)
	if err != nil {
		return plain()
	}
	defer release()
	warm := filepath.Join(pool, "warm")
	stamp := filepath.Join(pool, "warm-head")
	if stored, _ := os.ReadFile(stamp); string(stored) == head {
		if used, err := seedDispatchWorkspace(gitArgs, wt, head, warm, cloneWorkspaceTree); used || err != nil {
			return err
		}
	}
	if err := plain(); err != nil {
		return err
	}
	seedWarmWorkspace(pool, warm, stamp, wt, head)
	return nil
}

func seedWarmWorkspace(pool, warm, stamp, wt, head string) {
	stage, err := os.MkdirTemp(pool, "warm-build-")
	if err != nil {
		return
	}
	defer os.RemoveAll(stage)
	probe, err := os.MkdirTemp(filepath.Dir(wt), "warm-probe-")
	if err != nil {
		return
	}
	defer os.RemoveAll(probe)
	if os.WriteFile(filepath.Join(stage, "probe"), []byte("workspace"), 0600) != nil || !cloneWorkspaceTree(stage, probe) {
		return
	}
	if os.Remove(filepath.Join(stage, "probe")) != nil {
		return
	}
	tree, err := command(wt, "git", "rev-parse", head+"^{tree}")
	indexTree, indexErr := command(wt, "git", "write-tree")
	if err != nil || indexErr != nil || tree != indexTree {
		return
	}
	// Checkout only tracked files. Do not copy ignored files, credentials,
	// or the worktree's Git administration into the reusable seed.
	if _, err := command(wt, "git", "checkout-index", "--all", "--prefix="+stage+string(filepath.Separator)); err != nil {
		return
	}
	_ = os.Remove(stamp)
	if os.RemoveAll(warm) != nil || os.Rename(stage, warm) != nil {
		return
	}
	_ = os.WriteFile(stamp, []byte(head), 0600)
}

func seedDispatchWorkspace(gitArgs []string, wt, head, warm string, clone workspaceCloneDriver) (bool, error) {
	if info, err := os.Lstat(warm); err != nil || !info.IsDir() {
		return false, nil
	}
	stage, err := os.MkdirTemp(filepath.Dir(wt), "workspace-seed-")
	if err != nil {
		return false, nil
	}
	defer os.RemoveAll(stage)
	if !clone(warm, stage) {
		return false, nil
	}
	entries, err := os.ReadDir(stage)
	if err != nil {
		return false, nil
	}
	for _, entry := range entries {
		if strings.EqualFold(entry.Name(), ".git") {
			return false, nil
		}
	}
	args := append([]string{"git"}, gitArgs[:4]...)
	args = append(append(args, "--no-checkout"), gitArgs[4:]...)
	if _, err := command("", args...); err != nil {
		return true, fmt.Errorf("cannot register seeded Git worktree")
	}
	for _, entry := range entries {
		if err := os.Rename(filepath.Join(stage, entry.Name()), filepath.Join(wt, entry.Name())); err != nil {
			return true, fmt.Errorf("cannot install workspace seed")
		}
	}
	if _, err := command(wt, "git", "reset", "--mixed", head); err != nil {
		return true, fmt.Errorf("cannot initialize workspace index")
	}
	status, err := command(wt, "git", "status", "--porcelain=v1", "--untracked-files=all", "--ignored=matching")
	if err != nil || status != "" {
		// A stale seed is cache data. Reconstruct the checkout from Git.
		for _, entry := range entries {
			if err := os.RemoveAll(filepath.Join(wt, entry.Name())); err != nil {
				return true, fmt.Errorf("cannot discard workspace seed")
			}
		}
		if _, err := command(wt, "git", "reset", "--hard", head); err != nil {
			return true, fmt.Errorf("cannot check out workspace base")
		}
	}
	null := strings.Repeat("0", len(head))
	if _, err := command(wt, "git", "hook", "run", "--ignore-missing", "post-checkout", "--", null, head, "1"); err != nil {
		return true, fmt.Errorf("workspace post-checkout hook failed")
	}
	return true, nil
}
