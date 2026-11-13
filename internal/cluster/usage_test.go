package cluster

import (
	"encoding/json"
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
