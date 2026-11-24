package workidentity

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

const (
	workspaceID UUID = "10000000-0000-4000-8000-000000000001"
	askID       UUID = "20000000-0000-4000-8000-000000000001"
	requestID   UUID = "30000000-0000-4000-8000-000000000001"
	scopeID     UUID = "40000000-0000-4000-8000-000000000001"
	createKey   UUID = "50000000-0000-4000-8000-000000000001"
)

func fixtureClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := New(srv.URL, "synthetic-key", workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func snapshot() Ask {
	return Ask{
		ID: askID, WorkspaceID: workspaceID, Address: "hev-4", OriginAddress: "hev-1",
		Segments: []int32{4}, RequestRef: requestID, ScopeRef: scopeID, Revision: 2,
		ScopeRevision: 1, Links: []Link{},
	}
}

func assertWire(t *testing.T, r *http.Request, query bool) {
	t.Helper()
	path := "/v2/namespaces/work-" + string(workspaceID) + "/work-identity"
	if query {
		path += "/query"
	}
	if r.URL.Path != path || r.Method != http.MethodPost ||
		r.Header.Get("Authorization") != "Bearer synthetic-key" ||
		r.Header.Get("Content-Type") != "application/json" {
		t.Errorf("unexpected request method, path or headers: %s %s", r.Method, r.URL.Path)
	}
}

func TestWorkIdentityResolveAndPagination(t *testing.T) {
	var calls atomic.Int32
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		assertWire(t, r, true)
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		var want map[string]any
		var reply any
		cursor := int64(8)
		switch calls.Add(1) {
		case 1:
			want = map[string]any{"op": "resolve", "address": "hev-1"}
			reply = snapshot()
		case 2:
			want = map[string]any{"op": "get", "id": string(askID)}
			reply = snapshot()
		case 3:
			want = map[string]any{"op": "list", "after": float64(7), "limit": float64(1)}
			reply = Page[Ask]{Items: []Ask{snapshot()}, NextAfter: &cursor, HasMore: true}
		case 4:
			want = map[string]any{"op": "list", "after": float64(8)}
			reply = Page[Ask]{Items: []Ask{}}
		default:
			t.Error("unexpected request")
		}
		if !reflect.DeepEqual(request, want) {
			t.Errorf("request = %v, want %v", request, want)
		}
		_ = json.NewEncoder(w).Encode(reply)
	})
	ctx := context.Background()
	resolved, err := c.Resolve(ctx, "hev-1")
	if err != nil || resolved.ID != askID || resolved.Address != "hev-4" {
		t.Fatalf("resolve = %+v, %v", resolved, err)
	}
	if _, err := c.Get(ctx, resolved.ID); err != nil {
		t.Fatal(err)
	}
	page, err := c.List(ctx, PageRequest{After: 7, Limit: 1})
	if err != nil || !page.HasMore || page.NextAfter == nil || *page.NextAfter != 8 {
		t.Fatalf("first page = %+v, %v", page, err)
	}
	page, err = c.List(ctx, PageRequest{After: *page.NextAfter})
	if err != nil || page.HasMore || page.NextAfter != nil || len(page.Items) != 0 {
		t.Fatalf("empty page = %+v, %v", page, err)
	}
	if _, err := c.History(ctx, PageRequest{Limit: 101}); err == nil {
		t.Fatal("oversized page accepted")
	}
	if _, err := c.Get(ctx, "local-run-name"); err == nil {
		t.Fatal("local run name accepted as a UUID")
	}
	if calls.Load() != 4 {
		t.Fatalf("sent %d requests, want 4", calls.Load())
	}
}

func TestWorkIdentityCreateReplayAfterLostReply(t *testing.T) {
	var calls atomic.Int32
	expected := `{"op":"create","idempotency_key":"50000000-0000-4000-8000-000000000001","request_ref":"30000000-0000-4000-8000-000000000001","scope_ref":"40000000-0000-4000-8000-000000000001"}`
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		assertWire(t, r, false)
		body, err := io.ReadAll(r.Body)
		if err != nil || string(body) != expected {
			t.Errorf("create body = %s, %v", body, err)
		}
		if calls.Add(1) == 1 {
			// Simulate committing and losing the acknowledgement.
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			_ = conn.Close()
			return
		}
		_ = json.NewEncoder(w).Encode(snapshot())
	})
	payload := CreateRequest{IdempotencyKey: createKey, RequestRef: requestID, ScopeRef: scopeID}
	ctx := context.Background()
	_, err := c.Create(ctx, payload)
	var failure *Error
	if !errors.As(err, &failure) || !failure.UnknownCommit || !failure.Retryable || calls.Load() != 1 {
		t.Fatalf("first create = %v, requests = %d", err, calls.Load())
	}
	ask, err := c.Create(ctx, payload)
	if err != nil || ask.ID != askID || calls.Load() != 2 {
		t.Fatalf("create replay = %+v, %v, requests = %d", ask, err, calls.Load())
	}
}

func TestWorkIdentityDeadlineAndOutage(t *testing.T) {
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		// Hold the response open to exercise cancellation while reading the body.
		if r.URL.Path == "/v2/namespaces/work-"+string(workspaceID)+"/work-identity" {
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"error":"unavailable","retryable":true}`)
	})
	_, err := c.Get(context.Background(), askID)
	var failure *Error
	if !errors.As(err, &failure) || failure.UnknownCommit || !failure.Retryable || failure.Status != 503 {
		t.Fatalf("read outage = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err = c.Revise(ctx, ReviseRequest{Target: Target{ID: askID, ExpectedRevision: 2}, ScopeRef: scopeID})
	if !errors.Is(err, context.DeadlineExceeded) || !errors.As(err, &failure) || !failure.UnknownCommit || !failure.Retryable {
		t.Fatalf("write deadline = %v", err)
	}
}

func TestWorkIdentityRejectsUnconfirmedAcceptance(t *testing.T) {
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		ask := snapshot()
		ask.Revision = 3
		// A successful HTTP response alone cannot confirm acceptance.
		_ = json.NewEncoder(w).Encode(ask)
	})
	_, err := c.Accept(context.Background(), AcceptRequest{
		Target:     Target{ID: askID, ExpectedRevision: 2},
		Acceptance: Acceptance{ScopeRevision: 1, EvidenceRef: scopeID},
	})
	var failure *Error
	if !errors.As(err, &failure) || !failure.UnknownCommit {
		t.Fatalf("unconfirmed acceptance = %v", err)
	}
}
