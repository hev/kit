//go:build windows

package dispatch

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hev/kit/internal/platformops"
	"golang.org/x/sys/windows"
)

// supervisorTasks is Task Scheduler; the test suite replaces it so no test
// registers or runs a real task.
var supervisorTasks taskScheduler = schtasksScheduler{}

// newSupervisorTask runs as the launching user.
func newSupervisorTask(name, exe string, args []string, dir string, env []string) (supervisorTask, error) {
	token := windows.GetCurrentProcessToken()
	user, err := token.GetTokenUser()
	if err != nil {
		return supervisorTask{}, err
	}
	return supervisorTask{Name: name, Exe: exe, Args: args, Dir: dir, Env: env, UserSID: user.User.Sid.String()}, nil
}

type schtasksScheduler struct{}

func schtasks(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := helperCommand(ctx, "schtasks.exe", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("schtasks %s: %w: %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (schtasksScheduler) Register(name, xml string) error {
	f, err := platformops.CreateTemp("", "hev-task-*.xml")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(utf16TaskXML(xml))
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return schtasks("/Create", "/TN", name, "/XML", f.Name(), "/F")
}
func (schtasksScheduler) Run(name string) error { return schtasks("/Run", "/TN", name) }

// Remove fails when no such task is registered; callers ignore that.
func (schtasksScheduler) Remove(name string) error { return schtasks("/Delete", "/TN", name, "/F") }

// Status reads state and last result through COM, which reports numbers
// independent of the Windows display language.
func (schtasksScheduler) Status(name string) (taskStatus, error) {
	folder, task, _ := strings.Cut(name, `\`)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	// Names are validated run and orchestrator names: no quotes to escape.
	script := `$ErrorActionPreference='Stop'; try { $s=New-Object -ComObject Schedule.Service; $s.Connect(); try { $t=$s.GetFolder('\` + folder + `').GetTask('` + task + `') } catch { if ($_.Exception.GetBaseException().HResult -eq -2147024894 -or $_.Exception.GetBaseException().HResult -eq -2147024893) { exit 3 }; throw }; [Console]::Write([string]$t.State + ' ' + [string]$t.LastTaskResult) } catch { [Console]::Error.Write($_); exit 1 }`
	c := helperCommand(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	out, err := c.CombinedOutput()
	if c.ProcessState != nil && c.ProcessState.ExitCode() == 3 {
		return taskStatus{missing: true}, nil
	}
	if err != nil {
		return taskStatus{}, fmt.Errorf("query scheduled task %s: %w: %s", name, err, strings.TrimSpace(string(out)))
	}
	return parseTaskStatus(string(out))
}
