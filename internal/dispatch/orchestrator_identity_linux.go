package dispatch

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Infer identity from the recorded harness process, even if a descendant clears
// its environment. This is defense in depth, not an OS sandbox.
func roundProcessIdentity() string {
	ancestors := map[int]bool{}
	for pid := os.Getpid(); pid > 1 && !ancestors[pid]; {
		ancestors[pid] = true
		if name := roundIdentityForAncestors(map[int]bool{pid: true}); name != "" {
			return name
		}
		b, e := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
		if e != nil {
			break
		}
		end := strings.LastIndexByte(string(b), ')')
		if end < 0 {
			break
		}
		fields := strings.Fields(string(b[end+1:]))
		if len(fields) < 2 {
			break
		}
		pid, _ = strconv.Atoi(fields[1])
	}
	return ""
}
