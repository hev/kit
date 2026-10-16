//go:build darwin

package nativejob

import (
	"fmt"
	"strings"

	"golang.org/x/sys/unix"
)

// processBirth binds a process to the kernel's boot-session UUID and its kernel
// creation timeval. Neither PID nor estimated boot wall time proves ownership.
// The x/sys platform layouts cover both Darwin amd64 and arm64. Unavailable or
// restricted kernel inspection fails closed, with no PID-only fallback.
func processBirth(pid int) (Birth, error) {
	if pid <= 0 || uint64(pid) > uint64(^uint32(0)>>1) {
		return Birth{}, fmt.Errorf("invalid Darwin PID")
	}
	boot, err := unix.Sysctl("kern.bootsessionuuid")
	if err != nil {
		return Birth{}, fmt.Errorf("Darwin boot identity: %w", err)
	}
	boot = strings.TrimSpace(boot)
	if boot == "" || strings.Trim(boot, "0-") == "" {
		return Birth{}, fmt.Errorf("missing Darwin boot identity")
	}
	info, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return Birth{}, fmt.Errorf("Darwin process identity: %w", err)
	}
	if info == nil || int(info.Proc.P_pid) != pid || info.Proc.P_stat == 0 || info.Proc.P_stat == 5 {
		return Birth{}, fmt.Errorf("Darwin process missing or exited")
	}
	start := info.Proc.P_starttime
	if start.Sec <= 0 || start.Usec < 0 || start.Usec >= 1000000 {
		return Birth{}, fmt.Errorf("missing Darwin process creation identity")
	}
	return Birth{BootID: boot, PID: pid, Start: fmt.Sprintf("%d:%06d", start.Sec, start.Usec)}, nil
}
