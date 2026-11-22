//go:build !windows

package hygiene

import (
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// DiskFreeBytes returns available bytes for unprivileged users on the filesystem containing path.
func DiskFreeBytes(path string) (uint64, error) {
	probe := existingAncestor(path)
	var st unix.Statfs_t
	if err := unix.Statfs(probe, &st); err != nil {
		return 0, errors.New("could not inspect disk free space")
	}
	if st.Bsize <= 0 {
		return 0, errors.New("invalid filesystem block size")
	}
	return uint64(st.Bavail) * uint64(st.Bsize), nil
}

func existingAncestor(path string) string {
	cur := filepath.Clean(path)
	for {
		if _, err := os.Stat(cur); err == nil {
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return cur
		}
		cur = parent
	}
}
