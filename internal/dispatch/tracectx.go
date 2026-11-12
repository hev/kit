package dispatch

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

// Ask-rooted trace context. The ask that starts work is the trace root; the
// run's attempts, validation runs and worker sessions are spans inside it.
// Every field here is optional on the wire and on disk, so a hev one version
// behind reads and forwards these records unchanged (Go's decoder ignores
// unknown fields), and a record written before tracing reads as untraced.

const (
	// AskIntake is the root ask that started a trace.
	AskIntake = "intake"
	// AskFollowUp is a later ask in an existing trace.
	AskFollowUp = "ask"
)

// Caller is what the dispatching process could learn about who asked.
type Caller struct {
	// Session is the calling tmux session name, such as a FOH session.
	Session string `json:"session,omitempty"`
	// Harness and HarnessSession identify the calling agent's own session,
	// when its harness exposes one in the environment.
	Harness        string `json:"harness,omitempty"`
	HarnessSession string `json:"harness_session,omitempty"`
	Host           string `json:"host,omitempty"`
	Sender         string `json:"sender,omitempty"`
	// Run names the dispatch run the caller is itself a worker of, if any.
	Run string `json:"run,omitempty"`
}

// AskSpan is an ask recorded against a run: the root intake that started its
// trace, or a follow-up (continue, rework, amend, retry, say, validate).
type AskSpan struct {
	SpanID       string    `json:"span_id"`
	ParentSpanID string    `json:"parent_span_id,omitempty"`
	Kind         string    `json:"kind"`
	Action       string    `json:"action"`
	Text         string    `json:"text,omitempty"`
	Caller       Caller    `json:"caller,omitempty"`
	Time         time.Time `json:"time"`
}

var (
	traceIDPattern     = regexp.MustCompile(`^[0-9a-f]{32}$`)
	spanIDPattern      = regexp.MustCompile(`^[0-9a-f]{16}$`)
	traceparentPattern = regexp.MustCompile(`^00-([0-9a-f]{32})-([0-9a-f]{16})-[0-9a-f]{2}$`)
)

func randomHex(n int) string {
	b := make([]byte, n)
	for {
		if _, err := rand.Read(b); err != nil {
			panic(err)
		}
		for _, c := range b {
			if c != 0 {
				return hex.EncodeToString(b)
			}
		}
	}
}

// NewTraceID mints a nonzero 128-bit W3C trace ID.
func NewTraceID() string { return randomHex(16) }

// NewSpanID mints a nonzero 64-bit W3C span ID.
func NewSpanID() string { return randomHex(8) }

func ValidTraceID(s string) bool { return traceIDPattern.MatchString(s) && strings.Trim(s, "0") != "" }
func ValidSpanID(s string) bool  { return spanIDPattern.MatchString(s) && strings.Trim(s, "0") != "" }

// Traceparent formats a sampled W3C traceparent header value.
func Traceparent(traceID, spanID string) string {
	return "00-" + traceID + "-" + spanID + "-01"
}

// ParseTraceRef accepts a trace ID or a W3C traceparent. A traceparent also
// names the span the new ask continues from.
func ParseTraceRef(s string) (traceID, parentSpan string, err error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if m := traceparentPattern.FindStringSubmatch(s); m != nil && ValidTraceID(m[1]) && ValidSpanID(m[2]) {
		return m[1], m[2], nil
	}
	if ValidTraceID(s) {
		return s, "", nil
	}
	return "", "", fmt.Errorf("invalid trace %q: want a 32-hex trace ID or a W3C traceparent", s)
}

// DetectCaller reads the calling process's identity. It never fails: what
// cannot be learned is left empty.
func DetectCaller() Caller {
	c := Caller{Sender: LocalSender()}
	c.Host, _ = os.Hostname()
	if i := strings.Index(c.Host, "."); i > 0 {
		c.Host = c.Host[:i]
	}
	switch {
	case os.Getenv("CLAUDE_CODE_SESSION_ID") != "":
		c.Harness, c.HarnessSession = "claude", os.Getenv("CLAUDE_CODE_SESSION_ID")
	case os.Getenv("CLAUDE_SESSION_ID") != "":
		c.Harness, c.HarnessSession = "claude", os.Getenv("CLAUDE_SESSION_ID")
	case os.Getenv("CODEX_THREAD_ID") != "":
		c.Harness, c.HarnessSession = "codex", os.Getenv("CODEX_THREAD_ID")
	case os.Getenv("CLAUDECODE") != "":
		c.Harness = "claude"
	}
	c.Session = os.Getenv("HEV_CALLER_SESSION")
	if c.Session == "" && CallerSession != nil {
		c.Session = CallerSession()
	}
	c.Run = os.Getenv("HEV_RUN")
	return c
}

// CallerSession names the caller's own terminal session, such as a FOH tmux
// session. The CLI sets it: dispatch's own tmux use is confined to its
// private socket, and the caller's session lives on the caller's server.
var CallerSession func() string

// InheritedTrace is the trace context of the calling process, as set by a
// supervisor for its worker (TRACEPARENT, then HEV_TRACE_ID).
func InheritedTrace() (traceID, parentSpan string) {
	if t, p, err := ParseTraceRef(os.Getenv("TRACEPARENT")); err == nil && os.Getenv("TRACEPARENT") != "" {
		return t, p
	}
	if t := strings.ToLower(os.Getenv("HEV_TRACE_ID")); ValidTraceID(t) {
		return t, ""
	}
	return "", ""
}

// CallerOrchestrator names the orchestrator the calling process belongs to.
func CallerOrchestrator() string { return currentOrchestratorIdentity() }

// TraceStart is the sender's input to the new-or-continue rule.
type TraceStart struct {
	// Ref is an explicit --trace or trace: value. It continues that trace.
	Ref string
	// Orchestrator marks a launch by an orchestrator, which must inherit the
	// trace of what caused it or stay untraced.
	Orchestrator string
	Caller       Caller
	Text         string
	Now          time.Time
}

// StampTrace applies the new-or-continue rule to a run about to start:
//   - an explicit ref continues that trace with a follow-up ask;
//   - a caller inside a traced run (TRACEPARENT) continues its trace;
//   - an orchestrator without trace context starts the run untraced;
//   - otherwise the run starts a new trace with a root intake ask.
//
// The run's attempt span is minted at launch, not here.
func StampTrace(m *Meta, in TraceStart) error {
	if in.Now.IsZero() {
		in.Now = time.Now().UTC()
	}
	ask := AskSpan{SpanID: NewSpanID(), Kind: AskFollowUp, Action: "continue", Text: askText(in.Text), Caller: in.Caller, Time: in.Now}
	switch traceID, parent := InheritedTrace(); {
	case in.Ref != "":
		t, p, err := ParseTraceRef(in.Ref)
		if err != nil {
			return err
		}
		m.TraceID, ask.ParentSpanID = t, p
	case traceID != "":
		m.TraceID, ask.ParentSpanID, ask.Action = traceID, parent, "launch"
	case in.Orchestrator != "":
		m.TraceID, m.Asks, m.ParentSpanID = "", nil, ""
		m.Untraced = "orchestrator " + in.Orchestrator + " launched this run without trace context"
		return nil
	default:
		m.TraceID = NewTraceID()
		ask.Kind, ask.Action = AskIntake, "dispatch"
	}
	m.Untraced = ""
	m.Asks = []AskSpan{ask}
	m.ParentSpanID = ask.SpanID
	return nil
}

// RootAsk is the first intake ask recorded on a run, if it started its trace.
func RootAsk(m Meta) *AskSpan {
	for i := range m.Asks {
		if m.Asks[i].Kind == AskIntake {
			return &m.Asks[i]
		}
	}
	return nil
}

// recordAsk appends a follow-up ask to a traced run and returns its span ID.
// An untraced run stays untraced: no trace is invented for it.
func recordAsk(m *Meta, action, text string, caller *Caller) string {
	if !ValidTraceID(m.TraceID) {
		return ""
	}
	ask := AskSpan{SpanID: NewSpanID(), Kind: AskFollowUp, Action: action, Text: askText(text), Time: time.Now().UTC()}
	if len(m.Asks) > 0 {
		ask.ParentSpanID = m.Asks[0].SpanID
	}
	if caller != nil {
		ask.Caller = *caller
	} else {
		ask.Caller = Caller{Sender: m.Sender}
	}
	m.Asks = append(m.Asks, ask)
	return ask.SpanID
}

// beginAttempt mints the span for a new attempt of a traced run, parented by
// the ask that launched it.
func beginAttempt(m *Meta, parent string) {
	if !ValidTraceID(m.TraceID) {
		m.SpanID = ""
		return
	}
	m.SpanID = NewSpanID()
	if parent != "" {
		m.ParentSpanID = parent
	}
}

// traceEnvironment is the worker's trace context. The attempt span is the
// parent of everything the worker does.
func traceEnvironment(m Meta) []string {
	env := []string{"HEV_RUN=" + m.Name}
	if !ValidTraceID(m.TraceID) || !ValidSpanID(m.SpanID) {
		return env
	}
	return append(env, "TRACEPARENT="+Traceparent(m.TraceID, m.SpanID), "HEV_TRACE_ID="+m.TraceID, "HEV_SPAN_ID="+m.SpanID)
}

// withoutTraceEnv drops inherited trace variables so a worker never reports a
// trace other than its own run's.
func withoutTraceEnv(env []string) []string {
	out := env[:0:0]
	for _, item := range env {
		key, _, _ := strings.Cut(item, "=")
		switch strings.ToUpper(key) {
		case "TRACEPARENT", "TRACESTATE", "HEV_TRACE_ID", "HEV_SPAN_ID", "HEV_RUN":
			continue
		}
		out = append(out, item)
	}
	return out
}

const askTextLimit = 4000

func askText(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > askTextLimit {
		s = s[:askTextLimit] + "…"
	}
	return s
}

// OrderAskText is the ask text a new order carries: the body before its
// Acceptance section, without frontmatter.
func OrderAskText(order string) string {
	lines := strings.Split(strings.ReplaceAll(order, "\r\n", "\n"), "\n")
	if len(lines) > 0 && lines[0] == "---" {
		for i := 1; i < len(lines); i++ {
			if lines[i] == "---" {
				lines = lines[i+1:]
				break
			}
		}
	}
	var body []string
	for _, l := range lines {
		if strings.TrimSpace(l) == "## Acceptance" {
			break
		}
		body = append(body, l)
	}
	return askText(strings.Join(body, "\n"))
}

// TakeTraceKey removes a trace: frontmatter key from an order before it is
// sent, so a target one version behind never sees a key it would reject.
func TakeTraceKey(order string) (string, string, error) {
	lines := strings.Split(order, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return order, "", nil
	}
	ref := ""
	for i := 1; i < len(lines); i++ {
		line := strings.TrimRight(lines[i], "\r")
		if line == "---" {
			break
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(k) != "trace" {
			continue
		}
		if ref != "" {
			return order, "", fmt.Errorf("duplicate key: trace")
		}
		ref = strings.Trim(strings.TrimSpace(strings.SplitN(v, "#", 2)[0]), `"'`)
		if _, _, err := ParseTraceRef(ref); err != nil {
			return order, "", err
		}
		lines = append(lines[:i], lines[i+1:]...)
		i--
	}
	return strings.Join(lines, "\n"), ref, nil
}

// sanitizeTrace keeps only a well-formed trace context from a sender. The
// target mints attempt spans itself.
func sanitizeTrace(m *Meta) {
	m.SpanID = ""
	if !ValidTraceID(m.TraceID) {
		m.TraceID, m.ParentSpanID, m.Asks = "", "", nil
		return
	}
	if m.ParentSpanID != "" && !ValidSpanID(m.ParentSpanID) {
		m.ParentSpanID = ""
	}
	asks := m.Asks[:0]
	for _, a := range m.Asks {
		if ValidSpanID(a.SpanID) && (a.ParentSpanID == "" || ValidSpanID(a.ParentSpanID)) {
			a.Text = askText(a.Text)
			asks = append(asks, a)
		}
	}
	m.Asks = asks
}

func amendAskText(revision int, diff string) string {
	return fmt.Sprintf("Amend to revision %d.\n\n%s", revision, diff)
}

// OrderAcceptance lists an order's Acceptance items in order, each folded
// onto one line.
func OrderAcceptance(order string) []string {
	var out []string
	active, current := false, ""
	flush := func() {
		if current != "" {
			out = append(out, strings.Join(strings.Fields(current), " "))
			current = ""
		}
	}
	for _, line := range strings.Split(strings.ReplaceAll(order, "\r\n", "\n"), "\n") {
		text := strings.TrimSpace(line)
		if text == "## Acceptance" {
			active = true
			continue
		}
		if !active || text == "" {
			continue
		}
		if strings.HasPrefix(text, "## ") {
			break
		}
		if marker := acceptanceBullet.FindString(text); marker != "" {
			flush()
			current = text[len(marker):]
		} else if current != "" {
			current += " " + text
		}
	}
	flush()
	return out
}
