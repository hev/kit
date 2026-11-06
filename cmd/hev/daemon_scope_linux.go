package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// Fail closed if ownership or the process environment cannot be inspected.
func daemonInScope(pid int) bool {
	root := filepath.Join("/proc", strconv.Itoa(pid))
	st, err := os.Stat(root)
	if err != nil {
		return false
	}
	stat, ok := st.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Getuid()) {
		return false
	}
	b, err := os.ReadFile(filepath.Join(root, "environ"))
	if err != nil {
		return false
	}
	return daemonEnvironmentInScope(strings.Split(string(b), "\x00"))
}
