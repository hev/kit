package hookapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/hev/kit/internal/hooks"
	"github.com/hev/kit/pkg/kitapi"
)

func cachedAccount(t testing.TB, s *Service, id string) *account {
	t.Helper()
	a, err := s.acquire(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	s.release(id, a)
	return a
}

func seedHook(t testing.TB, s *Service, a *account, accountID, id string, blocking, content bool) {
	t.Helper()
	h := storedHook{hook: hook{ID: id, URL: "https://example.invalid", Enabled: true,
		Events: []string{"*"}, Blocking: blocking, IncludeContent: content, Timeout: 1500, Created: time.Now()}, Secret: "test-key"}
	if err := s.write(context.Background(), accountID, stateRow{kind: "hook", id: id, value: h}); err != nil {
		t.Fatal(err)
	}
	a.Hooks[id] = h
}

func seedEvents(t testing.TB, s *Service, accountID string, n int, jobs bool) *account {
	t.Helper()
	a := cachedAccount(t, s, accountID)
	old := time.Now().Add(-time.Hour)
	for start := 0; start < n; start += 128 {
		var rows []stateRow
		for i := start; i < min(n, start+128); i++ {
			key := fmt.Sprintf("prior-%05d", i)
			e := record{ID: key, Event: "stop", Time: old.Add(time.Duration(i) * time.Millisecond), Body: json.RawMessage("{}")}
			a.Events[key] = e
			rows = append(rows, eventRow(e))
			if jobs {
				d := delivery{ID: key, Created: e.Time, Status: "succeeded", Completed: &old}
				if i == 0 {
					d.Status, d.Completed, d.Blocking = "pending", nil, true
				}
				j := job{delivery: d}
				a.setJob(key, j)
				rows = append(rows, jobRow(j))
			}
		}
		if err := s.write(context.Background(), accountID, rows...); err != nil {
			t.Fatal(err)
		}
	}
	return a
}

func postHook(t *testing.T, s *Service, accountID, eventName, id string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(event{Event: hooks.Event{Schema: "hev.hook.event/v1", Event: eventName, Harness: "codex", ID: id, Time: time.Now().UTC()}})
	r := httptest.NewRequest(http.MethodPost, "/v1/hooks/events", strings.NewReader(string(body)))
	w := httptest.NewRecorder()
	s.Serve(w, r, Identity{Account: accountID})
	return w
}

func TestIngestEvictsOldCompletedRowsAndStillDelivers(t *testing.T) {
	var delivered int
	s := service(t, func(*http.Request) (*http.Response, error) { delivered++; return response(204, ""), nil })
	a := seedEvents(t, s, "team", rowLimit, true)
	seedHook(t, s, a, "team", "observe", false, false)
	w := postHook(t, s, "team", "stop", newID())
	if w.Code != 200 || !s.deliverNext(context.Background()) || delivered != 1 {
		t.Fatalf("expected an accepted event and a delivery past the cap, status %d, deliveries %d", w.Code, delivered)
	}
	if len(a.Events) != rowLimit || len(a.Jobs) != rowLimit || a.Events["prior-00000"].ID != "" ||
		a.Jobs["prior-00000"].ID == "" || a.Jobs["prior-00001"].ID != "" {
		t.Fatal("eviction must keep a pending job and remove the oldest completed rows")
	}
}

func TestIngressReservesContentBeforeRecording(t *testing.T) {
	s := service(t, nil)
	a := cachedAccount(t, s, "team")
	seedHook(t, s, a, "team", "opt-in", false, true)
	a.contentBytes = 32 << 20
	id := newID()
	if w := postHook(t, s, "team", "stop", id); w.Code != 503 || len(a.Events) != 0 || len(a.Jobs) != 0 {
		t.Fatal("capacity refusal must leave the event and delivery unrecorded")
	}
	a.contentBytes = 0
	if w := postHook(t, s, "team", "stop", id); w.Code != 200 || len(a.Events) != 1 || len(a.Jobs) != 1 {
		t.Fatal("a capacity refusal must permit the same event to be resent")
	}
}

func TestDelayedReplayKeepsFullGateAnswerAfterRestart(t *testing.T) {
	var called int
	s := service(t, func(*http.Request) (*http.Response, error) {
		called++
		return response(200, `{"decision":"halt","reason":"original reason","add_context":"original context","system_message":"original message"}`), nil
	})
	seedHook(t, s, cachedAccount(t, s, "team"), "team", "blocking", true, false)
	id := newID()
	result := postHook(t, s, "team", "prompt-submit", id)
	if result.Code != 200 || !strings.Contains(result.Body.String(), "original context") {
		t.Fatal("initial hook answer missing")
	}
	time.Sleep(5100 * time.Millisecond)
	if err := s.db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := openTestService(context.Background(), s.path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.db.Close()
	reopened.client.Transport = transport(func(*http.Request) (*http.Response, error) { called++; return response(204, ""), nil })
	replay := postHook(t, reopened, "team", "prompt-submit", id)
	if called != 1 || replay.Code != 200 || replay.Body.String() != result.Body.String() {
		t.Fatal("delayed replay must use the complete original answer without calling the receiver")
	}
	for _, path := range []string{s.path + ".sqlite", s.path + ".sqlite-wal"} {
		raw, err := os.ReadFile(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "original context") {
			t.Fatal("gate text exposed in storage")
		}
	}
}

func TestUnicodeBoundaryAndAggregateKeepHalt(t *testing.T) {
	ctx := strings.Repeat("😀", kitapi.HookTextRunes)
	escaped := strings.Repeat(`\ud83d\ude00`, kitapi.HookTextRunes)
	receiver := []byte(fmt.Sprintf(`{"decision":"halt","reason":"%[1]s","context":"%[1]s","add_context":"%[1]s","system_message":"%[1]s"}`, escaped))
	s := service(t, func(*http.Request) (*http.Response, error) { return response(200, string(receiver)), nil })
	a := cachedAccount(t, s, "team")
	for i := 0; i < 14; i++ {
		seedHook(t, s, a, "team", fmt.Sprintf("blocking-%02d", i), true, false)
	}
	w := postHook(t, s, "team", "prompt-submit", newID())
	var out gate
	if w.Code != 200 || w.Body.Len() > kitapi.HookResultBytes || json.Unmarshal(w.Body.Bytes(), &out) != nil ||
		out.Decision != "halt" || out.Blocking != 14 || out.Failed != 0 || out.Context != ctx || out.Reason != ctx || out.Message != ctx {
		t.Fatal("schema-valid Unicode or merged response lost its decision or bounded text")
	}
	receiver, _ = json.Marshal(map[string]string{"decision": "halt", "add_context": ctx + "文"})
	w = postHook(t, s, "team", "prompt-submit", newID())
	out = gate{}
	if json.Unmarshal(w.Body.Bytes(), &out) != nil || out.Decision != "allow" || out.Failed != 14 || !utf8.Valid(w.Body.Bytes()) {
		t.Fatal("context above the schema limit must fail open")
	}
}

func TestObserveRetryScheduleSurvivesRestart(t *testing.T) {
	want := []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 6 * time.Hour, 15 * time.Hour}
	var ids []string
	outbound := transport(func(r *http.Request) (*http.Response, error) {
		ids = append(ids, r.Header.Get("X-Hev-Delivery-Id"))
		return response(500, ""), nil
	})
	s := service(t, outbound)
	a := cachedAccount(t, s, "team")
	seedHook(t, s, a, "team", "observe", false, false)
	if w := postHook(t, s, "team", "stop", newID()); w.Code != 200 {
		t.Fatalf("ingress status %d", w.Code)
	}
	for i := 0; i < 7; i++ {
		before := time.Now()
		if !s.deliverNext(context.Background()) || len(ids) != i+1 || ids[i] != ids[0] {
			var due int64
			_ = s.db.QueryRow("SELECT due FROM hook_rows WHERE kind='job' AND id=?", ids[0]).Scan(&due)
			j := a.Jobs[ids[0]]
			h, ok := a.Hooks[j.HookID]
			t.Fatalf("attempt %d lost or changed delivery ID (calls %d, due %d, job %s, hook found %t, enabled %t, URL matches %t)", i+1, len(ids), due, j.Status, ok, h.Enabled, h.URL == j.URL)
		}
		j := a.Jobs[ids[0]]
		if len(j.Attempts) != i+1 {
			t.Fatal("attempt history lost")
		}
		if i == 6 {
			if j.Completed == nil || j.Next != nil || j.Status != "failed" || s.deliverNext(context.Background()) {
				t.Fatal("observe delivery must stop after seven attempts")
			}
			break
		}
		if j.Next == nil || j.Next.Before(before.Add(want[i])) || j.Next.After(time.Now().Add(want[i])) {
			t.Fatalf("attempt %d scheduled outside the contract interval", i+1)
		}
		if i == 0 {
			if err := s.db.Close(); err != nil {
				t.Fatal(err)
			}
			var err error
			s, err = openTestService(context.Background(), s.path)
			if err != nil {
				t.Fatal(err)
			}
			defer s.db.Close()
			s.client.Transport = outbound
			a = cachedAccount(t, s, "team")
			loaded := a.Jobs[j.ID]
			if loaded.Next == nil || !loaded.Next.Equal(*j.Next) || s.deliverNext(context.Background()) {
				t.Fatal("restart skipped the stored due time")
			}
			j = loaded
		}
		due := time.Now().Add(-time.Second)
		j.Next = &due
		if err := s.write(context.Background(), "team", jobRow(j)); err != nil {
			t.Fatal(err)
		}
		a.Jobs[j.ID] = j
	}
}

func BenchmarkIngestAtExistingEventCounts(b *testing.B) {
	for _, count := range []int{100, 8000, rowLimit} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			s, err := openTestService(context.Background(), filepath.Join(b.TempDir(), "hooks"))
			if err != nil {
				b.Fatal(err)
			}
			defer s.db.Close()
			seedEvents(b, s, "team", count, false)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				now := time.Now().UTC()
				e := record{ID: newID(), Event: "stop", Harness: "codex", Time: now, Decision: "allow", Body: json.RawMessage("{}")}
				if _, err := s.publish(context.Background(), "team", e, e.Body, false, now, nil); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestBoardPublicationAndOptInDeliveryRecoverFromSource(t *testing.T) {
	ctx := context.Background()
	s := service(t, nil)
	a := cachedAccount(t, s, "team")
	seedHook(t, s, a, "team", "board-hook", false, true)
	ref := json.RawMessage(`{"post":"t:1"}`)
	if err := s.StagePublication(ctx, "team", "board/1", ref); err != nil {
		t.Fatal(err)
	}
	if err := s.db.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := openTestService(ctx, s.path)
	if err != nil {
		t.Fatal(err)
	}
	a = cachedAccount(t, s, "team")
	calls := 0
	s.source = func(_ context.Context, account string, reference json.RawMessage) (Publication, error) {
		if account != "team" || string(reference) != string(ref) {
			t.Fatal("publication reference changed across restart")
		}
		calls++
		if calls == 1 {
			return Publication{}, errors.New("temporary board failure")
		}
		return Publication{ID: "t:1", Name: "board-post", Time: time.Now(), Full: json.RawMessage(`{"body":"original board content"}`), Redacted: json.RawMessage(`{}`)}, nil
	}
	if !s.publishNext(ctx) || len(a.Events) != 0 || len(a.Jobs) != 0 {
		t.Fatal("failed source read lost obligation or published an event")
	}
	if _, err := s.db.ExecContext(ctx, "UPDATE hook_rows SET due=? WHERE kind='publication'", time.Now().UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if !s.publishNext(ctx) || len(a.Events) != 1 || len(a.Jobs) != 1 || s.publishNext(ctx) {
		t.Fatal("publication must enqueue once and clear the durable obligation")
	}
	var id string
	for id = range a.Jobs {
	}
	if err := s.db.Close(); err != nil {
		t.Fatal(err)
	}
	source := s.source
	s, err = openTestService(ctx, s.path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()
	s.source = source
	var delivered bool
	s.client.Transport = transport(func(r *http.Request) (*http.Response, error) {
		var payload struct{ Body string }
		delivered = json.NewDecoder(r.Body).Decode(&payload) == nil && payload.Body == "original board content" &&
			r.Header.Get("X-Hev-Delivery-Id") == id
		return response(204, ""), nil
	})
	if !s.deliverNext(ctx) || !delivered {
		t.Fatal("opt-in board job lost payload or delivery ID across restart")
	}
}

func TestAccountCacheEvictionKeepsDurableObserveWork(t *testing.T) {
	s := service(t, nil)
	create := func(account string) {
		req := httptest.NewRequest(http.MethodPost, "/v1/hooks", strings.NewReader(`{"url":"https://8.8.8.8","events":["stop"]}`))
		w := httptest.NewRecorder()
		s.Serve(w, req, Identity{Account: account, Author: account})
		if w.Code != 201 {
			t.Fatalf("register hook for %s: %d %s", account, w.Code, w.Body)
		}
	}
	const first, last = "first", "last"
	create(first)
	a := cachedAccount(t, s, first)
	w := postHook(t, s, first, "stop", newID())
	if w.Code != 200 {
		t.Fatal("first account could not enqueue a delivery")
	}
	var deliveryID string
	for id := range a.Jobs {
		deliveryID = id
	}
	for i := range accountCacheSize {
		if w := postHook(t, s, fmt.Sprintf("middle-%04d", i), "stop", newID()); w.Code != 200 {
			t.Fatalf("account %d could not ingest: %d", i, w.Code)
		}
	}
	create(last)
	if w := postHook(t, s, last, "stop", newID()); w.Code != 200 {
		t.Fatal("last account could not enqueue a delivery")
	}
	if s.accounts[first] != nil || len(s.accounts) != accountCacheSize {
		t.Fatal("first account should have been evicted from the bounded cache")
	}

	var delivered []string
	s.client.Transport = transport(func(r *http.Request) (*http.Response, error) {
		delivered = append(delivered, r.Header.Get("X-Hev-Delivery-Id"))
		return response(204, ""), nil
	})
	if !s.deliverNext(context.Background()) || !s.deliverNext(context.Background()) ||
		len(delivered) != 2 || delivered[0] != deliveryID || delivered[1] == deliveryID {
		t.Fatalf("expected both accounts to deliver with distinct, stable IDs, got %v", delivered)
	}
	if got := cachedAccount(t, s, first); got == a || got.Jobs[deliveryID].Status != "succeeded" {
		t.Fatal("first account did not reload and settle its original work")
	}
}

func TestObserveWorkersLeaveCapacityForAnotherAccount(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	slow := make(chan struct{}, observeWorkers)
	fast := make(chan struct{}, 1)
	s := service(t, transport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/slow" {
			slow <- struct{}{}
			<-r.Context().Done()
			return nil, r.Context().Err()
		}
		fast <- struct{}{}
		return response(204, ""), nil
	}))
	var workers sync.WaitGroup
	defer func() {
		cancel()
		workers.Wait()
	}()
	for _, id := range []string{"tenant-a", "tenant-b"} {
		a := cachedAccount(t, s, id)
		seedHook(t, s, a, id, "observe", false, false)
		if id == "tenant-a" {
			h := a.Hooks["observe"]
			h.URL += "/slow"
			a.Hooks[h.ID] = h
		}
	}
	for range 2 * observeWorkers {
		if w := postHook(t, s, "tenant-a", "stop", newID()); w.Code != 200 {
			t.Fatal("could not enqueue hanging receiver fixtures")
		}
	}
	for range observeWorkers {
		workers.Go(func() { s.worker(ctx) })
	}
	select {
	case <-slow:
	case <-time.After(3 * time.Second):
		t.Fatal("hanging receiver did not start")
	}
	if w := postHook(t, s, "tenant-b", "stop", newID()); w.Code != 200 {
		t.Fatal("healthy ingress failed")
	}
	select {
	case <-fast:
	case <-time.After(3 * time.Second):
		t.Fatal("healthy tenant waited behind hanging receiver")
	}
	select {
	case <-slow:
		t.Fatal("hanging tenant occupied more than one observe worker")
	default:
	}
}

func TestFullObserveBacklogKeepsGateAndHealthyDelivery(t *testing.T) {
	var consulted, delivered bool
	s := service(t, transport(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/blocking":
			consulted = true
			return response(200, `{"decision":"halt"}`), nil
		case "/healthy":
			delivered = true
			return response(204, ""), nil
		default:
			t.Errorf("unexpected receiver call: %s", r.URL.Path)
			return response(503, ""), nil
		}
	}))
	const team = "team"
	a := cachedAccount(t, s, team)
	for _, name := range []string{"saturated", "blocking", "healthy"} {
		seedHook(t, s, a, team, name, name == "blocking", false)
		h := a.Hooks[name]
		h.URL += "/" + name
		a.Hooks[name] = h
	}
	now := time.Now().UTC()
	next := now.Add(time.Hour)
	for start := 0; start < rowLimit; start += 128 {
		var rows []stateRow
		for range min(128, rowLimit-start) {
			d := newDelivery(a.Hooks["saturated"].hook, newID(), "prompt-submit", false, false, now)
			d.Status, d.Next = "retrying", &next
			j := job{delivery: d, URL: a.Hooks["saturated"].URL}
			a.setJob(j.ID, j)
			rows = append(rows, jobRow(j))
		}
		if err := s.write(context.Background(), team, rows...); err != nil {
			t.Fatal(err)
		}
	}
	id := newID()
	w := postHook(t, s, team, "prompt-submit", id)
	var out gate
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || out.Decision != "halt" || !consulted {
		t.Fatalf("backlog prevented blocking hook from answering: %d %s", w.Code, w.Body)
	}
	if !s.deliverNext(context.Background()) || !delivered || a.Events[id].ID != id {
		t.Fatal("backlog prevented healthy observe delivery or the event log entry")
	}
	for _, j := range a.Jobs {
		if j.EventID == id && j.HookID == "saturated" {
			if j.Status != "failed" || j.Completed == nil || len(j.Attempts) != 0 {
				t.Fatal("over-quota job must be recorded as failed without an attempt")
			}
			return
		}
	}
	t.Fatal("over-quota failure missing from delivery history")
}

func TestObservePersistenceFailurePacesResend(t *testing.T) {
	ctx := context.Background()
	calls := 0
	s := service(t, transport(func(*http.Request) (*http.Response, error) {
		calls++
		return response(204, ""), nil
	}))
	seedHook(t, s, cachedAccount(t, s, "team"), "team", "observe", false, false)
	if w := postHook(t, s, "team", "stop", newID()); w.Code != 200 {
		t.Fatal("could not enqueue fixture")
	}
	_, err := s.db.ExecContext(ctx, `CREATE TRIGGER reject_settlement BEFORE INSERT ON hook_rows
		WHEN NEW.kind = 'job' AND NEW.done = 1 BEGIN SELECT RAISE(ABORT, 'write failure'); END`)
	if err != nil {
		t.Fatal(err)
	}
	if s.deliverNext(ctx) || calls != 1 || s.deliverNext(ctx) || calls != 1 {
		t.Fatal("persistent settlement failure caused an immediate resend")
	}
	if _, err := s.db.ExecContext(ctx, "DROP TRIGGER reject_settlement"); err != nil {
		t.Fatal(err)
	}
	s.retryAfter = time.Now().Add(-time.Second)
	if !s.deliverNext(ctx) || calls != 2 || s.deliverNext(ctx) {
		t.Fatal("delivery did not settle after the database recovered")
	}
}
