//go:build windows

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/platformops"
	"golang.org/x/sys/windows"
)

func scheduler(args ...string) (string, error) {
	out, err := exec.Command("schtasks.exe", args...).CombinedOutput()
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
		_, err = f.WriteString(renderTaskXML(binary, home, user.User.Sid.String(), config))
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
		fmt.Println("Task Scheduler: hev\\hevd installed and started.")
		return nil
	case "status":
		out, err := scheduler("/Query", "/TN", windowsTaskName, "/FO", "LIST", "/V")
		if err != nil {
			fmt.Printf("Task Scheduler: unavailable (%v)\n", err)
			return nil
		}
		fmt.Println("Task Scheduler:")
		fmt.Println(out)
		return nil
	case "stop", "uninstall":
		// Disable before signalling to prevent restart-on-failure or a logon race.
		// An absent task still permits stopping a manually started daemon.
		registered := false
		if _, err := scheduler("/Query", "/TN", windowsTaskName); err == nil {
			registered = true
			if _, err = scheduler("/Change", "/TN", windowsTaskName, "/DISABLE"); err != nil {
				return err
			}
		}
		pids := nativeDaemonPIDs()
		if registered && len(pids) == 0 {
			// Registration/startup is asynchronous. Before Run has published its stop
			// event there is no scan to finish, but the scheduler may already have a
			// process starting. End that instance after disabling future triggers.
			// End also returns an error for an already stopped task.
			_, _ = scheduler("/End", "/TN", windowsTaskName)
		}
		for _, pid := range pids {
			if err := platformops.SignalPID(pid, false); err != nil && !platformops.ProcessGone(err) {
				return err
			}
		}
		if !waitForDaemonExit(30 * time.Second) {
			for _, pid := range pids {
				if err := platformops.SignalPID(pid, true); err != nil && !platformops.ProcessGone(err) {
					return err
				}
			}
			if !waitForDaemonExit(5 * time.Second) {
				return fmt.Errorf("daemon did not stop")
			}
		}
		if action == "uninstall" {
			if _, err := scheduler("/Query", "/TN", windowsTaskName); err == nil {
				if _, err = scheduler("/Delete", "/TN", windowsTaskName, "/F"); err != nil {
					return err
				}
			}
		}
		fmt.Println("Daemon: stopped")
		return nil
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
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid == os.Getpid() || pid <= 0 {
		return nil
	}
	p, _ := windows.UTF16PtrFromString(fmt.Sprintf(`Global\hev-hevd-stop-%d`, pid))
	h, err := windows.OpenEvent(windows.SYNCHRONIZE, false, p)
	if err != nil {
		return nil
	}
	windows.CloseHandle(h)
	return []int{pid}
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
