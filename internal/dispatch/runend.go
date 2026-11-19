package dispatch

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"time"
)

// A harness can print its final result and then keep running because a
// background task it started is still alive: claude -p waits for a background
// bash poller, for example. The run is finished at that point. After the
// final event the supervisor waits resultGrace for the harness to exit on its
// own; if it is still running and has printed nothing since, the supervisor
// ends the process tree and finishes the run with the worker's status marker.
//
// Tests shorten the grace; HEV_DISPATCH_RESULT_GRACE overrides it on a host.
var resultGrace = 30 * time.Second

func resultGraceDuration() time.Duration {
	if d, err := time.ParseDuration(os.Getenv("HEV_DISPATCH_RESULT_GRACE")); err == nil && d > 0 {
		return d
	}
	return resultGrace
}

// resultWatch follows the harness's own log output from the launch offset.
type resultWatch struct {
	path    string
	offset  int64
	partial []byte
	// final is the type of the last harness event when that event ended the
	// worker's turn, else empty; since is when it was read.
	final   string
	isError bool
	since   time.Time
}

func newResultWatch(path string) *resultWatch {
	w := &resultWatch{path: path}
	if info, err := os.Stat(path); err == nil {
		// The log is appended across attempts; only this attempt counts.
		w.offset = info.Size()
	}
	return w
}

// poll reads output appended since the last call.
func (w *resultWatch) poll(now time.Time) {
	f, err := os.Open(w.path)
	if err != nil {
		return
	}
	defer f.Close()
	if _, err = f.Seek(w.offset, io.SeekStart); err != nil {
		return
	}
	b, err := io.ReadAll(io.LimitReader(f, 16<<20))
	if err != nil || len(b) == 0 {
		return
	}
	w.offset += int64(len(b))
	b = append(w.partial, b...)
	end := bytes.LastIndexByte(b, '\n')
	if end < 0 {
		w.partial = b
		return
	}
	w.partial = append([]byte(nil), b[end+1:]...)
	for _, line := range bytes.Split(b[:end], []byte("\n")) {
		w.observe(line, now)
	}
}

// observe classifies one output line. Only harness JSON events move the
// state: stderr noise and the supervisor's own dispatch.* records do not.
func (w *resultWatch) observe(line []byte, now time.Time) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 || line[0] != '{' {
		return
	}
	var v struct {
		Type            string `json:"type"`
		ParentToolUseID string `json:"parent_tool_use_id"`
		IsError         bool   `json:"is_error"`
	}
	if json.Unmarshal(line, &v) != nil || v.Type == "" || strings.HasPrefix(v.Type, "dispatch.") {
		return
	}
	if finalHarnessEvent(v.Type, v.ParentToolUseID) {
		if w.final == "" {
			w.since = now
		}
		w.final, w.isError = v.Type, v.IsError
		return
	}
	// Anything else means the worker is still in a turn: a result that is
	// followed by more events was not its final one.
	w.final, w.isError, w.since = "", false, time.Time{}
}

// finalHarnessEvent reports the event a harness prints once the worker's turn
// is over: claude's result (a subagent's would carry its parent tool), codex
// exec's turn.completed, and pi's agent_end. pi's turn_end and codex's
// item.completed are mid-turn and never count.
func finalHarnessEvent(kind, parentTool string) bool {
	switch kind {
	case "result":
		return parentTool == ""
	case "turn.completed", "agent_end":
		return true
	}
	return false
}

// expired reports a final event that has stood unanswered for the grace.
func (w *resultWatch) expired(now time.Time, grace time.Duration) bool {
	return w.final != "" && now.Sub(w.since) >= grace
}

// recordBackgroundEnded notes in the run log that the supervisor ended what
// the harness left running after its final result.
func recordBackgroundEnded(log io.Writer, w *resultWatch, grace time.Duration, what string) {
	_ = json.NewEncoder(log).Encode(map[string]any{
		"type":    "dispatch.background_ended",
		"event":   w.final,
		"grace":   grace.String(),
		"message": "harness printed its final " + w.final + " event and was still running after " + grace.String() + "; ended the background tasks it left running and finished the " + what,
	})
}
