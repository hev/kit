package foh

import (
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hev/kit/internal/harness"
	"github.com/oklog/ulid/v2"
)

const (
	// MaxSessions is how many session files are kept; older ones are pruned.
	MaxSessions = 500
	// MaxSessionBytes caps one session file.
	MaxSessionBytes   = 64 << 20
	maxRecordContent  = 64 << 10
	sessionRecVersion = 1
)

// SessionsDir is the registered FOH trace root: HEV_FOH_SESSIONS_DIR or
// ~/.hev/foh/sessions.
func SessionsDir() string {
	if d, ok := harness.Lookup(harness.FOH); ok && d.Trace != nil {
		if root := d.Trace.Root(); root != "" {
			return root
		}
	}
	return filepath.Join(homeDir(), ".hev", "foh", "sessions")
}

// Record is one JSONL line of a FOH session trace.
type Record struct {
	V          int        `json:"v"`
	Time       time.Time  `json:"ts"`
	Session    string     `json:"session"`
	Seq        int        `json:"seq"`
	Type       string     `json:"type"` // meta, user, assistant, tool, steer, model_switch, error
	Turn       int        `json:"turn,omitempty"`
	Model      string     `json:"model,omitempty"`
	Provider   string     `json:"provider,omitempty"`
	Role       string     `json:"role,omitempty"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolName   string     `json:"tool_name,omitempty"`
	IsError    bool       `json:"is_error,omitempty"`
	Usage      *Usage     `json:"usage,omitempty"`
	From       string     `json:"from,omitempty"`
	// FromProvider is the provider before a model_switch.
	FromProvider string `json:"from_provider,omitempty"`
	Cwd          string `json:"cwd,omitempty"`
}

// SessionLog appends redacted records to ~/.hev/foh/sessions/<ulid>.jsonl.
type SessionLog struct {
	ID   string
	Path string

	mu      sync.Mutex
	f       *os.File
	seq     int
	written int64
	secrets []string
	full    bool
}

// NewSessionID returns a fresh ULID.
func NewSessionID() string { return ulid.MustNew(ulid.Now(), rand.Reader).String() }

// OpenSession creates a session file in dir (SessionsDir when empty) and
// prunes the oldest files past MaxSessions. secrets are scrubbed from every
// record.
func OpenSession(dir string, secrets ...string) (*SessionLog, error) {
	if dir == "" {
		dir = SessionsDir()
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	id := NewSessionID()
	path := filepath.Join(dir, id+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	pruneSessions(dir, MaxSessions)
	return &SessionLog{ID: id, Path: path, f: f, secrets: secrets}, nil
}

func pruneSessions(dir string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
			names = append(names, e.Name())
		}
	}
	if len(names) <= keep {
		return
	}
	sort.Strings(names) // ULIDs sort by time
	for _, n := range names[:len(names)-keep] {
		_ = os.Remove(filepath.Join(dir, n))
	}
}

// maxSecrets bounds the scrub lists a session or toolset keeps.
const maxSecrets = 64

// addSecret appends v once, keeping the newest maxSecrets values.
func addSecret(list []string, v string) []string {
	if len(v) < 4 || slices.Contains(list, v) {
		return list
	}
	list = append(list, v)
	if len(list) > maxSecrets {
		list = append([]string(nil), list[len(list)-maxSecrets:]...)
	}
	return list
}

// AddSecret scrubs v from later records too.
func (s *SessionLog) AddSecret(v string) {
	if s == nil || v == "" {
		return
	}
	s.mu.Lock()
	s.secrets = addSecret(s.secrets, v)
	s.mu.Unlock()
}

// Write appends r after scrubbing secrets and capping content. A nil log
// drops records.
func (s *SessionLog) Write(r Record) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.f == nil || s.full {
		return
	}
	s.seq++
	r.V, r.Session, r.Seq = sessionRecVersion, s.ID, s.seq
	if r.Time.IsZero() {
		r.Time = time.Now().UTC()
	}
	r.Content = capText(Scrub(r.Content, s.secrets...), maxRecordContent)
	if len(r.ToolCalls) > 0 {
		calls := make([]ToolCall, len(r.ToolCalls))
		for i, c := range r.ToolCalls {
			c.Function.Arguments = capText(Scrub(c.Function.Arguments, s.secrets...), maxRecordContent)
			calls[i] = c
		}
		r.ToolCalls = calls
	}
	b, err := json.Marshal(r)
	if err != nil {
		return
	}
	b = append(b, '\n')
	if s.written+int64(len(b)) > MaxSessionBytes {
		s.full = true
		tail, _ := json.Marshal(Record{V: sessionRecVersion, Time: time.Now().UTC(), Session: s.ID, Seq: s.seq, Type: "truncated"})
		_, _ = s.f.Write(append(tail, '\n'))
		return
	}
	n, _ := s.f.Write(b)
	s.written += int64(n)
}

// Close closes the file.
func (s *SessionLog) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.f == nil {
		return nil
	}
	err := s.f.Close()
	s.f = nil
	return err
}
