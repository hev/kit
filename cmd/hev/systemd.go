package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// userService keeps every systemctl call replaceable in platform-independent tests.
type userService interface {
	Available() bool
	Run(args ...string) ([]byte, error)
}
type systemctlService struct{}

func (systemctlService) Available() bool {
	_, err := exec.LookPath("systemctl")
	_, bus := os.Stat("/run/systemd/system")
	return err == nil && bus == nil
}
func (systemctlService) Run(args ...string) ([]byte, error) {
	return exec.Command("systemctl", append([]string{"--user"}, args...)...).CombinedOutput()
}

var linuxService userService = systemctlService{}

func hasSystemd() bool { return hostOS == "linux" && linuxService.Available() }

const daemonUnit = "hevd.service"

func systemdPath(home string) string {
	return filepath.Join(home, ".config", "systemd", "user", daemonUnit)
}
func systemdRun(args ...string) error {
	out, err := linuxService.Run(args...)
	if err != nil {
		return fmt.Errorf("systemctl --user %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(string(out)), err)
	}
	return nil
}
func systemdQuote(s string) string {
	return strconv.Quote(strings.NewReplacer("%", "%%", "$", "$$").Replace(s))
}
func ensureSystemd(home, bin, config string, restart bool) error {
	path := systemdPath(home)
	unit := "[Unit]\nDescription=hev trace capture\nAfter=network.target\n\n[Service]\nType=simple\nExecStart=" + systemdQuote(bin) + " daemon\nEnvironment=" + strconv.Quote("HEV_CONFIG="+strings.ReplaceAll(config, "%", "%%")) + "\nRestart=on-failure\nRestartSec=5\n\n[Install]\nWantedBy=default.target\n"
	previous, _ := os.ReadFile(path)
	changed := !bytes.Equal(previous, []byte(unit))
	if changed {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(unit), 0644); err != nil {
			return err
		}
		if err := systemdRun("daemon-reload"); err != nil {
			return err
		}
	}
	if err := systemdRun("enable", "--now", daemonUnit); err != nil {
		return err
	}
	if restart || changed {
		return systemdRun("restart", daemonUnit)
	}
	return nil
}
func upSystemd(home, config string, restart bool) error {
	bin, changed, err := installSelf(home)
	if err != nil {
		return err
	}
	return ensureSystemd(home, bin, config, restart || changed)
}
