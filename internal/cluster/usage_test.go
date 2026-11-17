package cluster

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestUsageMetadataBoundsAndEncodedMessages(t *testing.T) {
	n := node(t, "alpha", nil, nil)
	now := time.Now().UTC()
	u := UsagePolicy{At: now, Daily: map[string]int{"op": 900}, Hourly: map[string]int{"op": 90}}
	if e := n.SetUsage(u); e != nil {
		t.Fatal(e)
	}
	u.Daily["op"] = 1
	if n.Usage()["alpha"].Daily["op"] != 900 {
		t.Fatal("mutable policy alias")
	}
	// Only fixed provider keys and positive counts are accepted; untrusted
	// free text cannot become a metadata carrier.
	for _, bad := range []UsagePolicy{{At: now, Daily: map[string]int{"op://vault/item/field": 1}}, {At: now, Hourly: map[string]int{"token-secret": 1}}, {At: now, Daily: map[string]int{"op": -1}}} {
		raw, _ := json.Marshal(counterState{Version: 1, Usage: map[string]UsagePolicy{"bravo": bad}})
		before := string(n.snapshot())
		if n.merge(raw) == nil {
			t.Fatal("accepted free text or invalid numeric metadata")
		}
		if string(n.snapshot()) != before {
			t.Fatal("rejection partially merged")
		}
	}
	messages := n.GetBroadcasts(0, 1<<20)
	if len(messages) == 0 {
		t.Fatal("usage heartbeat not broadcast")
	}
	for _, raw := range messages {
		var payload map[string]any
		if json.Unmarshal(raw, &payload) != nil {
			t.Fatal("invalid encoded message")
		}
		if _, ok := payload["usage"]; !ok {
			t.Fatalf("missing heartbeat delta: %s", raw)
		}
		for _, forbidden := range []string{"op://", "secret://", "token-secret", "value", "reference"} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatalf("forbidden gossip payload %s", raw)
			}
		}
	}
	other := newCounters("bravo", true)
	for _, raw := range messages {
		if e := other.merge(raw); e != nil {
			t.Fatal(e)
		}
	}
	if other.Usage()["alpha"].Daily["op"] != 900 {
		t.Fatal("heartbeat delta did not converge")
	}
}

func TestUsageExpiryAndFullTableRecovery(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	c := newCounters("local", true)
	c.now = func() time.Time { return now }
	stale := now.Add(-24*time.Hour - usageSkew)
	c.state.Usage = map[string]UsagePolicy{}
	for i := 0; i < 64; i++ {
		c.state.Usage[fmt.Sprintf("node-%d", i)] = UsagePolicy{At: stale}
	}
	if err := c.SetUsage(UsagePolicy{At: now}); err != ErrBound {
		t.Fatalf("full table: %v", err)
	}
	now = now.Add(time.Nanosecond)
	if err := c.SetUsage(UsagePolicy{At: now}); err != nil {
		t.Fatal(err)
	}
	if len(c.Usage()) != 1 {
		t.Fatal("stale usage not pruned")
	}
	// Delayed snapshots cannot resurrect expired writers or block other facts.
	raw, _ := json.Marshal(counterState{Version: 1, Usage: map[string]UsagePolicy{"old": {At: stale}}, Registers: map[string]time.Time{"cooldown": now}})
	if err := c.merge(raw); err != nil {
		t.Fatal(err)
	}
	if len(c.Usage()) != 1 || !c.Max("cooldown").Equal(now) {
		t.Fatal("expired usage affected convergence")
	}
	// Merge also recovers a full expired table before checking the bound.
	for i := 0; i < 64; i++ {
		c.state.Usage[fmt.Sprintf("node-%d", i)] = UsagePolicy{At: stale}
	}
	raw, _ = json.Marshal(counterState{Version: 1, Usage: map[string]UsagePolicy{"peer": {At: now}}})
	if err := c.merge(raw); err != nil {
		t.Fatal(err)
	}
	if len(c.Usage()) != 2 {
		t.Fatal("merge failed to prune full table")
	}
}

func TestUsageFutureTimestampRejectedAtomically(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	c := newCounters("local", true)
	c.now = func() time.Time { return now }
	future := UsagePolicy{At: now.Add(usageSkew + time.Nanosecond)}
	before := string(c.snapshot())
	if err := c.SetUsage(future); err != ErrBound {
		t.Fatalf("future local: %v", err)
	}
	if string(c.snapshot()) != before {
		t.Fatal("local rejection mutated state")
	}
	raw, _ := json.Marshal(counterState{Version: 1, Usage: map[string]UsagePolicy{"local": future, "peer": {At: now}}, Registers: map[string]time.Time{"cooldown": now}})
	if err := c.merge(raw); err != ErrBound {
		t.Fatalf("future incoming: %v", err)
	}
	if string(c.snapshot()) != before {
		t.Fatal("incoming rejection partially merged")
	}
	if err := c.SetUsage(UsagePolicy{At: now}); err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(counterState{Version: 1, Usage: map[string]UsagePolicy{"local": {At: now.Add(time.Second)}}})
	if err := c.merge(raw); err != nil {
		t.Fatal(err)
	}
	if !c.Usage()["local"].At.Equal(now.Add(time.Second)) {
		t.Fatal("correct heartbeat pinned by rejected future timestamp")
	}
}

func TestRollingWindowsIncludeStraddlingBucket(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 30, 0, time.UTC)
	for _, duration := range []time.Duration{time.Hour, 24 * time.Hour} {
		t.Run(duration.String(), func(t *testing.T) {
			c := newCounters("local", true)
			c.now = func() time.Time { return now }
			cut := now.Add(-duration).Truncate(time.Minute)
			c.state.Counts["requests"] = map[string]map[string]uint64{
				MinuteWindow(cut.Add(-time.Minute)): {"peer": 100},
				MinuteWindow(cut):                   {"peer": 7, "local": 3},
				MinuteWindow(now):                   {"local": 2},
			}
			per := c.RollingPerNode("requests", duration)
			if per["peer"] != 7 || per["local"] != 5 || c.RollingSum("requests", duration) != 12 {
				t.Fatalf("boundary totals: %v", per)
			}
		})
	}
}
