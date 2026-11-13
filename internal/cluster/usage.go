package cluster

import "time"

// UsagePolicy is a fixed metadata heartbeat, refreshed only after reading the
// durable ledger. The maps are restricted to two provider labels.
type UsagePolicy struct {
	At     time.Time      `json:"at"`
	Daily  map[string]int `json:"daily"`
	Hourly map[string]int `json:"hourly"`
}

const usageSkew = time.Minute

func validUsage(node string, u UsagePolicy, now time.Time) bool {
	if !identifier(node) || u.At.IsZero() || u.At.Year() < 1970 || u.At.Year() > 9999 || u.At.After(now.Add(usageSkew)) {
		return false
	}
	for _, m := range []map[string]int{u.Daily, u.Hourly} {
		if len(m) > 2 {
			return false
		}
		for k, v := range m {
			if (k != "op" && k != "azkv") || v <= 0 {
				return false
			}
		}
	}
	return true
}
func (c *Counters) SetUsage(u UsagePolicy) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.ready || !validUsage(c.node, u, c.now()) {
		return ErrBound
	}
	prune(&c.state, c.now())
	if c.state.Usage == nil {
		c.state.Usage = map[string]UsagePolicy{}
	}
	if len(c.state.Usage) >= 64 && c.state.Usage[c.node].At.IsZero() {
		return ErrBound
	}
	// Copy maps: callers must not be able to mutate counter state.
	u = copyUsage(u)
	if u.At.After(c.state.Usage[c.node].At) {
		old, existed := c.state.Usage[c.node]
		c.state.Usage[c.node] = u
		if !stateWithinBounds(c.state) {
			if existed {
				c.state.Usage[c.node] = old
			} else {
				delete(c.state.Usage, c.node)
			}
			return ErrBound
		}
		c.emit(counterState{Usage: map[string]UsagePolicy{c.node: u}})
	}
	return nil
}
func copyUsage(u UsagePolicy) UsagePolicy {
	d, h := map[string]int{}, map[string]int{}
	for k, v := range u.Daily {
		d[k] = v
	}
	for k, v := range u.Hourly {
		h[k] = v
	}
	u.Daily, u.Hourly = d, h
	return u
}
func (c *Counters) Usage() map[string]UsagePolicy {
	c.mu.Lock()
	defer c.mu.Unlock()
	prune(&c.state, c.now())
	out := map[string]UsagePolicy{}
	for n, u := range c.state.Usage {
		out[n] = copyUsage(u)
	}
	return out
}
