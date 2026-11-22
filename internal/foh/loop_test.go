package foh

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// scripted is a Provider that answers from a script and records requests.
type scripted struct {
	mu       sync.Mutex
	replies  []ChatResult
	requests []ChatRequest
}

func (s *scripted) Stream(_ context.Context, req ChatRequest, onDelta func(Delta)) (ChatResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, req)
	if len(s.replies) == 0 {
		return ChatResult{}, fmt.Errorf("script exhausted")
	}
	r := s.replies[0]
	s.replies = s.replies[1:]
	if r.Content != "" && onDelta != nil {
		onDelta(Delta{Text: r.Content})
	}
	r.Model = req.Model
	return r, nil
}

func (s *scripted) ID() string { return "scripted" }

func (s *scripted) Models(context.Context, bool) ([]Model, CatalogSource, error) {
	return nil, SourceStarter, nil
}

func call(id, name, args string) ToolCall {
	return ToolCall{ID: id, Type: "function", Function: FunctionCall{Name: name, Arguments: args}}
}

func readSession(t *testing.T, path string) []Record {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []Record
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		var r Record
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			t.Fatalf("bad jsonl line %q: %v", sc.Text(), err)
		}
		out = append(out, r)
	}
	return out
}

func noHev(context.Context, []string, string) ([]byte, error) {
	return nil, fmt.Errorf("hev not available in tests")
}

func TestLoopMultiTurnTools(t *testing.T) {
	home := isolateHome(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("alpha\nbeta\n"), 0o644)
	p := &scripted{replies: []ChatResult{
		{ToolCalls: []ToolCall{call("c1", "read_file", `{"path":"notes.txt"}`)}},
		{ToolCalls: []ToolCall{call("c2", "edit_file", `{"path":"notes.txt","old_string":"beta","new_string":"gamma"}`), call("c3", "grep", `{"pattern":"gam"}`)}},
		{Content: "done: beta is now gamma", Usage: Usage{PromptTokens: 30, CompletionTokens: 5, TotalTokens: 35}},
		{Content: "second turn"},
	}}
	session, err := OpenSession("")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(session.Path) != filepath.Join(home, ".hev", "foh", "sessions") {
		t.Fatalf("session path = %s", session.Path)
	}
	a := NewAgent(p, NewToolset(dir, noHev), "test/a", "SYSTEM")
	a.Session = session
	var kinds []EventKind
	if err := a.Run(context.Background(), "rename beta", func(ev Event) { kinds = append(kinds, ev.Kind) }); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "notes.txt")); string(b) != "alpha\ngamma\n" {
		t.Fatalf("file = %q", b)
	}
	if len(p.requests) != 3 {
		t.Fatalf("requests = %d", len(p.requests))
	}
	last := p.requests[2].Messages
	if last[0].Role != "system" || last[0].Content != "SYSTEM" {
		t.Fatalf("system message = %+v", last[0])
	}
	var tools []string
	for _, m := range last {
		if m.Role == "tool" {
			tools = append(tools, m.ToolCallID+"="+strings.SplitN(m.Content, "\n", 2)[0])
		}
	}
	if strings.Join(tools, ";") != "c1=alpha;c2=edited notes.txt (1 replacement(s));c3=notes.txt:2:gamma" {
		t.Fatalf("tool results = %q", tools)
	}
	if kinds[len(kinds)-1] != EventDone {
		t.Fatalf("events = %v", kinds)
	}
	// A second user turn continues the same thread.
	if err := a.Run(context.Background(), "again", nil); err != nil {
		t.Fatal(err)
	}
	if n := len(p.requests[3].Messages); n != len(last)+2 { // + final answer + new user message
		t.Fatalf("history not kept: %d messages", n)
	}
	session.Close()
	recs := readSession(t, session.Path)
	var usage *Usage
	for _, r := range recs {
		if r.Type == "assistant" && r.Content == "done: beta is now gamma" {
			usage = r.Usage
		}
	}
	if usage == nil || usage.TotalTokens != 35 {
		t.Fatalf("usage not recorded: %+v", recs)
	}
}

func TestLoopSteeringBetweenToolCalls(t *testing.T) {
	isolateHome(t)
	dir := t.TempDir()
	p := &scripted{replies: []ChatResult{
		{ToolCalls: []ToolCall{call("c1", "deploy", `{"env":"prod"}`), call("c2", "deploy", `{"env":"prod-2"}`)}},
		{Content: "switched to staging"},
	}}
	tools := NewToolset(dir, noHev)
	a := NewAgent(p, tools, "test/a", "")
	var ran []string
	tools.Register(Tool{Spec: FunctionSpec{Name: "deploy", Parameters: schema(`{"type":"object"}`)},
		Run: func(_ context.Context, raw json.RawMessage) ToolResult {
			ran = append(ran, string(raw))
			// The user steers while the first call is running.
			if !a.Steer("stop, use staging instead") {
				t.Error("steer refused")
			}
			return ToolResult{Content: "deployed"}
		}})
	var steered []string
	if err := a.Run(context.Background(), "deploy", func(ev Event) {
		if ev.Kind == EventSteer {
			steered = append(steered, ev.Text)
		}
	}); err != nil {
		t.Fatal(err)
	}
	// Refused: the call after the steer must not run.
	if len(ran) != 1 {
		t.Fatalf("ran %d calls: %v", len(ran), ran)
	}
	if len(steered) != 1 || steered[0] != "stop, use staging instead" {
		t.Fatalf("steer events = %v", steered)
	}
	msgs := p.requests[1].Messages
	n := len(msgs)
	// Kept: every tool call is answered, then the steer follows as a user message.
	if msgs[n-3].ToolCallID != "c1" || msgs[n-3].Content != "deployed" ||
		msgs[n-2].ToolCallID != "c2" || !strings.HasPrefix(msgs[n-2].Content, "skipped") ||
		msgs[n-1].Role != "user" || msgs[n-1].Content != SteerPrefix+"stop, use staging instead" {
		t.Fatalf("thread after steer = %+v", msgs[n-3:])
	}
}

func TestLoopSteerAfterFinalAnswerContinues(t *testing.T) {
	isolateHome(t)
	p := &scripted{replies: []ChatResult{{Content: "first"}, {Content: "after steer"}}}
	a := NewAgent(p, NewToolset(t.TempDir(), noHev), "m", "")
	a.Steer("also check the board")
	if err := a.Run(context.Background(), "go", nil); err != nil {
		t.Fatal(err)
	}
	if len(p.requests) != 2 || p.requests[1].Messages[len(p.requests[1].Messages)-1].Content != SteerPrefix+"also check the board" {
		t.Fatalf("requests = %+v", p.requests)
	}
}

func TestLoopModelHotSwap(t *testing.T) {
	isolateHome(t)
	p := &scripted{replies: []ChatResult{
		{ToolCalls: []ToolCall{call("c1", "swap", `{}`)}},
		{Content: "answered by b"},
		{Content: "turn two by c"},
	}}
	tools := NewToolset(t.TempDir(), noHev)
	a := NewAgent(p, tools, "vendor/model-a", "")
	tools.Register(Tool{Spec: FunctionSpec{Name: "swap", Parameters: schema(`{"type":"object"}`)},
		Run: func(context.Context, json.RawMessage) ToolResult {
			a.SetModel("vendor/model-b") // mid-turn, from the picker
			return ToolResult{Content: "ok"}
		}})
	session, err := OpenSession("")
	if err != nil {
		t.Fatal(err)
	}
	a.Session = session
	if err := a.Run(context.Background(), "one", nil); err != nil {
		t.Fatal(err)
	}
	a.SetModel("vendor/model-c")
	a.SetModel("vendor/model-c") // no-op
	if err := a.Run(context.Background(), "two", nil); err != nil {
		t.Fatal(err)
	}
	session.Close()
	got := []string{p.requests[0].Model, p.requests[1].Model, p.requests[2].Model}
	if strings.Join(got, ",") != "vendor/model-a,vendor/model-b,vendor/model-c" {
		t.Fatalf("request models = %v", got)
	}
	// History survives the swaps.
	if n := len(p.requests[2].Messages); n != 5 {
		t.Fatalf("history = %d messages", n)
	}
	matches, _ := filepath.Glob(filepath.Join(SessionsDir(), "*.jsonl"))
	if len(matches) != 1 {
		t.Fatalf("sessions = %v", matches)
	}
	var assistants, switches []string
	for _, r := range readSession(t, matches[0]) {
		switch r.Type {
		case "assistant":
			assistants = append(assistants, fmt.Sprintf("%d:%s", r.Turn, r.Model))
		case "model_switch":
			switches = append(switches, r.From+">"+r.Model)
		}
		if r.Session != session.ID || r.V != 1 {
			t.Fatalf("record header = %+v", r)
		}
	}
	if strings.Join(assistants, ",") != "1:vendor/model-a,1:vendor/model-b,2:vendor/model-c" {
		t.Fatalf("per-turn models = %v", assistants)
	}
	if strings.Join(switches, ",") != "vendor/model-a>vendor/model-b,vendor/model-b>vendor/model-c" {
		t.Fatalf("switches = %v", switches)
	}
}

func TestSessionTraceRedaction(t *testing.T) {
	isolateHome(t)
	dir := t.TempDir()
	// Refused: the key reaching a tool's environment or output, and the key
	// or a bearer token in any saved record. Kept: everything else.
	t.Setenv("OPENROUTER_API_KEY", testKey)
	p := &scripted{replies: []ChatResult{
		{ToolCalls: []ToolCall{call("c1", "bash", `{"command":"echo key=[$OPENROUTER_API_KEY] visible-output"}`)}},
		{Content: "the header was Authorization: Bearer tok_abcdefghijklmnop and model x/y"},
	}}
	session, err := OpenSession("", testKey)
	if err != nil {
		t.Fatal(err)
	}
	a := NewAgent(p, NewToolset(dir, noHev, testKey), "x/y", "")
	a.Session = session
	if err := a.Run(context.Background(), "my key is "+testKey+", keep the word pineapple", nil); err != nil {
		t.Fatal(err)
	}
	session.Close()
	raw, _ := os.ReadFile(session.Path)
	text := string(raw)
	for _, leak := range []string{testKey, "tok_abcdefghijklmnop"} {
		if strings.Contains(text, leak) {
			t.Fatalf("session leaks %q:\n%s", leak, text)
		}
	}
	for _, keep := range []string{"pineapple", "visible-output", "key=[]", "model x/y", redacted} {
		if !strings.Contains(text, keep) {
			t.Fatalf("session lost %q:\n%s", keep, text)
		}
	}
	// The provider saw the scrubbed tool result too.
	for _, m := range p.requests[1].Messages {
		if m.Role == "tool" && strings.Contains(m.Content, testKey) {
			t.Fatal("key sent to the model in a tool result")
		}
	}
}

func TestLoopWithOpenRouterClient(t *testing.T) {
	isolateHome(t)
	var mu sync.Mutex
	var models []string
	step := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ChatRequest
		json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		models = append(models, req.Model)
		step++
		n := step
		mu.Unlock()
		if n == 1 {
			sse(w, `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"t1","function":{"name":"glob","arguments":"{\"pattern\":\"*.md\"}"}}]}}]}`)
			return
		}
		sse(w, `{"choices":[{"delta":{"content":"found it"}}]}`, `{"choices":[],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`)
	}))
	defer srv.Close()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("x"), 0o644)
	a := NewAgent(NewClient(srv.URL, testKey), NewToolset(dir, noHev), "or/model", "")
	var text strings.Builder
	if err := a.Run(context.Background(), "find docs", func(ev Event) {
		if ev.Kind == EventText {
			text.WriteString(ev.Text)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if text.String() != "found it" || strings.Join(models, ",") != "or/model,or/model" {
		t.Fatalf("text=%q models=%v", text.String(), models)
	}
	h := a.History()
	if h[2].Role != "tool" || h[2].Content != "README.md" {
		t.Fatalf("history = %+v", h)
	}
}

func TestPromptCompositionAndReload(t *testing.T) {
	home := isolateHome(t)
	cwd := t.TempDir()
	os.WriteFile(filepath.Join(cwd, "AGENTS.md"), []byte("AGENTS RULES"), 0o644)
	os.WriteFile(filepath.Join(cwd, "MISSION.md"), []byte("MISSION TEXT"), 0o644)
	userPath := filepath.Join(home, ".config", "hev", "foh-prompt.md")
	os.MkdirAll(filepath.Dir(userPath), 0o700)
	os.WriteFile(userPath, []byte("USER CUSTOM v1"), 0o644)
	extra := filepath.Join(t.TempDir(), "extra.md")
	os.WriteFile(extra, []byte("EXTRA PROMPT"), 0o644)
	c := PromptComposer{Cwd: cwd, ConfigPrompt: "CONFIG INLINE", ExtraPath: extra, Header: func() string { return "host: test" }}
	parts, err := c.Parts()
	if err != nil {
		t.Fatal(err)
	}
	var labels []string
	for _, p := range parts {
		labels = append(labels, p.Label+":"+p.Source)
	}
	if strings.Join(labels, ",") != "base:built-in,workspace:AGENTS.md,workspace:MISSION.md,user:foh-prompt.md,config:[foh].system_prompt,extra:--prompt,live:workspace" {
		t.Fatalf("parts = %v", labels)
	}
	s, _ := c.Compose()
	order := []string{"hev foh", "AGENTS RULES", "MISSION TEXT", "USER CUSTOM v1", "CONFIG INLINE", "EXTRA PROMPT", "host: test"}
	last := -1
	for _, want := range order {
		i := strings.Index(s, want)
		if i <= last {
			t.Fatalf("%q out of order in:\n%s", want, s)
		}
		last = i
	}

	// Reload picks up edits.
	a := NewAgent(&scripted{}, nil, "m", s)
	os.WriteFile(userPath, []byte("USER CUSTOM v2"), 0o644)
	s2, _ := c.Compose()
	a.SetSystem(s2)
	if !strings.Contains(a.System(), "USER CUSTOM v2") || strings.Contains(a.System(), "USER CUSTOM v1") {
		t.Fatal("reload did not replace the prompt")
	}

	// A config prompt that names a file reads the file.
	cfgFile := filepath.Join(home, "p.md")
	os.WriteFile(cfgFile, []byte("FROM CONFIG FILE"), 0o644)
	if s, _ := (PromptComposer{ConfigPrompt: cfgFile}).Compose(); !strings.Contains(s, "FROM CONFIG FILE") {
		t.Fatal("config prompt file not read")
	}
	// A config path that does not exist is skipped, not sent as text.
	if s, _ := (PromptComposer{ConfigPrompt: "~/missing/prompt.md"}).Compose(); strings.Contains(s, "missing") {
		t.Fatal("missing config path sent as prompt text")
	}
	// A missing --prompt file is an error without the local path.
	_, err = PromptComposer{ExtraPath: filepath.Join(home, "nope.md")}.Parts()
	if err == nil || strings.Contains(err.Error(), home) {
		t.Fatalf("missing --prompt = %v", err)
	}
}

func TestHistoryBounded(t *testing.T) {
	a := NewAgent(&scripted{}, nil, "m", "")
	for i := 0; i < MaxHistory; i++ {
		a.append(Message{Role: "user", Content: "u"}, Message{Role: "assistant", ToolCalls: []ToolCall{call("x", "t", "{}")}}, Message{Role: "tool", ToolCallID: "x"})
	}
	h := a.History()
	if len(h) > MaxHistory || h[0].Role != "user" {
		t.Fatalf("history len=%d first=%s", len(h), h[0].Role)
	}
}

func TestSessionPruneAndCap(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 5; i++ {
		os.WriteFile(filepath.Join(dir, fmt.Sprintf("00000000000000000000000%03d.jsonl", i)), nil, 0o600)
	}
	pruneSessions(dir, 3)
	m, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if len(m) != 3 || filepath.Base(m[0]) != "00000000000000000000000002.jsonl" {
		t.Fatalf("pruned = %v", m)
	}
	s, err := OpenSession(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.written = MaxSessionBytes - 10
	s.Write(Record{Type: "user", Content: strings.Repeat("x", 100)})
	s.Write(Record{Type: "user", Content: "after"})
	s.Close()
	recs := readSession(t, s.Path)
	if len(recs) != 1 || recs[0].Type != "truncated" {
		t.Fatalf("records = %+v", recs)
	}
}
