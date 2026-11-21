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

func mergeCronDeadlines(dst *counterState, incoming map[string]time.Time, now time.Time) error {
	if len(incoming) > jobLimit {
		return ErrBound
	}
	if dst.CronDeadlines == nil {
		dst.CronDeadlines = make(map[string]time.Time)
	}
	for key, until := range incoming {
		if !validJobDeadline(key, until) {
			return ErrBound
		}
		if until.After(now) && until.After(dst.CronDeadlines[key]) {
			dst.CronDeadlines[key] = until
		}
	}
	if len(dst.CronDeadlines) > jobLimit {
		return ErrBound
	}
	return nil
}

// ReserveCronCooldown claims a deadline in this node's view and gossips it.
// True means an unexpired deadline was already known. This is an advisory
// cooldown for a SWIM winner, not a distributed lock.
func (c *Counters) ReserveCronCooldown(key string, until time.Time) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	prune(&c.state, now)
	if !c.ready || !validJobDeadline(key, until) {
		return false, ErrBound
	}
	if c.state.CronDeadlines[key].After(now) {
		return true, nil
	}
	if !until.After(now) {
		return false, nil
	}
	if len(c.state.CronDeadlines) >= jobLimit {
		return false, ErrBound
	}
	delta := map[string]time.Time{key: until.UTC()}
	_ = mergeCronDeadlines(&c.state, delta, now)
	if !stateWithinBounds(c.state) {
		delete(c.state.CronDeadlines, key)
		return false, ErrBound
	}
	c.emit(counterState{CronDeadlines: delta})
	return false, nil
}

func (s *Service) ReserveCronCooldown(key string, until time.Time) (bool, error) {
	c, err := s.Counters()
	if err != nil {
		return false, err
	}
	return c.ReserveCronCooldown(key, until)
}

// CronMembers withholds a seeded node's view until its first state exchange.
func (s *Service) CronMembers() []Member {
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
