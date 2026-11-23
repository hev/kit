package nativeinbox

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func processBirth(pid int) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	var created, exited, kernel, user windows.Filetime
	if result, err := windows.WaitForSingleObject(h, 0); err != nil || result != uint32(windows.WAIT_TIMEOUT) {
		return ""
	}
	if windows.GetProcessTimes(h, &created, &exited, &kernel, &user) != nil {
		return ""
	}
	return fmt.Sprintf("%d.%d", created.HighDateTime, created.LowDateTime)
}
