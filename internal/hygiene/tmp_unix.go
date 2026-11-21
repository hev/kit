//go:build !windows

package hygiene

import (
	"os"
	"syscall"
)

func isOwnedByCurrentUser(info os.FileInfo) bool {
	if info == nil {
		return false
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st == nil {
		return false
	}
	return int(st.Uid) == os.Getuid()
}
