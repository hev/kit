package update

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// renameSwap is how a link is switched on Windows, where a running executable
// cannot be overwritten or deleted but can be renamed: the running file is
// renamed aside and a copy of the new build is renamed into its place. On
// Unix the link becomes a symbolic link to the versioned build, swapped in
// with one rename, so the name never points at nothing.
var renameSwap = runtime.GOOS == "windows"

const symlinkBack = "symlink:"

// switchLink points link at bin and returns what restoreLink needs to put
// the old one back.
func switchLink(link, bin string) (string, error) {
	if renameSwap {
		return swapByRename(link, bin)
	}
	return swapSymlink(link, bin)
}

func restoreLink(link, back string) error {
	if strings.HasPrefix(back, symlinkBack) {
		return atomicSymlink(strings.TrimPrefix(back, symlinkBack), link)
	}
	if back == "" {
		return os.Remove(link)
	}
	if renameSwap {
		aside := fmt.Sprintf("%s.discard-%d", link, time.Now().UnixNano())
		if err := os.Rename(link, aside); err != nil && !os.IsNotExist(err) {
			return err
		}
		defer os.Remove(aside)
	}
	return os.Rename(back, link)
}

// discardBackup removes a backup once every link has switched. A Windows
// backup that is still running stays until a later switch removes it.
func discardBackup(back string) {
	if back != "" && !strings.HasPrefix(back, symlinkBack) {
		_ = os.Remove(back)
	}
}

func swapSymlink(link, bin string) (string, error) {
	back := ""
	if info, err := os.Lstat(link); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(link)
			if err != nil {
				return "", err
			}
			back = symlinkBack + target
		} else {
			// Keep the old file under another name until every link has
			// switched, so a failure part-way can put it back. A hard link
			// costs nothing and leaves the running inode untouched.
			back = link + ".hev-prev"
			_ = os.Remove(back)
			if err := os.Link(link, back); err != nil {
				if err := copyExecutable(link, back); err != nil {
					return "", err
				}
			}
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := atomicSymlink(bin, link); err != nil {
		discardBackup(back)
		return "", err
	}
	return back, nil
}

// atomicSymlink makes link a symbolic link to target by creating it under a
// temporary name in the same directory and renaming it over link.
func atomicSymlink(target, link string) error {
	tmp := fmt.Sprintf("%s.hev-link-%d", link, time.Now().UnixNano())
	if err := os.Symlink(target, tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, link); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func swapByRename(link, bin string) (string, error) {
	// Builds set aside by earlier switches that are no longer running.
	if old, _ := filepath.Glob(link + ".old-*"); len(old) > 0 {
		for _, o := range old {
			_ = os.Remove(o)
		}
	}
	next := link + ".new"
	_ = os.Remove(next)
	if err := copyExecutable(bin, next); err != nil {
		return "", err
	}
	back := ""
	if _, err := os.Lstat(link); err == nil {
		back = fmt.Sprintf("%s.old-%d", link, time.Now().UnixNano())
		if err := os.Rename(link, back); err != nil {
			os.Remove(next)
			return "", fmt.Errorf("move the running build aside: %w", err)
		}
	}
	if err := os.Rename(next, link); err != nil {
		if back != "" {
			_ = os.Rename(back, link)
		}
		os.Remove(next)
		return "", err
	}
	return back, nil
}
