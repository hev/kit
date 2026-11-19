//go:build !windows

package dispatch

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// resultHarness replaces the outcome fixture's claude with script.
func resultHarness(t *testing.T, script string) string {
	t.Helper()
	bin := filepath.SplitList(os.Getenv("PATH"))[0]
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\n"+script), 0700); err != nil {
		t.Fatal(err)
	}
	return bin
}

func shortGrace(t *testing.T, d time.Duration) {
	t.Helper()
	old := resultGrace
	resultGrace = d
	t.Cleanup(func() { resultGrace = old })
	t.Setenv("HEV_DISPATCH_RESULT_GRACE", "")
}

// The bcc case: the worker prints its result, but a background poller it
// started keeps the harness waiting. The run ends done within the grace, the
// poller is ended, and the log says so.
func TestFinalResultEndsRunWithBackgroundTasks(t *testing.T) {
	dir, _, _ := outcomeFixture(t, "bg-poller")
	shortGrace(t, 500*time.Millisecond)
	bin := resultHarness(t, `printf '{"type":"system","subtype":"init","session_id":"s-bg"}\n'
printf 'working\ndone\n' > ../status.md
sleep 300 &
echo $! > "$CHILD_PID"
printf '{"type":"result","subtype":"success","is_error":false,"result":"done"}\n'
wait
`)
	pidFile := filepath.Join(bin, "child.pid")
	t.Setenv("CHILD_PID", pidFile)

	start := time.Now()
	if err := superviseRound("bg-poller"); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took > 10*time.Second {
		t.Fatalf("run took %s; the grace is 500ms", took)
	}
	r := readResult(t, dir)
	if !r.BackgroundEnded || r.ExitCode != 0 || r.Stopped || r.Timeout || r.Outcome != OutcomeDone || State(&r, false) != "done" {
		t.Fatalf("result %+v state %s", r, State(&r, false))
	}
	b, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	deadline := time.Now().Add(5 * time.Second)
	for syscall.Kill(pid, 0) == nil {
		if time.Now().After(deadline) {
			t.Fatalf("background child %d still running", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
	log, _ := os.ReadFile(filepath.Join(dir, "log"))
	if !strings.Contains(string(log), `"type":"dispatch.background_ended"`) || !strings.Contains(string(log), "ended the background tasks it left running") {
		t.Fatalf("log does not record ending background tasks:\n%s", log)
	}
}

// A result followed by more harness events was mid-turn: the run keeps going
// past the grace and ends only when the harness does.
func TestMidTurnResultDoesNotEndRun(t *testing.T) {
	dir, _, _ := outcomeFixture(t, "mid-turn")
	shortGrace(t, time.Second)
	bin := resultHarness(t, `printf '{"type":"result","subtype":"success","is_error":false}\n'
sleep 0.1
printf '{"type":"assistant","message":{"content":[]}}\n'
sleep 1.6
printf '{"type":"turn_end"}\n'
sleep 1.6
printf 'working\ndone\n' > ../status.md
touch "$FINISHED"
printf '{"type":"result","subtype":"success","is_error":false}\n'
`)
	finished := filepath.Join(bin, "finished")
	t.Setenv("FINISHED", finished)
	if err := superviseRound("mid-turn"); err != nil {
		t.Fatal(err)
	}
	r := readResult(t, dir)
	if r.BackgroundEnded || r.ExitCode != 0 || r.Outcome != OutcomeDone {
		t.Fatalf("mid-turn result ended the run: %+v", r)
	}
	if !exists(finished) {
		t.Fatal("harness was ended before it finished its turn")
	}
	log, _ := os.ReadFile(filepath.Join(dir, "log"))
	if strings.Contains(string(log), "dispatch.background_ended") {
		t.Fatalf("log claims background tasks were ended:\n%s", log)
	}
}
