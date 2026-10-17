package outboxrelay

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeLayer struct {
	docs map[string]Document
	puts int
	fail bool
	lag  bool
}

func (f *fakeLayer) Put(_ context.Context, d Document) error {
	f.puts++
	f.docs[d.ID] = d
	if f.fail {
		return errors.New("lost ACK")
	}
	return nil
}
func (f *fakeLayer) Inspect(_ context.Context, id string) (*Document, error) {
	if f.lag {
		return nil, nil
	}
	d, ok := f.docs[id]
	if !ok {
		return nil, nil
	}
	return &d, nil
}

type memoryStore struct {
	queue        []Delivery
	sent         map[int64]bool
	claimed      map[int64]bool
	failComplete bool
}

func (m *memoryStore) Next(_ context.Context) (*Delivery, error) {
	for _, d := range m.queue {
		if !m.sent[d.OutboxID] {
			d.Claimed = !m.claimed[d.OutboxID]
			m.claimed[d.OutboxID] = true
			return &d, nil
		}
	}
	return nil, nil
}
func (m *memoryStore) Complete(_ context.Context, d Delivery) error {
	if m.failComplete {
		return errors.New("commit lost")
	}
	m.sent[d.OutboxID] = true
	return nil
}
func fixture(t *testing.T) (*memoryStore, *fakeLayer) {
	t.Helper()
	m := &memoryStore{sent: map[int64]bool{}, claimed: map[int64]bool{}}
	for i, key := range []string{"one", "two", "three"} {
		d, e := NewDocument("layer", key, "order", json.RawMessage(`{"x":1}`))
		if e != nil {
			t.Fatal(e)
		}
		m.queue = append(m.queue, Delivery{OutboxID: int64(i + 1), Document: d})
	}
	return m, &fakeLayer{docs: map[string]Document{}}
}
func TestPartialFailureRestartReconcilesWithoutResend(t *testing.T) {
	ctx := context.Background()
	m, f := fixture(t)
	r := Relay{m, f}
	if _, e := r.Step(ctx); e != nil {
		t.Fatal(e)
	}
	f.fail = true
	if _, e := r.Step(ctx); !errors.Is(e, ErrUncertain) {
		t.Fatal(e)
	}
	f.fail = false
	f.lag = true
	r = Relay{m, f} // restart has no in-memory delivery state
	for i := 0; i < 3; i++ {
		if _, e := r.Step(ctx); !errors.Is(e, ErrUncertain) {
			t.Fatal(e)
		}
	}
	if f.puts != 2 {
		t.Fatal("blind resend", f.puts)
	}
	f.lag = false
	if _, e := r.Step(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e := r.Step(ctx); e != nil {
		t.Fatal(e)
	}
	if work, e := r.Step(ctx); work || e != nil {
		t.Fatal(work, e)
	}
	if len(f.docs) != 3 || f.puts != 3 || len(m.sent) != 3 {
		t.Fatal(f, m)
	}
}
func TestLostCompletionAndConflictingObservation(t *testing.T) {
	ctx := context.Background()
	m, f := fixture(t)
	m.failComplete = true
	r := Relay{m, f}
	if _, e := r.Step(ctx); e == nil {
		t.Fatal("expected completion failure")
	}
	m.failComplete = false
	d := m.queue[0].Document
	bad := d
	bad.Payload = json.RawMessage(`{"x":2}`)
	f.docs[d.ID] = bad
	if _, e := r.Step(ctx); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
	f.docs[d.ID] = d
	if _, e := r.Step(ctx); e != nil {
		t.Fatal(e)
	}
	if f.puts != 1 {
		t.Fatal("replayed after lost commit", f.puts)
	}
}
func TestStableIdentityAndJSON(t *testing.T) {
	a, e := NewDocument("a", "b/c", "o", json.RawMessage(`{ "b":2,"a":9007199254740993}`))
	if e != nil {
		t.Fatal(e)
	}
	b, _ := NewDocument("a", "b/c", "o", json.RawMessage(`{"a":9007199254740993,"b":2}`))
	if a.ID != b.ID || string(a.Payload) != string(b.Payload) {
		t.Fatal(a, b)
	}
	c, _ := NewDocument("a/b", "c", "o", json.RawMessage(`{}`))
	if c.ID == a.ID {
		t.Fatal("ambiguous tuple")
	}
}
func TestHTTPWireAndAmbiguousAcknowledgement(t *testing.T) {
	ctx := context.Background()
	var stored wireDocument
	writes := 0
	ack := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer synthetic" {
			t.Error("auth")
		}
		if r.URL.Path == "/v2/namespaces/test/query" {
			json.NewEncoder(w).Encode(map[string]any{"rows": []wireDocument{stored}})
			return
		}
		if r.URL.Path != "/v2/namespaces/test" {
			t.Error(r.URL.Path)
		}
		var body struct {
			Rows []wireDocument `json:"upsert_rows"`
		}
		if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
			t.Fatal(e)
		}
		if len(body.Rows) != 1 {
			t.Fatal(body)
		}
		writes++
		stored = body.Rows[0]
		if !ack {
			w.Write([]byte(`{}`))
			return
		}
		w.Write([]byte(`{"rows_upserted":1}`))
	}))
	defer server.Close()
	h := HTTP{Endpoint: server.URL, Namespace: "test", APIKey: "synthetic", Client: server.Client()}
	d, _ := NewDocument("layer", "key", "o", json.RawMessage(`{"x":1}`))
	if e := h.Put(ctx, d); e != nil {
		t.Fatal(e)
	}
	got, e := h.Inspect(ctx, d.ID)
	if e != nil || string(got.Payload) != string(d.Payload) || got.ID != d.ID {
		t.Fatal(got, e)
	}
	ack = false
	if e := h.Put(ctx, d); e == nil {
		t.Fatal("unacknowledged write accepted")
	}
	if writes != 2 {
		t.Fatal(writes)
	}
}
