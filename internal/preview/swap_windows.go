//go:build windows

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

const windowsTargetFile = "current.target"

// swapCurrent updates the active build on Windows without requiring symlink
// privileges. current.target is replaced atomically via WriteFileAtomic so
// concurrent readers always see a complete build directory; current is also
// updated (via symlink when permitted, else directory copy swap).
func swapCurrent(previewDir, buildID string) error {
	targetRel := "builds/" + buildID
	targetPath := filepath.Join(previewDir, windowsTargetFile)
	if err := platformops.WriteFileAtomic(targetPath, []byte(targetRel+"\n"), 0600); err != nil {
		return err
	}

	current := filepath.Join(previewDir, "current")
	buildDir := filepath.Join(previewDir, "builds", buildID)

	// Clean any old backup directories left from earlier swaps.
	if old, _ := filepath.Glob(filepath.Join(previewDir, ".current.old-*")); len(old) > 0 {
		for _, o := range old {
			_ = os.RemoveAll(o)
		}
	}

	var nonce [8]byte
	_, _ = rand.Read(nonce[:])
	tag := hex.EncodeToString(nonce[:])

	// Try symlink first (works with Developer Mode or elevated privileges).
	tmpLink := filepath.Join(previewDir, ".current.link-"+tag)
	if err := os.Symlink(filepath.Join("builds", buildID), tmpLink); err == nil {
		if _, err := os.Lstat(current); err == nil {
			old := filepath.Join(previewDir, ".current.old-"+tag)
			if err := os.Rename(current, old); err == nil {
				defer os.RemoveAll(old)
			}
		}
		if err := os.Rename(tmpLink, current); err == nil {
			return nil
		}
		_ = os.Remove(tmpLink)
	}

	// Fallback: copy into staging sibling and swap directory names.
	nextDir := filepath.Join(previewDir, ".current.next-"+tag)
	if err := copyDirContained(buildDir, nextDir); err != nil {
		_ = os.RemoveAll(nextDir)
		return err
	}
	var oldDir string
	if _, err := os.Lstat(current); err == nil {
		oldDir = filepath.Join(previewDir, ".current.old-"+tag)
		if err := os.Rename(current, oldDir); err != nil {
			_ = os.RemoveAll(nextDir)
			return err
		}
	}
	if err := os.Rename(nextDir, current); err != nil {
		if oldDir != "" {
			_ = os.Rename(oldDir, current)
		}
		_ = os.RemoveAll(nextDir)
		return err
	}
	if oldDir != "" {
		_ = os.RemoveAll(oldDir)
	}
	return nil
}

// resolveCurrent resolves the active build on Windows, preferring the
// atomically-written current.target pointer over current.
func resolveCurrent(previewDir string) (dir string, buildID string, err error) {
	if raw, err := os.ReadFile(filepath.Join(previewDir, windowsTargetFile)); err == nil {
		rel := strings.TrimSpace(string(raw))
		if id, ok := strings.CutPrefix(rel, "builds/"); ok && id != "" && !strings.ContainsAny(id, `/\`) && id != "." && id != ".." {
			candidate := filepath.Join(previewDir, "builds", id)
			if st, err := os.Stat(candidate); err == nil && st.IsDir() {
				return candidate, id, nil
			}
		}
	}
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
		resolved := filepath.Clean(filepath.Join(previewDir, target))
		buildsDir := filepath.Join(previewDir, "builds")
		rel, err := filepath.Rel(buildsDir, resolved)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
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
