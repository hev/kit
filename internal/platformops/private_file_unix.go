//go:build !windows

package platformops

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

func openPrivateFile(path string) (*os.File, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	dir, err := unix.Open("/", unix.O_DIRECTORY|unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	defer func() { unix.Close(dir) }()
	uid := uint32(os.Geteuid())
	for _, component := range parts[:len(parts)-1] {
		next, err := unix.Openat(dir, component, unix.O_DIRECTORY|unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			return nil, err
		}
		unix.Close(dir)
		dir = next
		var stat unix.Stat_t
		if err := unix.Fstat(dir, &stat); err != nil || (stat.Uid != uid && stat.Uid != 0) ||
			(stat.Mode&0022 != 0 && stat.Mode&unix.S_ISVTX == 0) {
			return nil, errPrivateFile
		}
	}
	// Nonblocking open prevents a FIFO planted at the credential path from
	// hanging startup. Authorization and reads use this same descriptor.
	fd, err := unix.Openat(dir, parts[len(parts)-1], unix.O_NONBLOCK|unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG ||
		stat.Uid != uid || stat.Mode&0077 != 0 {
		unix.Close(fd)
		return nil, errPrivateFile
	}
	return os.NewFile(uintptr(fd), "protected file"), nil
}
