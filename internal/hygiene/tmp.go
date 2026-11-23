package hygiene

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hev/kit/internal/platformops"
)

// TmpAction describes one candidate temporary directory inspected by hygiene.
type TmpAction struct {
	Name       string `json:"name"`
	Action     string `json:"action"` // "pruned", "would_prune", "kept"
	Reason     string `json:"reason,omitempty"`
	BytesFreed int64  `json:"bytes_freed,omitempty"`
}

// TmpReport summarizes temporary directory pruning.
type TmpReport struct {
	Pruned     []string    `json:"pruned"`
	Kept       []TmpAction `json:"kept,omitempty"`
	Actions    []TmpAction `json:"actions,omitempty"`
	BytesFreed int64       `json:"bytes_freed"`
}

func matchesManagedTmpName(name string) bool {
	return strings.HasPrefix(name, "tmp.") ||
		strings.HasPrefix(name, "hev-") ||
		strings.HasPrefix(name, "go-build")
}

// PruneTmpDirs prunes stale current-UID-owned tmp.*, hev-*, and go-build*
// directories directly under tmpDir. Symlinks, non-matching names, other users'
// directories, recent directories, and directories with active lock files are kept.
func PruneTmpDirs(tmpDir string, olderThan time.Duration, now time.Time, dryRun bool, owned func(os.FileInfo) bool) (TmpReport, error) {
	rep := TmpReport{Pruned: []string{}}
	if tmpDir == "" {
		return rep, nil
	}
	if owned == nil {
		owned = isOwnedByCurrentUser
	}
	if now.IsZero() {
		now = time.Now()
	}

	entries, err := os.ReadDir(tmpDir)
	if os.IsNotExist(err) {
		return rep, nil
	}
	if err != nil {
		return rep, errors.New("cannot list temporary directory")
	}

	recordKept := func(name, reason string) {
		act := TmpAction{Name: name, Action: "kept", Reason: reason}
		rep.Kept = append(rep.Kept, act)
		rep.Actions = append(rep.Actions, act)
	}

	for _, entry := range entries {
		name := entry.Name()
		if !matchesManagedTmpName(name) {
			continue
		}
		full := filepath.Join(tmpDir, name)
		info, err := os.Lstat(full)
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			recordKept(name, "not a regular directory")
			continue
		}
		if !owned(info) {
			recordKept(name, "not owned by current user")
			continue
		}
		if now.Sub(info.ModTime()) < olderThan {
			recordKept(name, "newer than prune_tmp_after")
			continue
		}
		if reason := inspectTmpDirContents(full, olderThan, now); reason != "" {
			recordKept(name, reason)
			continue
		}

		size := dirSizeBounded(full, 100000)
		if dryRun {
			act := TmpAction{Name: name, Action: "would_prune", BytesFreed: size}
			rep.Pruned = append(rep.Pruned, name)
			rep.Actions = append(rep.Actions, act)
			rep.BytesFreed += size
			continue
		}

		if err := os.RemoveAll(full); err != nil {
			recordKept(name, "cannot remove temporary directory")
			continue
		}
		act := TmpAction{Name: name, Action: "pruned", BytesFreed: size}
		rep.Pruned = append(rep.Pruned, name)
		rep.Actions = append(rep.Actions, act)
		rep.BytesFreed += size
	}

	return rep, nil
}

func inspectTmpDirContents(dir string, olderThan time.Duration, now time.Time) string {
	children, err := os.ReadDir(dir)
	if err != nil {
		return "cannot inspect directory contents"
	}
	for _, child := range children {
		p := filepath.Join(dir, child.Name())
		info, err := os.Lstat(p)
		if err != nil {
			return "cannot inspect directory entry"
		}
		if now.Sub(info.ModTime()) < olderThan {
			return "contains recent entry"
		}
		if info.Mode().IsRegular() && isLockFileName(child.Name()) {
			if isFileActivelyLocked(p) {
				return "contains active lock"
			}
		}
	}
	return ""
}

func isLockFileName(name string) bool {
	return name == "lock" || strings.HasSuffix(name, ".lock")
}

func isFileActivelyLocked(path string) bool {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		// Fail closed if the lock file cannot be opened.
		return true
	}
	defer f.Close()
	if err := platformops.LockFile(f, true); err != nil {
		return true
	}
	return false
}

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
