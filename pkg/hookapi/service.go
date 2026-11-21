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
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hev/kit/internal/hooks"
	"github.com/oklog/ulid/v2"
)

const (
	retention            = 30 * 24 * time.Hour
	accountCacheSize     = 1024
	observeWorkers       = 32
	observeQueueLimit    = 96
	observeContentLimit  = 8 << 20
	volatileAccountLimit = 128
	volatileByteLimit    = 128 << 20
)

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
	mu             sync.Mutex
	content        map[string][]byte
	active         map[string]context.CancelFunc
	Hooks          map[string]storedHook
	Jobs           map[string]job
	Events         map[string]record
	Creates        map[string]createReceipt
	waiting        map[string]chan struct{}
	contentBytes   int
	contentByHook  map[string]int
	pendingObserve map[string]int
	service        *Service
	// Cache metadata is protected by Service.mu.
	refs    int
	used    uint64
	ready   chan struct{}
	loadErr error
}

// Service owns an encrypted single-process state database.
type Service struct {
	mu               sync.Mutex
	cacheClock       uint64
	volatileBytes    atomic.Int64
	volatileAccounts atomic.Int32
	lastAccount      string
	scheduleMu       sync.Mutex
	sending          map[string]bool
	retryAfter       time.Time
	accounts         map[string]*account
	db               *sql.DB
	source           PublicationSource
	client           *http.Client
	path             string
	seal             cipher.AEAD
	wake             chan struct{}
	done             chan struct{}
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
	go func() {
		workers.Wait()
		s.db.Close()
		close(s.done)
	}()
	return s, nil
}

// Done closes after the service context has ended, its workers have stopped,
// and its database is closed. Callers can use it before reopening the path.
func (s *Service) Done() <-chan struct{} { return s.done }

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
		path: path, seal: seal, wake: make(chan struct{}, observeWorkers), done: make(chan struct{}), sending: make(map[string]bool),
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

// acquire pins an account while callers use its mutable state. Durable queued
// jobs need no pin. Jobs with volatile payloads pin their account until settled.
func (s *Service) acquire(ctx context.Context, id string) (*account, error) {
	s.mu.Lock()
	a := s.accounts[id]
	load := a == nil
	if load {
		a = freshAccount()
		a.service = s
		a.ready = make(chan struct{})
		s.accounts[id] = a
	}
	a.refs++
	s.mu.Unlock()

	if load {
		a.loadErr = s.loadAccount(ctx, id, a)
		close(a.ready)
	} else {
		select {
		case <-a.ready:
		case <-ctx.Done():
			s.release(id, a)
			return nil, ctx.Err()
		}
	}
	if a.loadErr != nil {
		s.release(id, a)
		return nil, a.loadErr
	}
	return a, nil
}

func (s *Service) release(id string, a *account) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a.refs--
	s.cacheClock++
	a.used = s.cacheClock
	if a.refs == 0 && a.loadErr != nil {
		delete(s.accounts, id)
	}
	for len(s.accounts) > accountCacheSize {
		var oldest *account
		var key string
		for id, candidate := range s.accounts {
			if candidate.refs == 0 && candidate.contentBytes == 0 &&
				(oldest == nil || candidate.used < oldest.used) {
				oldest, key = candidate, id
			}
		}
		if oldest == nil {
			return
		}
		delete(s.accounts, key)
	}
}

func (s *Service) prune(ctx context.Context, now time.Time) {
	// Walk persisted accounts in bounded pages, including evicted accounts.
	for after := ""; ctx.Err() == nil; {
		ids, err := s.accountPage(ctx, after)
		if err != nil || len(ids) == 0 {
			return
		}
		for _, id := range ids {
			if a, err := s.acquire(ctx, id); err == nil {
				s.pruneAccount(ctx, id, a, now)
				s.release(id, a)
			}
		}
		after = ids[len(ids)-1]
	}
}

func (s *Service) pruneAccount(ctx context.Context, id string, a *account, now time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	var rows []stateRow
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
		return
	}
	applyRemovals(a, rows)
	for _, r := range rows {
		switch r.kind {
		case "hook":
			a.Hooks[r.id] = r.value.(storedHook)
		case "create":
			delete(a.Creates, r.id)
		}
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
			a.removeJob(r.id)
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
	accountID, id := s.claimDue(ctx)
	if id == "" {
		return false
	}
	defer func() {
		s.scheduleMu.Lock()
		delete(s.sending, accountID)
		s.scheduleMu.Unlock()
	}()
	a, err := s.acquire(ctx, accountID)
	if err != nil {
		return false
	}
	defer s.release(accountID, a)
	return s.deliver(ctx, accountID, id, a)
}

// Walk accounts in round-robin order and take the oldest due job within
// the selected account. A busy account cannot occupy multiple workers.
func (s *Service) claimDue(ctx context.Context) (accountID, id string) {
	s.scheduleMu.Lock()
	defer s.scheduleMu.Unlock()
	if time.Now().Before(s.retryAfter) {
		return "", ""
	}
	query, stop := context.WithTimeout(ctx, 500*time.Millisecond)
	defer stop()
	args := []any{time.Now().UnixMilli(), s.lastAccount}
	filter := ""
	if len(s.sending) != 0 {
		filter = " AND account NOT IN (" + strings.TrimSuffix(strings.Repeat("?,", len(s.sending)), ",") + ")"
		for account := range s.sending {
			args = append(args, account)
		}
	}
	const selectDue = "SELECT account,id FROM hook_rows WHERE kind='job' AND due!=0 AND due<=?"
	const ordered = " ORDER BY account,due,id LIMIT 1"
	err := s.db.QueryRowContext(query, selectDue+" AND account>?"+filter+ordered, args...).Scan(&accountID, &id)
	if errors.Is(err, sql.ErrNoRows) {
		err = s.db.QueryRowContext(query, selectDue+" AND account<=?"+filter+ordered, args...).Scan(&accountID, &id)
	}
	if err != nil {
		return "", ""
	}
	s.sending[accountID] = true
	s.lastAccount = accountID
	return accountID, id
}

func (s *Service) deliver(ctx context.Context, accountID, id string, a *account) bool {
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
	if s.finish(accountID, a, h.hook, j.delivery, result, answer) {
		return true
	}
	// A failed state write leaves the old due time in SQLite. Pace retries
	// during a database outage instead of repeatedly sending the same job.
	s.scheduleMu.Lock()
	s.retryAfter = time.Now().Add(time.Second)
	s.scheduleMu.Unlock()
	return false
}

func (h storedHook) private() hook {
	out := h.hook
	out.Secret, out.OldSecret, out.OldUntil = h.Secret, h.OldSecret, h.OldUntil
	return out
}

func (j job) pendingObserve() bool {
	return !j.Blocking && !j.Test && j.Completed == nil && j.ID != ""
}

func (a *account) setJob(id string, j job) {
	a.removeJob(id)
	a.Jobs[id] = j
	if j.pendingObserve() {
		a.pendingObserve[j.HookID]++
	}
}

func (a *account) removeJob(id string) {
	if j := a.Jobs[id]; j.pendingObserve() {
		if a.pendingObserve[j.HookID] <= 1 {
			delete(a.pendingObserve, j.HookID)
		} else {
			a.pendingObserve[j.HookID]--
		}
	}
	delete(a.Jobs, id)
}

func (a *account) reserveContent(size int) bool {
	if a.contentBytes == 0 && a.service.volatileAccounts.Add(1) > volatileAccountLimit {
		a.service.volatileAccounts.Add(-1)
		return false
	}
	if a.service.volatileBytes.Add(int64(size)) > volatileByteLimit {
		a.releaseContentReservation(size)
		return false
	}
	return true
}

func (a *account) releaseContentReservation(size int) {
	if size == 0 {
		return
	}
	a.service.volatileBytes.Add(-int64(size))
	if a.contentBytes == 0 {
		a.service.volatileAccounts.Add(-1)
	}
}

func (a *account) keepContent(j job, body []byte) {
	a.content[j.ID] = body
	a.contentBytes += len(body)
	a.contentByHook[j.HookID] += len(body)
}

func (a *account) dropContent(id string) {
	size := len(a.content[id])
	if size == 0 {
		return
	}
	hookID := a.Jobs[id].HookID
	a.contentByHook[hookID] -= size
	if a.contentByHook[hookID] == 0 {
		delete(a.contentByHook, hookID)
	}
	a.contentBytes -= size
	a.releaseContentReservation(size)
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
			a.removeJob(row.id)
		} else {
			a.setJob(row.id, row.value.(job))
		}
	}
}

func (s *Service) finish(accountID string, a *account, h hook, d delivery, result attempt, answer hooks.Response) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	rows, apply := a.finishRows(h, d, result, answer)
	if s.write(context.Background(), accountID, rows...) != nil {
		return false
	}
	apply()
	return true
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
		a.setJob(d.ID, j)
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
