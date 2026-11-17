// Package steer keeps steering messages and their receipts (RFC 0013): one
// JSON file per message in the target's steer directory, moved through
// queued -> injected -> acked by the hook layer and `hev steer ack`.
package steer

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
)

// Levels, mildest first.
const (
	Note     = "note"
	Urgent   = "urgent"
	HaltL    = "halt"
	Redirect = "redirect"
)

// Levels lists the valid --level values.
var Levels = []string{Note, Urgent, HaltL, Redirect}

// ValidLevel reports whether l is a steering level.
func ValidLevel(l string) bool {
	for _, v := range Levels {
		if v == l {
			return true
		}
	}
	return false
}

// States.
const (
	Queued   = "queued"
	Injected = "injected"
	Acked    = "acked"
)

// Deliveries.
const (
	ToolBoundary = "tool-boundary"
	TurnBoundary = "turn-boundary"
)

// Mark records when and where a receipt moved.
type Mark struct {
	At       time.Time `json:"at"`
	Event    string    `json:"event,omitempty"`
	Session  string    `json:"session,omitempty"`
	Turn     int       `json:"turn,omitempty"`
	ToolCall int       `json:"tool_call,omitempty"`
	Tool     string    `json:"tool,omitempty"`
	By       string    `json:"by,omitempty"`
}

// Message is one steering message and its receipt.
type Message struct {
	ID       string    `json:"id"`
	Target   string    `json:"target"`
	Level    string    `json:"level"`
	Text     string    `json:"text"`
	From     string    `json:"from,omitempty"`
	State    string    `json:"state"`
	Delivery string    `json:"delivery"`
	QueuedAt time.Time `json:"queued_at"`
	Injected *Mark     `json:"injected,omitempty"`
	Acked    *Mark     `json:"acked,omitempty"`
	// Blocked counts tool calls held while this message waited for an ack.
	Blocked int `json:"blocked,omitempty"`
	// HaltTimeout bounds a halt before the supervisor's hard stop.
	HaltTimeout string `json:"halt_timeout,omitempty"`
	Escalated   *Mark  `json:"escalated,omitempty"`
	// Redirected names the rework a redirect started.
	Redirected string `json:"redirected,omitempty"`
}

// Halting reports a halt or redirect that has not yet ended its turn.
func (m Message) Halting() bool {
	return (m.Level == HaltL || m.Level == Redirect) && m.State != Acked
}

// Latency is queued-to-injected time, when injected.
func (m Message) Latency() time.Duration {
	if m.Injected == nil {
		return 0
	}
	return m.Injected.At.Sub(m.QueuedAt)
}

// State is the hook's view of the target's current harness session.
type State struct {
	Harness     string    `json:"harness,omitempty"`
	Session     string    `json:"session,omitempty"`
	Turn        int       `json:"turn"`
	ToolCalls   int       `json:"tool_calls"`
	LastEvent   string    `json:"last_event,omitempty"`
	LastEventAt time.Time `json:"last_event_at,omitempty"`
	// Halted is set when a halt ended a turn and cleared by the next one.
	Halted   bool       `json:"halted,omitempty"`
	HaltedAt *time.Time `json:"halted_at,omitempty"`
	HaltID   string     `json:"halt_id,omitempty"`
	// AttemptEnded is when a supervisor last closed this target's halts
	// after its harness exited (EndHalts). A halt queued after it belongs to
	// the attempt that is starting or running, not to an ended one.
	AttemptEnded *time.Time `json:"attempt_ended,omitempty"`
}

// WiredFile marks a target whose harness launches a supervisor wires with
// kit's hooks (a dispatch run or orchestrator round).
const WiredFile = "wired.json"

// Supervised reports whether a supervisor launches this target's harness
// with kit's hooks, and so closes its halts whenever the harness exits.
func (s Store) Supervised() bool {
	_, err := os.Stat(filepath.Join(s.Dir, WiredFile))
	return err == nil
}

// CurrentAttempt reports whether a halt was admitted for the attempt that is
// starting or running: in a supervised target, every halt queued after the
// last closure. Elsewhere (a peers session) there is no attempt boundary, and
// no queued halt counts as current.
func (s Store) CurrentAttempt(st State, m Message) bool {
	if !s.Supervised() {
		return false
	}
	return st.AttemptEnded == nil || m.QueuedAt.After(*st.AttemptEnded)
}

// Store is one target's steer directory.
type Store struct {
	Dir    string
	Target string
}

// NewID returns a fresh message ID.
func NewID() string { return "st-" + strings.ToLower(ulid.Make().String()) }

// ValidID reports whether id looks like a steer message ID.
func ValidID(id string) bool {
	if !strings.HasPrefix(id, "st-") || len(id) != 29 {
		return false
	}
	for _, r := range id[3:] {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'z') {
			return false
		}
	}
	return true
}

// Lock serializes receipt changes in the directory. Hooks for parallel tool
// calls run concurrently; the lock is what makes an injection happen once.
// A lock older than ten seconds belongs to a dead process and is broken.
func (s Store) Lock(wait time.Duration) (func(), error) {
	if err := os.MkdirAll(s.Dir, 0700); err != nil {
		return nil, err
	}
	p := filepath.Join(s.Dir, ".lock")
	deadline := time.Now().Add(wait)
	for {
		f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			fmt.Fprint(f, os.Getpid())
			f.Close()
			return func() { os.Remove(p) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		if info, e := os.Stat(p); e == nil && time.Since(info.ModTime()) > 10*time.Second {
			os.Remove(p)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("steer store %s is locked", s.Dir)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.%d.tmp", path, os.Getpid())
	if err := os.WriteFile(tmp, append(b, '\n'), 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Put writes a message. Callers hold the lock for read-modify-write.
func (s Store) Put(m Message) error {
	if !ValidID(m.ID) {
		return fmt.Errorf("invalid steer ID %q", m.ID)
	}
	if err := os.MkdirAll(s.Dir, 0700); err != nil {
		return err
	}
	return writeJSON(filepath.Join(s.Dir, m.ID+".json"), m)
}

// Get reads one message.
func (s Store) Get(id string) (Message, error) {
	var m Message
	if !ValidID(id) {
		return m, fmt.Errorf("invalid steer ID %q", id)
	}
	b, err := os.ReadFile(filepath.Join(s.Dir, id+".json"))
	if err != nil {
		return m, err
	}
	return m, json.Unmarshal(b, &m)
}

// List returns the store's messages, oldest first.
func (s Store) List() ([]Message, error) {
	entries, err := os.ReadDir(s.Dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Message
	for _, e := range entries {
		id := strings.TrimSuffix(e.Name(), ".json")
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || !ValidID(id) {
			continue
		}
		m, err := s.Get(id)
		if err != nil {
			continue
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// LoadState reads the hook state; a missing file is the zero state.
func (s Store) LoadState() State {
	var st State
	b, err := os.ReadFile(filepath.Join(s.Dir, "state.json"))
	if err == nil {
		_ = json.Unmarshal(b, &st)
	}
	return st
}

// SaveState writes the hook state.
func (s Store) SaveState(st State) error {
	if err := os.MkdirAll(s.Dir, 0700); err != nil {
		return err
	}
	return writeJSON(filepath.Join(s.Dir, "state.json"), st)
}

// Enqueue stores a new queued message.
func (s Store) Enqueue(level, text, from, delivery string, haltTimeout time.Duration) (Message, error) {
	if !ValidLevel(level) {
		return Message{}, fmt.Errorf("unknown level %q (want %s)", level, strings.Join(Levels, ", "))
	}
	if strings.TrimSpace(text) == "" {
		return Message{}, fmt.Errorf("message required")
	}
	unlock, err := s.Lock(5 * time.Second)
	if err != nil {
		return Message{}, err
	}
	defer unlock()
	// QueuedAt is taken under the lock, so it orders strictly against an
	// attempt's closure (EndHalts), which the hooks use to tell a halt for
	// the starting attempt from one an ended attempt left behind.
	m := Message{ID: NewID(), Target: s.Target, Level: level, Text: text, From: from, State: Queued, Delivery: delivery, QueuedAt: time.Now().UTC()}
	if m.Halting() && haltTimeout > 0 {
		m.HaltTimeout = haltTimeout.String()
	}
	return m, s.Put(m)
}

// ErrNotInjected refuses an ack for a message the agent was never shown.
var ErrNotInjected = errors.New("not injected yet: the agent has not been shown this message, so it cannot acknowledge it")

// Ack acknowledges an injected message. Acking twice is harmless.
func (s Store) Ack(id, by string) (Message, error) {
	unlock, err := s.Lock(5 * time.Second)
	if err != nil {
		return Message{}, err
	}
	defer unlock()
	m, err := s.Get(id)
	if err != nil {
		return m, err
	}
	switch m.State {
	case Acked:
		return m, nil
	case Queued:
		return m, ErrNotInjected
	}
	st := s.LoadState()
	m.State = Acked
	m.Acked = &Mark{At: time.Now().UTC(), Event: "ack", Session: st.Session, Turn: st.Turn, ToolCall: st.ToolCalls, By: by}
	return m, s.Put(m)
}

// Counts is receipts by state.
type Counts struct {
	Queued   int  `json:"queued"`
	Injected int  `json:"injected"`
	Acked    int  `json:"acked"`
	Halting  bool `json:"halting,omitempty"`
	Halted   bool `json:"halted,omitempty"`
}

// Total is the number of receipts.
func (c Counts) Total() int { return c.Queued + c.Injected + c.Acked }

// String is the compact form ls prints: "q1 i0 a2".
func (c Counts) String() string {
	if c.Total() == 0 {
		return "-"
	}
	s := fmt.Sprintf("q%d i%d a%d", c.Queued, c.Injected, c.Acked)
	if c.Halted {
		s += " halted"
	} else if c.Halting {
		s += " halting"
	}
	return s
}

// Long is the sentence show prints.
func (c Counts) Long() string {
	s := fmt.Sprintf("steer: %d queued, %d injected, %d acked", c.Queued, c.Injected, c.Acked)
	if c.Halted {
		s += "; halted"
	} else if c.Halting {
		s += "; halting"
	}
	return s
}

// CountsOf summarizes a store. A missing store is all zeros.
func (s Store) Counts() Counts {
	var c Counts
	ms, _ := s.List()
	for _, m := range ms {
		switch m.State {
		case Queued:
			c.Queued++
		case Injected:
			c.Injected++
		case Acked:
			c.Acked++
		}
		if m.Halting() {
			c.Halting = true
		}
	}
	c.Halted = s.LoadState().Halted
	return c
}

// ActiveHalt is the oldest halt or redirect that has not ended its turn.
func (s Store) ActiveHalt() (Message, bool) {
	ms, _ := s.List()
	for _, m := range ms {
		if m.Halting() {
			return m, true
		}
	}
	return Message{}, false
}

// EndHalts closes halts after the harness process exited: an injected halt
// ended its turn; a queued one was overtaken. It reports the halt that ended
// a turn, if any, and records the target as halted.
func (s Store) EndHalts(by string) (Message, bool, error) {
	unlock, err := s.Lock(5 * time.Second)
	if err != nil {
		return Message{}, false, err
	}
	defer unlock()
	ms, err := s.List()
	if err != nil {
		return Message{}, false, err
	}
	st := s.LoadState()
	now := time.Now().UTC()
	var ended Message
	found := false
	for _, m := range ms {
		if !m.Halting() {
			continue
		}
		if m.State == Injected && !found {
			ended, found = m, true
		}
		m.State = Acked
		m.Acked = &Mark{At: now, Event: "exit", Session: st.Session, Turn: st.Turn, ToolCall: st.ToolCalls, By: by}
		if err := s.Put(m); err != nil {
			return ended, found, err
		}
		if m.ID == ended.ID {
			ended = m
		}
	}
	if found {
		st.Halted, st.HaltedAt, st.HaltID = true, &now, ended.ID
	}
	st.AttemptEnded = &now
	if err := s.SaveState(st); err != nil {
		return ended, found, err
	}
	return ended, found, nil
}

// Update rewrites one message under the lock.
func (s Store) Update(id string, f func(*Message)) (Message, error) {
	unlock, err := s.Lock(5 * time.Second)
	if err != nil {
		return Message{}, err
	}
	defer unlock()
	m, err := s.Get(id)
	if err != nil {
		return m, err
	}
	f(&m)
	return m, s.Put(m)
}
