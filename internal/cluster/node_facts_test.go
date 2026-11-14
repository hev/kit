package cluster

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func fullFacts(now time.Time) NodeFacts {
	return NodeFacts{Version: 1, At: now, Dispatch: &DispatchFacts{At: now, Cap: 4, Running: 1, Free: 3, Queue: 2}, Sessions: &SessionFacts{At: now, Harness: map[string]SessionCount{"claude": {Working: 1}, "codex": {Idle: 2}, "pi": {}, "opencode": {}, "other": {}}, Role: map[string]SessionCount{"foh": {Idle: 2}, "boh": {Working: 1}}}, Build: &BuildFacts{At: now, Commit: "abcdef012345", Staged: true, LastCheck: now, Outcome: "ok"}, Credentials: &CredentialFacts{At: now, Items: map[string]CredentialFact{"op": {At: now, State: "ok"}, "azkv": {State: "unknown"}, "layer": {At: now, State: "failing"}, "claude": {State: "unknown"}, "codex": {State: "unknown"}, "pi": {State: "unknown"}, "opencode": {State: "unknown"}}}, Capture: &CaptureFacts{At: now, Running: true, LastUpload: now, Backlog: 2, Error: "auth"}}
}
func TestNodeFactsValidationAtomicAndExpiry(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	c := newCounters("alpha", true)
	c.now = func() time.Time { return now }
	f := fullFacts(now)
	if e := c.SetNodeFacts(f); e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(f)
	t.Logf("fully populated node: %d bytes", len(raw))
	f.Sessions.Harness["codex"] = SessionCount{Working: 99}
	if c.NodeFacts()["alpha"].Sessions.Harness["codex"].Idle != 2 {
		t.Fatal("caller mutated stored facts")
	}
	good, _ := json.Marshal(fullFacts(now))
	for name, bad := range map[string][]byte{
		"oversized-record": append(append([]byte("{"), bytes.Repeat([]byte(" "), maxNodeFacts)...), good[1:]...),
		"free-text":        bytes.Replace(good, []byte(`"v":1`), []byte(`"v":1,"session":"planted-session /private/path repo-private op://private/reference raw-error"`), 1),
		"unknown-enum":     bytes.Replace(good, []byte(`"outcome":"ok"`), []byte(`"outcome":"raw-error"`), 1),
		"future":           bytes.Replace(good, []byte(now.Format(time.RFC3339)), []byte(now.Add(2*time.Minute).Format(time.RFC3339)), -1),
		"oversized":        append([]byte(`{"v":1,"build":{"commit":"`), append(bytes.Repeat([]byte("a"), maxWire), []byte(`"}}`)...)...),
	} {
		t.Run(name, func(t *testing.T) {
			before := c.snapshot()
			wire := append([]byte(`{"v":1,"registers":{"valid": "2026-11-13T12:00:00Z"},"facts":{"bravo":`), bad...)
			wire = append(wire, []byte(`}}`)...)
			if c.merge(wire) == nil {
				t.Fatal("accepted invalid record")
			}
			if !bytes.Equal(before, c.snapshot()) {
				t.Fatal("partial rejection")
			}
		})
	}
	older := fullFacts(now.Add(-time.Second))
	b, _ := json.Marshal(counterState{Version: 1, Facts: map[string]NodeFacts{"alpha": older}})
	if e := c.merge(b); e != nil {
		t.Fatal(e)
	}
	if !c.NodeFacts()["alpha"].At.Equal(now) {
		t.Fatal("older record won")
	}
	now = now.Add(24*time.Hour + usageSkew + time.Nanosecond)
	if len(c.NodeFacts()) != 0 {
		t.Fatal("record did not age out")
	}
	if e := c.merge(b); e != nil {
		t.Fatal(e)
	}
	if len(c.NodeFacts()) != 0 {
		t.Fatal("resurrected expired record")
	}
}
func TestThreeNodeFactsDeltaPushPullRestart(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	ns := []*Node{node(t, "alpha", nil, nil), node(t, "bravo", nil, nil), node(t, "charlie", nil, nil)}
	for _, n := range ns {
		n.Counters.mu.Lock()
		fixed := now
		n.Counters.now = func() time.Time { return fixed }
		n.Counters.mu.Unlock()
		if e := n.SetNodeFacts(fullFacts(now)); e != nil {
			t.Fatal(e)
		}
	}
	// Deliver exactly the queued UDP payloads, without sleeps or a probabilistic timer.
	for _, n := range ns {
		for _, b := range n.GetBroadcasts(0, 1400) {
			if len(b) > 1200 {
				t.Fatalf("delta exceeds datagram allowance: %d", len(b))
			}
			for _, dst := range ns {
				dst.NotifyMsg(b)
			}
		}
	}
	for _, n := range ns {
		if len(n.NodeFacts()) != 3 {
			t.Fatalf("delta facts: %d", len(n.NodeFacts()))
		}
	}
	if e := ns[2].Close(false); e != nil {
		t.Fatal(e)
	}
	ns[2] = node(t, "charlie", nil, nil)
	ns[2].Counters.mu.Lock()
	fixed := now
	ns[2].Counters.now = func() time.Time { return fixed }
	ns[2].Counters.mu.Unlock()
	ns[2].MergeRemoteState(ns[0].LocalState(true), true)
	if len(ns[2].NodeFacts()) != 3 {
		t.Fatal("restart push/pull recovery")
	}
	f := fullFacts(now.Add(time.Second))
	now = f.At
	if e := ns[2].SetNodeFacts(f); e != nil {
		t.Fatal(e)
	}
	for _, dst := range ns {
		dst.MergeRemoteState(ns[2].LocalState(false), false)
	}
	for _, n := range ns {
		if !n.NodeFacts()["charlie"].At.Equal(now) {
			t.Fatal("full exchange did not converge")
		}
	}
	for _, n := range ns {
		for _, bad := range []string{"planted-session", "/private/path", "repo-private", "op://private/reference", "raw-error"} {
			if strings.Contains(string(n.LocalState(false)), bad) {
				t.Fatal(fmt.Sprintf("leak: %s", bad))
			}
		}
	}
}
func TestNodeFactsAbsentSectionsAndCopies(t *testing.T) {
	now := time.Now().UTC()
	c := newCounters("alpha", true)
	if e := c.SetNodeFacts(NodeFacts{Version: 1, At: now}); e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(c.NodeFacts())
	if strings.Contains(string(raw), "dispatch") || strings.Contains(string(raw), "capture") {
		t.Fatal("absent section filled")
	}
}

func TestNodeFactsAcceptsRateLimitedCredential(t *testing.T) {
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	c := newCounters("alpha", true)
	c.now = func() time.Time { return now }
	f := fullFacts(now)
	f.Credentials.Items["op"] = CredentialFact{At: now, State: "rate-limited"}
	if err := c.SetNodeFacts(f); err != nil {
		t.Fatal(err)
	}
	if got := c.NodeFacts()["alpha"].Credentials.Items["op"].State; got != "rate-limited" {
		t.Fatal(got)
	}
}
