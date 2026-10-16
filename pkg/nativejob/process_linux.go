//go:build linux

package nativejob

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

func processBirth(pid int) (Birth, error) {
	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return Birth{}, err
	}
	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return Birth{}, err
	}
	// comm may contain spaces and parentheses; fields after its final ')' start
	// at stat field 3. Birth is field 22 (starttime), never PID alone.
	text := string(stat)
	end := strings.LastIndex(text, ")")
	if end < 0 {
		return Birth{}, fmt.Errorf("invalid proc stat")
	}
	fields := strings.Fields(text[end+1:])
	if len(fields) < 20 {
		return Birth{}, fmt.Errorf("short proc stat")
	}
	if fields[0] == "Z" || fields[0] == "X" {
		return Birth{}, fmt.Errorf("process exited")
	}
	if _, err = strconv.ParseUint(fields[19], 10, 64); err != nil {
		return Birth{}, err
	}
	return Birth{BootID: strings.TrimSpace(string(boot)), PID: pid, Start: fields[19]}, nil
}
