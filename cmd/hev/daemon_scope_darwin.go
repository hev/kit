//go:build darwin

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// macOS can't safely read another process's environment, so scope comes from
// the service manager: a daemon is ours when it is the PID launchd reports for
// this user's hevd job, or a descendant of it (a launcher script may exec or
// spawn the daemon), and that job's plist lives under this HOME, so a run with
// a temporary HOME never matches the real daemon. Never signal a process found
// only by a host-wide name scan.
func daemonInScope(pid int) bool {
	root, plist := launchdJobInfo(launchdJob{Label: launchdLabel()}.target())
	home, err := os.UserHomeDir()
	if root <= 0 || err != nil || !strings.HasPrefix(plist, filepath.Join(home, "Library", "LaunchAgents")+string(filepath.Separator)) {
		return false
	}
	for p, depth := pid, 0; p > 1 && depth < 4; depth++ {
		if p == root {
			return true
		}
		p = parentPID(p)
	}
	return false
}

// launchdJobInfo returns the job's running PID and its plist path from
// `launchctl print`, or zero and "" when the job isn't loaded.
func launchdJobInfo(target string) (pid int, plist string) {
	out, err := exec.Command("launchctl", "print", target).Output()
	if err != nil {
		return 0, ""
	}
	return parseLaunchctlPrint(string(out))
}

func parseLaunchctlPrint(out string) (pid int, plist string) {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, "pid = "); ok && pid == 0 {
			pid, _ = strconv.Atoi(strings.TrimSpace(v))
		}
		if v, ok := strings.CutPrefix(line, "path = "); ok && plist == "" {
			plist = strings.TrimSpace(v)
		}
	}
	return pid, plist
}

func parentPID(pid int) int {
	out, err := exec.Command("ps", "-o", "ppid=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0
	}
	return n
}
