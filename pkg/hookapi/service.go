// Package hookapi serves hook routes after the host has authenticated a caller.
package hookapi

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hev/kit/internal/hooks"
	"github.com/oklog/ulid/v2"
)

const retention = 30 * 24 * time.Hour

// Identity is supplied by the host's authenticated router.
type Identity struct {
	Account, Author, AuthorName, Host string
	ReadOnly                          bool
}

type hook struct {
	ID             string            `json:"id"`
	URL            string            `json:"url"`
	Description    string            `json:"description,omitempty"`
	Events         []string          `json:"events"`
	Enabled        bool              `json:"enabled"`
	DisabledReason *string           `json:"disabled_reason"`
	Blocking       bool              `json:"blocking"`
	Timeout        int               `json:"timeout_ms"`
	IncludeContent bool              `json:"include_content"`
	Hint           string            `json:"secret_hint"`
	CreatedBy      map[string]string `json:"created_by"`
	Created        time.Time         `json:"created_at"`
	Updated        time.Time         `json:"updated_at"`
	Rotated        *time.Time        `json:"secret_rotated_at,omitempty"`
	// Private fields are serialized only inside the authenticated ciphertext.
	Secret      string    `json:"-"`
	OldSecret   string    `json:"-"`
	OldUntil    time.Time `json:"-"`
	HealthSince time.Time `json:"-"`
	Failures    int       `json:"-"`
	LastSuccess time.Time `json:"-"`
}

type storedHook struct {
	hook
	Secret      string
	OldSecret   string
	OldUntil    time.Time
	HealthSince time.Time
	Failures    int
	LastSuccess time.Time
}

type job struct {
	delivery
	Payload json.RawMessage
	URL     string
	Content bool
}

type event struct {
	hooks.Event
	Host   string          `json:"host,omitempty"`
	Reason string          `json:"reason,omitempty"`
	Data   json.RawMessage `json:"data,omitempty"`
}

type gate struct {
	ID       string `json:"event_id"`
	Decision string `json:"decision"`
	Reason   string `json:"reason,omitempty"`
	Context  string `json:"add_context,omitempty"`
	Message  string `json:"system_message,omitempty"`
	Blocking int    `json:"blocking_hooks"`
	Failed   int    `json:"failed_open,omitempty"`
	Elapsed  int64  `json:"elapsed_ms"`
}

type record struct {
	Body                                                          json.RawMessage
	ID, Event, Harness, Host, Session, Target, ToolKind, Decision string
	Time                                                          time.Time
	Blocking, Failed                                              int
	Elapsed                                                       int64
}

type createReceipt struct {
	ID     string
	Digest [32]byte
	At     time.Time
}

type account struct {
	Hooks   map[string]storedHook
	Jobs    map[string]job
	Events  map[string]record
	Creates map[string]createReceipt
}

// Service owns one encrypted, single-process state file. Run one instance per
// file. The context controls the delivery workers' lifetime.
type Service struct {
	mu       sync.Mutex
	accounts map[string]*account
	content  map[string][]byte // full opt-in observe bodies are transient
	answers  map[string]gate
	active   map[string]context.CancelFunc
	waiting  map[string]chan struct{}
	client   *http.Client
	path     string
	seal     cipher.AEAD
	wake     chan struct{}
}

// Open loads encrypted state and starts bounded workers for observe deliveries.
func Open(ctx context.Context, path string, key []byte) (*Service, error) {
	if path == "" || len(key) < 32 {
		return nil, errors.New("hook state path and encryption key required")
	}
	digest := sha256.Sum256(key)
	block, err := aes.NewCipher(digest[:])
	if err != nil {
		return nil, err
	}
	seal, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return nil, err
	}
	s := &Service{
		accounts: make(map[string]*account), content: make(map[string][]byte),
		answers: make(map[string]gate), active: make(map[string]context.CancelFunc),
		waiting: make(map[string]chan struct{}), client: webhookClient(),
		path: path, seal: seal, wake: make(chan struct{}, 4),
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	for range cap(s.wake) {
		go s.worker(ctx)
	}
	go s.expire(ctx)
	return s, nil
}

func (s *Service) load() error {
	info, err := os.Stat(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || info.Size() > 64<<20 || info.Mode().Perm()&0077 != 0 {
		return errors.New("invalid hook state file")
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		return errors.New("cannot read hook state")
	}
	plain, err := s.seal.Open(nil, nil, raw, nil)
	if err != nil || json.Unmarshal(plain, &s.accounts) != nil || s.accounts == nil {
		return errors.New("cannot decode hook state")
	}
	// Content is never persisted. Interrupted blocking attempts are never
	// replayed, and sending a partial opt-in payload would change its meaning.
	now := time.Now().UTC()
	for _, a := range s.accounts {
		for id, j := range a.Jobs {
			if j.Completed == nil && (j.Content || j.Blocking || j.Test) {
				j.Status, j.Completed, j.Next, j.Payload = "failed", &now, nil, nil
				j.FailedOpen = j.Blocking
				if j.Blocking {
					j.Decision = "allow"
				}
				a.Jobs[id] = j
			}
		}
	}
	return nil
}

func (s *Service) save() error {
	plain, err := json.Marshal(s.accounts)
	if err != nil || len(plain) > 63<<20 {
		return errors.New("hook state capacity exceeded")
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".hooks-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(s.seal.Seal(nil, nil, plain, nil))
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), s.path)
}

func (s *Service) signal() {
	for range cap(s.wake) {
		select {
		case s.wake <- struct{}{}:
		default:
			return
		}
	}
}

func (s *Service) prune(now time.Time) (changed bool) {
	cutoff := now.Add(-retention)
	for id, a := range s.accounts {
		for k, e := range a.Events {
			if e.Time.Before(cutoff) && s.waiting[id+":"+k] == nil {
				delete(a.Events, k)
				changed = true
				delete(s.answers, id+":"+k)
			}
		}
		for k, r := range a.Creates {
			if r.At.Before(now.Add(-24 * time.Hour)) {
				delete(a.Creates, k)
				changed = true
			}
		}
		for k, j := range a.Jobs {
			if j.Created.Before(cutoff) && s.active[k] == nil {
				delete(a.Jobs, k)
				changed = true
				delete(s.content, k)
			}
		}
		for k, h := range a.Hooks {
			if h.OldSecret != "" && !h.OldUntil.After(now) {
				h.OldSecret = ""
				changed = true
				h.OldUntil = time.Time{}
				a.Hooks[k] = h
			}
		}
		if len(a.Hooks)+len(a.Jobs)+len(a.Events)+len(a.Creates) == 0 {
			delete(s.accounts, id)
			changed = true
		}
	}
	return changed
}

func (s *Service) expire(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.mu.Lock()
			if s.prune(now.UTC()) {
				_ = s.save()
			}
			s.mu.Unlock()
		}
	}
}

func (s *Service) worker(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-s.wake:
		}
		for ctx.Err() == nil && s.deliverNext(ctx) {
		}
	}
}

func (s *Service) deliverNext(ctx context.Context) bool {
	s.mu.Lock()
	for accountID, a := range s.accounts {
		for id, j := range a.Jobs {
			h, ok := a.Hooks[j.HookID]
			if !ok || !h.Enabled || h.URL != j.URL || j.Blocking || j.Test || j.Completed != nil ||
				(j.Next != nil && time.Now().Before(*j.Next)) || s.active[id] != nil {
				continue
			}
			body := []byte(j.Payload)
			if j.Content {
				body = s.content[id]
			}
			call, cancel := context.WithCancel(ctx)
			s.active[id] = cancel
			s.mu.Unlock()
			attempt, answer := s.send(call, h.private(), j.delivery, body)
			cancel()
			s.finish(accountID, h.hook, j.delivery, attempt, answer)
			return true
		}
	}
	s.mu.Unlock()
	return false
}

func (h storedHook) private() hook {
	out := h.hook
	out.Secret, out.OldSecret, out.OldUntil = h.Secret, h.OldSecret, h.OldUntil
	return out
}

func (s *Service) cancelJobs(a *account, hookID string, remove bool) {
	now := time.Now().UTC()
	for id, j := range a.Jobs {
		if j.HookID != hookID {
			continue
		}
		if cancel := s.active[id]; cancel != nil {
			cancel()
		}
		delete(s.content, id)
		if remove {
			delete(a.Jobs, id)
			continue
		}
		if j.Completed == nil {
			j.Status, j.Completed, j.Payload, j.Next = "failed", &now, nil, nil
			a.Jobs[id] = j
		}
	}
}

func (s *Service) finish(accountID string, h hook, d delivery, attempt attempt, answer hooks.Response) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.active, d.ID)
	a := s.accounts[accountID]
	if a == nil {
		return
	}
	j, exists := a.Jobs[d.ID]
	current, ok := a.Hooks[h.ID]
	if !exists || !ok || j.Completed != nil {
		return
	}
	now := time.Now().UTC()
	settle(&j.delivery, attempt, answer, now)
	if j.Completed != nil {
		j.Payload = nil
		delete(s.content, d.ID)
	}
	a.Jobs[d.ID] = j
	if !d.Test && current.URL == h.URL && !d.Created.Before(current.HealthSince) {
		reason := ""
		switch {
		case !attempt.nongone():
			reason = "gone"
		case j.Status == "succeeded":
			current.Failures = 0
			current.LastSuccess = now
		case j.Status == "failed":
			current.Failures++
			if current.Failures >= 50 && now.Sub(current.LastSuccess) >= 72*time.Hour {
				reason = "failing"
			}
		}
		if reason != "" {
			current.Enabled, current.DisabledReason, current.Updated = false, &reason, now
			s.cancelJobs(a, h.ID, false)
		}
		a.Hooks[h.ID] = current
	}
	// The previously saved job remains recoverable if this write fails.
	_ = s.save()
}

func newDelivery(h hook, eventID, name string, blocking, test bool, now time.Time) delivery {
	return delivery{ID: "dlv_" + newID(), HookID: h.ID, EventID: eventID, Event: name, Blocking: blocking, Test: test, Status: "pending", Attempts: []attempt{}, Created: now}
}

func newID() string { return lowerULID(ulid.Make().String()) }

func lowerULID(id string) string {
	b := []byte(id)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 'a' - 'A'
		}
	}
	return string(b)
}

func newSecret() string {
	raw := make([]byte, 32)
	rand.Read(raw)
	return "whsec_" + base64.RawURLEncoding.EncodeToString(raw)
}
