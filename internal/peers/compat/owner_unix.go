//go:build !windows

package compat

import (
	"os"
	"syscall"
)

func ownedByMe(path string) bool {
	st, err := os.Stat(path)
	if err != nil {
		return false
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	return ok && int(sys.Uid) == os.Getuid()
}
