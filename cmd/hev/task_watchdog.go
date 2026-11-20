package main

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/hev/kit/internal/daemon"
)

// windowsWatchdogTaskName starts hevd's task again after hevd exits.
//
// Task Scheduler's RestartOnFailure only retries a task that fails to start;
// an action that runs and exits non-zero is a finished run, so a daemon that
// exits to move onto an updated build, or crashes, is never restarted by it.
// The repetitions on hevd's boot and logon triggers only run once that
// trigger has fired, so a task registered (or re-registered) after boot has
// no next run at all. A separate task with a time trigger repeats from the
// moment it is registered.
//
// It runs `schtasks /Run` on hevd's task rather than hevd itself, so it
// restarts whatever that task's action is: kit's own `hev daemon`, or a
// wrapper script that resolves a credential and then runs hevd. hevd's task
// ignores a new instance while one is running, so the repetition is a no-op
// while hevd is up. A plain stop leaves hevd's task enabled, so the watchdog
// starts the daemon again within a minute, like the task's own triggers;
// stop --disable disables hevd's task, and the watchdog's /Run then fails, so
// a daemon disabled on purpose stays down.
const windowsWatchdogTaskName = `hev\hevd-watchdog`

// renderWatchdogXML is independent of the host OS, like renderTaskXML.
func renderWatchdogXML(userSID string) string {
	escape := func(s string) string { var b strings.Builder; _ = xml.EscapeText(&b, []byte(s)); return b.String() }
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Task version="1.3" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
 <RegistrationInfo><Description>Starts hev\hevd again within a minute after hevd exits (an update or a crash)</Description><URI>\hev\hevd-watchdog</URI></RegistrationInfo>
 <Triggers><TimeTrigger><StartBoundary>2020-01-01T00:00:00</StartBoundary><Repetition><Interval>PT1M</Interval><StopAtDurationEnd>false</StopAtDurationEnd></Repetition><Enabled>true</Enabled></TimeTrigger></Triggers>
 <Principals><Principal id="User"><UserId>%s</UserId><LogonType>S4U</LogonType><RunLevel>LeastPrivilege</RunLevel></Principal></Principals>
 <Settings><MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy><DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries><StopIfGoingOnBatteries>false</StopIfGoingOnBatteries><AllowHardTerminate>true</AllowHardTerminate><StartWhenAvailable>true</StartWhenAvailable><RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable><Enabled>true</Enabled><Hidden>true</Hidden><ExecutionTimeLimit>PT5M</ExecutionTimeLimit></Settings>
 <Actions Context="User"><Exec><Command>%%SystemRoot%%\System32\schtasks.exe</Command><Arguments>/Run /TN %s</Arguments></Exec></Actions>
</Task>
`, escape(userSID), escape(windowsTaskName))
}

// ensureWindowsWatchdog registers (or refreshes) the watchdog for hevd's task.
// Without hevd's task there is nothing to restart, so it registers nothing.
func ensureWindowsWatchdog(queryDaemon func() (bool, bool, error), run func(...string) (string, error), userSID func() (string, error), dir string) error {
	registered, _, err := queryDaemon()
	if err != nil {
		return err
	}
	if !registered {
		return nil
	}
	sid, err := userSID()
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "watchdog-*.xml")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(taskXMLBytes(renderWatchdogXML(sid)))
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	_, err = run("/Create", "/TN", windowsWatchdogTaskName, "/XML", f.Name(), "/F")
	return err
}

// removeWindowsWatchdog deletes the watchdog, when it is registered.
func removeWindowsWatchdog(queryWatchdog func() (bool, bool, error), run func(...string) (string, error)) error {
	registered, _, err := queryWatchdog()
	if err != nil || !registered {
		return err
	}
	_, err = run("/Delete", "/TN", windowsWatchdogTaskName, "/F")
	return err
}

// windowsDaemonExit reports why hevd stopped. On an update it makes sure the
// watchdog is registered, because nothing else starts hevd's task again.
func windowsDaemonExit(err error, ensureWatchdog func() error, stderr io.Writer) error {
	if !errors.Is(err, daemon.ErrRestart) {
		if err != nil {
			fmt.Fprintln(stderr, err)
		}
		return err
	}
	if werr := ensureWatchdog(); werr != nil {
		fmt.Fprintf(stderr, "hevd: exiting to move onto the updated build, but the %s task could not be registered (%v); start hevd again with: schtasks /Run /TN %s\n", windowsWatchdogTaskName, werr, windowsTaskName)
		return err
	}
	fmt.Fprintf(stderr, "hevd: exiting so the %s task starts %s again on the updated build within a minute\n", windowsWatchdogTaskName, windowsTaskName)
	return err
}
