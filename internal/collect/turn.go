package collect

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"
)

// The Vercel AI SDK's experimental_telemetry attributes app capture reads.
const (
	attrUserID     = "ai.telemetry.metadata.userId"
	attrChatID     = "ai.telemetry.metadata.chatId"
	attrTurnID     = "ai.telemetry.metadata.turnId"
	attrTenant     = "ai.telemetry.metadata.tenant"
	attrOperation  = "ai.operationId"
	attrPrompt     = "ai.prompt"
	attrMessages   = "ai.prompt.messages"
	attrRespText   = "ai.response.text"
	attrToolName   = "ai.toolCall.name"
	attrToolCallID = "ai.toolCall.id"
	attrService    = "service.name"
)

// Tool is one tool call in a turn.
type Tool struct {
	Name  string `json:"name"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// Turn is the turn document: one per (namespace, tenant, owner, turnId).
// Namespace is the app namespace the turn's spans declared; each namespace's
// turns live in their own Layer namespace.
type Turn struct {
	Namespace string    `json:"-"`
	Owner     string    `json:"owner"`
	Tenant    string    `json:"tenant"`
	ChatID    string    `json:"chatId"`
	TurnID    string    `json:"turnId"`
	TS        time.Time `json:"ts"`
	Prompt    string    `json:"prompt"`
	Answer    string    `json:"answer"`
	Tools     []Tool    `json:"tools"`
	Service   string    `json:"-"`
}

// DocID is the turn's row id: stable for its (namespace, tenant, owner,
// turnId), so a later export of the same turn rewrites the same row.
func (t Turn) DocID() string {
	sum := sha256.Sum256([]byte(t.Namespace + "\x00" + t.Tenant + "\x00" + t.Owner + "\x00" + t.TurnID))
	return "turn_" + hex.EncodeToString(sum[:16])
}

type turnKey struct{ namespace, tenant, owner, turnID string }

// traceKey names a trace within a namespace: two namespaces never share a
// trace's turn, even if a trace id repeats.
type traceKey struct{ namespace, traceID string }

func traceOf(sp Span) traceKey { return traceKey{namespace: spanNamespace(sp), traceID: sp.TraceID} }

type toolState struct {
	start time.Time
	tool  Tool
}

type turnState struct {
	key     turnKey
	chatID  string
	service string
	ts      time.Time
	seen    time.Time

	rootPrompt   string
	stepPrompt   string
	rootAnswer   string
	stepAnswer   string
	stepAnswerAt time.Time
	tools        map[string]toolState
}

func (s *turnState) turn() Turn {
	t := Turn{Namespace: s.key.namespace, Owner: s.key.owner, Tenant: s.key.tenant, TurnID: s.key.turnID, ChatID: s.chatID, TS: s.ts, Service: s.service, Tools: []Tool{}}
	t.Prompt = firstNonEmpty(s.rootPrompt, s.stepPrompt)
	t.Answer = firstNonEmpty(s.rootAnswer, s.stepAnswer)
	tools := make([]toolState, 0, len(s.tools))
	for _, ts := range s.tools {
		tools = append(tools, ts)
	}
	sort.SliceStable(tools, func(i, j int) bool { return tools[i].start.Before(tools[j].start) })
	for _, ts := range tools {
		t.Tools = append(t.Tools, ts.tool)
	}
	return t
}

func (s *turnState) add(sp Span) {
	if s.ts.IsZero() || (!sp.Start.IsZero() && sp.Start.Before(s.ts)) {
		s.ts = sp.Start
	}
	if s.chatID == "" {
		s.chatID = attrString(sp.Attrs, attrChatID)
	}
	if s.service == "" {
		s.service = attrString(sp.Resource, attrService)
	}
	op := operation(sp)
	switch {
	case op == "ai.toolCall":
		id := firstNonEmpty(attrString(sp.Attrs, attrToolCallID), sp.SpanID)
		tool := Tool{Name: attrString(sp.Attrs, attrToolName), OK: sp.StatusCode != StatusError}
		if !tool.OK {
			tool.Error = sp.StatusMessage
		}
		s.tools[id] = toolState{start: sp.Start, tool: tool}
	case isRootOp(op):
		if p := lastUserMessage(sp.Attrs); p != "" {
			s.rootPrompt = p
		}
		if a := attrString(sp.Attrs, attrRespText); a != "" {
			s.rootAnswer = a
		}
	case isStepOp(op):
		if s.stepPrompt == "" {
			s.stepPrompt = lastUserMessage(sp.Attrs)
		}
		if a := attrString(sp.Attrs, attrRespText); a != "" && !sp.End.Before(s.stepAnswerAt) {
			s.stepAnswer, s.stepAnswerAt = a, sp.End
		}
	}
}

func operation(sp Span) string {
	if op := attrString(sp.Attrs, attrOperation); op != "" {
		return op
	}
	return sp.Name
}

func isRootOp(op string) bool {
	switch op {
	case "ai.streamText", "ai.generateText", "ai.streamObject", "ai.generateObject":
		return true
	}
	return false
}

func isStepOp(op string) bool {
	switch op {
	case "ai.streamText.doStream", "ai.generateText.doGenerate", "ai.streamObject.doStream", "ai.generateObject.doGenerate":
		return true
	}
	return false
}

// lastUserMessage is the turn's prompt: the last user message in the span's
// recorded input, from `ai.prompt` ({prompt} or {messages}) on the root span
// or `ai.prompt.messages` on a step.
func lastUserMessage(attrs map[string]any) string {
	if raw := attrString(attrs, attrPrompt); raw != "" {
		var p struct {
			Prompt   json.RawMessage   `json:"prompt"`
			Messages []json.RawMessage `json:"messages"`
		}
		if json.Unmarshal([]byte(raw), &p) == nil {
			if m := lastUser(p.Messages); m != "" {
				return m
			}
			var s string
			if json.Unmarshal(p.Prompt, &s) == nil && s != "" {
				return s
			}
			var msgs []json.RawMessage
			if json.Unmarshal(p.Prompt, &msgs) == nil {
				if m := lastUser(msgs); m != "" {
					return m
				}
			}
		}
	}
	if raw := attrString(attrs, attrMessages); raw != "" {
		var msgs []json.RawMessage
		if json.Unmarshal([]byte(raw), &msgs) == nil {
			return lastUser(msgs)
		}
	}
	return ""
}

func lastUser(msgs []json.RawMessage) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		var m struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
		if json.Unmarshal(msgs[i], &m) != nil || m.Role != "user" {
			continue
		}
		if text := contentText(m.Content); text != "" {
			return text
		}
	}
	return ""
}

func contentText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) != nil {
		return ""
	}
	var texts []string
	for _, p := range parts {
		if p.Type == "text" && p.Text != "" {
			texts = append(texts, p.Text)
		}
	}
	return strings.Join(texts, "\n")
}

func attrString(attrs map[string]any, key string) string {
	switch v := attrs[key].(type) {
	case string:
		return v
	case int64:
		return jsonString(v)
	case float64:
		return jsonString(v)
	}
	return ""
}

func jsonString(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

// spanTurn is the turn a span names through its telemetry metadata, if any.
func spanTurn(sp Span) (turnKey, bool) {
	k := turnKey{namespace: spanNamespace(sp), tenant: attrString(sp.Attrs, attrTenant), owner: attrString(sp.Attrs, attrUserID), turnID: attrString(sp.Attrs, attrTurnID)}
	return k, k.turnID != "" && k.owner != ""
}

// Attributed is a span with the turn it belongs to. A span that never found
// its turn before it expired is written with the turn fields empty.
type Attributed struct {
	Span      Span
	Namespace string
	Owner     string
	Tenant    string
	ChatID    string
	TurnID    string
}

// Aggregator collapses spans into turns across export requests. A turn's
// spans arrive over several exports (a tool call ends, and is exported,
// before the run that called it), and in the AI SDK only the run and step
// spans carry the telemetry metadata, so tool spans are joined to their turn
// by trace id. A span whose trace has not yet named its turn waits, up to
// Hold, for one that does.
type Aggregator struct {
	// Hold is how long an unattributed span waits for its trace's turn.
	Hold time.Duration
	// Keep is how long a turn stays in memory after its last span, so a
	// late span still merges into the whole turn rather than overwriting it.
	Keep time.Duration

	mu      sync.Mutex
	turns   map[turnKey]*turnState
	traces  map[traceKey]turnKey
	pending map[traceKey][]pendingSpan
	now     func() time.Time
}

type pendingSpan struct {
	span Span
	at   time.Time
}

func NewAggregator() *Aggregator {
	return &Aggregator{Hold: 5 * time.Minute, Keep: 2 * time.Hour, turns: map[turnKey]*turnState{}, traces: map[traceKey]turnKey{}, pending: map[traceKey][]pendingSpan{}, now: time.Now}
}

// Add merges a batch. It returns every span that is now attributed (this
// batch's and any earlier ones its trace was waiting on) and the turns they
// changed, whole.
func (a *Aggregator) Add(spans []Span) ([]Attributed, []Turn) {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.now()
	for _, sp := range spans {
		if k, ok := spanTurn(sp); ok && sp.TraceID != "" {
			a.traces[traceOf(sp)] = k
		}
	}
	var out []Attributed
	changed := map[turnKey]bool{}
	attribute := func(sp Span, k turnKey) {
		st := a.turns[k]
		if st == nil {
			st = &turnState{key: k, tools: map[string]toolState{}}
			a.turns[k] = st
		}
		st.add(sp)
		st.seen = now
		changed[k] = true
		out = append(out, Attributed{Span: sp, Namespace: k.namespace, Owner: k.owner, Tenant: k.tenant, TurnID: k.turnID, ChatID: st.chatID})
	}
	for _, sp := range spans {
		k, ok := spanTurn(sp)
		if !ok {
			k, ok = a.traces[traceOf(sp)]
		}
		if !ok {
			a.pending[traceOf(sp)] = append(a.pending[traceOf(sp)], pendingSpan{span: sp, at: now})
			continue
		}
		attribute(sp, k)
	}
	for tk, waiting := range a.pending {
		k, ok := a.traces[tk]
		if !ok {
			continue
		}
		for _, p := range waiting {
			attribute(p.span, k)
		}
		delete(a.pending, tk)
	}
	turns := make([]Turn, 0, len(changed))
	for k := range changed {
		turns = append(turns, a.turns[k].turn())
	}
	sort.Slice(turns, func(i, j int) bool { return turns[i].TS.Before(turns[j].TS) })
	return out, turns
}

// Expire drops turns idle past Keep and returns spans that waited past Hold
// without a turn, so their envelopes are still written.
func (a *Aggregator) Expire() []Span {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.expire(a.now(), false)
}

// Drain returns every span still waiting, for shutdown.
func (a *Aggregator) Drain() []Span {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.expire(a.now(), true)
}

func (a *Aggregator) expire(now time.Time, all bool) []Span {
	var out []Span
	for tk, waiting := range a.pending {
		keep := waiting[:0]
		for _, p := range waiting {
			if all || now.Sub(p.at) >= a.Hold {
				out = append(out, p.span)
			} else {
				keep = append(keep, p)
			}
		}
		if len(keep) == 0 {
			delete(a.pending, tk)
		} else {
			a.pending[tk] = keep
		}
	}
	for k, st := range a.turns {
		if now.Sub(st.seen) >= a.Keep {
			delete(a.turns, k)
			for trace, tk := range a.traces {
				if tk == k {
					delete(a.traces, trace)
				}
			}
		}
	}
	return out
}
