package cluster

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/memberlist"
	"github.com/hev/kit/internal/secrets"
	"github.com/hev/kit/internal/secrets/fakeop"
)

var testKey = []byte("0123456789abcdef0123456789abcdef")

func fast(c *memberlist.Config) {
	c.ProbeInterval = 100 * time.Millisecond
	c.ProbeTimeout = 40 * time.Millisecond
	c.SuspicionMult = 2
	c.SuspicionMaxTimeoutMult = 1
	c.AwarenessMaxMultiplier = 1
	c.GossipInterval = 20 * time.Millisecond
	c.PushPullInterval = 200 * time.Millisecond
	c.GossipToTheDeadTime = time.Second
	c.TCPTimeout = 200 * time.Millisecond
}
func eventually(t *testing.T, d time.Duration, what string, check func() bool) {
	t.Helper()
	timer := time.NewTimer(d)
	defer timer.Stop()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		if check() {
			return
		}
		select {
		case <-timer.C:
			t.Fatalf("deadline waiting for %s", what)
		case <-tick.C:
		}
	}
}
func node(t *testing.T, name string, seeds []string, logger *slog.Logger) *Node {
	t.Helper()
	n, e := start(Config{Bind: "127.0.0.1:0", Name: name, KeyRef: "secret://test", Seeds: seeds}, testKey, LocalFacts(name, true, 3), logger, fast)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = n.Close(false) })
	return n
}
func allAlive(ns []*Node, count int) bool {
	for _, n := range ns {
		n.Counters.mu.Lock()
		ready := n.ready
		n.Counters.mu.Unlock()
		if !ready {
			return false
		}
		ms := n.Members()
		alive := 0
		for _, m := range ms {
			if m.State == "alive" {
				alive++
			}
		}
		if alive != count {
			return false
		}
	}
	return true
}
func stateOf(n *Node, name string) string {
	for _, m := range n.Members() {
		if m.Name == name {
			return m.State
		}
	}
	return ""
}
func TestThreeMembersLifecycleAndReplication(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	a := node(t, "alpha", nil, logger)
	b := node(t, "bravo", []string{a.Address()}, logger)
	c := node(t, "charlie", []string{a.Address()}, logger)
	ns := []*Node{a, b, c}
	eventually(t, 5*time.Second, "three alive with metadata", func() bool { return allAlive(ns, 3) })
	for _, n := range ns {
		for _, m := range n.Members() {
			if m.Encoding != 1 || m.OS == "" || m.Arch == "" || m.Cap != 3 || !m.Dispatch || m.Version == "" || m.Started.IsZero() {
				t.Fatalf("bad metadata: %+v", m)
			}
		}
	}
	window := time.Now().UTC().Format("2006-01-02")
	minute := MinuteWindow(time.Now())
	now := time.Now().UTC().Truncate(time.Second)
	for i, n := range ns {
		if e := n.Add("op-reads", window, uint64(i+1)); e != nil {
			t.Fatal(e)
		}
		if e := n.SetOwn("requests", minute, uint64((i+1)*10)); e != nil {
			t.Fatal(e)
		}
		if e := n.SetOwn("requests", minute, 1); e != nil {
			t.Fatal(e)
		}
		if e := n.SetMax("1password", now.Add(time.Duration(i)*time.Hour)); e != nil {
			t.Fatal(e)
		}
		if e := n.SetMax("azure-key-vault", now.Add(time.Duration(i)*time.Minute)); e != nil {
			t.Fatal(e)
		}
		if e := n.SetMeter("1password", Meter{Label: "account", Limit: 1000, Used: uint64(i), Remaining: 998, Taken: now}); e != nil {
			t.Fatal(e)
		}
	}
	want := map[string]uint64{"alpha": 1, "bravo": 2, "charlie": 3}
	converged := func() bool {
		for _, n := range ns {
			if n.Sum("op-reads", window) != 6 || !reflect.DeepEqual(n.PerNode("op-reads", window), want) || n.RollingSum("requests", time.Hour) != 60 || n.RollingSum("requests", 24*time.Hour) != 60 || n.Max("1password") != now.Add(2*time.Hour) || !n.Max("azure-key-vault").Equal(now.Add(2*time.Minute)) || n.Meter("1password").Node != "charlie" {
				return false
			}
		}
		return true
	}
	eventually(t, 5*time.Second, "counter, rolling windows, provider cooldowns and meter convergence", converged)
	stopped := time.Now()
	addr := c.Address()
	if e := c.Close(false); e != nil {
		t.Fatal(e)
	}
	eventually(t, 5*time.Second, "both observers declare crashed peer dead", func() bool { return stateOf(a, "charlie") == "dead" && stateOf(b, "charlie") == "dead" })
	t.Logf("crash detection: %s (loopback bound 5s)", time.Since(stopped))
	cfg := Config{Bind: addr, Name: "charlie", KeyRef: "secret://test", Seeds: []string{a.Address()}}
	c, e := start(cfg, testKey, LocalFacts("charlie", true, 3), logger, fast)
	if e != nil {
		t.Fatal(e)
	}
	ns[2] = c
	t.Cleanup(func() { _ = c.Close(false) })
	eventually(t, 5*time.Second, "automatic rejoin after death", func() bool { return allAlive(ns, 3) })
	eventually(t, 5*time.Second, "own count recovered after restart", converged)
	if e := c.Add("op-reads", window, 1); e != nil {
		t.Fatal(e)
	}
	want["charlie"] = 4
	eventually(t, 5*time.Second, "restarted increment", func() bool {
		for _, n := range ns {
			if n.Sum("op-reads", window) != 7 {
				return false
			}
		}
		return true
	})
	if e := b.Leave(); e != nil {
		t.Fatal(e)
	}
	eventually(t, 5*time.Second, "graceful leave", func() bool { return stateOf(a, "bravo") == "left" && stateOf(c, "bravo") == "left" })
	if e := b.Join(a.Address()); e != nil {
		t.Fatal(e)
	}
	eventually(t, 5*time.Second, "join after explicit leave", func() bool { return allAlive(ns, 3) })
	// A notice from the previous startup must not turn this crash into left.
	if e := b.Close(false); e != nil {
		t.Fatal(e)
	}
	eventually(t, 5*time.Second, "rejoined incarnation crashes as dead", func() bool { return stateOf(a, "bravo") == "dead" && stateOf(c, "bravo") == "dead" })
	for _, n := range ns {
		_ = n.Close(true)
	}
	for _, bad := range []string{string(testKey), base64.StdEncoding.EncodeToString(testKey), "secret://test"} {
		if strings.Contains(logs.String(), bad) {
			t.Fatal("key or reference leaked in logs")
		}
	}
	for _, event := range []string{"cluster join", "cluster dead", "cluster leave"} {
		if !strings.Contains(logs.String(), event) {
			t.Fatalf("missing transition %s", event)
		}
	}
}
func TestEncryptionAndWildcard(t *testing.T) {
	a := node(t, "alpha", nil, nil)
	cfg := Config{Bind: "127.0.0.1:0", Name: "wrong", KeyRef: "secret://test"}
	wrong, e := start(cfg, bytes.Repeat([]byte{42}, 32), LocalFacts("wrong", false, 0), nil, fast)
	if e != nil {
		t.Fatal(e)
	}
	defer wrong.Close(false)
	if e = wrong.Join(a.Address()); e == nil {
		t.Fatal("wrong key joined")
	}
	if len(wrong.Members()) != 1 || len(a.Members()) != 1 {
		t.Fatal("wrong key learned members")
	}
	if _, e = start(cfg, nil, LocalFacts("missing", false, 0), nil, fast); e == nil {
		t.Fatal("missing key accepted")
	}
	plainConfig := memberlist.DefaultLocalConfig()
	plainConfig.Name = "plaintext"
	plainConfig.BindAddr = "127.0.0.1"
	plainConfig.BindPort = 0
	plainConfig.LogOutput = io.Discard
	plain, e := memberlist.Create(plainConfig)
	if e != nil {
		t.Fatal(e)
	}
	defer plain.Shutdown()
	if joined, e := plain.Join([]string{a.Address()}); e == nil || joined != 0 {
		t.Fatal("plaintext node joined")
	}
	if len(plain.Members()) != 1 || len(a.Members()) != 1 {
		t.Fatal("plaintext learned member names")
	}
	cfg.Bind = "0.0.0.0:0"
	if _, e = start(cfg, testKey, LocalFacts("wild", false, 0), nil, fast); e == nil {
		t.Fatal("wildcard accepted")
	}
	cfg.AllowWildcard = true
	if e = cfg.Validate(); e != nil {
		t.Fatal(e)
	}
}
func TestDisabledNoSocketsAndConfigRedaction(t *testing.T) {
	endpoint := filepath.Join(t.TempDir(), "never.sock")
	if strings.Contains(endpoint, ":") {
		t.Skip("Unix assertion")
	}
	t.Setenv(EndpointEnv, endpoint)
	s := NewService(context.Background(), nil, func(context.Context, string) ([]byte, time.Time, error) {
		t.Fatal("key read while disabled")
		return nil, time.Time{}, nil
	}, Facts{}, nil)
	defer s.Close()
	stop, e := Serve(context.Background(), s)
	if e != nil {
		t.Fatal(e)
	}
	defer stop()
	if _, e = os.Stat(endpoint); !os.IsNotExist(e) {
		t.Fatal("disabled cluster opened IPC")
	}
	if s.Status().Running {
		t.Fatal("disabled cluster running")
	}
	p := filepath.Join(t.TempDir(), "config.toml")
	if e = os.WriteFile(p, []byte("[cluster]\nbind = \""+string(testKey)+"\"\nkey_ref = 123\n"), 0600); e != nil {
		t.Fatal(e)
	}
	_, e = LoadConfig(p)
	if e == nil || strings.Contains(e.Error(), string(testKey)) {
		t.Fatal("config error not sanitized")
	}
	if e = os.WriteFile(p, []byte("[other]\nvalue=1\n"), 0600); e != nil {
		t.Fatal(e)
	}
	cfg, e := LoadConfig(p)
	if e != nil || cfg != nil {
		t.Fatal("absent config enabled cluster")
	}
	// A disabled service can coexist with both TCP/UDP already bound at the
	// would-be cluster port; it has no gossip transport at all.
	tcp, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer tcp.Close()
	udp, e := net.ListenPacket("udp", tcp.Addr().String())
	if e != nil {
		t.Fatal(e)
	}
	defer udp.Close()
}
func TestFailedKeyCooldownWithFake(t *testing.T) {
	fake := fakeop.Install(t)
	t.Setenv("HEV_SECRETS_POLICY", filepath.Join(t.TempDir(), "policy.json"))
	t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "fake-token")
	t.Setenv("HEV_SECRETS_COOLDOWN", filepath.Join(t.TempDir(), "cooldown.json"))
	t.Setenv("HEV_SECRETS_READ_LOG", filepath.Join(t.TempDir(), "reads.jsonl"))
	now := time.Now().UTC()
	reset := now.Add(4 * time.Hour).Truncate(time.Second)
	t.Setenv("FAKE_OP_RATE_LIMIT", reset.Format(time.RFC3339))
	cache := secrets.New()
	cfg := &Config{Bind: "127.0.0.1:0", Name: "alpha", KeyRef: "op://test/item/key"}
	s := &Service{cfg: cfg, loader: CacheKeyLoader(cache)}
	next := s.attempt(context.Background(), now)
	if next.Before(reset) {
		t.Fatalf("retry %s before cooldown %s", next, reset)
	}
	count := fake.Count(t)
	if count != 1 {
		t.Fatalf("read count %d", count)
	}
	for i := 0; i < 10; i++ {
		s.attempt(context.Background(), now.Add(time.Duration(i)*time.Minute))
	}
	if fake.Count(t) != count {
		t.Fatal("key retried in cooldown")
	}
	st := s.Status()
	if !st.Enabled || st.Running || st.KeyLoaded || st.Error == "" {
		t.Fatalf("bad status %+v", st)
	}
	b, _ := json.Marshal(st)
	if strings.Contains(string(b), fakeop.Value) || strings.Contains(string(b), cfg.KeyRef) {
		t.Fatal("status leaked secret")
	}
}
func TestCounterBoundsAndClock(t *testing.T) {
	c := newCounters("alpha", true)
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }
	w := now.Format("2006-01-02")
	for i := 0; i < 32; i++ {
		if e := c.Add(fmt.Sprintf("n%d", i), w, 1); e != nil {
			t.Fatal(e)
		}
	}
	if c.Add("overflow", w, 1) == nil {
		t.Fatal("name bound")
	}
	if c.Add("n0", "2026-11-15", 1) == nil {
		t.Fatal("future window")
	}
	if c.Add("n0", "2026-11-10", 1) == nil {
		t.Fatal("old window")
	}
	before := string(c.snapshot())
	if c.merge(bytes.Repeat([]byte{'x'}, maxWire+1)) == nil {
		t.Fatal("wire bound")
	}
	if string(c.snapshot()) != before {
		t.Fatal("partial merge")
	}
	for i := 0; i < 32; i++ {
		if e := c.SetMax(fmt.Sprintf("r%d", i), now); e != nil {
			t.Fatal(e)
		}
	}
	if c.SetMax("overflow", now) == nil {
		t.Fatal("register bound")
	}
	if c.SetMeter("1password", Meter{Label: "free-secret-text", Taken: now}) == nil {
		t.Fatal("unrestricted meter")
	}
	if c.SetMeter("unlisted", Meter{Label: "account", Taken: now}) == nil {
		t.Fatal("unrestricted provider")
	}
	incoming := newCounters("bravo", true)
	incoming.now = c.now
	incoming.Add("n0", w, 9)
	old := incoming.snapshot()
	now = now.Add(72 * time.Hour)
	if e := c.merge(old); e != nil {
		t.Fatal(e)
	}
	if c.Sum("n0", w) != 0 {
		t.Fatal("expired resurrected")
	}
	minute := MinuteWindow(now)
	if e := c.SetOwn("requests", minute, 5); e != nil {
		t.Fatal(e)
	}
	if e := c.SetOwn("requests", minute, 2); e != nil {
		t.Fatal(e)
	}
	if c.Sum("requests", minute) != 5 {
		t.Fatal("SetOwn decreased")
	}
	now = now.Add(61 * time.Minute)
	if c.RollingSum("requests", time.Hour) != 0 || c.RollingSum("requests", 24*time.Hour) != 5 {
		t.Fatal("rolling expiration")
	}
}

func TestCounterMergeBoundsAtomic(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	window := now.Format("2006-01-02")
	c := newCounters("alpha", true)
	c.now = func() time.Time { return now }
	if e := c.Add("existing", window, 3); e != nil {
		t.Fatal(e)
	}
	original := string(c.snapshot())
	ns := map[string]uint64{}
	for i := 0; i < 65; i++ {
		ns[fmt.Sprintf("node%d", i)] = 1
	}
	bad := counterState{Version: 1, Counts: map[string]map[string]map[string]uint64{"bad": {window: ns}}}
	raw, _ := json.Marshal(bad)
	if c.merge(raw) == nil || string(c.snapshot()) != original {
		t.Fatal("node bound or atomicity failed")
	}
	bad.Counts = map[string]map[string]map[string]uint64{}
	for i := 0; i < 33; i++ {
		bad.Counts[fmt.Sprintf("n%d", i)] = map[string]map[string]uint64{window: {"alpha": 1}}
	}
	raw, _ = json.Marshal(bad)
	if c.merge(raw) == nil {
		t.Fatal("merged name bound")
	}
	bad.Counts = map[string]map[string]map[string]uint64{"existing": {window: {"alpha": ^uint64(0), "bravo": 1}}}
	raw, _ = json.Marshal(bad)
	if c.merge(raw) == nil {
		t.Fatal("aggregate overflow accepted")
	}
	bad.Counts = map[string]map[string]map[string]uint64{"existing": {window: {"bravo": 0}}}
	raw, _ = json.Marshal(bad)
	if c.merge(raw) == nil {
		t.Fatal("zero component accepted")
	}
	if string(c.snapshot()) != original {
		t.Fatal("rejection modified state")
	}
	// Estimate bounds independently of the global encoded-size guard.
	large := counterState{Version: 1, Counts: map[string]map[string]map[string]uint64{}, Registers: map[string]time.Time{}, Meters: map[string]Meter{}}
	for i := 0; i < 4; i++ {
		ws := map[string]map[string]uint64{}
		for minute := 0; minute < 1400; minute++ {
			ws[MinuteWindow(now.Add(-time.Duration(minute)*time.Minute))] = map[string]uint64{"alpha": 1}
		}
		large.Counts[fmt.Sprintf("n%d", i)] = ws
	}
	if stateWithinBounds(large) {
		t.Fatal("per-writer state bound not enforced")
	}
	tied := newCounters("bravo", true)
	tied.now = c.now
	if e := c.SetMeter("1password", Meter{Label: "account", Taken: now, Used: 2}); e != nil {
		t.Fatal(e)
	}
	if e := tied.SetMeter("1password", Meter{Label: "account", Taken: now, Used: 3}); e != nil {
		t.Fatal(e)
	}
	if e := c.merge(tied.snapshot()); e != nil {
		t.Fatal(e)
	}
	if c.Meter("1password").Node != "bravo" {
		t.Fatal("tie-break failed")
	}
	if e := c.SetMeter("1password", Meter{Label: "account", Taken: now.Add(-time.Second), Used: 9}); e != nil {
		t.Fatal(e)
	}
	if c.Meter("1password").Used != 3 {
		t.Fatal("older meter replaced newer")
	}
}

func TestKeyLoadedOnceAndRedacted(t *testing.T) {
	fake := fakeop.Install(t)
	t.Setenv("HEV_SECRETS_POLICY", filepath.Join(t.TempDir(), "policy.json"))
	encoded := base64.StdEncoding.EncodeToString(testKey)
	script := filepath.Join(fake.Bin, "op")
	b, e := os.ReadFile(script)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(script, bytes.ReplaceAll(b, []byte(fakeop.Value), []byte(encoded)), 0700); e != nil {
		t.Fatal(e)
	}
	t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "fake-token")
	t.Setenv("HEV_SECRETS_COOLDOWN", filepath.Join(t.TempDir(), "cooldown.json"))
	t.Setenv("HEV_SECRETS_READ_LOG", filepath.Join(t.TempDir(), "reads.jsonl"))
	var logs bytes.Buffer
	s := NewService(context.Background(), &Config{Bind: "127.0.0.1:0", Name: "example", KeyRef: "op://test/item/key"}, CacheKeyLoader(secrets.New()), LocalFacts("example", false, 0), slog.New(slog.NewTextHandler(&logs, nil)))
	defer s.Close()
	eventually(t, 5*time.Second, "key loaded", func() bool { return s.Status().Running })
	s.attempt(context.Background(), time.Now().Add(48*time.Hour))
	if fake.Count(t) != 1 {
		t.Fatalf("loaded key reread: %d", fake.Count(t))
	}
	status, _ := json.Marshal(s.Status())
	s.Close()
	for _, sensitive := range []string{string(testKey), encoded, "op://test/item/key"} {
		if strings.Contains(logs.String(), sensitive) || strings.Contains(string(status), sensitive) {
			t.Fatal("key or reference leaked")
		}
	}
}

func TestSeedRetryAfterSeedStarts(t *testing.T) {
	reserved, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	addr := reserved.Addr().String()
	reserved.Close()
	a := node(t, "alpha", []string{addr}, nil)
	eventually(t, 5*time.Second, "first seed attempt unavailable", func() bool { a.mu.Lock(); defer a.mu.Unlock(); return a.lastJoin == "seed unavailable" })
	b, e := start(Config{Bind: addr, Name: "bravo", KeyRef: "secret://test"}, testKey, LocalFacts("bravo", true, 3), nil, fast)
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close(false)
	eventually(t, 8*time.Second, "retry joins seed without command", func() bool { return allAlive([]*Node{a, b}, 2) })
}
