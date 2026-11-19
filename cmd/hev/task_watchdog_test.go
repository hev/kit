package main

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/hev/kit/internal/daemon"
)

type watchdogTask struct {
	XMLName  xml.Name
	Triggers struct {
		Time []struct {
			StartBoundary string
			Enabled       bool
			Repetition    struct {
				Interval, Duration string
				StopAtDurationEnd  bool
			}
		} `xml:"TimeTrigger"`
		Boot  []struct{} `xml:"BootTrigger"`
		Logon []struct{} `xml:"LogonTrigger"`
	}
	Principals struct {
		Principal struct {
			User                string `xml:"UserId"`
			LogonType, RunLevel string
		}
	}
	Settings struct {
		MultipleInstancesPolicy string
		Hidden, Enabled         bool
		ExecutionTimeLimit      string
	}
	Actions struct {
		Exec []struct{ Command, Arguments string }
	}
}

func TestRenderWatchdogTask(t *testing.T) {
	text := renderWatchdogXML("S-1-5-21-123")
	var task watchdogTask
	if err := xml.Unmarshal([]byte(text), &task); err != nil {
		t.Fatal(err)
	}
	// A time trigger repeats from registration; boot and logon repetitions
	// wait for an event that may never come again.
	if len(task.Triggers.Time) != 1 || len(task.Triggers.Boot)+len(task.Triggers.Logon) != 0 {
		t.Fatalf("want one time trigger: %s", text)
	}
	tr := task.Triggers.Time[0]
	if !tr.Enabled || tr.StartBoundary == "" || tr.Repetition.Interval != "PT1M" || tr.Repetition.Duration != "" || tr.Repetition.StopAtDurationEnd {
		t.Fatalf("watchdog must repeat every minute indefinitely: %s", text)
	}
	if task.Principals.Principal.User != "S-1-5-21-123" || task.Principals.Principal.LogonType != "S4U" || task.Principals.Principal.RunLevel != "LeastPrivilege" {
		t.Fatal(text)
	}
	if task.Settings.MultipleInstancesPolicy != "IgnoreNew" || !task.Settings.Hidden || !task.Settings.Enabled || task.Settings.ExecutionTimeLimit == "PT0S" {
		t.Fatal(text)
	}
	// It starts hevd's task, whatever that task's action is, not hevd itself.
	if len(task.Actions.Exec) != 1 || task.Actions.Exec[0].Command != `%SystemRoot%\System32\schtasks.exe` || task.Actions.Exec[0].Arguments != `/Run /TN hev\hevd` {
		t.Fatalf("bad action: %+v", task.Actions.Exec)
	}
	if strings.Contains(text, "Password") {
		t.Fatal("task must not contain a password")
	}
}

func decodeTaskFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) < 2 || b[0] != 0xff || b[1] != 0xfe || len(b)%2 != 0 {
		t.Fatal("want UTF-16LE with BOM")
	}
	units := make([]uint16, (len(b)-2)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(b[2+2*i:])
	}
	return string(utf16.Decode(units))
}

func TestEnsureWindowsWatchdog(t *testing.T) {
	t.Run("registers it beside hevd's task", func(t *testing.T) {
		dir := t.TempDir()
		var calls [][]string
		var registered string
		run := func(args ...string) (string, error) {
			calls = append(calls, args)
			if args[0] == "/Create" {
				registered = decodeTaskFile(t, args[4])
			}
			return "SUCCESS", nil
		}
		query := func() (bool, bool, error) { return true, false, nil }
		if err := ensureWindowsWatchdog(query, run, func() (string, error) { return "S-1-5-21-9", nil }, dir); err != nil {
			t.Fatal(err)
		}
		if len(calls) != 1 || !reflect.DeepEqual([]string{calls[0][0], calls[0][1], calls[0][2], calls[0][3], calls[0][5]}, []string{"/Create", "/TN", `hev\hevd-watchdog`, "/XML", "/F"}) {
			t.Fatalf("calls = %q", calls)
		}
		if !strings.Contains(registered, `encoding="UTF-16"`) || !strings.Contains(registered, "<UserId>S-1-5-21-9</UserId>") {
			t.Fatalf("registered:\n%s", registered)
		}
		if left, _ := os.ReadDir(dir); len(left) != 0 {
			t.Fatalf("temporary XML left behind: %v", left)
		}
	})
	t.Run("nothing to restart without hevd's task", func(t *testing.T) {
		run := func(args ...string) (string, error) { t.Fatalf("unexpected schtasks %q", args); return "", nil }
		sid := func() (string, error) { t.Fatal("unexpected SID lookup"); return "", nil }
		if err := ensureWindowsWatchdog(func() (bool, bool, error) { return false, false, nil }, run, sid, t.TempDir()); err != nil {
			t.Fatal(err)
		}
	})
	boom := errors.New("access denied")
	for _, failure := range []string{"query", "sid", "/Create"} {
		t.Run("fails on "+failure, func(t *testing.T) {
			query := func() (bool, bool, error) {
				if failure == "query" {
					return false, false, boom
				}
				return true, true, nil
			}
			sid := func() (string, error) {
				if failure == "sid" {
					return "", boom
				}
				return "S-1-5-21-9", nil
			}
			run := func(args ...string) (string, error) {
				if args[0] == failure {
					return "", boom
				}
				return "", nil
			}
			if err := ensureWindowsWatchdog(query, run, sid, t.TempDir()); !errors.Is(err, boom) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestRemoveWindowsWatchdog(t *testing.T) {
	for _, registered := range []bool{true, false} {
		var calls [][]string
		run := func(args ...string) (string, error) { calls = append(calls, args); return "", nil }
		if err := removeWindowsWatchdog(func() (bool, bool, error) { return registered, false, nil }, run); err != nil {
			t.Fatal(err)
		}
		want := [][]string(nil)
		if registered {
			want = [][]string{{"/Delete", "/TN", `hev\hevd-watchdog`, "/F"}}
		}
		if !reflect.DeepEqual(calls, want) {
			t.Fatalf("registered=%v: calls = %q", registered, calls)
		}
	}
}

// The update path: hevd exits with ErrRestart and the watchdog, registered
// through schtasks, is what starts hev\hevd again.
func TestWindowsDaemonExitOnUpdate(t *testing.T) {
	var calls [][]string
	run := func(args ...string) (string, error) { calls = append(calls, args); return "SUCCESS", nil }
	ensure := func() error {
		return ensureWindowsWatchdog(func() (bool, bool, error) { return true, false, nil }, run, func() (string, error) { return "S-1-5-21-9", nil }, t.TempDir())
	}
	var stderr bytes.Buffer
	err := windowsDaemonExit(fmt.Errorf("stop: %w", daemon.ErrRestart), ensure, &stderr)
	if !errors.Is(err, daemon.ErrRestart) {
		t.Fatalf("err = %v", err)
	}
	if len(calls) != 1 || calls[0][0] != "/Create" || calls[0][2] != windowsWatchdogTaskName {
		t.Fatalf("calls = %q", calls)
	}
	if !strings.Contains(stderr.String(), `hev\hevd-watchdog task starts hev\hevd again on the updated build`) {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestWindowsDaemonExitWatchdogFails(t *testing.T) {
	var stderr bytes.Buffer
	err := windowsDaemonExit(daemon.ErrRestart, func() error { return errors.New("access denied") }, &stderr)
	if !errors.Is(err, daemon.ErrRestart) {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(stderr.String(), "access denied") || !strings.Contains(stderr.String(), `schtasks /Run /TN hev\hevd`) {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestWindowsDaemonExitOtherwise(t *testing.T) {
	ensure := func() error { t.Fatal("watchdog is only refreshed on an update exit"); return nil }
	var stderr bytes.Buffer
	if err := windowsDaemonExit(nil, ensure, &stderr); err != nil || stderr.Len() != 0 {
		t.Fatalf("err = %v, stderr = %q", err, stderr.String())
	}
	boom := errors.New("another daemon is already running")
	if err := windowsDaemonExit(boom, ensure, &stderr); err != boom || !strings.Contains(stderr.String(), boom.Error()) {
		t.Fatalf("err = %v, stderr = %q", err, stderr.String())
	}
}
