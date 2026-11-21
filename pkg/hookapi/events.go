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
	"strings"
	"sync"
	"time"

	"github.com/hev/kit/internal/attention"
	"github.com/hev/kit/internal/hooks"
	"github.com/hev/kit/pkg/kitapi"
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
	out, err := s.publish(ctx, id.Account, e, full, gateable(e.Event), start, nil)
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
	if !validPublication(accountID, eventID, name, full, redacted) {
		return errors.New("invalid kit hook event")
	}
	now := time.Now()
	e := record{ID: eventID, Event: name, Harness: "kit", Host: host, Time: now.UTC(), Body: slices.Clone(redacted)}
	_, err := s.publish(ctx, accountID, e, full, false, now, nil)
	return err
}

func validPublication(accountID, eventID, name string, full, redacted []byte) bool {
	return accountID != "" && eventID != "" && slices.Contains(metaEvents, name) &&
		len(full) <= maxBody && len(redacted) <= maxBody && json.Valid(full) && json.Valid(redacted)
}

func (s *Service) publish(ctx context.Context, accountID string, e record, full []byte, canBlock bool, start time.Time, publication *publication) (gate, error) {
	out := gate{ID: e.ID, Decision: "allow"}
	a := s.account(accountID, true)
	if a == nil {
		return out, errors.New("account capacity reached")
	}
	a.mu.Lock()
	now := time.Now().UTC()
	if wait := a.waiting[e.ID]; wait != nil {
		a.mu.Unlock()
		select {
		case <-ctx.Done():
			return out, nil
		case <-wait:
			a.mu.Lock()
		}
	}
	if old, exists := a.Events[e.ID]; exists {
		a.mu.Unlock()
		if publication != nil {
			return out, s.write(ctx, accountID, stateRow{kind: "publication", id: publication.Key})
		}
		if old.Answer != nil {
			return *old.Answer, nil
		}
		return gate{ID: e.ID, Decision: old.Decision, Blocking: old.Blocking, Failed: old.Failed, Elapsed: old.Elapsed}, nil
	}
	var candidates []storedHook
	newContent := 0
	for _, h := range a.Hooks {
		if h.Enabled && matches(h.Events, e.Event) {
			candidates = append(candidates, h)
			if canBlock && h.Blocking {
				e.Blocking++
			} else if h.IncludeContent && publication == nil {
				newContent += len(full)
			}
		}
	}
	if (e.Blocking > 0 && len(a.waiting) >= 32) || a.contentBytes+newContent > 32<<20 {
		a.mu.Unlock()
		return out, errors.New("hook queue capacity reached")
	}
	slices.SortFunc(candidates, func(a, b storedHook) int {
		if order := a.Created.Compare(b.Created); order != 0 {
			return order
		}
		return strings.Compare(a.ID, b.ID)
	})
	evicted, err := s.oldest(ctx, accountID, "event", len(a.Events)+1-rowLimit)
	if err == nil {
		var jobs []stateRow
		jobs, err = s.oldest(ctx, accountID, "job", len(a.Jobs)+len(candidates)-rowLimit)
		evicted = append(evicted, jobs...)
	}
	if err != nil {
		a.mu.Unlock()
		return out, err
	}
	observeBody := e.Body
	e.Pending = e.Blocking > 0
	if !e.Pending {
		e.Elapsed = time.Since(start).Milliseconds()
		out.Elapsed = e.Elapsed
		e = withEventStats(e, len(candidates), 0)
	}
	rows := append(evicted, eventRow(e))
	if publication != nil {
		rows = append(rows, stateRow{kind: "publication", id: publication.Key})
	}
	var calls []pendingCall
	var jobs []job
	for _, h := range candidates {
		d := newDelivery(h.hook, e.ID, e.Event, canBlock && h.Blocking, false, now)
		j := job{delivery: d, URL: h.URL, Content: h.IncludeContent}
		if d.Blocking {
			payload := []byte(e.Body)
			if h.IncludeContent {
				payload = full
			}
			call, cancel := context.WithCancel(ctx)
			calls = append(calls, pendingCall{h.private(), d, payload, call, cancel})
		} else if !h.IncludeContent {
			j.Payload = observeBody
		} else if publication != nil {
			j.Reference = publication.Reference
		}
		jobs = append(jobs, j)
		rows = append(rows, jobRow(j))
	}
	if err := s.write(ctx, accountID, rows...); err != nil {
		for _, call := range calls {
			call.cancel()
		}
		a.mu.Unlock()
		return out, errors.New("cannot record hook event")
	}
	applyRemovals(a, evicted)
	a.Events[e.ID] = e
	for _, j := range jobs {
		a.Jobs[j.ID] = j
		if j.Content && !j.Blocking && len(j.Reference) == 0 {
			a.content[j.ID] = full
			a.contentBytes += len(full)
		}
	}
	for _, call := range calls {
		a.active[call.delivery.ID] = call.cancel
	}
	var wait chan struct{}
	if e.Pending {
		wait = make(chan struct{})
		a.waiting[e.ID] = wait
	}
	a.mu.Unlock()
	s.signal()
	if wait == nil {
		return out, nil
	}
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
	out.Decision = apiDecision(merged.Decision)
	out.Reason = limitText(merged.Reason, kitapi.HookTextRunes)
	out.Context = limitText(merged.Context, kitapi.HookTextRunes)
	out.Message = limitText(merged.SystemMessage, kitapi.HookTextRunes)
	out.Elapsed = time.Since(start).Milliseconds()
	e.Decision, e.Blocking, e.Failed, e.Elapsed = out.Decision, out.Blocking, out.Failed, out.Elapsed
	e.Answer, e.Pending = &out, false
	e = withEventStats(e, len(jobs), len(out.Context))
	a.mu.Lock()
	completed := []stateRow{eventRow(e)}
	var apply []func()
	for i, call := range calls {
		rows, commit := a.finishRows(call.hook, call.delivery, results[i], answers[i])
		completed = append(completed, rows...)
		apply = append(apply, commit)
	}
	err = s.write(context.Background(), accountID, completed...)
	// Even if persistence is unavailable, use this decision for retries in
	// the running process. A pending durable record is never re-dispatched.
	a.Events[e.ID] = e
	if err == nil {
		for _, commit := range apply {
			commit()
		}
	}
	close(wait)
	delete(a.waiting, e.ID)
	a.mu.Unlock()
	return out, nil
}

func withEventStats(e record, deliveries, injected int) record {
	if e.Harness != "kit" {
		var body map[string]any
		_ = json.Unmarshal(e.Body, &body)
		body["decision"], body["injected_bytes"], body["elapsed_ms"], body["deliveries"] = e.Decision, injected, e.Elapsed, deliveries
		e.Body, _ = json.Marshal(body)
	}
	return e
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
	a := s.account(id.Account, false)
	if a == nil {
		missing(w)
		return
	}
	a.mu.Lock()
	h, ok := a.Hooks[key]
	if !ok {
		a.mu.Unlock()
		missing(w)
		return
	}
	if len(a.active) >= 4096 {
		a.mu.Unlock()
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
	j := job{delivery: d, URL: h.URL}
	evicted, err := s.oldest(r.Context(), id.Account, "job", len(a.Jobs)+1-rowLimit)
	if err != nil || s.write(r.Context(), id.Account, append(evicted, jobRow(j))...) != nil {
		a.mu.Unlock()
		unavailable(w)
		return
	}
	applyRemovals(a, evicted)
	a.Jobs[d.ID] = j
	call, cancel := context.WithCancel(r.Context())
	a.active[d.ID] = cancel
	a.mu.Unlock()
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
