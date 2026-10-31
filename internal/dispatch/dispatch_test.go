package dispatch

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const good = "---\nname: example\nhost: buildbox\nrepo: kit\nissue: TASK-1\n---\n## Acceptance\n- done\n"

func TestParse(t *testing.T) {
	o, e := Parse(good)
	if e != nil || o.Name != "example" || o.Timeout != 120 || o.Issue != "TASK-1" {
		t.Fatalf("%+v %v", o, e)
	}
	for _, s := range []string{strings.Replace(good, "name: example", "name: --help", 1), strings.Replace(good, "host:", "surprise:", 1), strings.Replace(good, "## Acceptance", "## Other", 1), strings.Replace(good, "name: example", "name: ../escape", 1), strings.Replace(good, "repo: kit", "repo: kit\ntimeout: -1", 1), strings.Replace(good, "repo: kit", "repo: kit\nissue: duplicate", 1)} {
		if _, e := Parse(s); e == nil {
			t.Fatalf("accepted %s", s)
		}
	}
}
func TestCap(t *testing.T) {
	if e := CheckCap(3, 3); e == nil || e.Error() != "host full (3/3)" {
		t.Fatal(e)
	}
	if e := CheckCap(2, 3); e != nil {
		t.Fatal(e)
	}
}
func TestState(t *testing.T) {
	for _, tc := range []struct {
		r    *Result
		live bool
		want string
	}{{nil, true, "running"}, {nil, false, "died"}, {&Result{ExitCode: 0, End: time.Now()}, false, "done"}, {&Result{ExitCode: 143, End: time.Now()}, false, "failed"}, {&Result{Timeout: true, End: time.Now()}, false, "timeout"}} {
		if got := State(tc.r, tc.live); got != tc.want {
			t.Fatalf("%s != %s", got, tc.want)
		}
	}
}
func TestNotifyFailureIsolation(t *testing.T) {
	r := Result{ExitCode: 0, End: time.Now()}
	e := Notify(context.Background(), []string{filepath.Join(t.TempDir(), "absent")}, Meta{}, r)
	if e == nil || State(&r, false) != "done" || r.ExitCode != 0 {
		t.Fatalf("%v %+v", e, r)
	}
}
func TestWindowsLauncher(t *testing.T) {
	exe := `C:\Program Files\hev\hev.exe`
	if !reflect.DeepEqual(WindowsLaunch(exe, "example"), []string{exe, "dispatch", "supervise", "--", "example"}) {
		t.Fatal("launcher must preserve argument boundaries")
	}
}
func TestSessionID(t *testing.T) {
	p := filepath.Join(t.TempDir(), "log")
	for _, s := range []string{`{"type":"thread.started","thread_id":"codex-session"}`, `{"type":"system","session_id":"claude-session"}`} {
		if e := os.WriteFile(p, []byte("noise\n"+s+"\n"), 0600); e != nil {
			t.Fatal(e)
		}
		if sessionFromLog(p) == "" {
			t.Fatal("session missing")
		}
	}
}
func TestAttribution(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := filepath.Join(Root(), "example")
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(dir, "order.md"), []byte(good), 0600); e != nil {
		t.Fatal(e)
	}
	n, i := Attribution(filepath.Join(dir, "wt", "sub"))
	if n != "example" || i != "TASK-1" {
		t.Fatalf("%s %s", n, i)
	}
	if n, _ = Attribution(filepath.Join(Root(), "..", "escape", "wt")); n != "" {
		t.Fatal(n)
	}
}
func TestResultRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "result.json")
	want := Result{Start: time.Now().UTC(), ExitCode: 143, Session: "abc", Stopped: true}
	if e := WriteJSON(p, want); e != nil {
		t.Fatal(e)
	}
	var got Result
	if e := readJSON(p, &got); e != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("%+v %v", got, e)
	}
}

func TestWorkerPromptReportsFlakyTests(t *testing.T) {
	for _, instruction := range []string{"flaky test, not a pass", "Report it by name in ../report.md", "fix it if it is in scope", "never rerun until green"} {
		if !strings.Contains(Prompt, instruction) {
			t.Fatalf("worker prompt missing %q", instruction)
		}
	}
}
