package nativeinbox

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func processBirth(pid int) string {
	const zombie = 5 // SZOMB in the Darwin process state API.
	proc, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil || int(proc.Proc.P_pid) != pid || proc.Proc.P_stat == zombie {
		return ""
	}
	start := proc.Proc.P_starttime
	return fmt.Sprintf("%d.%d", start.Sec, start.Usec)
}
