// Package hookapi serves hook routes after the host has authenticated a caller.
package hookapi

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
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
	Payload   json.RawMessage
	Reference json.RawMessage `json:",omitempty"`
	URL       string
	Content   bool
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
	Answer                                                        *gate
	Pending                                                       bool
}

type createReceipt struct {
	ID     string
	Digest [32]byte
	At     time.Time
}

type account struct {
	mu           sync.Mutex
	content      map[string][]byte
	active       map[string]context.CancelFunc
	Hooks        map[string]storedHook
	Jobs         map[string]job
	Events       map[string]record
	Creates      map[string]createReceipt
	waiting      map[string]chan struct{}
	contentBytes int
}

// Service owns an encrypted single-process state database.
type Service struct {
	mu       sync.RWMutex
	accounts map[string]*account
	db       *sql.DB
	source   PublicationSource
	client   *http.Client
	path     string
	seal     cipher.AEAD
	wake     chan struct{}
}

// Open loads encrypted state and starts bounded workers for observe deliveries.
func Open(ctx context.Context, path string, key []byte) (*Service, error) {
	return OpenWithPublications(ctx, path, key, nil)
}

// OpenWithPublications enables board publication and source-backed retries.
func OpenWithPublications(ctx context.Context, path string, key []byte, source PublicationSource) (*Service, error) {
	s, err := open(ctx, path, key)
	if err != nil {
		return nil, err
	}
	s.source = source
	var workers sync.WaitGroup
	if source != nil {
		workers.Go(func() { s.publicationWorker(ctx) })
	}
	for range cap(s.wake) {
		workers.Go(func() { s.worker(ctx) })
	}
	workers.Go(func() { s.expire(ctx) })
	go func() { workers.Wait(); s.db.Close() }()
	return s, nil
}

func open(ctx context.Context, path string, key []byte) (*Service, error) {
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
		accounts: make(map[string]*account), client: webhookClient(),
		path: path, seal: seal, wake: make(chan struct{}, 4),
	}
	if err := s.openState(ctx); err != nil {
		if s.db != nil {
			s.db.Close()
		}
		return nil, errors.New("cannot open hook database")
	}
	return s, nil
}

func (s *Service) loadLegacy() error {
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
	return nil
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

func (s *Service) account(id string, create bool) *account {
	s.mu.RLock()
	a := s.accounts[id]
	s.mu.RUnlock()
	if a != nil || !create {
		return a
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if a = s.accounts[id]; a == nil && len(s.accounts) < 1024 {
		a = freshAccount()
		s.accounts[id] = a
	}
	return a
}

func (s *Service) prune(ctx context.Context, now time.Time) {
	s.mu.RLock()
	ids := make([]string, 0, len(s.accounts))
	for id := range s.accounts {
		ids = append(ids, id)
	}
	s.mu.RUnlock()
	for _, id := range ids {
		a := s.account(id, false)
		a.mu.Lock()
		rows := []stateRow{}
		for key, e := range a.Events {
			if e.Time.Before(now.Add(-retention)) && !e.Pending {
				rows = append(rows, stateRow{kind: "event", id: key})
			}
		}
		for key, j := range a.Jobs {
			if j.Created.Before(now.Add(-retention)) && j.Completed != nil {
				rows = append(rows, stateRow{kind: "job", id: key})
			}
		}
		for key, r := range a.Creates {
			if r.At.Before(now.Add(-24 * time.Hour)) {
				rows = append(rows, stateRow{kind: "create", id: key})
			}
		}
		for key, h := range a.Hooks {
			if h.OldSecret != "" && !h.OldUntil.After(now) {
				h.OldSecret, h.OldUntil = "", time.Time{}
				rows = append(rows, stateRow{kind: "hook", id: key, value: h})
			}
		}
		if s.write(ctx, id, rows...) != nil {
			a.mu.Unlock()
			continue
		}
		applyRemovals(a, rows)
		for _, r := range rows {
			if r.kind == "hook" {
				a.Hooks[r.id] = r.value.(storedHook)
			}
			if r.kind == "create" {
				delete(a.Creates, r.id)
			}
		}
		a.mu.Unlock()
	}
}

func applyRemovals(a *account, rows []stateRow) {
	for _, r := range rows {
		if r.value != nil {
			continue
		}
		switch r.kind {
		case "event":
			delete(a.Events, r.id)
		case "job":
			delete(a.Jobs, r.id)
		}
	}
}

func (s *Service) expire(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.prune(ctx, now.UTC())
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
	query, stop := context.WithTimeout(ctx, 500*time.Millisecond)
	rows, err := s.db.QueryContext(query, "SELECT account,id FROM hook_rows WHERE kind='job' AND due!=0 AND due<=? ORDER BY due,account,id LIMIT ?", time.Now().UnixMilli(), cap(s.wake))
	type candidate struct{ account, id string }
	var due []candidate
	if err == nil {
		for rows.Next() {
			var c candidate
			if rows.Scan(&c.account, &c.id) != nil {
				break
			}
			due = append(due, c)
		}
		rows.Close()
	}
	stop()
	for _, c := range due {
		if s.deliver(ctx, c.account, c.id) {
			return true
		}
	}
	return false
}

func (s *Service) deliver(ctx context.Context, accountID, id string) bool {
	a := s.account(accountID, false)
	if a == nil {
		return false
	}
	a.mu.Lock()
	j, exists := a.Jobs[id]
	h, ok := a.Hooks[j.HookID]
	if !exists || !ok || !h.Enabled || h.URL != j.URL || j.Completed != nil || a.active[id] != nil ||
		j.Next != nil && time.Now().Before(*j.Next) {
		a.mu.Unlock()
		return false
	}
	body := []byte(j.Payload)
	if j.Content {
		body = a.content[id]
	}
	call, cancel := context.WithCancel(ctx)
	a.active[id] = cancel
	a.mu.Unlock()
	var result attempt
	var answer hooks.Response
	if len(j.Reference) != 0 {
		resolve, stop := context.WithTimeout(call, 2*time.Second)
		var p Publication
		var err error
		if s.source != nil {
			p, err = s.source(resolve, accountID, j.Reference)
		}
		stop()
		if err != nil || p.ID != j.EventID || p.Name != j.Event || len(p.Full) > maxBody || !json.Valid(p.Full) {
			result = attempt{N: len(j.Attempts) + 1, At: time.Now().UTC(), Error: "publication source unavailable"}
		} else {
			body = p.Full
		}
	}
	if result.Error == "" {
		result, answer = s.send(call, h.private(), j.delivery, body)
	}
	cancel()
	s.finish(accountID, h.hook, j.delivery, result, answer)
	return true
}

func (h storedHook) private() hook {
	out := h.hook
	out.Secret, out.OldSecret, out.OldUntil = h.Secret, h.OldSecret, h.OldUntil
	return out
}

func (a *account) dropContent(id string) {
	a.contentBytes -= len(a.content[id])
	delete(a.content, id)
}

func canceledJobs(a *account, hookID string, remove bool) []stateRow {
	now := time.Now().UTC()
	var rows []stateRow
	for id, j := range a.Jobs {
		if j.HookID != hookID {
			continue
		}
		if remove {
			rows = append(rows, stateRow{kind: "job", id: id})
		} else if j.Completed == nil {
			j.Status, j.Completed, j.Payload, j.Reference, j.Next = "failed", &now, nil, nil, nil
			rows = append(rows, jobRow(j))
		}
	}
	return rows
}

func (a *account) applyCanceledJobs(rows []stateRow) {
	for _, row := range rows {
		if cancel := a.active[row.id]; cancel != nil {
			cancel()
		}
		a.dropContent(row.id)
		if row.value == nil {
			delete(a.Jobs, row.id)
		} else {
			a.Jobs[row.id] = row.value.(job)
		}
	}
}

func (s *Service) finish(accountID string, h hook, d delivery, result attempt, answer hooks.Response) {
	a := s.account(accountID, false)
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	rows, apply := a.finishRows(h, d, result, answer)
	if s.write(context.Background(), accountID, rows...) == nil {
		apply()
	}
}

func (a *account) finishRows(h hook, d delivery, result attempt, answer hooks.Response) ([]stateRow, func()) {
	delete(a.active, d.ID)
	j, exists := a.Jobs[d.ID]
	current, ok := a.Hooks[h.ID]
	if !exists || !ok || j.Completed != nil {
		return nil, func() {}
	}
	now := time.Now().UTC()
	j.Attempts = append([]attempt(nil), j.Attempts...)
	settle(&j.delivery, result, answer, now)
	if j.Completed != nil {
		j.Payload, j.Reference = nil, nil
	}
	rows := []stateRow{jobRow(j)}
	var canceled []stateRow
	if !d.Test && current.URL == h.URL && !d.Created.Before(current.HealthSince) {
		reason := ""
		switch {
		case !result.nongone():
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
			for _, row := range canceledJobs(a, h.ID, false) {
				if row.id != d.ID {
					canceled = append(canceled, row)
				}
			}
		}
		rows = append(rows, stateRow{kind: "hook", id: h.ID, value: current})
	}
	return append(rows, canceled...), func() {
		a.Jobs[d.ID] = j
		if j.Completed != nil {
			a.dropContent(d.ID)
		}
		a.Hooks[h.ID] = current
		a.applyCanceledJobs(canceled)
	}
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
