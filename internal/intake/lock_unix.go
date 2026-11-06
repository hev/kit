//go:build !windows

package intake

import (
	"fmt"
	"os"
	"syscall"
)

func lockState(path string) (func(), error) {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		f.Close()
		return nil, fmt.Errorf("intake already running: %w", e)
	}
	return func() { f.Close() }, nil
}
func replaceState(from, to string) error { return os.Rename(from, to) }
