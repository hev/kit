package dispatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResultWatchFinalEvents(t *testing.T) {
	now := time.Now()
	for _, c := range []struct {
		name, out string
		final     bool
	}{
		{"claude result", `{"type":"result","subtype":"success"}`, true},
		{"claude subagent result", `{"type":"result","parent_tool_use_id":"toolu_1"}`, false},
		{"codex turn.completed", `{"type":"turn.completed","usage":{}}`, true},
		{"codex agent message", `{"type":"item.completed","item":{"type":"agent_message"}}`, false},
		{"pi agent_end", `{"type":"agent_end"}`, true},
		{"pi turn_end", `{"type":"turn_end"}`, false},
		{"result then more", "{\"type\":\"result\"}\n{\"type\":\"assistant\"}", false},
		{"result then stderr and our own record", "{\"type\":\"result\"}\nwarning: az not found\n{\"type\":\"dispatch.round\"}", true},
		{"partial line", `{"type":"result"`, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "log")
			if err := os.WriteFile(p, []byte("{\"type\":\"result\"}\n"), 0600); err != nil {
				t.Fatal(err)
			}
			// Output from an earlier attempt never counts.
			w := newResultWatch(p)
			w.poll(now)
			if w.final != "" {
				t.Fatal("earlier attempt's result counted")
			}
			out := c.out
			if c.name != "partial line" {
				out += "\n"
			}
			f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0600)
			_, _ = f.WriteString(out)
			_ = f.Close()
			w.poll(now)
			if got := w.expired(now.Add(time.Second), time.Second); got != c.final {
				t.Fatalf("expired = %v, want %v", got, c.final)
			}
			if c.final && w.expired(now.Add(time.Second-time.Millisecond), time.Second) {
				t.Fatal("expired before the grace")
			}
		})
	}
}

// An orchestrator round whose harness printed its result but stays alive for
// a background task ends without error, and the round trace says so.
func TestOrchestratorRoundEndsAfterFinalResult(t *testing.T) {
	defer func(d time.Duration, g time.Duration) { roundHaltPoll, resultGrace = d, g }(roundHaltPoll, resultGrace)
	roundHaltPoll, resultGrace = 5*time.Millisecond, 50*time.Millisecond
	t.Setenv("HEV_DISPATCH_RESULT_GRACE", "")
	orch := t.TempDir()
	trace := filepath.Join(orch, "trace")
	f, err := os.OpenFile(trace, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	watch := newResultWatch(trace)
	_, _ = f.WriteString("{\"type\":\"assistant\"}\n{\"type\":\"result\",\"is_error\":false}\n")
	killed := make(chan struct{})
	start := time.Now()
	err = superviseOrchestratorRoundWatch(orch, func() error { <-killed; return os.ErrProcessDone }, func() { close(killed) }, watch, f)
	if err != nil || time.Since(start) > 5*time.Second {
		t.Fatalf("%v after %s", err, time.Since(start))
	}
	b, _ := os.ReadFile(trace)
	if !strings.Contains(string(b), "dispatch.background_ended") {
		t.Fatalf("trace %s", b)
	}
}
