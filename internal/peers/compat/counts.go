package compat

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

// Counts deliberately returns only aggregates from the same local inventory
// and state reader as peers ls. Remote beacon snapshots are not included.
type Counts struct {
	Harness map[string][2]uint32
	Role    map[string][2]uint32
}

func LocalCounts() (out Counts, err error) {
	// CLI initialization mutates PATH and consumes stdin envelope environment;
	// an embedded read-only observer must not run that initialization.
	home, _ := os.UserHomeDir()
	state := os.Getenv("HEV_PEERS_STATE")
	if state == "" {
		state = filepath.Join(home, ".local/state/peers-v2")
	}
	p := &P{home: home, stateDir: state, out: io.Discard, err: io.Discard}
	defer func() {
		if recover() != nil {
			out = Counts{}
			err = errors.New("local peers observation unavailable")
		}
	}()
	return countRows(p.panes(false), p.state), nil
}
func countRows(rows []*Row, state func(*Row) string) Counts {
	out := Counts{map[string][2]uint32{}, map[string][2]uint32{}}
	for _, r := range rows {
		if r.Agent == "" || !participates(r.Role) {
			continue
		}
		h := r.Agent
		switch h {
		case "claude", "codex", "pi", "opencode":
		default:
			h = "other"
		}
		i := 0
		if state(r) != "working" {
			i = 1
		}
		v := out.Harness[h]
		v[i]++
		out.Harness[h] = v
		role := peerRole(r.Role)
		v = out.Role[role]
		v[i]++
		out.Role[role] = v
	}
	return out
}
