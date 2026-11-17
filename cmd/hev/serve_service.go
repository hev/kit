package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hev/kit/internal/update"
)

// With no key the dashboard is `hev serve` over the file archive, run by the
// user's service manager next to hevd: a systemd user unit on Linux, a
// LaunchAgent on macOS. It listens on 127.0.0.1:[local] serve_port. Nothing
// runs in Docker.

const serveUnit = "hev-serve.service"

func serveUnitPath(home string) string {
	return filepath.Join(filepath.Dir(systemdPath(home)), serveUnit)
}

// serveUnitOwnership refuses to touch a dashboard unit the operator wrote.
func serveUnitOwnership(home string) error {
	path := serveUnitPath(home)
	if _, err := os.Lstat(path + ".d"); err == nil {
		return fmt.Errorf("%w: drop-in directory %s.d", errUnmanaged, path)
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !bytes.HasPrefix(raw, []byte(managedUnitMarker+"\n")) {
		return fmt.Errorf("%w: unit %s", errUnmanaged, path)
	}
	return nil
}

func serveArgs(port int) []string {
	return []string{"serve", "--bind", "127.0.0.1", "--port", strconv.Itoa(port)}
}

func ensureServeSystemd(home, bin, config string, port int, restart bool) error {
	if err := serveUnitOwnership(home); err != nil {
		return err
	}
	path := serveUnitPath(home)
	exec := systemdQuote(bin)
	for _, a := range serveArgs(port) {
		exec += " " + systemdQuote(a)
	}
	unit := managedUnitMarker + "\n[Unit]\nDescription=hev dashboard (hev serve over the trace archive)\nAfter=network.target\n\n[Service]\nType=simple\nExecStart=" + exec +
		"\nEnvironment=" + strconv.Quote("HEV_CONFIG="+strings.ReplaceAll(config, "%", "%%")) +
		"\nEnvironment=" + strconv.Quote("PATH="+strings.ReplaceAll(os.Getenv("PATH"), "%", "%%")) +
		"\nRestart=on-failure\nRestartSec=5\n\n[Install]\nWantedBy=default.target\n"
	previous, _ := os.ReadFile(path)
	changed := !bytes.Equal(previous, []byte(unit))
	if changed {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(unit), 0o644); err != nil {
			return err
		}
		if err := systemdRun("daemon-reload"); err != nil {
			return err
		}
	}
	if err := systemdRun("enable", "--now", serveUnit); err != nil {
		return err
	}
	if restart || changed {
		return systemdRun("restart", serveUnit)
	}
	return nil
}

// downServeSystemd stops and removes the dashboard unit `hev up` installed.
func downServeSystemd(home string) (bool, error) {
	if err := serveUnitOwnership(home); err != nil {
		return false, err
	}
	path := serveUnitPath(home)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false, nil
	}
	if err := systemdRun("disable", "--now", serveUnit); err != nil {
		return false, err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return false, err
	}
	return true, systemdRun("daemon-reload")
}

func serveJob(bin, home, configPath string, port int) launchdJob {
	return launchdJob{
		Label: serveLabel(), Bin: bin, Args: serveArgs(port),
		Env: jobEnv(home, configPath), LogStem: filepath.Join(home, ".hev", "serve"),
	}
}

// upServe installs and starts the dashboard service and waits for it to
// answer. It reports the URL, or "" with a line saying why it is not
// running; only a failure to install what it set out to install is an error.
func upServe(ctx context.Context, out io.Writer, home, configPath string, port int, restart bool) (string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	switch {
	case hostOS == "linux" && hasSystemd():
		if err := serveUnitOwnership(home); err != nil {
			fmt.Fprintf(out, "  • %s; run `hev serve` for the dashboard\n", err)
			return "", nil
		}
		if !servedByUs(home, port) && !portFree(port) {
			fmt.Fprintf(out, "  • port %d is in use: set [local] serve_port (or HEV_LOCAL_SERVE_PORT) and run `hev up` again, or run `hev serve --port <port>`\n", port)
			return "", nil
		}
		bin, changed, err := installSelf(home)
		if err != nil {
			return "", err
		}
		if err := ensureServeSystemd(home, bin, configPath, port, restart || changed); err != nil {
			return "", err
		}
	case hasLaunchd():
		bin, changed, err := installSelf(home)
		if err != nil {
			return "", err
		}
		job := serveJob(bin, home, configPath, port)
		if !job.loaded() && !portFree(port) {
			fmt.Fprintf(out, "  • port %d is in use: set [local] serve_port (or HEV_LOCAL_SERVE_PORT) and run `hev up` again, or run `hev serve --port <port>`\n", port)
			return "", nil
		}
		if _, err := job.ensure(home, restart || changed); err != nil {
			return "", err
		}
	default:
		fmt.Fprintf(out, "  • no service manager for the dashboard here: run `hev serve`\n")
		return "", nil
	}
	if err := waitFor(ctx, 20*time.Second, func() error { return reachable(url) }); err != nil {
		return "", fmt.Errorf("the dashboard service did not answer on %s (see %s): %w", url, serveLogHint(home), err)
	}
	tick(out, "dashboard running at %s", url)
	return url, nil
}

// servedByUs reports whether our dashboard unit is what holds port.
func servedByUs(home string, port int) bool {
	raw, err := os.ReadFile(serveUnitPath(home))
	if err != nil || !strings.Contains(string(raw), `"--port" "`+strconv.Itoa(port)+`"`) {
		return false
	}
	out, _ := linuxService.Run("is-active", serveUnit)
	return strings.TrimSpace(string(out)) == "active"
}

func serveLogHint(home string) string {
	if hasLaunchd() {
		return filepath.Join(home, ".hev", "serve.err")
	}
	return "journalctl --user -u " + serveUnit
}

// downServe stops the dashboard service, if `hev up` installed one.
func downServe(out io.Writer, home string) error {
	switch {
	case hostOS == "linux" && hasSystemd():
		stopped, err := downServeSystemd(home)
		if errors.Is(err, errUnmanaged) {
			fmt.Fprintf(out, "  • %s; stop it yourself\n", err)
			return nil
		}
		if err != nil {
			return err
		}
		if stopped {
			tick(out, "dashboard stopped")
		}
	case hasLaunchd():
		// unloadJobs boots out the serve label with hevd's.
	}
	return nil
}

// serveSystemdService and serveLaunchdService restart the dashboard on
// `hev update`, so it serves from the new binary.
type serveSystemdService struct{ home string }

func (serveSystemdService) Name() string { return "dashboard (systemd --user " + serveUnit + ")" }
func (s serveSystemdService) Present() bool {
	if hostOS != "linux" {
		return false
	}
	if _, err := os.Stat(serveUnitPath(s.home)); err != nil {
		return false
	}
	return serveUnitOwnership(s.home) == nil
}
func (s serveSystemdService) Restart(ctx context.Context) error {
	if !linuxService.Available() {
		return fmt.Errorf("the systemd user manager is not reachable from this session; enable lingering (loginctl enable-linger) or run hev update in a login session")
	}
	return restartSystemdUnit(ctx, serveUnit)
}

type serveLaunchdService struct{ home string }

func (serveLaunchdService) Name() string { return "dashboard (launchd " + serveLabel() + ")" }
func (s serveLaunchdService) Present() bool {
	_, err := os.Stat(serveJob("", s.home, "", 0).plistPath(s.home))
	return err == nil
}
func (s serveLaunchdService) Restart(ctx context.Context) error {
	return kickstartLaunchd(ctx, serveJob("", s.home, "", 0).target())
}

var (
	_ update.Service = serveSystemdService{}
	_ update.Service = serveLaunchdService{}
)
