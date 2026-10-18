//go:build linux || darwin

package nativegate

import (
	"os"
	"syscall"
)

func ownedByCurrentUser(st os.FileInfo) bool {
	s, ok := st.Sys().(*syscall.Stat_t)
	return ok && s.Uid == uint32(os.Geteuid())
}
