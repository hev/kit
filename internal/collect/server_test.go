package collect

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/redact"
)

const testKey = "svc-key-7f3a9c0e1d5b"

// githubToken is a credential gitleaks' default rules recognize.
const githubToken = "ghp_" + "AbCdEfGhIjKlMnOpQrStUvWxYz0123456789"

// leakyIndex ignores owner and tenant on reads and returns every turn it
// holds, so a test against it shows the server's own isolation, not the
// store's.
type leakyIndex struct {
	mu      sync.Mutex
	turns   map[string]Turn
	upserts [][]Turn
}

func newLeakyIndex() *leakyIndex { return &leakyIndex{turns: map[string]Turn{}} }

func (l *leakyIndex) Upsert(_ context.Context, turns []Turn) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.upserts = append(l.upserts, append([]Turn(nil), turns...))
	for _, t := range turns {
		l.turns[t.DocID()] = t
	}
	return nil
}

func (l *leakyIndex) all() []Turn {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []Turn
	for _, t := range l.turns {
		out = append(out, t)
	}
	return out
}

func (l *leakyIndex) Search(_ context.Context, _, _, _ string, _ time.Time, _ int) ([]TurnHit, error) {
	var hits []TurnHit
	for i, t := range l.all() {
		hits = append(hits, TurnHit{Turn: t, Score: 1 / float64(i+1)})
	}
	return hits, nil
}

func (l *leakyIndex) Conversation(_ context.Context, _, _, _ string) ([]Turn, error) {
	return l.all(), nil
}

type fakeBoards struct {
	posts []BoardPost
}

func (f *fakeBoards) Post(_ context.Context, board string, p BoardPost) (BoardPosted, error) {
	if board == "Bad Board" {
		return BoardPosted{}, fmt.Errorf("%w: no board", ErrBadBoardRequest)
	}
	f.posts = append(f.posts, p)
	return BoardPosted{ID: fmt.Sprint(len(f.posts)), Created: "2026-11-17T00:00:00Z"}, nil
}

func (f *fakeBoards) Search(_ context.Context, _ string, q BoardQuery) ([]BoardHit, error) {
	var out []BoardHit
	for i, p := range f.posts {
		if hasAll(p.Tags, cleanTags(q.Tags)) {
			out = append(out, BoardHit{ID: fmt.Sprint(i + 1), Title: p.Title, Body: p.Body, Author: p.Author, AuthorName: p.AuthorName, Tags: p.Tags, Score: 1})
		}
	}
	return out, nil
}

type memSink struct {
	mu   sync.Mutex
	envs []*daemon.TraceEnvelope
	// byNS is the namespace each envelope was written under.
	byNS map[string][]*daemon.TraceEnvelope
}

func (m *memSink) Write(ns string, envs []*daemon.TraceEnvelope) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.byNS == nil {
		m.byNS = map[string][]*daemon.TraceEnvelope{}
	}
	m.envs = append(m.envs, envs...)
	m.byNS[ns] = append(m.byNS[ns], envs...)
	return nil
}

type harness struct {
	t    *testing.T
	srv  *Server
	http *httptest.Server
	// index is story-a's; indexes holds every allowed namespace's.
	index   *leakyIndex
	indexes map[string]*leakyIndex
	boards  *fakeBoards
	sink    *memSink
	logs    *bytes.Buffer
	bodies  []string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	scrubber, err := redact.New(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{t: t, indexes: map[string]*leakyIndex{nsA: newLeakyIndex(), nsB: newLeakyIndex()}, boards: &fakeBoards{}, sink: &memSink{}, logs: &bytes.Buffer{}}
	h.index = h.indexes[nsA]
	indexes := map[string]TurnIndex{}
	for ns, idx := range h.indexes {
		indexes[ns] = idx
	}
	logger := slog.New(slog.NewTextHandler(h.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	h.srv, err = New(Config{Key: testKey, Indexes: indexes, Boards: h.boards, Sink: h.sink, Scrubber: scrubber, Host: "test-host", Logger: logger})
	if err != nil {
		t.Fatal(err)
	}
	h.http = httptest.NewServer(h.srv.Handler())
	t.Cleanup(h.http.Close)
	return h
}

func (h *harness) do(method, path, key string, body any) (int, string) {
	h.t.Helper()
	var rd io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		rd = strings.NewReader(b)
	case []byte:
		rd = bytes.NewReader(b)
	default:
		raw, _ := json.Marshal(b)
		rd = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, h.http.URL+path, rd)
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	h.bodies = append(h.bodies, string(raw))
	return resp.StatusCode, string(raw)
}

// ---- AI SDK-shaped OTLP/JSON ----------------------------------------------

func kvs(pairs ...any) []map[string]any {
	var out []map[string]any
	for i := 0; i+1 < len(pairs); i += 2 {
		var v map[string]any
		switch x := pairs[i+1].(type) {
		case string:
			v = map[string]any{"stringValue": x}
		case int:
			v = map[string]any{"intValue": fmt.Sprint(x)}
		case bool:
			v = map[string]any{"boolValue": x}
		}
		out = append(out, map[string]any{"key": pairs[i], "value": v})
	}
	return out
}

// The harness's collector allows nsA and nsB.
const (
	nsA = "story-a"
	nsB = "story-b"
	// noNamespace sends a turn whose resource has no hev.namespace.
	noNamespace = "-"
)

type turnSpec struct {
	// ns is the resource's hev.namespace; empty means nsA.
	ns                                      string
	user, chat, turn, prompt, answer, trace string
	start                                   time.Time
	toolOK                                  bool
}

func meta(s turnSpec) []any {
	return []any{
		"ai.telemetry.metadata.userId", s.user, "ai.telemetry.metadata.chatId", s.chat,
		"ai.telemetry.metadata.turnId", s.turn, "ai.telemetry.metadata.tenant", "story",
		"ai.telemetry.functionId", "ki-agent.run",
	}
}

func span(s turnSpec, id, parent, name string, offset time.Duration, attrs []any, status map[string]any) map[string]any {
	start := s.start.Add(offset)
	sp := map[string]any{
		"traceId": s.trace, "spanId": id, "name": name,
		"startTimeUnixNano": fmt.Sprint(start.UnixNano()), "endTimeUnixNano": fmt.Sprint(start.Add(time.Millisecond).UnixNano()),
		"attributes": kvs(attrs...), "status": status,
	}
	if parent != "" {
		sp["parentSpanId"] = parent
	}
	return sp
}

// aiSDKBatches is one turn the way the AI SDK exports it: the tool call ends
// (and is exported) first and carries no telemetry metadata; the step and
// the run follow in a later export.
func aiSDKBatches(s turnSpec) [][]byte {
	prompt, _ := json.Marshal(map[string]any{"messages": []any{
		map[string]any{"role": "system", "content": "You are the story agent."},
		map[string]any{"role": "user", "content": "earlier question"},
		map[string]any{"role": "assistant", "content": "earlier answer"},
		map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": s.prompt}}},
	}})
	stepMsgs, _ := json.Marshal([]any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": s.prompt}}}})
	status := map[string]any{"code": 1}
	if !s.toolOK {
		status = map[string]any{"code": 2, "message": "tool failed"}
	}
	root := "a1a1a1a1a1a1a1a1"
	tool := span(s, "c3c3c3c3c3c3c3c3", root, "ai.toolCall", 2*time.Millisecond, []any{
		"ai.operationId", "ai.toolCall", "ai.toolCall.name", "searchStories", "ai.toolCall.id", "call-" + s.turn,
		"ai.toolCall.args", `{"q":"lisbon"}`, "ai.toolCall.result", `{"n":3}`,
	}, status)
	step := span(s, "b2b2b2b2b2b2b2b2", root, "ai.streamText.doStream", time.Millisecond, append(meta(s),
		"ai.operationId", "ai.streamText.doStream", "ai.prompt.messages", string(stepMsgs), "ai.response.text", s.answer,
	), map[string]any{})
	run := span(s, root, "", "ai.streamText", 0, append(meta(s),
		"ai.operationId", "ai.streamText", "ai.prompt", string(prompt), "ai.response.text", s.answer,
	), map[string]any{})
	resource := kvs("service.name", "ki-agent", "hev.namespace", firstNonEmpty(s.ns, nsA))
	if s.ns == noNamespace {
		resource = kvs("service.name", "ki-agent")
	}
	wrap := func(spans ...map[string]any) []byte {
		raw, _ := json.Marshal(map[string]any{"resourceSpans": []any{map[string]any{
			"resource":   map[string]any{"attributes": resource},
			"scopeSpans": []any{map[string]any{"scope": map[string]any{"name": "ai"}, "spans": spans}},
		}}})
		return raw
	}
	return [][]byte{wrap(tool), wrap(step), wrap(run)}
}

func (h *harness) export(s turnSpec) {
	h.t.Helper()
	for _, b := range aiSDKBatches(s) {
		if code, body := h.do("POST", "/v1/traces", testKey, b); code != 200 {
			h.t.Fatalf("export: %d %s", code, body)
		}
	}
}

var (
	userA = turnSpec{user: "101", chat: "chat-shared", turn: "ki-run-1", prompt: "Open a heist story in Lisbon", answer: "The tram bell rang as Ines palmed the key.", trace: "0102030405060708090a0b0c0d0e0f10", toolOK: true}
	userB = turnSpec{user: "202", chat: "chat-shared", turn: "ki-run-2", prompt: "Write a noir opening in Chicago", answer: "Rain hit the window like a debt collector.", trace: "1112131415161718191a1b1c1d1e1f20", toolOK: false}
)

func init() {
	userA.start = time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)
	userB.start = time.Date(2026, 11, 17, 11, 0, 0, 0, time.UTC)
}

// ---- tests -----------------------------------------------------------------

func TestNewRefusesWithoutKey(t *testing.T) {
	_, err := New(Config{Indexes: map[string]TurnIndex{nsA: newLeakyIndex()}, Boards: &fakeBoards{}, Sink: &memSink{}})
	if err != ErrNoKey {
		t.Fatalf("New without key = %v, want ErrNoKey", err)
	}
}

func TestNewRefusesWithoutNamespaces(t *testing.T) {
	for _, idx := range []map[string]TurnIndex{nil, {}} {
		if _, err := New(Config{Key: testKey, Indexes: idx, Boards: &fakeBoards{}, Sink: &memSink{}}); err != ErrNoNamespaces {
			t.Fatalf("New with indexes %v = %v, want ErrNoNamespaces", idx, err)
		}
	}
	if _, err := New(Config{Key: testKey, Indexes: map[string]TurnIndex{"Story_A": newLeakyIndex()}, Boards: &fakeBoards{}, Sink: &memSink{}}); err == nil {
		t.Fatal("New took an invalid namespace")
	}
}

func TestHealthzNeedsNoKey(t *testing.T) {
	h := newHarness(t)
	code, body := h.do("GET", "/healthz", "", nil)
	if code != 200 || strings.TrimSpace(body) != `{"ok":true}` {
		t.Fatalf("healthz = %d %s", code, body)
	}
}

func TestEveryRouteNeedsTheKey(t *testing.T) {
	h := newHarness(t)
	routes := []struct{ method, path, body string }{
		{"POST", "/v1/traces", `{"resourceSpans":[]}`},
		{"POST", "/v1/app/traces/search", `{"namespace":"story-a","owner":"101","tenant":"story","query":"x"}`},
		{"GET", "/v1/app/traces/conversations/chat-shared?namespace=story-a&owner=101&tenant=story", ""},
		{"POST", "/v1/app/boards/story-memory/posts", `{"author":"101","title":"t","body":"b","tags":[]}`},
		{"POST", "/v1/app/boards/story-memory/search", `{"query":"x"}`},
	}
	for _, r := range routes {
		for _, key := range []string{"", "wrong-key", testKey + "x", strings.ToUpper(testKey)} {
			var body any
			if r.body != "" {
				body = r.body
			}
			code, resp := h.do(r.method, r.path, key, body)
			if code != http.StatusUnauthorized {
				t.Errorf("%s %s with key %q = %d %s, want 401", r.method, r.path, key, code, resp)
			}
		}
	}
	// A scheme other than Bearer is not the key either.
	req, _ := http.NewRequest("POST", h.http.URL+"/v1/app/traces/search", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Basic "+testKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("Basic auth = %d, want 401", resp.StatusCode)
	}
	if len(h.boards.posts) != 0 || len(h.index.all()) != 0 {
		t.Fatal("an unauthorized request reached a store")
	}
}

func TestSearchRequiresOwnerAndTenant(t *testing.T) {
	h := newHarness(t)
	h.export(userA)
	for _, body := range []string{
		`{"namespace":"story-a","tenant":"story","query":"heist"}`,
		`{"namespace":"story-a","owner":"101","query":"heist"}`,
		`{"namespace":"story-a","query":"heist"}`,
		`{"namespace":"story-a","owner":"","tenant":"story","query":"heist"}`,
	} {
		if code, resp := h.do("POST", "/v1/app/traces/search", testKey, body); code != 400 {
			t.Errorf("search %s = %d %s, want 400", body, code, resp)
		}
	}
	for _, q := range []string{"&tenant=story", "&owner=101", ""} {
		if code, resp := h.do("GET", "/v1/app/traces/conversations/chat-shared?namespace=story-a"+q, testKey, nil); code != 400 {
			t.Errorf("conversation %s = %d %s, want 400", q, code, resp)
		}
	}
}

type searchResp struct {
	Hits []struct {
		ChatID        string  `json:"chatId"`
		TurnID        string  `json:"turnId"`
		TS            string  `json:"ts"`
		Prompt        string  `json:"prompt"`
		AnswerSnippet string  `json:"answerSnippet"`
		Score         float64 `json:"score"`
	} `json:"hits"`
}

type convResp struct {
	ChatID string `json:"chatId"`
	Turns  []struct {
		TurnID string `json:"turnId"`
		TS     string `json:"ts"`
		Prompt string `json:"prompt"`
		Answer string `json:"answer"`
		Tools  []Tool `json:"tools"`
	} `json:"turns"`
}

// The index here returns every owner's turns; the server must still answer
// each owner with only their own, in search and in conversation fetch, even
// when two owners' chats share an id.
func TestOwnerNeverSeesAnotherOwnersTurns(t *testing.T) {
	h := newHarness(t)
	h.export(userA)
	h.export(userB)
	if n := len(h.index.all()); n != 2 {
		t.Fatalf("indexed %d turns, want 2", n)
	}
	for _, c := range []struct {
		owner, want, not string
	}{{"101", "ki-run-1", "ki-run-2"}, {"202", "ki-run-2", "ki-run-1"}} {
		code, body := h.do("POST", "/v1/app/traces/search", testKey, map[string]any{"namespace": "story-a", "owner": c.owner, "tenant": "story", "query": "opening"})
		if code != 200 {
			t.Fatalf("search = %d %s", code, body)
		}
		var sr searchResp
		if err := json.Unmarshal([]byte(body), &sr); err != nil {
			t.Fatal(err)
		}
		if len(sr.Hits) != 1 || sr.Hits[0].TurnID != c.want {
			t.Fatalf("owner %s search hits = %+v, want only %s", c.owner, sr.Hits, c.want)
		}
		if strings.Contains(body, c.not) {
			t.Fatalf("owner %s saw %s: %s", c.owner, c.not, body)
		}

		code, body = h.do("GET", "/v1/app/traces/conversations/chat-shared?namespace=story-a&owner="+c.owner+"&tenant=story", testKey, nil)
		if code != 200 {
			t.Fatalf("conversation = %d %s", code, body)
		}
		var cr convResp
		if err := json.Unmarshal([]byte(body), &cr); err != nil {
			t.Fatal(err)
		}
		if len(cr.Turns) != 1 || cr.Turns[0].TurnID != c.want || strings.Contains(body, c.not) {
			t.Fatalf("owner %s conversation = %s, want only %s", c.owner, body, c.want)
		}
	}
	// Another tenant, or an owner with no turns in the chat, gets a 404.
	for _, q := range []string{"&owner=303&tenant=story", "&owner=101&tenant=other"} {
		if code, body := h.do("GET", "/v1/app/traces/conversations/chat-shared?namespace=story-a"+q, testKey, nil); code != 404 {
			t.Fatalf("conversation %s = %d %s, want 404", q, code, body)
		}
	}
	code, body := h.do("POST", "/v1/app/traces/search", testKey, map[string]any{"namespace": "story-a", "owner": "101", "tenant": "other", "query": "opening"})
	if code != 200 || strings.Contains(body, "ki-run") {
		t.Fatalf("other tenant search = %d %s", code, body)
	}
}

func TestConversationInOrder(t *testing.T) {
	h := newHarness(t)
	second := userA
	second.turn, second.trace, second.prompt = "ki-run-3", "2122232425262728292a2b2c2d2e2f30", "Now the second scene"
	second.start = userA.start.Add(time.Hour)
	h.export(second)
	h.export(userA)
	_, body := h.do("GET", "/v1/app/traces/conversations/chat-shared?namespace=story-a&owner=101&tenant=story", testKey, nil)
	var cr convResp
	if err := json.Unmarshal([]byte(body), &cr); err != nil {
		t.Fatal(err)
	}
	if cr.ChatID != "chat-shared" || len(cr.Turns) != 2 || cr.Turns[0].TurnID != "ki-run-1" || cr.Turns[1].TurnID != "ki-run-3" {
		t.Fatalf("conversation = %s", body)
	}
	if len(cr.Turns[0].Tools) != 1 || cr.Turns[0].Tools[0] != (Tool{Name: "searchStories", OK: true}) {
		t.Fatalf("tools = %+v", cr.Turns[0].Tools)
	}
}

// Spans for one turnId arrive in three exports, the first without telemetry
// metadata; they become one turn document with the whole turn in it.
func TestSpansOfOneTurnCollapseIntoOneDocument(t *testing.T) {
	h := newHarness(t)
	h.export(userB)
	turns := h.index.all()
	if len(turns) != 1 {
		t.Fatalf("turn documents = %d, want 1: %+v", len(turns), turns)
	}
	got := turns[0]
	want := Turn{Owner: "202", Tenant: "story", ChatID: "chat-shared", TurnID: "ki-run-2", TS: userB.start, Prompt: userB.prompt, Answer: userB.answer,
		Tools: []Tool{{Name: "searchStories", OK: false, Error: "tool failed"}}, Service: "ki-agent"}
	gotJSON, _ := json.Marshal(got)
	wantJSON, _ := json.Marshal(want)
	if string(gotJSON) != string(wantJSON) || got.Service != want.Service {
		t.Fatalf("turn =\n %s\nwant\n %s", gotJSON, wantJSON)
	}
	// Every upsert names the same row: the doc id is the turn's.
	ids := map[string]bool{}
	for _, batch := range h.index.upserts {
		for _, t := range batch {
			ids[t.DocID()] = true
		}
	}
	if len(ids) != 1 {
		t.Fatalf("upserts wrote %d distinct rows, want 1", len(ids))
	}
	// Every span is one envelope, all three attributed to the turn.
	if len(h.sink.envs) != 3 {
		t.Fatalf("envelopes = %d, want 3", len(h.sink.envs))
	}
	for _, e := range h.sink.envs {
		if e.TurnID != "ki-run-2" || e.SessionID != "chat-shared" || e.Harness != "app:ki-agent" || e.IngestSource != "otlp_http" || e.SignalType != "trace" {
			t.Fatalf("envelope = %+v", e)
		}
		var res map[string]any
		if err := json.Unmarshal([]byte(*e.Resource), &res); err != nil {
			t.Fatal(err)
		}
		if res["end_user"] != "202" || res["tenant"] != "story" || res["service.name"] != "ki-agent" {
			t.Fatalf("resource = %v", res)
		}
	}
}

func TestRedactionAppliedBeforeIndexing(t *testing.T) {
	h := newHarness(t)
	s := userA
	s.prompt = "use my token " + githubToken + " to fetch the draft"
	s.answer = "I used " + githubToken + " and fetched it."
	h.export(s)
	turns := h.index.all()
	if len(turns) != 1 {
		t.Fatalf("turns = %d", len(turns))
	}
	for _, field := range []string{turns[0].Prompt, turns[0].Answer, turnText(turns[0], 0)} {
		if strings.Contains(field, githubToken) || strings.Contains(field, "ghp_") {
			t.Fatalf("indexed text holds the secret: %q", field)
		}
		if !strings.Contains(field, "[REDACTED:") {
			t.Fatalf("indexed text has no redaction marker: %q", field)
		}
	}
	for _, e := range h.sink.envs {
		raw, _ := json.Marshal(e)
		if strings.Contains(string(raw), githubToken) {
			t.Fatalf("envelope holds the secret: %s", raw)
		}
		if e.RedactionPolicy != RedactionPolicy || e.RedactionApplied == nil || e.RedactionCount == nil {
			t.Fatalf("envelope redaction fields = %q %v %v", e.RedactionPolicy, e.RedactionApplied, e.RedactionCount)
		}
		if e.EventKind != "ai.toolCall" && (!*e.RedactionApplied || *e.RedactionCount == 0) {
			t.Fatalf("%s: redaction not recorded", e.EventKind)
		}
	}
	// Memories pass the same policy.
	h.do("POST", "/v1/app/boards/story-memory/posts", testKey, map[string]any{"author": "101", "title": "key " + githubToken, "body": "remember " + githubToken, "tags": []string{"job:123"}})
	if len(h.boards.posts) != 1 || strings.Contains(h.boards.posts[0].Body+h.boards.posts[0].Title, githubToken) {
		t.Fatalf("board post holds the secret: %+v", h.boards.posts)
	}
}

func TestKeyNeverInLogsOrResponses(t *testing.T) {
	h := newHarness(t)
	h.export(userA)
	h.do("POST", "/v1/app/traces/search", testKey, map[string]any{"namespace": "story-a", "owner": "101", "tenant": "story", "query": "heist"})
	h.do("POST", "/v1/app/traces/search", testKey, map[string]any{"namespace": "story-a", "tenant": "story"})
	h.do("POST", "/v1/app/traces/search", "wrong-"+testKey, map[string]any{"namespace": "story-a", "owner": "101", "tenant": "story", "query": "heist"})
	h.do("GET", "/v1/app/traces/conversations/chat-shared?namespace=story-a&owner=101&tenant=story", testKey, nil)
	h.do("GET", "/v1/app/traces/conversations/nope?namespace=story-a&owner=101&tenant=story", testKey, nil)
	h.do("POST", "/v1/app/boards/story-memory/posts", testKey, map[string]any{"author": "101", "title": "t", "body": "b", "tags": []string{"job:123"}})
	h.do("POST", "/v1/app/boards/Bad Board/posts", testKey, map[string]any{"author": "101", "title": "t", "body": "b"})
	h.do("POST", "/v1/app/boards/story-memory/search", testKey, map[string]any{"tags": []string{"job:123"}})
	h.do("POST", "/v1/app/boards/story-memory/search", testKey, "not json")
	h.do("POST", "/v1/traces", testKey, "{broken")
	if h.logs.Len() == 0 {
		t.Fatal("captured no logs; the test would prove nothing")
	}
	if strings.Contains(h.logs.String(), testKey) {
		t.Fatalf("key in logs:\n%s", h.logs.String())
	}
	for _, b := range h.bodies {
		if strings.Contains(b, testKey) {
			t.Fatalf("key in response body: %s", b)
		}
	}
	for _, e := range h.sink.envs {
		raw, _ := json.Marshal(e)
		if strings.Contains(string(raw), testKey) {
			t.Fatalf("key in envelope: %s", raw)
		}
	}
}

func TestBoardRoutes(t *testing.T) {
	h := newHarness(t)
	if code, body := h.do("POST", "/v1/app/boards/story-memory/posts", testKey, map[string]any{"title": "t", "body": "b"}); code != 400 {
		t.Fatalf("post without author = %d %s", code, body)
	}
	if code, body := h.do("POST", "/v1/app/boards/Bad Board/posts", testKey, map[string]any{"author": "a", "title": "t", "body": "b"}); code != 400 {
		t.Fatalf("post to a bad board = %d %s", code, body)
	}
	code, body := h.do("POST", "/v1/app/boards/story-memory/posts", testKey, map[string]any{"author": "101", "authorName": "Ana", "title": "Second person", "body": "Ana wants second person.", "tags": []string{"job:123", "style"}})
	if code != 200 {
		t.Fatalf("post = %d %s", code, body)
	}
	var posted BoardPosted
	if err := json.Unmarshal([]byte(body), &posted); err != nil || posted.ID == "" || posted.Created == "" {
		t.Fatalf("post answer = %s (%v)", body, err)
	}
	if code, body := h.do("POST", "/v1/app/boards/story-memory/search", testKey, map[string]any{}); code != 400 {
		t.Fatalf("search with neither query nor tags = %d %s", code, body)
	}
	_, body = h.do("POST", "/v1/app/boards/story-memory/search", testKey, map[string]any{"tags": []string{"job:123"}})
	if !strings.Contains(body, `"authorName":"Ana"`) || !strings.Contains(body, `"id":"1"`) {
		t.Fatalf("search = %s", body)
	}
	_, body = h.do("POST", "/v1/app/boards/story-memory/search", testKey, map[string]any{"tags": []string{"job:999"}})
	if strings.TrimSpace(body) != `{"hits":[]}` {
		t.Fatalf("search by another tag = %s", body)
	}
}

func TestSearchLimitAndSince(t *testing.T) {
	if clampLimit(0) != 10 || clampLimit(50) != MaxLimit || clampLimit(5) != 5 {
		t.Fatal("clampLimit")
	}
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	if got, _ := parseSince("7d", now); !got.Equal(now.Add(-7 * 24 * time.Hour)) {
		t.Fatalf("7d = %v", got)
	}
	if got, _ := parseSince("2026-11-01T00:00:00Z", now); !got.Equal(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("rfc3339 = %v", got)
	}
	if _, err := parseSince("yesterday", now); err == nil {
		t.Fatal("bad since accepted")
	}
	h := newHarness(t)
	h.export(userA)
	_, body := h.do("POST", "/v1/app/traces/search", testKey, map[string]any{"namespace": "story-a", "owner": "101", "tenant": "story", "query": "heist", "since": "2026-11-17T10:30:00Z"})
	if strings.Contains(body, "ki-run-1") {
		t.Fatalf("since did not exclude an older turn: %s", body)
	}
}

func TestOrphanSpansAreStillWritten(t *testing.T) {
	a := NewAggregator()
	now := time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)
	a.now = func() time.Time { return now }
	lone := Span{TraceID: "ff", SpanID: "01", Name: "ai.toolCall", Attrs: map[string]any{"ai.toolCall.name": "x"}}
	attributed, turns := a.Add([]Span{lone})
	if len(attributed) != 0 || len(turns) != 0 {
		t.Fatalf("an unattributed span was attributed: %v %v", attributed, turns)
	}
	if got := a.Expire(); len(got) != 0 {
		t.Fatalf("expired early: %v", got)
	}
	now = now.Add(a.Hold)
	if got := a.Expire(); len(got) != 1 || got[0].SpanID != "01" {
		t.Fatalf("expired = %v", got)
	}
}
