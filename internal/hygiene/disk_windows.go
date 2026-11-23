//go:build windows

package hygiene

import (
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// DiskFreeBytes returns available bytes for the current user on the volume containing path.
func DiskFreeBytes(path string) (uint64, error) {
	probe := existingAncestor(path)
	ptr, err := windows.UTF16PtrFromString(probe)
	if err != nil {
		return 0, errors.New("could not inspect disk free space")
	}
	var freeBytesAvailable uint64
	if err := windows.GetDiskFreeSpaceEx(ptr, &freeBytesAvailable, nil, nil); err != nil {
		return 0, errors.New("could not inspect disk free space")
	}
	return freeBytesAvailable, nil
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
