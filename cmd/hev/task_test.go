package main

import (
	"encoding/xml"
	"strings"
	"testing"
)

func TestRenderWindowsTask(t *testing.T) {
	text := renderTaskXML(`C:\Program Files\hev & co\hev.exe`, `C:\Users\hev`, "S-1-5-21-123", `C:\Users\hev\a & b.toml`)
	var task struct {
		XMLName  xml.Name
		Triggers struct {
			Boot *struct {
				Repetition struct{ Interval, Duration string }
			} `xml:"BootTrigger"`
			Logon struct {
				User       string `xml:"UserId"`
				Repetition struct{ Interval, Duration string }
			} `xml:"LogonTrigger"`
		}
		Principals struct {
			Principal struct {
				User                string `xml:"UserId"`
				LogonType, RunLevel string
			}
		}
		Settings struct {
			Hidden             bool
			ExecutionTimeLimit string
			RestartOnFailure   struct {
				Interval string
				Count    int
			}
		}
		Actions struct {
			Exec struct{ Command, Arguments, WorkingDirectory string }
		}
	}
	if err := xml.Unmarshal([]byte(text), &task); err != nil {
		t.Fatal(err)
	}
	if task.XMLName.Local != "Task" || task.Triggers.Boot == nil || task.Triggers.Logon.User != "S-1-5-21-123" {
		t.Fatalf("bad triggers: %s", text)
	}
	if task.Triggers.Boot.Repetition.Interval != "PT1M" || task.Triggers.Boot.Repetition.Duration != "" || task.Triggers.Logon.Repetition.Interval != "PT1M" || task.Triggers.Logon.Repetition.Duration != "" {
		t.Fatal("retries must continue indefinitely", text)
	}
	if task.Principals.Principal.User != "S-1-5-21-123" || task.Principals.Principal.LogonType != "S4U" || task.Principals.Principal.RunLevel != "LeastPrivilege" {
		t.Fatal(text)
	}
	if !task.Settings.Hidden || task.Settings.ExecutionTimeLimit != "PT0S" || task.Settings.RestartOnFailure.Interval != "PT1M" || task.Settings.RestartOnFailure.Count < 999 {
		t.Fatal(text)
	}
	if task.Actions.Exec.Command != `C:\Program Files\hev & co\hev.exe` || task.Actions.Exec.Arguments != `daemon --config "C:\Users\hev\a & b.toml"` || task.Actions.Exec.WorkingDirectory != `C:\Users\hev` {
		t.Fatal(text)
	}
	if strings.Contains(text, "Password") {
		t.Fatal("task must not contain a password")
	}
}
func TestWindowsQuoteArg(t *testing.T) {
	for _, tc := range []struct{ in, want string }{{`C:\x\`, `"C:\x\\"`}, {`a"b`, `"a\"b"`}, {"", `""`}} {
		if got := windowsQuoteArg(tc.in); got != tc.want {
			t.Errorf("%q: got %q want %q", tc.in, got, tc.want)
		}
	}
}
