package daemon

import (
	"crypto/sha256"
	"fmt"
	"reflect"
	"time"

	"github.com/hev/kit/internal/cluster"
	"github.com/hev/kit/internal/secrets"
)

// usageBridge performs only in-memory counter operations. Gossip transmission
// is owned by the existing cluster transport, never by a secret read.
type usageBridge struct {
	service *cluster.Service
	node    string
}

func (b *usageBridge) PublishUsage(counts map[string]map[string]uint64, p secrets.Policy, now time.Time) error {
	c, err := b.service.Counters()
	if err != nil {
		return err
	}
	for provider, windows := range counts {
		for w, n := range windows {
			if err = c.SetOwn("secrets-"+provider, w, n); err != nil {
				return err
			}
		}
	}
	return c.SetUsage(cluster.UsagePolicy{At: now, Daily: p.AccountDaily, Hourly: p.AccountHourly})
}

const usageFresh = 5 * time.Second

func (b *usageBridge) Usage(provider string, now time.Time) secrets.AccountStatus {
	a := secrets.AccountStatus{Provider: provider, Nodes: map[string]secrets.PeerUsage{}, Note: "cannot see cluster; using host budget"}
	c, err := b.service.Counters()
	if err != nil {
		return a
	}
	facts := c.Usage()
	own := facts[b.node]
	day, hour := c.RollingPerNode("secrets-"+provider, 24*time.Hour), c.RollingPerNode("secrets-"+provider, time.Hour)
	names := map[string]bool{b.node: true}
	for n := range facts {
		names[n] = true
	}
	for n := range day {
		names[n] = true
	}
	for _, m := range b.service.Members() {
		names[m.Name] = true
	}
	fresh := false
	mismatch := false
	stale := false
	for n := range names {
		u := facts[n]
		age := int64(-1)
		if !u.At.IsZero() {
			age = max(0, int64(now.Sub(u.At).Seconds()))
		}
		a.Nodes[n] = secrets.PeerUsage{Day: day[n], Hour: hour[n], AgeSeconds: age}
		a.UsedDay += day[n]
		a.UsedHour += hour[n]
		if n != b.node {
			if !u.At.IsZero() && now.Sub(u.At) <= usageFresh && !u.At.After(now.Add(time.Second)) {
				fresh = true
			} else {
				stale = true
			}
			if !u.At.IsZero() && (!reflect.DeepEqual(u.Daily, own.Daily) || !reflect.DeepEqual(u.Hourly, own.Hourly)) {
				mismatch = true
			}
		}
	}
	a.Visible = fresh
	if fresh {
		a.Note = "cluster visible"
		if stale {
			a.Note += "; stale peers retained in totals"
		}
	}
	if mismatch {
		a.Note += "; account policy mismatch: configure identical budgets on every host"
	}
	m := b.SharedMeter()
	if provider == "op" && !m.Taken.IsZero() {
		a.Meter = &m
		a.MeterAgeSeconds = max(0, int64(now.Sub(m.Taken).Seconds()))
		if provider == "op" && m.Used > a.UsedDay {
			a.Unexplained = m.Used - a.UsedDay
		}
	}
	return a
}
func (b *usageBridge) PublishCooldown(s string, t time.Time) {
	if c, e := b.service.Counters(); e == nil && !t.IsZero() {
		_ = c.SetMax(coolKey(s, b.node), t)
	}
}
func (b *usageBridge) PeerCooldown(s string) (time.Time, string) {
	c, e := b.service.Counters()
	if e != nil {
		return time.Time{}, ""
	}
	var reset time.Time
	source := ""
	for n := range c.Usage() {
		if n == b.node {
			continue
		}
		t := c.Max(coolKey(s, n))
		if t.After(reset) {
			reset, source = t, n
		}
	}
	return reset, source
}
func (b *usageBridge) PublishMeter(m secrets.SharedMeter) {
	if c, e := b.service.Counters(); e == nil {
		_ = c.SetMeter("1password", cluster.Meter{Label: "account", Limit: m.Limit, Used: m.Used, Remaining: m.Remaining, Taken: m.Taken})
	}
}
func (b *usageBridge) SharedMeter() secrets.SharedMeter {
	c, e := b.service.Counters()
	if e != nil {
		return secrets.SharedMeter{}
	}
	m := c.Meter("1password")
	return secrets.SharedMeter{Limit: m.Limit, Used: m.Used, Remaining: m.Remaining, Taken: m.Taken, Node: m.Node}
}

var _ secrets.ClusterUsage = (*usageBridge)(nil)

func coolKey(s, node string) string {
	return fmt.Sprintf("cool-%s-%x", s, sha256.Sum256([]byte(node)))[:48]
}
