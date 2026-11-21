package cluster

import (
	"encoding/hex"
	"time"
)

const jobLimit = 256

func validJobDeadline(key string, deadline time.Time) bool {
	_, err := hex.DecodeString(key)
	return len(key) == 32 && err == nil && deadline.Year() >= 1970 && deadline.Year() <= 9999
}

func mergeJobDeadlines(dst *counterState, incoming map[string]time.Time, now time.Time) error {
	if len(incoming) > jobLimit {
		return ErrBound
	}
	if dst.JobDeadlines == nil {
		dst.JobDeadlines = make(map[string]time.Time)
	}
	for key, until := range incoming {
		if !validJobDeadline(key, until) {
			return ErrBound
		}
		if until.After(now) && until.After(dst.JobDeadlines[key]) {
			dst.JobDeadlines[key] = until
		}
	}
	if len(dst.JobDeadlines) > jobLimit {
		return ErrBound
	}
	return nil
}

// ReserveJobCooldown claims a deadline in this node's view and gossips it.
// True means an unexpired deadline was already known. This is an advisory
// cooldown for a SWIM winner, not a distributed lock.
func (c *Counters) ReserveJobCooldown(key string, until time.Time) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	prune(&c.state, now)
	if !c.ready || !validJobDeadline(key, until) {
		return false, ErrBound
	}
	if c.state.JobDeadlines[key].After(now) {
		return true, nil
	}
	if !until.After(now) {
		return false, nil
	}
	if len(c.state.JobDeadlines) >= jobLimit {
		return false, ErrBound
	}
	delta := map[string]time.Time{key: until.UTC()}
	_ = mergeJobDeadlines(&c.state, delta, now)
	if !stateWithinBounds(c.state) {
		delete(c.state.JobDeadlines, key)
		return false, ErrBound
	}
	c.emit(counterState{JobDeadlines: delta})
	return false, nil
}

func (s *Service) ReserveJobCooldown(key string, until time.Time) (bool, error) {
	c, err := s.Counters()
	if err != nil {
		return false, err
	}
	return c.ReserveJobCooldown(key, until)
}

// JobMembers withholds a seeded node's view until its first state exchange.
func (s *Service) JobMembers() []Member {
	s.mu.Lock()
	n := s.node
	s.mu.Unlock()
	if n == nil {
		return nil
	}
	n.Counters.mu.Lock()
	ready := n.ready
	n.Counters.mu.Unlock()
	if !ready {
		return nil
	}
	n.mu.Lock()
	inactive := n.paused || n.closed
	n.mu.Unlock()
	if inactive {
		return nil
	}
	return n.Members()
}
