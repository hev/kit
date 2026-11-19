package compat

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// AgentCounts is the local agent-pane tally hev peers ls would show: panes
// in the foh/boh roles running a recognised agent, split by peers' own state
// reading. Headless rows (dispatch workers, managed runners, orphan agent
// processes) are not panes and are left out.
type AgentCounts struct {
	Working int // peers state "working": mid-turn
	Idle    int // any other state: at the prompt, a dialog, a draft, or unreadable
}

// ErrNoTmux means this node has no tmux, so there are no panes to count.
var ErrNoTmux = errors.New("tmux unavailable")

// LocalAgentCounts reads local panes the way peers ls does, but builds its
// reader directly: CLI initialisation mutates PATH and consumes the stdin
// envelope, which an embedded observer must not do.
func LocalAgentCounts() (out AgentCounts, err error) {
	if runtime.GOOS == "windows" {
		return out, ErrNoTmux
	}
	if _, e := exec.LookPath("tmux"); e != nil {
		return out, ErrNoTmux
	}
	home, _ := os.UserHomeDir()
	state := os.Getenv("HEV_PEERS_STATE")
	if state == "" {
		state = filepath.Join(home, ".local/state/peers-v2")
	}
	p := &P{home: home, stateDir: state, out: io.Discard, err: io.Discard}
	defer func() {
		if recover() != nil {
			out, err = AgentCounts{}, errors.New("local peers observation unavailable")
		}
	}()
	return countAgents(p.panes(false), p.state), nil
}

func countAgents(rows []*Row, state func(*Row) string) AgentCounts {
	var out AgentCounts
	for _, r := range rows {
		if r.Agent == "" || r.Headless || r.Managed != nil || !participates(r.Role) {
			continue
		}
		if safeState(state, r) == "working" {
			out.Working++
		} else {
			out.Idle++
		}
	}
	return out
}

// safeState keeps one pane closing between list and capture from voiding the
// whole sample: tmux's failure there reads as not working.
func safeState(state func(*Row) string, r *Row) (s string) {
	defer func() {
		if recover() != nil {
			s = "-"
		}
	}()
	return state(r)
}
