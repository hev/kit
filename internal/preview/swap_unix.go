//go:build !windows

package preview

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/hev/kit/internal/platformops"
)

// swapCurrent atomically replaces <previewDir>/current with a relative symlink
// to builds/<buildID>.
func swapCurrent(previewDir, buildID string) error {
	target := filepath.Join("builds", buildID)
	var nonce [8]byte
	_, _ = rand.Read(nonce[:])
	tmp := filepath.Join(previewDir, ".current.tmp-"+hex.EncodeToString(nonce[:]))
	if err := os.Symlink(target, tmp); err != nil {
		return err
	}
	current := filepath.Join(previewDir, "current")
	if err := os.Rename(tmp, current); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	dir, err := os.Open(previewDir)
	if err != nil {
		return err
	}
	defer dir.Close()
	return platformops.SyncDir(dir)
}

// resolveCurrent resolves <previewDir>/current to its build directory and ID.
func resolveCurrent(previewDir string) (dir string, buildID string, err error) {
	current := filepath.Join(previewDir, "current")
	info, err := os.Lstat(current)
	if err != nil {
		return "", "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(current)
		if err != nil {
			return "", "", err
		}
		if filepath.IsAbs(target) {
			return "", "", errors.New("current symlink must be relative")
		}
		resolved := filepath.Clean(filepath.Join(previewDir, target))
		buildsDir := filepath.Join(previewDir, "builds")
		rel, err := filepath.Rel(buildsDir, resolved)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || strings.ContainsRune(rel, filepath.Separator) {
			return "", "", errors.New("current symlink escapes builds directory")
		}
		if st, err := os.Stat(resolved); err != nil || !st.IsDir() {
			return "", "", os.ErrNotExist
		}
		return resolved, rel, nil
	}
	if !info.IsDir() {
		return "", "", os.ErrNotExist
	}
	return current, "", nil
}
