package cluster

import (
	"encoding/json"
	"errors"
	"math"
	"sync"
	"time"
)

const maxWire = 1 << 20

var ErrBound = errors.New("cluster fact exceeds bounds")

type counterState struct {
	Usage     map[string]UsagePolicy                  `json:"usage,omitempty"`
	Version   int                                     `json:"v"`
	Counts    map[string]map[string]map[string]uint64 `json:"counts"`
	Registers map[string]time.Time                    `json:"registers"`
	Meters    map[string]Meter                        `json:"meters"`
}
type Counters struct {
	mu      sync.Mutex
	node    string
	now     func() time.Time
	state   counterState
	changed func([]byte)
	ready   bool
}

func newCounters(node string, ready bool) *Counters {
	return &Counters{node: node, now: time.Now, ready: ready, state: counterState{Version: 1, Counts: map[string]map[string]map[string]uint64{}, Registers: map[string]time.Time{}, Meters: map[string]Meter{}}}
}

// MinuteWindow identifies one UTC minute of a rolling ledger. Day keys remain
// available for callers with calendar-day counters.
func MinuteWindow(t time.Time) string { return t.UTC().Format("2006-01-02T15:04Z") }
func parseWindow(w string) (time.Time, error) {
	if len(w) == 10 {
		return time.Parse("2006-01-02", w)
	}
	return time.Parse("2006-01-02T15:04Z", w)
}
func validWindow(w string, now time.Time) bool {
	if len(w) == 10 {
		return w == now.UTC().Format("2006-01-02") || w == now.UTC().AddDate(0, 0, -1).Format("2006-01-02")
	}
	t, e := parseWindow(w)
	return e == nil && !t.After(now.UTC().Truncate(time.Minute).Add(time.Minute)) && !t.Before(now.UTC().Truncate(time.Minute).Add(-24*time.Hour-time.Minute))
}
func prune(s *counterState, now time.Time) {
	for node, u := range s.Usage {
		if u.At.Before(now.Add(-24*time.Hour - usageSkew)) {
			delete(s.Usage, node)
		}
	}
	for n, ws := range s.Counts {
		for w := range ws {
			if !validWindow(w, now) {
				delete(ws, w)
			}
		}
		if len(ws) == 0 {
			delete(s.Counts, n)
		}
	}
}
func (c *Counters) Add(name, window string, n uint64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	prune(&c.state, c.now())
	return c.setLocked(name, window, n, false)
}
func (c *Counters) SetOwn(name, window string, n uint64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	prune(&c.state, c.now())
	return c.setLocked(name, window, n, true)
}
func (c *Counters) setLocked(name, window string, n uint64, absolute bool) error {
	if !c.ready {
		return errors.New("cluster counters awaiting initial seed exchange")
	}
	if !identifier(name) || !validWindow(window, c.now()) {
		return ErrBound
	}
	if n == 0 {
		return nil
	}
	ws := c.state.Counts[name]
	if ws == nil {
		if len(c.state.Counts) >= 32 {
			return ErrBound
		}
		ws = map[string]map[string]uint64{}
	}
	ns := ws[window]
	if ns == nil {
		ns = map[string]uint64{}
	}
	if _, ok := ns[c.node]; !ok && len(ns) >= 64 {
		return ErrBound
	}
	if absolute {
		if n <= ns[c.node] {
			return nil
		}
		n -= ns[c.node]
	}
	if math.MaxUint64-ns[c.node] < n {
		return ErrBound
	}
	// Keep the aggregate representable too.
	var sum uint64
	for _, v := range ns {
		if math.MaxUint64-sum < v {
			return ErrBound
		}
		sum += v
	}
	if math.MaxUint64-sum < n {
		return ErrBound
	}
	old := ns[c.node]
	ns[c.node] += n
	ws[window] = ns
	c.state.Counts[name] = ws
	if !stateWithinBounds(c.state) {
		if old == 0 {
			delete(ns, c.node)
		} else {
			ns[c.node] = old
		}
		if len(ns) == 0 {
			delete(ws, window)
		}
		if len(ws) == 0 {
			delete(c.state.Counts, name)
		}
		return ErrBound
	}
	c.emit(counterState{Counts: map[string]map[string]map[string]uint64{name: {window: {c.node: ns[c.node]}}}})
	return nil
}
func (c *Counters) PerNode(name, window string) map[string]uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	prune(&c.state, c.now())
	out := map[string]uint64{}
	for n, v := range c.state.Counts[name][window] {
		out[n] = v
	}
	return out
}
func (c *Counters) Sum(name, window string) uint64 {
	var n uint64
	for _, v := range c.PerNode(name, window) {
		n += v
	}
	return n
}
func (c *Counters) SetMax(name string, t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !identifier(name) || t.IsZero() || t.Year() < 1970 || t.Year() > 9999 {
		return ErrBound
	}
	old, ok := c.state.Registers[name]
	if !ok && len(c.state.Registers) >= 32 {
		return ErrBound
	}
	if t.After(old) {
		c.state.Registers[name] = t.UTC()
		if !stateWithinBounds(c.state) {
			if ok {
				c.state.Registers[name] = old
			} else {
				delete(c.state.Registers, name)
			}
			return ErrBound
		}
		c.emit(counterState{Registers: map[string]time.Time{name: t.UTC()}})
	}
	return nil
}
func (c *Counters) Max(name string) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state.Registers[name]
}
func (c *Counters) snapshot() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	prune(&c.state, c.now())
	b, _ := json.Marshal(c.state)
	return b
}
func (c *Counters) emit(s counterState) {
	if c.changed != nil {
		s.Version = 1
		b, _ := json.Marshal(s)
		c.changed(b)
	}
}
func (c *Counters) merge(b []byte) error {
	if len(b) > maxWire {
		return ErrBound
	}
	var incoming counterState
	if json.Unmarshal(b, &incoming) != nil || incoming.Version != 1 {
		return ErrBound
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	// Work on a copy: rejection must never partially merge a message.
	raw, _ := json.Marshal(c.state)
	var next counterState
	_ = json.Unmarshal(raw, &next)
	prune(&next, c.now())
	if len(incoming.Usage) > 64 {
		return ErrBound
	}
	if next.Usage == nil {
		next.Usage = map[string]UsagePolicy{}
	}
	for node, u := range incoming.Usage {
		if !validUsage(node, u, c.now()) {
			return ErrBound
		}
		if u.At.Before(c.now().Add(-24*time.Hour - usageSkew)) {
			continue
		}
		if u.At.After(next.Usage[node].At) {
			next.Usage[node] = u
		}
	}
	if len(next.Usage) > 64 {
		return ErrBound
	}
	if len(incoming.Counts) > 32 || len(incoming.Registers) > 32 || len(incoming.Meters) > 8 {
		return ErrBound
	}
	for name, ws := range incoming.Counts {
		if !identifier(name) || len(ws) > 1445 {
			return ErrBound
		}
		for w, ns := range ws {
			if _, e := parseWindow(w); e != nil || len(ns) > 64 {
				return ErrBound
			}
			if !validWindow(w, c.now()) {
				continue
			}
			if next.Counts[name] == nil {
				next.Counts[name] = map[string]map[string]uint64{}
			}
			if next.Counts[name][w] == nil {
				next.Counts[name][w] = map[string]uint64{}
			}
			dst := next.Counts[name][w]
			for node, v := range ns {
				if !identifier(node) || v == 0 {
					return ErrBound
				}
				if v > dst[node] {
					dst[node] = v
				}
			}
			if len(dst) > 64 {
				return ErrBound
			}
			var sum uint64
			for _, v := range dst {
				if math.MaxUint64-sum < v {
					return ErrBound
				}
				sum += v
			}
		}
	}
	for n, t := range incoming.Registers {
		if !identifier(n) || t.IsZero() || t.Year() < 1970 || t.Year() > 9999 {
			return ErrBound
		}
		if t.After(next.Registers[n]) {
			next.Registers[n] = t
		}
	}
	if next.Meters == nil {
		next.Meters = map[string]Meter{}
	}
	for provider, m := range incoming.Meters {
		if !validMeter(provider, m) {
			return ErrBound
		}
		if newerMeter(m, next.Meters[provider]) {
			next.Meters[provider] = m
		}
	}
	if len(next.Counts) > 32 || len(next.Registers) > 32 {
		return ErrBound
	}
	if !stateWithinBounds(next) {
		return ErrBound
	}
	c.state = next
	return nil
}

// Meter carries only a fixed schema of non-secret account quota facts.
// No arbitrary strings, references or item labels are accepted.
type Meter struct {
	Label     string    `json:"label"` // account or token
	Limit     uint64    `json:"limit"`
	Used      uint64    `json:"used"`
	Remaining uint64    `json:"remaining"`
	Taken     time.Time `json:"taken"`
	Node      string    `json:"node"`
}

func validMeter(provider string, m Meter) bool {
	return (provider == "1password" || provider == "azure-key-vault") && (m.Label == "account" || m.Label == "token") && identifier(m.Node) && !m.Taken.IsZero() && m.Taken.Year() >= 1970 && m.Taken.Year() <= 9999
}
func newerMeter(a, b Meter) bool {
	return a.Taken.After(b.Taken) || (a.Taken.Equal(b.Taken) && a.Node > b.Node)
}
func (c *Counters) SetMeter(provider string, m Meter) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	m.Node = c.node
	m.Taken = m.Taken.UTC()
	if !validMeter(provider, m) {
		return ErrBound
	}
	if newerMeter(m, c.state.Meters[provider]) {
		old, exists := c.state.Meters[provider]
		c.state.Meters[provider] = m
		if !stateWithinBounds(c.state) {
			if exists {
				c.state.Meters[provider] = old
			} else {
				delete(c.state.Meters, provider)
			}
			return ErrBound
		}
		c.emit(counterState{Meters: map[string]Meter{provider: m}})
	}
	return nil
}
func (c *Counters) Meter(provider string) Meter {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state.Meters[provider]
}

// RollingPerNode includes the bucket straddling the rolling cutoff. This
// conservatively overcounts by up to one minute of events.
func (c *Counters) RollingPerNode(name string, d time.Duration) map[string]uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := map[string]uint64{}
	if d != time.Hour && d != 24*time.Hour {
		return out
	}
	prune(&c.state, c.now())
	cut := c.now().UTC().Add(-d).Truncate(time.Minute)
	for w, ns := range c.state.Counts[name] {
		if len(w) == 10 {
			continue
		}
		t, e := parseWindow(w)
		if e != nil || t.Before(cut) {
			continue
		}
		for n, v := range ns {
			if math.MaxUint64-out[n] < v {
				out[n] = math.MaxUint64
			} else {
				out[n] += v
			}
		}
	}
	return out
}
func (c *Counters) RollingSum(name string, d time.Duration) uint64 {
	var sum uint64
	for _, v := range c.RollingPerNode(name, d) {
		if math.MaxUint64-sum < v {
			return math.MaxUint64
		}
		sum += v
	}
	return sum
}

// Bound each name and writer as well as the complete push/pull message.
func stateWithinBounds(s counterState) bool {
	all, _ := json.Marshal(s)
	if len(all) > maxWire {
		return false
	}
	byNode := map[string]int{}
	for _, ws := range s.Counts {
		b, _ := json.Marshal(ws)
		if len(b) > 256<<10 {
			return false
		}
		for w, ns := range ws {
			for n, v := range ns {
				if v == 0 {
					return false
				}
				byNode[n] += len(w) + len(n) + 24
			}
		}
	}
	for _, size := range byNode {
		if size > 96<<10 {
			return false
		}
	}
	return true
}
