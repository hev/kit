package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"time"
)

// ExecSpec contains no environment values. Job secrets travel only over the
// supervisor's anonymous stdin pipe and are never written with this record.
type ExecSpec struct {
	Argv       []string  `json:"argv"`
	Cwd        string    `json:"cwd"`
	Deadline   time.Time `json:"deadline"`
	Job        string    `json:"job"`
	Occurrence string    `json:"occurrence"`
}

// StartExec admits a reserved command run under the ordinary dispatch host cap.
// Existing names are never re-admitted, including an uncertain launch.
func StartExec(name, host string, spec ExecSpec, env []string) error {
	if !ValidName(name) || len(spec.Argv) == 0 {
		return fmt.Errorf("invalid command run")
	}
	if err := os.MkdirAll(Root(), 0700); err != nil {
		return err
	}
	unlock, err := lockRoot()
	if err != nil {
		return err
	}
	defer unlock()
	dir := filepath.Join(Root(), name)
	if _, err = os.Stat(dir); err == nil {
		return fmt.Errorf("run already reserved")
	} else if !os.IsNotExist(err) {
		return err
	}
	policy, err := targetPolicy()
	if err != nil {
		return err
	}
	live, err := liveNames()
	if err != nil {
		return err
	}
	// tmux is account-wide; only this dispatch root owns these capacity slots.
	for n := range live {
		if _, e := os.Stat(filepath.Join(Root(), n, "meta.json")); e != nil {
			delete(live, n)
		}
	}
	if err = addReservations(live); err != nil {
		return err
	}
	if err = CheckCap(len(live), policy.Cap); err != nil {
		return err
	}
	entries, err := queueEntries()
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return fmt.Errorf("dispatch queue has priority")
	}
	if err = os.Mkdir(dir, 0700); err != nil {
		return err
	}
	m := Meta{Order: Order{Name: name, Host: host}, Job: spec.Job, Occurrence: spec.Occurrence, Exec: &spec, Start: time.Now().UTC(), RequestedBy: "local:hev"}
	if err = WriteJSON(filepath.Join(dir, "meta.json"), m); err != nil {
		return err
	}
	r := Result{Start: m.Start, ExitCode: 1, LaunchUnconfirmed: true}
	if err = WriteJSON(filepath.Join(dir, "result.json"), r); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	child := exec.Command(exe, "jobs", "supervise", name)
	child.Env = MinimalExecEnvironment()
	// The pipe is closed once consumed; the daemon retains no secret-valued file.
	reader, writer, err := os.Pipe()
	if err != nil {
		return err
	}
	defer reader.Close()
	child.Stdin = reader
	setupChild(child)
	if err = child.Start(); err != nil {
		writer.Close()
		return err
	}
	// The child is blocked reading its anonymous pipe until its birth record
	// is durable, keeping this reservation visible under the dispatch cap.
	if err = writeProcessRecord(filepath.Join(dir, "supervisor.json"), child.Process.Pid); err != nil {
		writer.Close()
		return err
	}
	go func() { _ = json.NewEncoder(writer).Encode(env); _ = writer.Close() }()
	go func() { _ = child.Wait() }()
	return nil
}

func MinimalExecEnvironment() []string {
	out := []string{}
	for _, k := range []string{"PATH", "HOME", "USERPROFILE", "TMPDIR", "TMP", "TEMP", "SystemRoot", "WINDIR", "COMSPEC", "PATHEXT"} {
		if v, ok := os.LookupEnv(k); ok {
			out = append(out, k+"="+v)
		}
	}
	return out
}

func SuperviseExec(name string, input io.Reader) error {
	if !ValidName(name) {
		return fmt.Errorf("invalid run")
	}
	dir := filepath.Join(Root(), name)
	var m Meta
	if err := readJSON(filepath.Join(dir, "meta.json"), &m); err != nil {
		return err
	}
	if m.Exec == nil {
		return fmt.Errorf("not a command run")
	}
	unlock, err := lockDirectory(dir)
	if err != nil {
		return err
	}
	defer unlock()
	var previous Result
	if readJSON(filepath.Join(dir, "result.json"), &previous) == nil && (!previous.End.IsZero() || !previous.LaunchUnconfirmed) {
		return fmt.Errorf("command already admitted")
	}
	if err = writePID(dir); err != nil {
		return err
	}
	defer os.Remove(filepath.Join(dir, "supervisor.json"))
	defer os.Remove(filepath.Join(dir, "harness.json"))
	var env []string
	if err = json.NewDecoder(io.LimitReader(input, 1<<20)).Decode(&env); err != nil {
		return err
	}
	log, err := os.OpenFile(filepath.Join(dir, "log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer log.Close()
	c := exec.Command(m.Exec.Argv[0], m.Exec.Argv[1:]...)
	c.Dir = m.Exec.Cwd
	c.Env = env
	safeLog := newCommandLog(log, env)
	c.Stdout = safeLog
	c.Stderr = safeLog
	signals := make(chan os.Signal, 4)
	signal.Notify(signals, terminationSignals()...)
	defer signal.Stop(signals)
	r := Result{Start: time.Now().UTC(), ExitCode: 1}
	var startErr error
	if !r.Start.Before(m.Exec.Deadline) {
		startErr = context.DeadlineExceeded
		r.Timeout = true
	}
	r = superviseProcess(dir, c, r, time.Until(m.Exec.Deadline), signals, startErr, nil)
	if e := safeLog.Flush(); e != nil {
		r.Error = "command log write failed"
		r.ExitCode = 1
	}
	r.End = time.Now().UTC()
	return WriteJSON(filepath.Join(dir, "result.json"), r)
}

// ExecOutcome reconciles by the reserved name and process birth; absence of
// evidence is unknown, never permission to launch again.
func ExecOutcome(name string) (string, *Result) {
	dir := filepath.Join(Root(), name)
	var r Result
	if readJSON(filepath.Join(dir, "result.json"), &r) != nil {
		if _, e := os.Stat(dir); e == nil {
			return "unknown", nil
		}
		return "absent", nil
	}
	if !r.End.IsZero() {
		return State(&r, false), &r
	}
	if supervisorLive(dir) {
		return "running", &r
	}
	if time.Since(r.Start) < launchGrace {
		return "queued", &r
	}
	return "unknown", &r
}
