package hookapi

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const root = "/v1/hooks"
const maxBody = 4 << 20

var harnessEvents = []string{"session-start", "prompt-submit", "pre-tool", "post-tool", "stop", "pre-compact"}
var metaEvents = []string{
	"needs-you", "ready", "done", "order-created", "order-state", "order-amended", "order-deleted",
	"steer-queued", "steer-injected", "steer-acked", "steer-escalated", "admission-queued",
	"admission-admitted", "admission-refused", "cluster-join", "cluster-suspect", "cluster-leave",
	"peer-online", "peer-stale", "peer-message", "peer-acked", "board-post", "board-updated", "board-deleted",
}
var patterns = []string{"*", "order-*", "steer-*", "admission-*", "cluster-*", "peer-*", "board-*"}
var eventID = regexp.MustCompile("^[0-9a-z]{26}$")

func eventName(name string) bool {
	return slices.Contains(harnessEvents, name) || slices.Contains(metaEvents, name)
}
func subscription(name string) bool { return eventName(name) || slices.Contains(patterns, name) }

func matches(subs []string, name string) bool {
	return slices.ContainsFunc(subs, func(sub string) bool {
		return sub == name || strings.HasSuffix(sub, "*") && strings.HasPrefix(name, strings.TrimSuffix(sub, "*"))
	})
}

func gateable(name string) bool { return slices.Contains(harnessEvents[:4], name) }

type edit struct {
	URL         *string   `json:"url"`
	Description *string   `json:"description"`
	Events      *[]string `json:"events"`
	Enabled     *bool     `json:"enabled"`
	Blocking    *bool     `json:"blocking"`
	Timeout     *int      `json:"timeout_ms"`
	Content     *bool     `json:"include_content"`
	Secret      *string   `json:"secret"`
	Rotate      *bool     `json:"rotate_secret"`
}

func (e edit) valid() bool {
	if e.Description != nil && utf8.RuneCountInString(*e.Description) > 200 ||
		e.Timeout != nil && (*e.Timeout < 100 || *e.Timeout > 8000) ||
		e.Secret != nil && (utf8.RuneCountInString(*e.Secret) < 32 || utf8.RuneCountInString(*e.Secret) > 256) ||
		e.Secret != nil && e.Rotate != nil && *e.Rotate {
		return false
	}
	if e.Events != nil {
		names := *e.Events
		if len(names) == 0 || len(names) > 64 {
			return false
		}
		seen := map[string]bool{}
		for _, name := range names {
			if !subscription(name) || seen[name] {
				return false
			}
			seen[name] = true
		}
	}
	return true
}

func (e edit) apply(h *storedHook, now time.Time) bool {
	if e.URL != nil {
		h.URL = *e.URL
	}
	if e.Description != nil {
		h.Description = *e.Description
	}
	if e.Events != nil {
		h.Events = *e.Events
	}
	if e.Enabled != nil {
		h.Enabled = *e.Enabled
		if h.Enabled {
			h.DisabledReason = nil
			h.Failures = 0
		}
	}
	if e.Blocking != nil {
		h.Blocking = *e.Blocking
	}
	if e.Timeout != nil {
		h.Timeout = *e.Timeout
	}
	if e.Content != nil {
		h.IncludeContent = *e.Content
	}
	h.Updated = now
	if h.Secret == "" || e.Secret != nil || e.Rotate != nil && *e.Rotate {
		if h.Secret != "" {
			h.OldSecret, h.OldUntil, h.Rotated = h.Secret, now.Add(24*time.Hour), &now
		}
		if e.Secret != nil {
			h.Secret = *e.Secret
		} else {
			h.Secret = newSecret()
		}
		runes := []rune(h.Secret)
		h.Hint = string(runes[len(runes)-4:])
		return true
	}
	return false
}

// Error writes the shared v1 error envelope. Messages and codes are fixed by
// the caller and never incorporate outbound errors or user data.
func Error(w http.ResponseWriter, status int, code, message string) {
	sendJSON(w, status, map[string]string{"code": code, "error": message})
}

func sendJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func missing(w http.ResponseWriter)     { Error(w, 404, "not_found", "Hook not found") }
func invalid(w http.ResponseWriter)     { Error(w, 400, "bad_request", "Invalid hook request") }
func unavailable(w http.ResponseWriter) { Error(w, 503, "unavailable", "Hook state unavailable") }

func readBody(w http.ResponseWriter, r *http.Request, max int64) ([]byte, error) {
	return io.ReadAll(http.MaxBytesReader(w, r.Body, max))
}

// Serve handles /v1/hooks after authentication by the host router.
func (s *Service) Serve(w http.ResponseWriter, r *http.Request, id Identity) {
	w.Header().Set("Cache-Control", "no-store")
	if id.Account == "" {
		Error(w, 401, "unauthorized", "Valid kit key required")
		return
	}
	if id.ReadOnly && r.Method != "GET" {
		Error(w, 403, "forbidden", "Key cannot write hooks")
		return
	}
	path := r.URL.Path
	method := r.Method
	switch {
	case path == root && method == "POST":
		s.change(w, r, id, "")
	case path == root && method == "GET":
		s.list(w, r, id, "", "hooks")
	case path == root+"/events" && method == "POST":
		s.ingest(w, r, id)
	case path == root+"/events" && method == "GET":
		s.list(w, r, id, "", "events")
	case strings.HasPrefix(path, root+"/"):
		key, action, _ := strings.Cut(strings.TrimPrefix(path, root+"/"), "/")
		if !strings.HasPrefix(key, "hook_") || !eventID.MatchString(strings.TrimPrefix(key, "hook_")) {
			missing(w)
			return
		}
		switch {
		case action == "" && method == "PATCH":
			s.change(w, r, id, key)
		case action == "" && method == "DELETE":
			s.remove(w, id, key)
		case action == "" && method == "GET":
			s.get(w, id, key)
		case action == "deliveries" && method == "GET":
			s.list(w, r, id, key, action)
		case action == "test" && method == "POST":
			s.test(w, r, id, key)
		default:
			missing(w)
		}
	default:
		missing(w)
	}
}

func (s *Service) get(w http.ResponseWriter, id Identity, key string) {
	s.mu.Lock()
	a := s.accounts[id.Account]
	if a == nil {
		s.mu.Unlock()
		missing(w)
		return
	}
	h, ok := a.Hooks[key]
	out := view(a, h, time.Now())
	s.mu.Unlock()
	if !ok {
		missing(w)
		return
	}
	sendJSON(w, 200, out)
}

func freshAccount() *account {
	return &account{Hooks: map[string]storedHook{}, Jobs: map[string]job{}, Events: map[string]record{}, Creates: map[string]createReceipt{}}
}

func (s *Service) change(w http.ResponseWriter, r *http.Request, id Identity, key string) {
	raw, err := readBody(w, r, 32<<10)
	var e edit
	var fields map[string]json.RawMessage
	if err != nil || json.Unmarshal(raw, &e) != nil || json.Unmarshal(raw, &fields) != nil || len(fields) == 0 ||
		key == "" && (e.URL == nil || e.Events == nil || e.Rotate != nil) {
		invalid(w)
		return
	}
	for k, v := range fields {
		if string(v) == "null" && slices.Contains([]string{"url", "description", "events", "enabled", "blocking", "timeout_ms", "include_content", "secret", "rotate_secret"}, k) {
			invalid(w)
			return
		}
	}
	if !e.valid() {
		Error(w, 422, "bad_request", "Invalid hook settings")
		return
	}
	if e.URL != nil && validateURL(r.Context(), *e.URL) != nil {
		Error(w, 422, "bad_request", "Webhook URL must resolve to public HTTPS")
		return
	}
	idem := r.Header.Get("Idempotency-Key")
	if utf8.RuneCountInString(idem) > 255 {
		invalid(w)
		return
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune(now)
	a := s.accounts[id.Account]
	var h storedHook
	status := 200
	if key == "" {
		if a == nil {
			if len(s.accounts) >= 1024 {
				unavailable(w)
				return
			}
			a = freshAccount()
			s.accounts[id.Account] = a
		}
		if receipt, ok := a.Creates[idem]; idem != "" && ok {
			h, exists := a.Hooks[receipt.ID]
			if receipt.Digest != sha256.Sum256(raw) {
				invalid(w)
				return
			}
			if !exists {
				missing(w)
				return
			}
			sendJSON(w, 201, view(a, h, now))
			return
		}
		if len(a.Hooks) >= 128 || len(a.Creates) >= 4096 {
			unavailable(w)
			return
		}
		h = storedHook{hook: hook{ID: "hook_" + newID(), Enabled: true, Timeout: 1500, Created: now,
			CreatedBy: map[string]string{"author": id.Author, "author_name": id.AuthorName, "host": id.Host}}}
		key, status = h.ID, 201
	} else {
		if a == nil {
			missing(w)
			return
		}
		var ok bool
		h, ok = a.Hooks[key]
		if !ok {
			missing(w)
			return
		}
	}
	before := h
	rotated := e.apply(&h, now)
	if before.URL != h.URL {
		h.HealthSince, h.Failures, h.LastSuccess = now, 0, time.Time{}
	}
	a.Hooks[key] = h
	if status == 201 && idem != "" {
		a.Creates[idem] = createReceipt{key, sha256.Sum256(raw), now}
	}
	if err := s.save(); err != nil {
		if status == 201 {
			delete(a.Hooks, key)
			delete(a.Creates, idem)
		} else {
			a.Hooks[key] = before
		}
		unavailable(w)
		return
	}
	if status == 200 && (before.URL != h.URL || !h.Enabled || before.IncludeContent && !h.IncludeContent) {
		s.cancelJobs(a, key, false)
		_ = s.save()
	}
	out := view(a, h, now)
	var secret string
	if rotated {
		secret = h.Secret
	}
	sendJSON(w, status, struct {
		hookView
		Secret string `json:"secret,omitempty"`
	}{out, secret})
}

func (s *Service) remove(w http.ResponseWriter, id Identity, key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.accounts[id.Account]
	if a == nil {
		missing(w)
		return
	}
	h, ok := a.Hooks[key]
	if !ok {
		missing(w)
		return
	}
	delete(a.Hooks, key)
	removed := map[string]job{}
	for jobID, j := range a.Jobs {
		if j.HookID == key {
			removed[jobID] = j
			delete(a.Jobs, jobID)
		}
	}
	if s.save() != nil {
		a.Hooks[key] = h
		for jobID, j := range removed {
			a.Jobs[jobID] = j
		}
		unavailable(w)
		return
	}
	for jobID := range removed {
		if cancel := s.active[jobID]; cancel != nil {
			cancel()
		}
		delete(s.content, jobID)
	}
	w.WriteHeader(204)
}

type hookView struct {
	hook
	Health map[string]any `json:"health"`
}

func view(a *account, h storedHook, now time.Time) hookView {
	state := "idle"
	var last, success, failure *job
	var samples []int64
	succeeded, failedOpen, pending := 0, 0, 0
	for _, j := range a.Jobs {
		if j.HookID != h.ID || j.Test || j.Created.Before(h.HealthSince) {
			continue
		}
		if j.Status == "retrying" {
			pending++
		}
		if len(j.Attempts) == 0 {
			continue
		}
		at := j.Attempts[len(j.Attempts)-1].At
		newer := func(other *job) bool { return other == nil || at.After(other.Attempts[len(other.Attempts)-1].At) }
		if newer(last) {
			last = &j
		}
		if j.Status == "succeeded" && newer(success) {
			success = &j
		}
		if j.Status != "succeeded" && newer(failure) {
			failure = &j
		}
		if at.Before(now.Add(-24 * time.Hour)) {
			continue
		}
		samples = append(samples, j.Latency)
		if j.Status == "succeeded" {
			succeeded++
		}
		if j.FailedOpen {
			failedOpen++
		}
	}
	health := map[string]any{"state": state, "consecutive_failures": h.Failures, "deliveries_24h": len(samples),
		"failed_open_24h": failedOpen, "pending_retries": pending}
	if last != nil {
		a := last.Attempts[len(last.Attempts)-1]
		health["last_delivery_at"], health["last_status_code"] = a.At, a.Code
		if a.Error != "" {
			health["last_error"] = a.Error
		}
		if len(samples) > 0 {
			state = "healthy"
			if a.Error != "" {
				state = "failing"
			}
		}
	}
	if success != nil {
		health["last_success_at"] = success.Attempts[len(success.Attempts)-1].At
	}
	if failure != nil {
		health["last_failure_at"] = failure.Attempts[len(failure.Attempts)-1].At
	}
	if len(samples) > 0 {
		slices.Sort(samples)
		health["success_rate_24h"] = float64(succeeded) / float64(len(samples))
		health["latency_p50_ms_24h"] = samples[(len(samples)*50+99)/100-1]
		health["latency_p95_ms_24h"] = samples[(len(samples)*95+99)/100-1]
	}
	if !h.Enabled {
		state = "disabled"
	}
	health["state"] = state
	return hookView{h.hook, health}
}

type listRow struct {
	at   time.Time
	id   string
	body any
}

func rangeTime(raw string, now time.Time) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t, nil
	}
	if len(raw) > 1 && (strings.HasSuffix(raw, "d") || strings.HasSuffix(raw, "w")) {
		n, err := strconv.Atoi(raw[:len(raw)-1])
		if err == nil && n >= 0 && n <= 36500 {
			if strings.HasSuffix(raw, "w") {
				n *= 7
			}
			return now.AddDate(0, 0, -n), nil
		}
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < 0 {
		return time.Time{}, errors.New("invalid time")
	}
	return now.Add(-d), nil
}

func queryMatch(q url.Values, k, v string) bool { return len(q[k]) == 0 || slices.Contains(q[k], v) }

func (s *Service) list(w http.ResponseWriter, r *http.Request, id Identity, key, kind string) {
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		invalid(w)
		return
	}
	now := time.Now().UTC()
	limit := 50
	if q.Has("limit") {
		limit, err = strconv.Atoi(q.Get("limit"))
		if err != nil || limit < 1 || limit > 200 {
			invalid(w)
			return
		}
	}
	since, e1 := rangeTime(q.Get("since"), now)
	until, e2 := rangeTime(q.Get("until"), now)
	if e1 != nil || e2 != nil || !until.IsZero() && since.After(until) {
		invalid(w)
		return
	}
	for _, field := range []string{"enabled", "blocking"} {
		if q.Has(field) && q.Get(field) != "true" && q.Get(field) != "false" {
			invalid(w)
			return
		}
	}
	for field, valid := range map[string]func(string) bool{
		"event":     subscription,
		"harness":   func(v string) bool { return slices.Contains([]string{"kit", "claude", "codex"}, v) },
		"decision":  func(v string) bool { return slices.Contains([]string{"allow", "block", "halt"}, v) },
		"tool_kind": func(v string) bool { return slices.Contains([]string{"read", "mutate", "steer", "other"}, v) },
		"status":    func(v string) bool { return slices.Contains([]string{"pending", "succeeded", "retrying", "failed"}, v) },
	} {
		for _, v := range q[field] {
			if !valid(v) {
				invalid(w)
				return
			}
		}
	}
	var cursor listRow
	if q.Has("cursor") {
		raw, err := base64.RawURLEncoding.DecodeString(q.Get("cursor"))
		parts := strings.Split(string(raw), "|")
		if err != nil || len(parts) != 2 || parts[1] == "" {
			invalid(w)
			return
		}
		cursor.at, err = time.Parse(time.RFC3339Nano, parts[0])
		cursor.id = parts[1]
		if err != nil {
			invalid(w)
			return
		}
	}
	rows := []listRow{}
	add := func(at time.Time, key string, body any) {
		if at.Before(since) || !until.IsZero() && at.After(until) ||
			!cursor.at.IsZero() && (at.After(cursor.at) || at.Equal(cursor.at) && key >= cursor.id) {
			return
		}
		rows = append(rows, listRow{at, key, body})
	}
	eventMatches := func(name string) bool { return len(q["event"]) == 0 || matches(q["event"], name) }
	s.mu.Lock()
	a := s.accounts[id.Account]
	if a == nil {
		a = freshAccount()
	}
	switch kind {
	case "hooks":
		for _, h := range a.Hooks {
			if !queryMatch(q, "enabled", strconv.FormatBool(h.Enabled)) || !queryMatch(q, "blocking", strconv.FormatBool(h.Blocking)) {
				continue
			}
			if len(q["event"]) != 0 && !slices.ContainsFunc(q["event"], func(v string) bool {
				return matches(h.Events, v) || slices.ContainsFunc(h.Events, func(sub string) bool {
					return slices.ContainsFunc(harnessEvents, func(n string) bool { return matches([]string{v}, n) && matches([]string{sub}, n) }) ||
						slices.ContainsFunc(metaEvents, func(n string) bool { return matches([]string{v}, n) && matches([]string{sub}, n) })
				})
			}) {
				continue
			}
			add(h.Created, h.ID, view(a, h, now))
		}
	case "events":
		if since.Before(now.Add(-retention)) {
			since = now.Add(-retention)
		}
		for _, e := range a.Events {
			if eventMatches(e.Event) && queryMatch(q, "host", e.Host) && queryMatch(q, "harness", e.Harness) &&
				queryMatch(q, "session_id", e.Session) && queryMatch(q, "target", e.Target) && queryMatch(q, "decision", e.Decision) && queryMatch(q, "tool_kind", e.ToolKind) {
				add(e.Time, e.ID, e.Body)
			}
		}
	case "deliveries":
		if _, ok := a.Hooks[key]; !ok {
			s.mu.Unlock()
			missing(w)
			return
		}
		if since.Before(now.Add(-retention)) {
			since = now.Add(-retention)
		}
		for _, j := range a.Jobs {
			if j.HookID == key && eventMatches(j.Event) && queryMatch(q, "status", j.Status) && queryMatch(q, "event_id", j.EventID) {
				// Attempts can be appended by a worker while the result is encoded.
				d := j.delivery
				d.Attempts = slices.Clone(d.Attempts)
				add(j.Created, j.ID, d)
			}
		}
	}
	s.mu.Unlock()
	slices.SortFunc(rows, func(a, b listRow) int {
		if c := b.at.Compare(a.at); c != 0 {
			return c
		}
		return cmp.Compare(b.id, a.id)
	})
	var next *string
	if len(rows) > limit {
		last := rows[limit-1]
		token := base64.RawURLEncoding.EncodeToString([]byte(last.at.Format(time.RFC3339Nano) + "|" + last.id))
		next = &token
		rows = rows[:limit]
	}
	data := make([]any, len(rows))
	for i, row := range rows {
		data[i] = row.body
	}
	sendJSON(w, 200, map[string]any{"data": data, "next_cursor": next})
}

func contextForEvent(ctx context.Context) (context.Context, context.CancelFunc) {
	// Respect disconnects and bound callers without a client deadline.
	return context.WithTimeout(ctx, 8*time.Second)
}
