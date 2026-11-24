//go:build !windows

package nodes

import (
	"os"
	"path/filepath"
)

func secure(path string, _ bool) error {
	if err := os.Chmod(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.Chmod(path, 0600)
}
