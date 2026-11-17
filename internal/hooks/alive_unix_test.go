//go:build !windows

package hooks

import (
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func processAlive(pid string) bool {
	n, err := strconv.Atoi(pid)
	if err != nil || syscall.Kill(n, 0) != nil {
		return false
	}
	// A killed child may linger as a zombie until its parent reaps it.
	out, _ := exec.Command("ps", "-o", "stat=", "-p", pid).Output()
	return !strings.HasPrefix(strings.TrimSpace(string(out)), "Z")
}
