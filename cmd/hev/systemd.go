package main

import (
	"bytes"
	"debug/buildinfo"
	"errors"
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
	if err != nil || bus != nil {
		return false
	}
	return exec.Command("systemctl", "--user", "show-environment").Run() == nil
}
func (systemctlService) Run(args ...string) ([]byte, error) {
	return exec.Command("systemctl", append([]string{"--user"}, args...)...).CombinedOutput()
}

var linuxService userService = systemctlService{}

func hasSystemd() bool { return hostOS == "linux" && linuxService.Available() }

const daemonUnit = "hevd.service"
const managedUnitMarker = "# Managed by hev up"

var errUnmanaged = errors.New("operator-managed installation")

// Refuse all lifecycle mutations if any component belongs to the operator.
func systemdOwnership(home string) error {
	path := systemdPath(home)
	if _, err := os.Lstat(path + ".d"); err == nil {
		return fmt.Errorf("%w: drop-in directory %s.d", errUnmanaged, path)
	} else if !os.IsNotExist(err) {
		return err
	}
	if info, err := os.Lstat(path); err == nil {
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if info.Mode()&os.ModeSymlink != 0 || !bytes.HasPrefix(raw, []byte(managedUnitMarker+"\n")) {
			return fmt.Errorf("%w: unit %s", errUnmanaged, path)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	bin := filepath.Join(home, ".local", "bin", "hev")
	if _, err := os.Lstat(bin); err == nil {
		info, readErr := buildinfo.ReadFile(bin)
		if readErr != nil || info.Path != "github.com/hev/kit/cmd/hev" {
			return fmt.Errorf("%w: binary or wrapper %s", errUnmanaged, bin)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}
func downSystemd(home string) error {
	if err := systemdOwnership(home); err != nil {
		return err
	}
	if _, err := os.Stat(systemdPath(home)); os.IsNotExist(err) {
		return fmt.Errorf("%w: no installed unit; stop `hev d` yourself", errUnmanaged)
	} else if err != nil {
		return err
	}
	return systemdRun("disable", "--now", daemonUnit)
}

func systemdPath(home string) string {
	config := os.Getenv("XDG_CONFIG_HOME")
	if config == "" {
		config = filepath.Join(home, ".config")
	}
	return filepath.Join(config, "systemd", "user", daemonUnit)
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
	if err := systemdOwnership(home); err != nil {
		return err
	}
	path := systemdPath(home)
	unit := managedUnitMarker + "\n[Unit]\nDescription=hev trace capture\nAfter=network.target\n\n[Service]\nType=simple\nExecStart=" + systemdQuote(bin) + " daemon\nEnvironment=" + strconv.Quote("HEV_CONFIG="+strings.ReplaceAll(config, "%", "%%")) + "\nEnvironment=" + strconv.Quote("PATH="+strings.ReplaceAll(os.Getenv("PATH"), "%", "%%")) + "\nRestart=on-failure\nRestartSec=5\n\n[Install]\nWantedBy=default.target\n"
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
	if err := systemdOwnership(home); err != nil {
		return err
	}
	bin, changed, err := installSelf(home)
	if err != nil {
		return err
	}
	return ensureSystemd(home, bin, config, restart || changed)
}
