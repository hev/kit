//go:build windows

package dispatch

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

// orchestratorLaunchTimeout bounds a launch from start to confirmation,
// including a scheduled task's /Create and /Run and a supervisor that waits
// briefly for the root lock. Removing the task has its own bound, so a launch
// returns within their sum.
const (
	orchestratorLaunchTimeout   = 15 * time.Second
	scheduledTaskCleanupTimeout = 5 * time.Second
)

// Tests replace these to force breakaway denial and Task Scheduler failures.
var (
	startDetached      = func(c *exec.Cmd) error { return c.Start() }
	schtasks           = runSchtasks
	profileEnvironment = userProfileEnvironment
)

// startOrchestratorSupervisor detaches the supervisor from the caller's console
// and job, so it outlives an OpenSSH session. A job that forbids breakaway falls
// back to Task Scheduler. It returns only after supervisor.json names a live
// process; a stale record from a dead supervisor never confirms.
func startOrchestratorSupervisor(dir, name string, args []string) error {
	deadline := time.Now().Add(orchestratorLaunchTimeout)
	log := filepath.Join(dir, "supervisor.log")
	f, e := os.OpenFile(log, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	c := exec.Command(args[0], args[1:]...)
	c.Env = controlEnvironment() // Code gathering uses the child spec; sessions get its identity.
	c.Stdout = f
	c.Stderr = f
	c.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windowsSupervisorFlags()}
	if e = startDetached(c); e != nil {
		// Task Scheduler starts outside the SSH service's restrictive job.
		return startScheduledSupervisor(dir, name, args, e, f, deadline)
	}
	exited := make(chan error, 1)
	go func() { exited <- c.Wait() }()
	return confirmSupervisor(dir, "see "+log, exited, deadline)
}

// startScheduledSupervisor runs the supervisor from a one-shot task with no
// trigger. The task starts with the user's profile environment, so the
// launcher carries the variables that choose the spec, and refuses before
// creating anything when the caller differs in a way a task cannot reproduce.
// Each attempt owns a uniquely named task; the root lock still admits one
// supervisor.
func startScheduledSupervisor(dir, name string, args []string, denied error, log *os.File, deadline time.Time) (err error) {
	profile, e := profileEnvironment()
	if e != nil {
		return fmt.Errorf("breakaway: %v; profile environment: %w", denied, e)
	}
	// The task must reproduce what a direct launch passes, held values included.
	carried, refused := scheduledLaunchEnvironment(controlEnvironment(), profile, pluginOnlyEnvNames())
	if len(refused) > 0 {
		return fmt.Errorf("breakaway: %v; a scheduled task cannot reproduce %s from this environment", denied, strings.Join(refused, ", "))
	}
	for _, kv := range carried {
		args = append(args, "--launch-env", kv)
	}
	nonce := make([]byte, 8)
	if _, e = rand.Read(nonce); e != nil {
		return e
	}
	task := "hev-orchestrator-" + name + "-" + hex.EncodeToString(nonce)
	definition, e := scheduledTaskXML(dir, args)
	if e != nil {
		return e
	}
	file := filepath.Join(dir, task+".xml")
	if e = os.WriteFile(file, definition, 0600); e != nil {
		return e
	}
	defer os.Remove(file)
	fmt.Fprintf(log, "launch: breakaway denied (%v); scheduled task %s\n", denied, task)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	created := schtasks(ctx, "/Create", "/TN", task, "/XML", file)
	defer func() {
		if e := removeScheduledTask(task, created == nil); e != nil {
			if err == nil {
				e = fmt.Errorf("orchestrator supervisor started, but %w", e)
			}
			err = errors.Join(err, e)
		}
	}()
	if created != nil {
		return fmt.Errorf("breakaway: %v; scheduled task: %w", denied, created)
	}
	if e = schtasks(ctx, "/Run", "/TN", task); e != nil {
		return e
	}
	return confirmSupervisor(dir, "scheduled task "+task, nil, deadline)
}

// removeScheduledTask deletes an attempt's task. A failed or timed-out /Create
// may still have registered it, so cleanup always runs, and a failed /Delete is
// an error unless /Query shows the task was never registered.
func removeScheduledTask(task string, created bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), scheduledTaskCleanupTimeout)
	defer cancel()
	e := schtasks(ctx, "/Delete", "/TN", task, "/F")
	if e == nil || !created && schtasks(ctx, "/Query", "/TN", task) != nil && ctx.Err() == nil {
		return nil
	}
	return fmt.Errorf("remove scheduled task %s: %w", task, e)
}

// confirmSupervisor waits until supervisor.json names a live process. exited
// is nil when the supervisor is not this process's child.
func confirmSupervisor(dir, detail string, exited <-chan error, deadline time.Time) error {
	record := filepath.Join(dir, "supervisor.json")
	for {
		if processRecordLive(record) {
			return nil
		}
		select {
		case err := <-exited:
			// A concurrent launch may own the live record.
			if processRecordLive(record) {
				return nil
			}
			return fmt.Errorf("orchestrator supervisor exited before confirming (%v); %s", err, detail)
		case <-time.After(50 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%w; %s", errLaunchUnconfirmed, detail)
		}
	}
}

func runSchtasks(ctx context.Context, args ...string) error {
	b, e := helperCommand(ctx, "schtasks", args...).CombinedOutput()
	if ctx.Err() != nil {
		return fmt.Errorf("schtasks %s: %w", args[0], ctx.Err())
	}
	if e != nil {
		return fmt.Errorf("schtasks %s: %w: %s", args[0], e, bytes.TrimSpace(b))
	}
	return nil
}

// userProfileEnvironment is the environment Task Scheduler gives the user's
// tasks: the profile's, without anything this process inherited.
func userProfileEnvironment() ([]string, error) {
	var t windows.Token
	if e := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY|windows.TOKEN_DUPLICATE|windows.TOKEN_IMPERSONATE, &t); e != nil {
		return nil, e
	}
	defer t.Close()
	return t.Environ(false)
}

// scheduledTaskXML has no trigger, so a task left behind can only run on
// demand, and no execution time limit, so Task Scheduler never ends the
// supervisor. S4U runs it without a desktop logon, as over OpenSSH.
func scheduledTaskXML(dir string, args []string) ([]byte, error) {
	t, e := windows.OpenCurrentProcessToken()
	if e != nil {
		return nil, e
	}
	defer t.Close()
	user, e := t.GetTokenUser()
	if e != nil {
		return nil, e
	}
	escape := func(s string) string { var b strings.Builder; _ = xml.EscapeText(&b, []byte(s)); return b.String() }
	text := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
 <RegistrationInfo><Description>hev orchestrator supervisor launch</Description></RegistrationInfo>
 <Principals><Principal id="User"><UserId>%s</UserId><LogonType>S4U</LogonType><RunLevel>LeastPrivilege</RunLevel></Principal></Principals>
 <Settings><MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy><DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries><StopIfGoingOnBatteries>false</StopIfGoingOnBatteries><AllowStartOnDemand>true</AllowStartOnDemand><Enabled>true</Enabled><Hidden>true</Hidden><ExecutionTimeLimit>PT0S</ExecutionTimeLimit></Settings>
 <Actions Context="User"><Exec><Command>%s</Command><Arguments>%s</Arguments><WorkingDirectory>%s</WorkingDirectory></Exec></Actions>
</Task>
`, escape(user.User.Sid.String()), escape(args[0]), escape(windows.ComposeCommandLine(args[1:])), escape(dir))
	// Task Scheduler expects the declared encoding to match the file bytes.
	units := utf16.Encode([]rune(text))
	out := make([]byte, 2+2*len(units))
	out[0], out[1] = 0xff, 0xfe
	for i, u := range units {
		binary.LittleEndian.PutUint16(out[2+i*2:], u)
	}
	return out, nil
}
