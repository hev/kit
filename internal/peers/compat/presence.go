package compat

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"

	"github.com/hev/kit/internal/hevpaths"
)

// LocalPeer identifies one live local agent without exposing its process
// arguments, worktree or screen.
type LocalPeer struct {
	Address string
	Agent   string
}

// LocalPeers samples the same local participants as peers ls. PID and pane
// identity distinguish a replacement agent with the same address.
func LocalPeers() (peers map[string]LocalPeer, err error) {
	if runtime.GOOS == "windows" {
		return nil, ErrNoTmux
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		return nil, ErrNoTmux
	}
	home, _ := os.UserHomeDir()
	p := &P{home: home, stateDir: hevpaths.Dir(hevpaths.Peers), out: io.Discard, err: io.Discard}
	defer func() {
		if recover() != nil {
			peers, err = nil, errors.New("local peer inventory unavailable")
		}
	}()
	rows := p.panes(false)
	const maxPeers = 4096
	if len(rows) > maxPeers {
		return nil, errors.New("local peer inventory exceeds limit")
	}
	peers = make(map[string]LocalPeer, len(rows))
	for _, r := range rows {
		if r.Agent == "" {
			continue
		}
		peers[r.ID+":"+strconv.Itoa(r.AgentPID)] = LocalPeer{p.address(r, rows), r.Agent}
	}
	return peers, nil
}
