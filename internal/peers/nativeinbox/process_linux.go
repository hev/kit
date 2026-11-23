package nativeinbox

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

// The boot ID prevents a retained registration from matching the same PID
// and start tick after a reboot.
var bootID = sync.OnceValue(func() string {
	data, _ := os.ReadFile("/proc/sys/kernel/random/boot_id")
	return strings.TrimSpace(string(data))
})

func processBirth(pid int) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return ""
	}
	index := strings.LastIndexByte(string(data), ')')
	fields := strings.Fields(string(data[index+1:]))
	// After the command, field 3 is state and field 22 is starttime.
	if index < 0 || len(fields) <= 19 || fields[0] == "Z" || fields[0] == "X" {
		return ""
	}
	return bootID() + ":" + fields[19]
}
