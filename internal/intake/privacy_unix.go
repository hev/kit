//go:build !windows

package intake

import "os"

func privateDirectory(path string) error { return os.Chmod(path, 0700) }
func privateOrder(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Mode().Perm() != 0600 {
		return os.ErrPermission
	}
	return nil
}
