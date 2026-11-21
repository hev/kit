package hookapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/hev/kit/internal/attention"
	"github.com/hev/kit/internal/hooks"
)

type pendingCall struct {
	hook     hook
	delivery delivery
	body     []byte
	ctx      context.Context
	cancel   context.CancelFunc
}

func redactEvent(ev event) event {
	ev.Event = ev.Event.Redacted()
	ev.Cwd, ev.TranscriptPath = "", ""
	return ev
}

func (s *Service) ingest(w http.ResponseWriter, r *http.Request, id Identity) {
	start := time.Now()
	raw, err := readBody(w, r, maxBody)
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		Error(w, 413, "payload_too_large", "Hook event is too large")
		return
	}
	var ev event
	if err != nil || json.Unmarshal(raw, &ev) != nil || ev.Schema != "hev.hook.event/v1" ||
		!eventID.MatchString(ev.ID) || ev.Time.IsZero() || !slices.Contains(harnessEvents, ev.Event.Event) ||
		(ev.Harness != "claude" && ev.Harness != "codex") ||
		ev.Tool != nil && (ev.Tool.Name == "" || !slices.Contains([]string{"read", "mutate", "steer", "other"}, ev.Tool.Kind)) {
		invalid(w)
		return
	}
	// Ignore kit-only fields received through harness ingress.
	ev.Data, ev.Reason = nil, ""
	if ev.Host == "" {
		ev.Host = id.Host
	}
	full, _ := json.Marshal(ev)
	redacted, _ := json.Marshal(redactEvent(ev))
	e := record{ID: ev.ID, Event: ev.Event.Event, Harness: ev.Harness, Host: ev.Host, Session: ev.SessionID,
		Target: ev.Target, Time: ev.Time, Decision: "allow", Body: redacted}
	if ev.Tool != nil {
		e.ToolKind = ev.Tool.Kind
	}
	ctx, cancel := contextForEvent(r.Context())
	defer cancel()
	out, err := s.publish(ctx, id.Account, e, full, gateable(e.Event), start)
	if err != nil {
		unavailable(w)
		return
	}
	sendJSON(w, 200, out)
}

// Publish publishes a server-raised kit event using the same envelope as the
// local handler. The trusted producer supplies the content-free form for the
// event log and for receivers that did not opt in.
func (s *Service) Publish(ctx context.Context, accountID, eventID, name, host string, full, redacted json.RawMessage) error {
	if accountID == "" || eventID == "" || !slices.Contains(metaEvents, name) ||
		len(full) > maxBody || len(redacted) > maxBody || !json.Valid(full) || !json.Valid(redacted) {
		return errors.New("invalid kit hook event")
	}
	now := time.Now()
	e := record{ID: eventID, Event: name, Harness: "kit", Host: host, Time: now.UTC(), Body: slices.Clone(redacted)}
	_, err := s.publish(ctx, accountID, e, full, false, now)
	return err
}

func (s *Service) publish(ctx context.Context, accountID string, e record, full []byte, canBlock bool, start time.Time) (gate, error) {
	out := gate{ID: e.ID, Decision: "allow"}
	cacheKey := accountID + ":" + e.ID
	s.mu.Lock()
	if wait := s.waiting[cacheKey]; wait != nil {
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return out, nil
		case <-wait:
		}
		s.mu.Lock()
	}
	if answer, exists := s.answers[cacheKey]; exists {
		s.mu.Unlock()
		return answer, nil
	}
	now := time.Now().UTC()
	s.prune(now)
	a := s.accounts[accountID]
	if a == nil {
		if len(s.accounts) >= 1024 {
			s.mu.Unlock()
			return out, errors.New("account capacity reached")
		}
		a = freshAccount()
		s.accounts[accountID] = a
	}
	if old, exists := a.Events[e.ID]; exists {
		s.mu.Unlock()
		return gate{ID: e.ID, Decision: old.Decision, Blocking: old.Blocking, Failed: old.Failed, Elapsed: old.Elapsed}, nil
	}
	if len(a.Events) >= 16384 || len(s.waiting) >= 32 {
		s.mu.Unlock()
		return out, errors.New("event capacity reached")
	}
	a.Events[e.ID] = e
	var candidates []storedHook
	for _, h := range a.Hooks {
		if h.Enabled && matches(h.Events, e.Event) {
			candidates = append(candidates, h)
		}
	}
	slices.SortFunc(candidates, func(a, b storedHook) int { return a.Created.Compare(b.Created) })
	if len(a.Jobs)+len(candidates) > 16384 {
		delete(a.Events, e.ID)
		s.mu.Unlock()
		return out, errors.New("delivery capacity reached")
	}
	var calls []pendingCall
	var jobIDs []string
	totalContent := 0
	for _, p := range s.content {
		totalContent += len(p)
	}
	for _, h := range candidates {
		d := newDelivery(h.hook, e.ID, e.Event, canBlock && h.Blocking, false, now)
		payload := []byte(e.Body)
		if h.IncludeContent {
			payload = full
		}
		j := job{delivery: d, URL: h.URL, Content: h.IncludeContent}
		if d.Blocking {
			call, cancel := context.WithCancel(ctx)
			s.active[d.ID] = cancel
			calls = append(calls, pendingCall{h.private(), d, payload, call, cancel})
		} else if h.IncludeContent {
			if totalContent+len(payload) > 32<<20 {
				j.Status, j.Completed = "failed", &now
			} else {
				s.content[d.ID] = slices.Clone(payload)
				totalContent += len(payload)
			}
		} else {
			j.Payload = payload
		}
		a.Jobs[d.ID] = j
		jobIDs = append(jobIDs, d.ID)
	}
	if s.save() != nil {
		delete(a.Events, e.ID)
		for _, id := range jobIDs {
			delete(a.Jobs, id)
			delete(s.content, id)
			delete(s.active, id)
		}
		for _, call := range calls {
			call.cancel()
		}
		s.mu.Unlock()
		return out, errors.New("cannot record hook event")
	}
	wait := make(chan struct{})
	s.waiting[cacheKey] = wait
	s.mu.Unlock()
	s.signal()
	answers := make([]hooks.Response, len(calls))
	results := make([]attempt, len(calls))
	var wg sync.WaitGroup
	for i, call := range calls {
		wg.Go(func() {
			defer call.cancel()
			results[i], answers[i] = s.send(call.ctx, call.hook, call.delivery, call.body)
		})
	}
	wg.Wait()
	out.Blocking = len(calls)
	for _, a := range results {
		if a.Error != "" {
			out.Failed++
		}
	}
	merged := hooks.Merge(answers...)
	out.Decision, out.Reason, out.Context, out.Message = apiDecision(merged.Decision), merged.Reason, merged.Context, merged.SystemMessage
	out.Elapsed = time.Since(start).Milliseconds()
	e.Decision, e.Blocking, e.Failed, e.Elapsed = out.Decision, out.Blocking, out.Failed, out.Elapsed
	if e.Harness != "kit" {
		var body map[string]any
		_ = json.Unmarshal(e.Body, &body)
		body["decision"], body["injected_bytes"], body["elapsed_ms"], body["deliveries"] = out.Decision, len(out.Context), out.Elapsed, len(jobIDs)
		e.Body, _ = json.Marshal(body)
	}
	s.mu.Lock()
	a.Events[e.ID] = e
	// Full blocking answers are only retained briefly for concurrent requests.
	// Event log rows contain the decision and lengths, never response content.
	if len(s.answers) < 256 {
		s.answers[cacheKey] = out
	}
	_ = s.save()
	close(wait)
	delete(s.waiting, cacheKey)
	s.mu.Unlock()
	time.AfterFunc(5*time.Second, func() {
		s.mu.Lock()
		delete(s.answers, cacheKey)
		s.mu.Unlock()
	})
	for i, call := range calls {
		s.finish(accountID, call.hook, call.delivery, results[i], answers[i])
	}
	return out, nil
}

func (s *Service) test(w http.ResponseWriter, r *http.Request, id Identity, key string) {
	raw, err := readBody(w, r, 4096)
	var input struct {
		Event string `json:"event"`
	}
	raw = bytes.TrimSpace(raw)
	if err != nil || len(raw) != 0 && (raw[0] != '{' || json.Unmarshal(raw, &input) != nil) ||
		input.Event != "" && !eventName(input.Event) {
		Error(w, 422, "bad_request", "Invalid test event")
		return
	}
	now := time.Now().UTC()
	s.mu.Lock()
	a := s.accounts[id.Account]
	if a == nil {
		s.mu.Unlock()
		missing(w)
		return
	}
	h, ok := a.Hooks[key]
	if !ok {
		s.mu.Unlock()
		missing(w)
		return
	}
	if len(a.Jobs) >= 16384 || len(s.active) >= 4096 {
		s.mu.Unlock()
		unavailable(w)
		return
	}
	if input.Event == "" {
		input.Event = "pre-tool"
		for _, name := range h.Events {
			if eventName(name) {
				input.Event = name
				break
			}
		}
	}
	eventID, body := testPayload(input.Event, id.Host, h.URL, now)
	d := newDelivery(h.hook, eventID, input.Event, h.Blocking && gateable(input.Event), true, now)
	if h.Blocking && !d.Blocking {
		callDeadline := now.Add(time.Duration(h.Timeout) * time.Millisecond)
		call, stop := context.WithDeadline(r.Context(), callDeadline)
		defer stop()
		r = r.WithContext(call)
	}
	a.Jobs[d.ID] = job{delivery: d, URL: h.URL}
	call, cancel := context.WithCancel(r.Context())
	s.active[d.ID] = cancel
	s.mu.Unlock()
	defer cancel()
	sent, answer := s.send(call, h.private(), d, body)
	s.finish(id.Account, h.hook, d, sent, answer)
	settle(&d, sent, answer, time.Now().UTC())
	sendJSON(w, 200, d)
}

func testPayload(name, host, endpoint string, now time.Time) (string, []byte) {
	id := newID()
	activity := hooks.Activity{ID: id, Time: now, Event: name, Host: host}
	var payload any
	switch name {
	case "needs-you", "ready", "done":
		var random [12]byte
		rand.Read(random[:])
		id = "att_" + hex.EncodeToString(random[:])
		payload = attention.Notice{Schema: "hev.attention.event/v1", ID: id, Time: now, Event: name,
			Source: "run", Name: "test", Host: host, Text: "Webhook test", Thread: "run:kit/test"}
	case "peer-message":
		activity.Schema = "hev.peer.message/v1"
		payload = hooks.PeerMessage{Activity: activity, From: "kit/webhook-test", To: "kit/webhook-test", Kind: "note"}
	case "board-post":
		activity.Schema = "hev.board.post/v1"
		payload = hooks.BoardPost{Activity: activity, Board: "test", Kind: "post", Title: "Webhook test", Tags: []string{}, URL: endpoint, Author: "kit"}
	default:
		harness := "kit"
		if slices.Contains(harnessEvents, name) {
			harness = "codex"
		}
		ev := event{Event: hooks.Event{Schema: "hev.hook.event/v1", ID: id, Time: now, Event: name, Harness: harness}, Host: host}
		if name == "pre-tool" || name == "post-tool" {
			ev.Tool = &hooks.Tool{Name: "test", Kind: "other"}
		}
		payload = ev
	}
	body, _ := json.Marshal(payload)
	return id, body
}
