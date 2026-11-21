//go:build !windows

package compat

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// waiter is a background `peers inbox --wait` acting from a pane.
type waiter struct {
	cmd    *exec.Cmd
	stderr bytes.Buffer
	done   chan struct{}
	code   int
}

func (h *host) wait(pane string, args ...string) *waiter {
	h.c.t.Helper()
	exe, _ := os.Executable()
	w := &waiter{cmd: exec.Command(exe, append([]string{"inbox", "--wait"}, args...)...), done: make(chan struct{})}
	w.cmd.Env = append(h.env(pane), "PEERS_COMPAT_MAIN=1")
	w.cmd.Dir = h.home
	w.cmd.Stderr = &w.stderr
	must(h.c.t, w.cmd.Start())
	go func() {
		err := w.cmd.Wait()
		if exitErr, ok := err.(*exec.ExitError); ok {
			w.code = exitErr.ExitCode()
		} else if err != nil {
			w.code = -1
		}
		close(w.done)
	}()
	h.c.t.Cleanup(func() { w.cmd.Process.Kill(); <-w.done })
	return w
}

// exited reports whether the waiter ended within d, and its exit status.
func (w *waiter) exited(d time.Duration) (bool, int) {
	select {
	case <-w.done:
		return true, w.code
	case <-time.After(d):
		return false, 0
	}
}

func TestInboxWaitWakesWithoutConsuming(t *testing.T) {
	c := newGoCluster(t)
	h := c.host("laptop", "go")
	h.agent("alpha", true)
	h.agent("bravo", true)
	receipts := filepath.Join(h.home, ".local/state/peers-v2/inbox/alpha.jsonl.receipts.jsonl")

	// An idle bound session with no items blocks.
	w := h.wait("alpha", "--timeout", "60s")
	if done, code := w.exited(1500 * time.Millisecond); done {
		t.Fatalf("waiter exited %d with an empty inbox: %s", code, w.stderr.String())
	}
	// A send wakes it within two seconds, with the item, exit 2.
	h.ok(goImpl, "bravo", "", "send", "alpha", "wake up", "--id", "wake-1")
	sent := time.Now()
	done, code := w.exited(2 * time.Second)
	if !done || code != 2 {
		t.Fatalf("waiter after send: exited=%t code=%d after %s: %s", done, code, time.Since(sent), w.stderr.String())
	}
	for _, want := range []string{"Peer inbox wake: 1 new item(s)", `"id": "wake-1"`, "wake up", "inbox consume ID", "inbox ack ID"} {
		if !strings.Contains(w.stderr.String(), want) {
			t.Fatalf("wake text lacks %q:\n%s", want, w.stderr.String())
		}
	}
	// Waking wrote no consumption or ACK.
	if n := len(lines(receipts)); n != 0 {
		t.Fatalf("waking wrote %d receipts", n)
	}
	if got := strings.TrimSpace(h.ok(goImpl, "alpha", "", "inbox", "--count")); got != "1" {
		t.Fatalf("unread after wake: %s", got)
	}
	// The next Stop's waiter does not rewake for the same item.
	if _, errout, code := h.run(goImpl, "alpha", "", "inbox", "--wait", "--timeout", "1"); code != 0 {
		t.Fatalf("rewoke for an announced item: %d %s", code, errout)
	}
	// The item replays through the prompt hook until it is acknowledged.
	hook := func() string {
		exe, _ := os.Executable()
		cmd := exec.Command(exe, "inbox-hook")
		cmd.Env = append(h.env("alpha"), "PEERS_COMPAT_MAIN=1", "PEERS_INBOX_BIN="+filepath.Join(h.bin, "peers"))
		cmd.Stdin = strings.NewReader(`{"hook_event_name": "UserPromptSubmit", "prompt": "hi"}`)
		out, err := cmd.Output()
		must(t, err)
		return string(out)
	}
	for i := 0; i < 2; i++ {
		if out := hook(); !strings.Contains(out, `\"id\": \"wake-1\"`) {
			t.Fatalf("replay %d lacks the item: %s", i, out)
		}
	}
	h.ok(goImpl, "alpha", "", "inbox", "ack", "wake-1")
	if out := hook(); out != "" {
		t.Fatalf("acknowledged item replayed: %s", out)
	}
}

func TestInboxWaitTimeoutAndAcked(t *testing.T) {
	c := newGoCluster(t)
	h := c.host("laptop", "go")
	h.agent("alpha", true)
	h.agent("bravo", true)
	start := time.Now()
	if out, errout, code := h.run(goImpl, "alpha", "", "inbox", "--wait", "--timeout", "1s"); code != 0 || out != "" || errout != "" {
		t.Fatalf("timeout: %d %q %q", code, out, errout)
	}
	if d := time.Since(start); d < time.Second {
		t.Fatalf("timed out after %s", d)
	}
	// An item consumed and acknowledged before the waiter starts never wakes it.
	h.ok(goImpl, "bravo", "", "send", "alpha", "already handled", "--id", "done-1")
	h.ok(goImpl, "alpha", "", "inbox")
	h.ok(goImpl, "alpha", "", "inbox", "ack", "done-1")
	if _, errout, code := h.run(goImpl, "alpha", "", "inbox", "--wait", "--timeout", "1"); code != 0 {
		t.Fatalf("woke for an acknowledged item: %d %s", code, errout)
	}
	// A consumed, unacknowledged item replays on the prompt, not as a wake.
	h.ok(goImpl, "bravo", "", "send", "alpha", "read not acked", "--id", "read-1")
	h.ok(goImpl, "alpha", "", "inbox")
	if _, errout, code := h.run(goImpl, "alpha", "", "inbox", "--wait", "--timeout", "1"); code != 0 {
		t.Fatalf("woke for a consumed item: %d %s", code, errout)
	}
	// An item already waiting when the turn ends wakes at once.
	h.ok(goImpl, "bravo", "", "send", "alpha", "queued mid-turn", "--id", "mid-1")
	if _, errout, code := h.run(goImpl, "alpha", "", "inbox", "--wait", "--timeout", "10"); code != 2 || !strings.Contains(errout, "mid-1") || strings.Contains(errout, "read-1") {
		t.Fatalf("pending item: %d %s", code, errout)
	}
	for _, args := range [][]string{{"inbox", "--wait", "--timeout", "soon"}, {"inbox", "--wait", "--timeout"}, {"inbox", "ack", "x", "--wait"}} {
		if _, _, code := h.run(goImpl, "alpha", "", args...); code != 1 {
			t.Fatalf("%v: exit %d", args, code)
		}
	}
}

func TestInboxWaitNewestWaiterWins(t *testing.T) {
	c := newGoCluster(t)
	h := c.host("laptop", "go")
	h.agent("alpha", true)
	h.agent("bravo", true)
	h.agent("plain", false)
	// Claude starts one hook per Stop; an older waiter stands down silently.
	old := h.wait("alpha", "--timeout", "60s")
	time.Sleep(500 * time.Millisecond)
	newer := h.wait("alpha", "--timeout", "60s")
	if done, code := old.exited(3 * time.Second); !done || code != 0 {
		t.Fatalf("superseded waiter: exited=%t code=%d %s", done, code, old.stderr.String())
	}
	h.ok(goImpl, "bravo", "", "send", "alpha", "one wake", "--id", "one-1")
	if done, code := newer.exited(2 * time.Second); !done || code != 2 {
		t.Fatalf("newest waiter: exited=%t code=%d %s", done, code, newer.stderr.String())
	}
	// A session without an inbox has nothing to wait for.
	if _, errout, code := h.run(goImpl, "plain", "", "inbox", "--wait", "--timeout", "60"); code != 0 || !strings.Contains(errout, "inbox off") {
		t.Fatalf("inbox off: %d %s", code, errout)
	}
}

// A dispatch notice is queued by the native engine with --inbox-only; it wakes
// the idle recipient the same way.
func TestInboxWaitWakesOnDispatchNotice(t *testing.T) {
	c := newGoCluster(t)
	h := c.host("laptop", "go")
	h.agent("alpha", true)
	w := h.wait("alpha", "--timeout", "60s")
	exe, _ := os.Executable()
	send := exec.Command(exe, "send", "alpha", "run finished", "--from", "laptop/o-example", "--id", "notice-1", "--inbox-only")
	send.Env = append(h.env(""), "PEERS_NATIVE_MAIN=1")
	if out, err := send.CombinedOutput(); err != nil {
		t.Fatalf("native send: %v %s", err, out)
	}
	if done, code := w.exited(2 * time.Second); !done || code != 2 || !strings.Contains(w.stderr.String(), `"id": "notice-1"`) {
		t.Fatalf("notice: exited=%t code=%d %s", done, code, w.stderr.String())
	}
}
