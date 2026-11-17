package cluster

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/memberlist"
)

// ActivityRefresh is how often hevd republishes its activity summary.
const ActivityRefresh = 20 * time.Second

// ActivityStale is the age past which a member's summary is marked stale.
// Stale summaries are still shown: an old count beats no count.
const ActivityStale = 2 * time.Minute

const maxActivityCount = 65536

// Activity is the small summary each node gossips in its member metadata.
// A nil count means the node cannot tell (for example agent panes on a node
// without tmux); it is left out of the JSON rather than reported as 0.
type Activity struct {
	AgentsWorking *int      `json:"agents_working,omitempty"`
	AgentsIdle    *int      `json:"agents_idle,omitempty"`
	HeadlessRuns  *int      `json:"headless_runs,omitempty"`
	Expediters    *int      `json:"expediters,omitempty"`
	At            time.Time `json:"at"`
	// Stale is computed by the reader from At; publishers never send it.
	Stale bool `json:"stale,omitempty"`
}

func (a *Activity) valid() bool {
	for _, p := range []*int{a.AgentsWorking, a.AgentsIdle, a.HeadlessRuns, a.Expediters} {
		if p != nil && (*p < 0 || *p > maxActivityCount) {
			return false
		}
	}
	return !a.At.IsZero() && a.At.Year() >= 2000 && a.At.Year() <= 9999
}

// Short renders a compact human summary such as "2w/1i 3r 1x" or "- (stale)".
// w/i are agents working/idle, r headless runs, x Expediters.
func (a *Activity) Short() string {
	if a == nil {
		return "-"
	}
	var parts []string
	if a.AgentsWorking != nil || a.AgentsIdle != nil {
		parts = append(parts, count(a.AgentsWorking)+"w/"+count(a.AgentsIdle)+"i")
	}
	if a.HeadlessRuns != nil {
		parts = append(parts, count(a.HeadlessRuns)+"r")
	}
	if a.Expediters != nil {
		parts = append(parts, count(a.Expediters)+"x")
	}
	s := strings.Join(parts, " ")
	if s == "" {
		s = "-"
	}
	if a.Stale {
		s += " (stale)"
	}
	return s
}

func count(p *int) string {
	if p == nil {
		return "?"
	}
	return fmt.Sprint(*p)
}

// SetActivity replaces this node's gossiped activity and re-advertises its
// metadata. It does nothing while the node has explicitly left: re-advertising
// would announce it alive again.
func (n *Node) SetActivity(a Activity) error {
	a.Stale = false
	a.At = a.At.UTC().Truncate(time.Second)
	if !a.valid() {
		return errors.New("invalid cluster activity")
	}
	n.life.Lock()
	defer n.life.Unlock()
	n.mu.Lock()
	closed, paused := n.closed, n.paused
	n.mu.Unlock()
	if closed || paused {
		return errors.New("cluster not joined")
	}
	n.metaMu.Lock()
	var f Facts
	if json.Unmarshal(n.facts, &f) != nil {
		n.metaMu.Unlock()
		return errors.New("cluster facts unreadable")
	}
	f.Activity = &a
	b, _ := json.Marshal(f)
	if len(b) > memberlist.MetaMaxSize {
		n.metaMu.Unlock()
		return ErrBound
	}
	n.facts = b
	n.metaMu.Unlock()
	// The local entry follows immediately; peers follow through gossip.
	n.mu.Lock()
	if m, ok := n.members[n.cfg.Name]; ok {
		m.Activity = &a
		n.members[n.cfg.Name] = m
	}
	n.mu.Unlock()
	return n.ml.UpdateNode(5 * time.Second)
}
