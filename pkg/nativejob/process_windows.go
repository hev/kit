//go:build windows

package nativejob

import (
	"fmt"
	"strconv"
	"unsafe"

	"golang.org/x/sys/windows"
)

// processBirth uses a kernel boot identifier plus process creation FILETIME.
// PID alone, wall-clock-derived boot time and VM labels are not ownership proof.
// All unavailable/restricted kernel queries fail closed. Native Windows proof
// remains required; this adapter does not enable Windows journal execution.
func processBirth(pid int) (Birth, error) {
	if pid <= 0 || uint64(pid) > uint64(^uint32(0)) {
		return Birth{}, fmt.Errorf("invalid Windows PID")
	}
	var boot struct {
		ID       windows.GUID
		Firmware uint32
		Padding  uint32
		Flags    uint64
	}
	var size uint32
	if err := windows.NtQuerySystemInformation(windows.SystemBootEnvironmentInformation, unsafe.Pointer(&boot), uint32(unsafe.Sizeof(boot)), &size); err != nil {
		return Birth{}, fmt.Errorf("query Windows boot identity: %w", err)
	}
	if size < 16 || boot.ID == (windows.GUID{}) {
		return Birth{}, fmt.Errorf("missing Windows boot identity")
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return Birth{}, err
	}
	defer windows.CloseHandle(handle)
	state, err := windows.WaitForSingleObject(handle, 0)
	if err != nil {
		return Birth{}, err
	}
	if state != uint32(windows.WAIT_TIMEOUT) {
		return Birth{}, fmt.Errorf("Windows process not live")
	}
	var creation, exit, kernel, user windows.Filetime
	if err = windows.GetProcessTimes(handle, &creation, &exit, &kernel, &user); err != nil {
		return Birth{}, err
	}
	ticks := uint64(creation.HighDateTime)<<32 | uint64(creation.LowDateTime)
	if ticks == 0 {
		return Birth{}, fmt.Errorf("missing Windows process creation identity")
	}
	return Birth{BootID: boot.ID.String(), PID: pid, Start: strconv.FormatUint(ticks, 10)}, nil
}
