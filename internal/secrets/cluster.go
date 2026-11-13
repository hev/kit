package secrets

import (
	"fmt"
	"time"
)

// ClusterUsage is an in-process, nonblocking metadata seam. Implementations must
// never perform network or provider I/O here. No references or values enter it.
type ClusterUsage interface {
	PublishUsage(map[string]map[string]uint64, Policy, time.Time) error
	Usage(string, time.Time) AccountStatus
	PublishCooldown(string, time.Time)
	PeerCooldown(string) (time.Time, string)
	PublishMeter(SharedMeter)
	SharedMeter() SharedMeter
}
type SharedMeter struct {
	Limit     uint64    `json:"limit"`
	Used      uint64    `json:"used"`
	Remaining uint64    `json:"remaining"`
	Taken     time.Time `json:"taken"`
	Node      string    `json:"node"`
}
type PeerUsage struct {
	Day        uint64 `json:"day"`
	Hour       uint64 `json:"hour"`
	AgeSeconds int64  `json:"age_seconds"` // -1 means no ledger heartbeat received
}
type AccountStatus struct {
	Provider        string               `json:"provider"`
	Daily           int                  `json:"daily"`
	Hourly          int                  `json:"hourly"`
	UsedDay         uint64               `json:"used_day"`
	UsedHour        uint64               `json:"used_hour"`
	Visible         bool                 `json:"visible"`
	Note            string               `json:"note"`
	Nodes           map[string]PeerUsage `json:"nodes"`
	Meter           *SharedMeter         `json:"meter,omitempty"`
	MeterAgeSeconds int64                `json:"meter_age_seconds"`
	Unexplained     uint64               `json:"unexplained"`
}

type clusterBinding struct{ usage ClusterUsage }

func (c *Cache) clusterUsage() ClusterUsage {
	b := c.cluster.Load()
	if b == nil {
		return nil
	}
	return b.usage
}

// AttachCluster safely binds the optional metadata seam while reads may run.
func (c *Cache) AttachCluster(u ClusterUsage) { c.cluster.Store(&clusterBinding{u}) }

// PublishCluster refreshes ledger facts without spending a provider request.
// Persistence failures leave the previous heartbeat stale, never fresh zeroes.
func (c *Cache) PublishCluster() {
	if c.clusterUsage() == nil {
		return
	}
	p, err := hostPolicy()
	if err != nil {
		return
	}
	now := c.now()
	if m := readMeter(); m != nil {
		c.publishMeterReading(m.Reading)
	}
	_ = withLedger(now, func(l *ledger) error { return c.publishLedger(l, p, now) })
	for _, scheme := range []string{"op", "azkv"} {
		if cd, ok := readCooldownFor(scheme, now); ok {
			c.clusterUsage().PublishCooldown(scheme, cd.Reset)
		}
	}
}
func (c *Cache) publishLedger(l *ledger, p Policy, now time.Time) error {
	counts := map[string]map[string]uint64{}
	for _, s := range l.Spends {
		// Only fixed provider labels and numeric totals cross the seam.
		if s.Provider != "op" && s.Provider != "azkv" {
			continue
		}
		if counts[s.Provider] == nil {
			counts[s.Provider] = map[string]uint64{}
		}
		w := s.At.UTC().Format("2006-01-02T15:04Z")
		counts[s.Provider][w] += uint64(s.Requests)
	}
	return c.clusterUsage().PublishUsage(counts, p, now)
}
func (c *Cache) reserve(now time.Time, caller, provider string, n int) error {
	return reserveWith(now, caller, provider, n, c)
}
func (c *Cache) accountReason(l *ledger, p Policy, now time.Time, provider string, n int) string {
	if c.clusterUsage() == nil {
		return ""
	}
	publishErr := c.publishLedger(l, p, now)
	s := c.clusterUsage().Usage(provider, now)
	if !s.Visible {
		return ""
	}
	if publishErr != nil {
		return "account accounting unavailable; inspect cluster metadata bounds"
	}
	if p.AccountDaily[provider] > 0 && s.UsedDay+uint64(n) > uint64(p.AccountDaily[provider]) {
		return fmt.Sprintf("account daily allowance exhausted; nodes=%+v", s.Nodes)
	}
	if p.AccountHourly[provider] > 0 && s.UsedHour+uint64(n) > uint64(p.AccountHourly[provider]) {
		return fmt.Sprintf("account hourly allowance exhausted; nodes=%+v", s.Nodes)
	}
	return ""
}
