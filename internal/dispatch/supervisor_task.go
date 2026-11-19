package dispatch

import (
	"encoding/binary"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/hev/kit/internal/platformops"
)

// On Windows a supervisor is started by its own Task Scheduler task, the
// mechanism hev\hevd already uses, so it never belongs to the launching
// process's console or job object: not an SSH session's, and not the job an
// orchestrator round's harness runs in. hev registers the task, runs it,
// confirms the supervisor's process record, and removes the task when the
// supervisor exits or is stopped. Other platforms use tmux and never call
// these functions; the logic is shared so every platform tests it.

// taskScheduler is Task Scheduler on Windows; tests replace it.
type taskScheduler interface {
	Register(name, xml string) error
	Run(name string) error
	Remove(name string) error
	Status(name string) (taskStatus, error)
}

// taskStatus is the task's state as Task Scheduler reports it.
type taskStatus struct {
	Running bool
	// Ran is true once an instance has started and ended; Result is then
	// its exit code (or a scheduler HRESULT when it could not start).
	Ran    bool
	Result uint32
	// missing is true when no task is registered under the name.
	missing bool
}

// Task Scheduler result codes for a task that has not run or is running.
const (
	schedTaskRunning   = 0x41301
	schedTaskHasNotRun = 0x41303
)

// parseTaskStatus reads "STATE LASTRESULT" from the Schedule.Service COM
// object: state 4 is running; 3 is ready, after a run or before the first.
func parseTaskStatus(out string) (taskStatus, error) {
	f := strings.Fields(out)
	if len(f) != 2 {
		return taskStatus{}, fmt.Errorf("unexpected scheduled task status %q", out)
	}
	state, err := strconv.Atoi(f[0])
	if err != nil {
		return taskStatus{}, fmt.Errorf("unexpected scheduled task state %q", f[0])
	}
	result, err := strconv.ParseInt(f[1], 10, 64)
	if err != nil {
		return taskStatus{}, fmt.Errorf("unexpected scheduled task result %q", f[1])
	}
	code := uint32(result) // COM reports HRESULTs as signed 32-bit values.
	if state == 4 {
		return taskStatus{Running: true}, nil
	}
	if code == schedTaskHasNotRun || code == schedTaskRunning {
		return taskStatus{}, nil
	}
	return taskStatus{Ran: true, Result: code}, nil
}

// utf16TaskXML encodes a task definition the way schtasks /XML expects:
// UTF-16LE with a byte order mark, declared as such.
func utf16TaskXML(text string) []byte {
	text = strings.Replace(text, `encoding="UTF-8"`, `encoding="UTF-16"`, 1)
	units := utf16.Encode([]rune(text))
	out := make([]byte, 2+2*len(units))
	out[0], out[1] = 0xff, 0xfe
	for i, u := range units {
		binary.LittleEndian.PutUint16(out[2+i*2:], u)
	}
	return out
}

// supervisorTask describes the task that starts one supervisor.
type supervisorTask struct {
	Name    string // Task Scheduler path, for example hev\orch-NAME
	Exe     string
	Args    []string
	Dir     string // state directory: working directory, launch file, supervisor.json
	Env     []string
	Log     string
	UserSID string
}

// supervisorLaunch is what the task's supervisor adopts at start, written
// beside its state because Task Scheduler does not pass the launching
// environment. The supervisor deletes it before doing anything else; a
// launch that gives up deletes it too (withdrawing the launch), and a task's
// supervisor that finds none refuses to start.
type supervisorLaunch struct {
	Task string   `json:"task"`
	Env  []string `json:"env"`
	Log  string   `json:"log,omitempty"`
}

// supervisorExit is what a task's supervisor leaves as it exits, before it
// removes its task, so the launcher can report its exit code and error even
// though Task Scheduler no longer knows the task.
type supervisorExit struct {
	Task  string `json:"task"`
	PID   int    `json:"pid"`
	Code  int    `json:"code"`
	Error string `json:"error,omitempty"`
}

const (
	supervisorLaunchFile = "supervisor-launch.json"
	supervisorExitFile   = "supervisor-exit.json"
	// scheduledFlag tells supervise it was started by its scheduled task and
	// must adopt a launch file; a supervisor started any other way never reads one.
	scheduledFlag = "--scheduled"
)

func orchestratorTaskName(name string) string { return `hev\orch-` + name }
func runTaskName(name string) string          { return `hev\run-` + name }

// supervisorConfirmWait is how long a launch waits for the supervisor's
// process record. It matches the grace dispatch ls gives an unconfirmed run.
var supervisorConfirmWait = launchGrace

// startSupervisorTask registers and runs t, then waits until its supervisor
// records itself or exits. Every failure names the task, the reason and the
// log; none leaves the launch file behind unless a supervisor has already
// adopted it.
func startSupervisorTask(s taskScheduler, t supervisorTask, wait time.Duration) error {
	launchPath := filepath.Join(t.Dir, supervisorLaunchFile)
	exitPath := filepath.Join(t.Dir, supervisorExitFile)
	b, err := json.Marshal(supervisorLaunch{Task: t.Name, Env: t.Env, Log: t.Log})
	if err != nil {
		return err
	}
	// A fresh registration has no earlier result to mistake for this run's.
	_ = s.Remove(t.Name)
	_ = os.Remove(exitPath)
	// Owner-only: the launch environment can carry credentials.
	if err = platformops.WriteFile(launchPath, b, 0600); err != nil {
		return err
	}
	fail := func(err error) error {
		_ = os.Remove(launchPath)
		_ = os.Remove(exitPath)
		_ = s.Remove(t.Name)
		return err
	}
	// exited reports the supervisor's own exit record for this task.
	exited := func() (supervisorExit, bool) {
		var x supervisorExit
		if readJSON(exitPath, &x) != nil || x.Task != t.Name {
			return x, false
		}
		return x, true
	}
	exitError := func(x supervisorExit) error {
		reason := ""
		if x.Error != "" {
			reason = fmt.Sprintf(" (%q)", x.Error)
		}
		return fail(fmt.Errorf("%w: supervisor of scheduled task %s exited with code %d before it confirmed%s; log: %s", errLaunchUnconfirmed, t.Name, x.Code, reason, t.Log))
	}
	if err = s.Register(t.Name, renderSupervisorTaskXML(t)); err != nil {
		return fail(fmt.Errorf("%w: register scheduled task %s: %v", errLaunchUnconfirmed, t.Name, err))
	}
	if err = s.Run(t.Name); err != nil {
		return fail(fmt.Errorf("%w: run scheduled task %s: %v", errLaunchUnconfirmed, t.Name, err))
	}
	record := filepath.Join(t.Dir, "supervisor.json")
	deadline := time.Now().Add(wait)
	nextStatus := time.Now()
	last := "not started"
	for {
		if processRecordLive(record) {
			return nil
		}
		if x, ok := exited(); ok {
			if x.Code == 0 {
				// It ran and finished cleanly before this poll saw its
				// record, for example a run whose stop was already queued.
				_ = os.Remove(exitPath)
				return nil
			}
			return exitError(x)
		}
		if !time.Now().Before(nextStatus) {
			nextStatus = time.Now().Add(time.Second)
			st, err := s.Status(t.Name)
			switch {
			case err != nil:
				last = "status unavailable: " + err.Error()
			case st.Running:
				last = "task running, no supervisor record yet"
			case st.missing || st.Ran:
				// The supervisor writes its exit record before it removes
				// its task: look once more before calling this an early exit.
				if processRecordLive(record) {
					return nil
				}
				if x, ok := exited(); ok {
					if x.Code == 0 {
						_ = os.Remove(exitPath)
						return nil
					}
					return exitError(x)
				}
				if st.missing {
					return fail(fmt.Errorf("%w: scheduled task %s was removed before its supervisor started%s; log: %s", errLaunchUnconfirmed, t.Name, lastLogLine(t.Log), t.Log))
				}
				return fail(fmt.Errorf("%w: scheduled task %s exited with code %#x before its supervisor started%s; log: %s", errLaunchUnconfirmed, t.Name, st.Result, lastLogLine(t.Log), t.Log))
			}
		}
		if time.Now().After(deadline) {
			// Withdraw the launch: without its launch file a late supervisor
			// refuses to start, so nothing runs that this command reported as
			// failed, and no environment stays on disk.
			switch err := os.Remove(launchPath); {
			case err == nil:
				_ = s.Remove(t.Name)
				return fmt.Errorf("%w: scheduled task %s did not start a supervisor within %s (%s); launch withdrawn and task removed; log: %s", errLaunchUnconfirmed, t.Name, wait, last, t.Log)
			default:
				// Already adopted (or being read): the supervisor runs and
				// removes its task itself; it may still confirm.
				return fmt.Errorf("%w: supervisor of scheduled task %s adopted its launch but did not record itself within %s (%s); it may still confirm; log: %s", errLaunchUnconfirmed, t.Name, wait, last, t.Log)
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// clearSupervisorLaunch withdraws whatever a scheduled launch left in dir
// once no supervisor may own it: its launch file (and the environment in it),
// its exit record and its task. Stop, retire, clean and a lapsed launch call it.
func clearSupervisorLaunch(s taskScheduler, dir, task string) {
	if processRecordMayBeLive(filepath.Join(dir, "supervisor.json")) {
		return
	}
	_ = os.Remove(filepath.Join(dir, supervisorLaunchFile))
	_ = os.Remove(filepath.Join(dir, supervisorExitFile))
	_ = s.Remove(task)
}

// lastLogLine quotes the final line of a supervisor log, if it has one.
func lastLogLine(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if line := strings.TrimSpace(lines[len(lines)-1]); line != "" {
		return fmt.Sprintf(" (%q)", line)
	}
	return ""
}

// adoptSupervisorLaunch applies the launch file task's launcher left in dir:
// its environment and log. task is empty for a supervisor not started by a
// scheduled task, which has nothing to adopt. The returned func records the
// supervisor's exit for the launcher and removes the task.
func adoptSupervisorLaunch(s taskScheduler, dir, task string) (func(error), error) {
	none := func(error) {}
	if task == "" {
		return none, nil
	}
	path := filepath.Join(dir, supervisorLaunchFile)
	var l supervisorLaunch
	if err := readJSON(path, &l); err != nil {
		_ = os.Remove(path)
		_ = s.Remove(task)
		if errors.Is(err, os.ErrNotExist) {
			return none, fmt.Errorf("scheduled task %s has no %s: its launch was withdrawn; launch again", task, supervisorLaunchFile)
		}
		return none, fmt.Errorf("supervisor launch file: %w", err)
	}
	if err := os.Remove(path); err != nil {
		// The launcher withdrew it while this supervisor read it.
		_ = s.Remove(task)
		return none, fmt.Errorf("scheduled task %s: launch withdrawn: %w", task, err)
	}
	if l.Task != task {
		_ = s.Remove(task)
		return none, fmt.Errorf("%s names task %q, not %s", supervisorLaunchFile, l.Task, task)
	}
	if len(l.Env) > 0 {
		os.Clearenv()
		for _, kv := range l.Env {
			if k, v, ok := strings.Cut(kv, "="); ok && k != "" {
				_ = os.Setenv(k, v)
			}
		}
	}
	if l.Log != "" {
		f, err := os.OpenFile(l.Log, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			return none, err
		}
		os.Stdout, os.Stderr = f, f
	}
	return func(runErr error) {
		x := supervisorExit{Task: task, PID: os.Getpid()}
		if runErr != nil {
			x.Code, x.Error = 1, runErr.Error()
		}
		_ = WriteJSON(filepath.Join(dir, supervisorExitFile), x)
		// Leave the registration alone if another supervisor owns this state.
		var p pidRecord
		if readJSON(filepath.Join(dir, "supervisor.json"), &p) == nil && p.PID != os.Getpid() && processRecordLive(filepath.Join(dir, "supervisor.json")) {
			return
		}
		_ = s.Remove(task)
	}, nil
}

// AdoptOrchestratorLaunch is called by hev orchestrate supervise before
// anything reads the environment; scheduled is its --scheduled flag.
func AdoptOrchestratorLaunch(name string, scheduled bool) (func(error), error) {
	if !ValidName(name) {
		return func(error) {}, fmt.Errorf("invalid orchestrator name")
	}
	task := ""
	if scheduled {
		task = orchestratorTaskName(name)
	}
	return adoptSupervisorLaunch(supervisorTasks, OrchestratorDir(name), task)
}

// AdoptRunLaunch is AdoptOrchestratorLaunch for hev dispatch supervise.
func AdoptRunLaunch(name string, scheduled bool) (func(error), error) {
	if !ValidName(name) {
		return func(error) {}, fmt.Errorf("invalid name")
	}
	task := ""
	if scheduled {
		task = runTaskName(name)
	}
	return adoptSupervisorLaunch(supervisorTasks, filepath.Join(Root(), name), task)
}

// clearOrchestratorLaunch removes what an orchestrator launch registered.
// Stop and retire call it when no supervisor is left to remove its own.
func clearOrchestratorLaunch(name string) {
	clearSupervisorLaunch(supervisorTasks, OrchestratorDir(name), orchestratorTaskName(name))
}

// clearRunLaunch is clearOrchestratorLaunch for a run's directory.
func clearRunLaunch(dir string) {
	clearSupervisorLaunch(supervisorTasks, dir, runTaskName(filepath.Base(dir)))
}

// renderSupervisorTaskXML runs one supervisor on demand as the current user,
// without a stored password (S4U, like hev\hevd), with no time limit and no
// restart: a supervisor that exits is relaunched by hev, never the scheduler.
// LeastPrivilege, like hev\hevd: an elevated supervisor's process could not
// be queried from a filtered session of the same user, which would then
// take it for exited.
func renderSupervisorTaskXML(t supervisorTask) string {
	escape := func(s string) string { var b strings.Builder; _ = xml.EscapeText(&b, []byte(s)); return b.String() }
	args := make([]string, len(t.Args))
	for i, a := range t.Args {
		args[i] = windowsCommandLineArg(a)
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Task version="1.3" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
 <RegistrationInfo><Description>hev supervisor (removed when it exits)</Description><URI>\%s</URI></RegistrationInfo>
 <Principals><Principal id="User"><UserId>%s</UserId><LogonType>S4U</LogonType><RunLevel>LeastPrivilege</RunLevel></Principal></Principals>
 <Settings><MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy><DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries><StopIfGoingOnBatteries>false</StopIfGoingOnBatteries><AllowHardTerminate>true</AllowHardTerminate><StartWhenAvailable>false</StartWhenAvailable><RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable><AllowStartOnDemand>true</AllowStartOnDemand><Enabled>true</Enabled><Hidden>true</Hidden><ExecutionTimeLimit>PT0S</ExecutionTimeLimit><Priority>5</Priority></Settings>
 <Actions Context="User"><Exec><Command>%s</Command><Arguments>%s</Arguments><WorkingDirectory>%s</WorkingDirectory></Exec></Actions>
</Task>
`, escape(t.Name), escape(t.UserSID), escape(t.Exe), escape(strings.Join(args, " ")), escape(t.Dir))
}

// windowsCommandLineArg quotes for CommandLineToArgvW only when needed.
func windowsCommandLineArg(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\n\v\"") {
		return s
	}
	var b strings.Builder
	b.WriteByte('"')
	slashes := 0
	for _, r := range s {
		if r == '\\' {
			slashes++
			continue
		}
		if r == '"' {
			b.WriteString(strings.Repeat(`\`, slashes*2+1))
		} else {
			b.WriteString(strings.Repeat(`\`, slashes))
		}
		slashes = 0
		b.WriteRune(r)
	}
	b.WriteString(strings.Repeat(`\`, slashes*2))
	b.WriteByte('"')
	return b.String()
}
