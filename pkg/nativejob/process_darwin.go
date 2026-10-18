//go:build darwin

package nativejob

import (
	"errors"
	"fmt"
	"os"
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
	// The scalar x/sys wrapper turns an empty kernel reply into EIO. The
	// slice form preserves a successful empty PID lookup, which can be checked
	// independently without converting permission/inspection errors into Gone.
	infos, err := unix.SysctlKinfoProcSlice("kern.proc.pid", pid)
	if err != nil {
		return Birth{}, fmt.Errorf("Darwin process identity: %w", err)
	}
	if len(infos) == 0 {
		return Birth{}, darwinAbsenceError(unix.Kill(pid, 0))
	}
	if len(infos) != 1 {
		return Birth{}, fmt.Errorf("ambiguous Darwin process identity")
	}
	info := &infos[0]
	if info == nil || int(info.Proc.P_pid) != pid || info.Proc.P_stat == 0 || info.Proc.P_stat == 5 {
		return Birth{}, fmt.Errorf("Darwin process missing or exited")
	}
	start := info.Proc.P_starttime
	if start.Sec <= 0 || start.Usec < 0 || start.Usec >= 1000000 {
		return Birth{}, fmt.Errorf("missing Darwin process creation identity")
	}
	return Birth{BootID: boot, PID: pid, Start: fmt.Sprintf("%d:%06d", start.Sec, start.Usec)}, nil
}

// Signal zero observes existence only; it neither adopts nor terminates a PID.
// Only ESRCH after a successful empty kernel lookup establishes absence.
func darwinAbsenceError(existenceError error) error {
	if errors.Is(existenceError, unix.ESRCH) {
		return fmt.Errorf("Darwin process absent: %w", os.ErrNotExist)
	}
	return fmt.Errorf("Darwin process absence unverified")
}
