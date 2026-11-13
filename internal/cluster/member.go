package cluster

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/memberlist"
	"github.com/hev/kit/internal/version"
)

type Facts struct {
	Encoding int       `json:"encoding"`
	Name     string    `json:"name"`
	Version  string    `json:"version"`
	Channel  string    `json:"channel"`
	OS       string    `json:"os"`
	Arch     string    `json:"arch"`
	Started  time.Time `json:"started"`
	Dispatch bool      `json:"dispatch"`
	Cap      int       `json:"cap"`
}

func LocalFacts(name string, dispatch bool, cap int) Facts {
	channel := "release"
	if version.Version == "dev" || strings.Contains(version.Version, "edge") {
		channel = "edge"
	}
	return Facts{1, name, version.Version, channel, runtime.GOOS, runtime.GOARCH, time.Now().UTC(), dispatch, cap}
}

type Member struct {
	Facts
	Address string    `json:"address"`
	State   string    `json:"state"`
	Changed time.Time `json:"changed"`
	Since   string    `json:"since"`
}

// membershipTransport keeps lifecycle calls separate from replicated facts
// and observation. Stock memberlist supplies the protocol; our event table owns display state.
type membershipTransport interface {
	LocalNode() *memberlist.Node
	Join([]string) (int, error)
	Leave(time.Duration) error
	Shutdown() error
	SendReliable(*memberlist.Node, []byte) error
}
type Node struct {
	departures  map[string]time.Time
	bound       string
	memberCount atomic.Int32
	*Counters
	mu       sync.Mutex
	life     sync.Mutex
	config   *memberlist.Config
	ml       membershipTransport
	ring     *memberlist.Keyring
	queue    *memberlist.TransmitLimitedQueue
	facts    []byte
	members  map[string]Member
	logger   *slog.Logger
	cfg      Config
	stop     chan struct{}
	done     chan struct{}
	closed   bool
	paused   bool
	retry    time.Duration
	next     time.Time
	lastJoin string
	// Tests may replace WAN timing, never encryption or delegates.
}
type message struct {
	key  string
	body []byte
}

func (m message) Invalidates(b memberlist.Broadcast) bool {
	other, ok := b.(message)
	return ok && m.key == other.key
}
func (m message) Message() []byte { return m.body }
func (m message) Finished()       {}

// Split full snapshots into bounded deltas so every broadcast fits UDP.
func (n *Node) broadcastState(b []byte) {
	var st counterState
	if json.Unmarshal(b, &st) != nil {
		return
	}
	enqueue := func(key string, s counterState) {
		s.Version = 1
		body, _ := json.Marshal(s)
		n.queue.QueueBroadcast(message{key, body})
	}
	for name, ws := range st.Counts {
		for w, ns := range ws {
			for node, v := range ns {
				enqueue("c/"+name+"/"+w+"/"+node, counterState{Counts: map[string]map[string]map[string]uint64{name: {w: {node: v}}}})
			}
		}
	}
	for name, t := range st.Registers {
		enqueue("r/"+name, counterState{Registers: map[string]time.Time{name: t}})
	}
	for name, m := range st.Meters {
		enqueue("m/"+name, counterState{Meters: map[string]Meter{name: m}})
	}
}

// Start requires a raw 32-byte key. It never permits memberlist's plaintext mode.
func Start(cfg Config, key []byte, facts Facts, logger *slog.Logger) (*Node, error) {
	return start(cfg, key, facts, logger, nil)
}
func start(cfg Config, key []byte, facts Facts, logger *slog.Logger, tune func(*memberlist.Config)) (*Node, error) {
	if e := cfg.Validate(); e != nil {
		return nil, e
	}
	if len(key) != 32 {
		return nil, errors.New("cluster requires a 32-byte key")
	}
	if cfg.Name == "" {
		cfg.Name, _ = os.Hostname()
	}
	if !identifier(cfg.Name) {
		return nil, errors.New("invalid cluster name")
	}
	facts.Name = cfg.Name
	facts.Encoding = 1
	b, _ := json.Marshal(facts)
	if len(b) > memberlist.MetaMaxSize {
		return nil, ErrBound
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	n := &Node{Counters: newCounters(cfg.Name, len(cfg.Seeds) == 0), facts: b, members: map[string]Member{}, departures: map[string]time.Time{}, logger: logger, cfg: cfg, stop: make(chan struct{}), done: make(chan struct{}), retry: time.Second}
	n.queue = &memberlist.TransmitLimitedQueue{NumNodes: func() int { return max(1, int(n.memberCount.Load())) }, RetransmitMult: 4}
	n.changed = func(b []byte) {
		if len(b) <= maxWire {
			n.broadcastState(b)
		}
	}
	c := memberlist.DefaultWANConfig()
	if tune != nil {
		tune(c)
	}
	c.Name = cfg.Name
	c.BindAddr, c.BindPort, _ = address(cfg.Bind, cfg.AllowWildcard)
	if cfg.Advertise != "" {
		c.AdvertiseAddr, c.AdvertisePort, _ = address(cfg.Advertise, false)
	}
	ring, e := memberlist.NewKeyring(nil, key)
	if e != nil {
		return nil, errors.New("invalid cluster key")
	}
	n.ring = ring
	c.Keyring = ring
	c.GossipVerifyIncoming = true
	c.GossipVerifyOutgoing = true
	c.Delegate = n
	c.Events = n
	c.Alive = n
	c.DelegateProtocolVersion = 1
	c.DelegateProtocolMin = 1
	c.DelegateProtocolMax = 1
	c.LogOutput = io.Discard
	ml, e := memberlist.Create(c)
	if e != nil {
		return nil, errors.New("cluster transport could not start")
	}
	n.ml = ml
	n.bound = ml.LocalNode().Address()
	n.config = c
	go n.seedLoop()
	return n, nil
}
func (n *Node) NodeMeta(limit int) []byte {
	if len(n.facts) > limit {
		return nil
	}
	return n.facts
}

type departure struct {
	Leaving string    `json:"leaving"`
	Started time.Time `json:"started"`
}
type stateEnvelope struct {
	Version    int                  `json:"version"`
	Facts      json.RawMessage      `json:"facts"`
	Departures map[string]time.Time `json:"departures"`
}

func (n *Node) acceptDeparture(d departure) {
	if !identifier(d.Leaving) || d.Started.IsZero() {
		return
	}
	n.mu.Lock()
	m, ok := n.members[d.Leaving]
	if !ok || !m.Started.Equal(d.Started) {
		n.mu.Unlock()
		return
	}
	n.departures[d.Leaving] = d.Started
	if m.State == "dead" {
		m.State = "left"
		m.Changed = time.Now().UTC()
		n.members[d.Leaving] = m
		n.logger.Info("cluster leave", "member", d.Leaving)
	}
	n.mu.Unlock()
}
func (n *Node) NotifyMsg(b []byte) {
	if len(b) < 512 {
		var d departure
		if json.Unmarshal(b, &d) == nil && d.Leaving != "" {
			n.acceptDeparture(d)
			return
		}
	}
	_ = n.merge(b)
}
func (n *Node) GetBroadcasts(overhead, limit int) [][]byte {
	return n.queue.GetBroadcasts(overhead, limit)
}
func (n *Node) LocalState(join bool) []byte {
	st := stateEnvelope{Version: 1, Facts: n.snapshot(), Departures: map[string]time.Time{}}
	n.mu.Lock()
	for name, t := range n.departures {
		st.Departures[name] = t
	}
	n.mu.Unlock()
	b, _ := json.Marshal(st)
	return b
}
func (n *Node) MergeRemoteState(b []byte, join bool) {
	if len(b) > maxWire+16384 {
		return
	}
	var st stateEnvelope
	if json.Unmarshal(b, &st) != nil || st.Version != 1 || len(st.Departures) > 64 {
		return
	}
	if n.merge(st.Facts) != nil {
		return
	}
	for name, t := range st.Departures {
		n.acceptDeparture(departure{name, t})
	}
}
func (n *Node) NotifyAlive(p *memberlist.Node) error {
	var f Facts
	if !identifier(p.Name) || len(p.Meta) > memberlist.MetaMaxSize || json.Unmarshal(p.Meta, &f) != nil || f.Encoding != 1 || f.Name != p.Name || len(f.Version) > 64 || len(f.Channel) > 16 || len(f.OS) > 16 || len(f.Arch) > 16 || f.Cap < 0 || f.Cap > 65536 {
		return errors.New("invalid cluster metadata")
	}
	n.mu.Lock()
	_, ok := n.members[p.Name]
	full := !ok && len(n.members) >= 64
	n.mu.Unlock()
	if full {
		return ErrBound
	}
	return nil
}

// Upstream Node.State is shadowed by its internal nodeState.State. Never
// consult it: join/leave callbacks and incarnation-scoped intent own our table.
func (n *Node) NotifyJoin(p *memberlist.Node) { n.transition(p.Name, "alive", p, "join") }
func (n *Node) NotifyUpdate(p *memberlist.Node) {
	n.mu.Lock()
	old := n.members[p.Name]
	n.mu.Unlock()
	state := old.State
	if state == "" {
		state = "alive"
	}
	n.transition(p.Name, state, p, "update")
}
func (n *Node) NotifyLeave(p *memberlist.Node) {
	n.mu.Lock()
	left := n.departures[p.Name].Equal(n.members[p.Name].Started)
	n.mu.Unlock()
	if left {
		n.transition(p.Name, "left", p, "leave")
	} else {
		n.transition(p.Name, "dead", p, "dead")
	}
}
func (n *Node) transition(name, state string, p *memberlist.Node, event string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	v, ok := n.members[name]
	if p != nil {
		var f Facts
		if json.Unmarshal(p.Meta, &f) != nil {
			return
		}
		v.Facts = f
		v.Address = net.JoinHostPort(p.Addr.String(), strconv.Itoa(int(p.Port)))
	}
	if !ok && p == nil {
		return
	}
	if ok && v.State == state {
		n.members[name] = v
		return
	}
	v.State = state
	v.Changed = time.Now().UTC()
	n.members[name] = v
	n.memberCount.Store(int32(len(n.members)))
	n.logger.Info("cluster "+event, "member", name)
}

func (n *Node) Members() []Member {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make([]Member, 0, len(n.members))
	for _, v := range n.members {
		v.Since = time.Since(v.Changed).Round(time.Millisecond).String()
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
func (n *Node) Address() string        { n.mu.Lock(); defer n.mu.Unlock(); return n.bound }
func (n *Node) Join(addr string) error { return n.join(addr, false) }
func (n *Node) join(addr string, automatic bool) error {
	if _, p, e := address(addr, false); e != nil || p == 0 {
		return errors.New("invalid cluster join address")
	}
	n.life.Lock()
	defer n.life.Unlock()
	n.mu.Lock()
	closed, paused := n.closed, n.paused
	n.mu.Unlock()
	if closed {
		return errors.New("cluster stopped")
	}
	if paused && automatic {
		return errors.New("cluster explicitly left")
	}
	if paused {
		_ = n.ml.Shutdown()
		var f Facts
		_ = json.Unmarshal(n.facts, &f)
		f.Started = time.Now().UTC()
		n.facts, _ = json.Marshal(f)
		ml, e := memberlist.Create(n.config)
		if e != nil {
			return errors.New("cluster restart failed")
		}
		n.ml = ml
		n.mu.Lock()
		n.bound = ml.LocalNode().Address()
		n.paused = false
		n.mu.Unlock()
	}
	count, e := n.ml.Join([]string{addr})
	n.mu.Lock()
	defer n.mu.Unlock()
	if e != nil || count == 0 {
		n.lastJoin = "seed unavailable"
		return errors.New("cluster join failed (address unreachable or key mismatch)")
	}
	n.lastJoin = "joined"
	n.mu.Unlock()
	n.Counters.mu.Lock()
	n.ready = true
	n.Counters.mu.Unlock()
	n.mu.Lock()
	return nil
}
func (n *Node) seedLoop() {
	defer close(n.done)
	if len(n.cfg.Seeds) == 0 {
		n.mu.Lock()
		n.lastJoin = "no seeds configured"
		n.mu.Unlock()
		<-n.stop
		return
	}
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-n.stop:
			return
		case <-timer.C:
			n.mu.Lock()
			paused := n.paused
			n.mu.Unlock()
			success := false
			if !paused {
				for _, s := range n.cfg.Seeds {
					if n.join(s, true) == nil {
						success = true
					}
				}
			}
			n.mu.Lock()
			if success {
				n.retry = 60 * time.Second
			} else {
				n.retry = min(n.retry*2, 60*time.Second)
			}
			n.next = time.Now().Add(n.retry)
			delay := n.retry
			n.mu.Unlock()
			timer.Reset(delay)
		}
	}
}
func (n *Node) Leave() error {
	n.life.Lock()
	defer n.life.Unlock()
	n.mu.Lock()
	n.paused = true
	local := n.members[n.cfg.Name]
	n.departures[n.cfg.Name] = local.Started
	n.mu.Unlock()
	// Reliable messages plus gossip and push/pull make intent survive packet
	// loss/reordering. It is scoped to startup time so stale notices cannot
	// classify a later incarnation's crash as a graceful departure.
	notice, _ := json.Marshal(departure{n.cfg.Name, local.Started})
	n.queue.QueueBroadcast(message{"leave/" + n.cfg.Name, notice})
	for _, m := range n.Members() {
		if m.Name == n.cfg.Name || m.State != "alive" {
			continue
		}
		host, port, e := net.SplitHostPort(m.Address)
		if e != nil {
			continue
		}
		p, _ := strconv.Atoi(port)
		_ = n.ml.SendReliable(&memberlist.Node{Name: m.Name, Addr: net.ParseIP(host), Port: uint16(p)}, notice)
	}
	if e := n.ml.Leave(3 * time.Second); e != nil {
		return errors.New("cluster leave timed out")
	}
	return nil
}
func (n *Node) Close(graceful bool) error {
	n.mu.Lock()
	if n.closed {
		n.mu.Unlock()
		return nil
	}
	n.closed = true
	close(n.stop)
	n.mu.Unlock()
	<-n.done
	if graceful {
		_ = n.Leave()
	}
	n.life.Lock()
	defer n.life.Unlock()
	return n.ml.Shutdown()
}

// Rotation methods never log or transmit key material. Call locally on all
// members in add/use/remove phases; only 32-byte AES-GCM keys are accepted.
func (n *Node) AddKey(key []byte) error {
	if len(key) != 32 {
		return errors.New("invalid cluster key")
	}
	return n.ring.AddKey(key)
}
func (n *Node) UseKey(key []byte) error {
	if len(key) != 32 {
		return errors.New("invalid cluster key")
	}
	return n.ring.UseKey(key)
}
func (n *Node) RemoveKey(key []byte) error {
	if len(key) != 32 {
		return errors.New("invalid cluster key")
	}
	return n.ring.RemoveKey(key)
}
