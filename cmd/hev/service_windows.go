//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/platformops"
	"golang.org/x/sys/windows"
)

func scheduler(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "schtasks.exe", args...).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("schtasks %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}
func windowsService(action string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	switch action {
	case "install":
		binary, err := os.Executable()
		if err != nil {
			return err
		}
		binary, err = filepath.Abs(binary)
		if err != nil {
			return err
		}
		config, err := daemon.WriteDefaultConfig(false)
		if err != nil {
			return err
		}
		if err = platformops.SecureFile(config); err != nil {
			return err
		}
		config, err = filepath.Abs(config)
		if err != nil {
			return err
		}
		if err = platformops.MkdirAll(filepath.Join(home, ".hev"), 0700); err != nil {
			return err
		}
		token, err := windows.OpenCurrentProcessToken()
		if err != nil {
			return err
		}
		defer token.Close()
		user, err := token.GetTokenUser()
		if err != nil {
			return err
		}
		f, err := platformops.CreateTemp(filepath.Join(home, ".hev"), "task-*.xml")
		if err != nil {
			return err
		}
		defer os.Remove(f.Name())
		_, err = f.Write(taskXMLBytes(renderTaskXML(binary, home, user.User.Sid.String(), config)))
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			return err
		}
		if _, err = scheduler("/Create", "/TN", windowsTaskName, "/XML", f.Name(), "/F"); err != nil {
			return err
		}
		if _, err = scheduler("/Run", "/TN", windowsTaskName); err != nil {
			return err
		}
		if err = confirmWindowsStartup(filepath.Join(home, ".hev", "daemon.pid"), 15*time.Second); err != nil {
			return err
		}
		fmt.Println("Task Scheduler: hev\\hevd installed and started.")
		return nil
	case "status":
		out, err := scheduler("/Query", "/TN", windowsTaskName, "/FO", "LIST", "/V")
		if err != nil {
			return err
		}
		fmt.Println("Task Scheduler:")
		fmt.Println(out)
		return nil
	case "stop", "uninstall":
		err := stopWindowsTask(action, queryWindowsTask, scheduler, func() (daemonControl, error) {
			return daemonControlResult(platformops.OpenDaemon(filepath.Join(home, ".hev", "daemon.pid")))
		}, 30*time.Second)
		if err == nil {
			fmt.Println("Daemon: stopped")
		}
		return err

	}
	return fmt.Errorf("unknown Windows service action: %s", action)
}
func nativeDaemonPIDs() []int {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	b, err := os.ReadFile(filepath.Join(home, ".hev", "daemon.pid"))
	if err != nil {
		return nil
	}
	var id platformops.DaemonIdentity
	if json.Unmarshal(b, &id) != nil || id.PID == os.Getpid() {
		return nil
	}
	p, err := platformops.OpenDaemon(filepath.Join(home, ".hev", "daemon.pid"))
	if err != nil {
		return nil
	}
	defer p.Close()
	gone, err := p.Exited()
	if err != nil || gone {
		return nil
	}
	return []int{id.PID}
}
func nativeDaemonRun(ctx context.Context) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if err = platformops.MkdirAll(filepath.Join(home, ".hev"), 0700); err != nil {
		return err
	}
	out, err := platformops.OpenFile(filepath.Join(home, ".hev", "hevd.out"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer out.Close()
	stderr, err := platformops.OpenFile(filepath.Join(home, ".hev", "hevd.err"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer stderr.Close()
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = out, stderr
	defer func() { os.Stdout, os.Stderr = oldOut, oldErr }()
	err = daemon.Run(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
	}
	return err
}

// COM exposes numeric state and HRESULTs independent of Windows display language.
func queryWindowsTask() (bool, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	script := `$ErrorActionPreference='Stop'; try { $s=New-Object -ComObject Schedule.Service; $s.Connect(); try { $t=$s.GetFolder('\hev').GetTask('hevd'); [Console]::Write($t.State) } catch { if ($_.Exception.GetBaseException().HResult -eq -2147024894 -or $_.Exception.GetBaseException().HResult -eq -2147024893) { exit 3 }; throw } } catch { [Console]::Error.Write($_); exit 1 }`
	out, err := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script).CombinedOutput()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 3 {
			return false, false, nil
		}
		return false, false, fmt.Errorf("query scheduled task: %w: %s", err, out)
	}
	state := strings.TrimSpace(string(out))
	if state != "1" && state != "2" && state != "3" && state != "4" {
		return false, false, fmt.Errorf("unknown task state %q", state)
	}
	return true, state == "2" || state == "4", nil
}

func confirmWindowsStartup(path string, timeout time.Duration) error {
	return confirmTaskStartup(queryWindowsTask, func() (daemonControl, error) { return daemonControlResult(platformops.OpenDaemon(path)) }, timeout)
}
