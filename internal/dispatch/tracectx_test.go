package dispatch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

// clearTraceEnv isolates a test from a TRACEPARENT the test runner inherited,
// for example when the suite runs inside a traced dispatch worker.
func clearTraceEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"TRACEPARENT", "HEV_TRACE_ID", "HEV_SPAN_ID", "HEV_RUN", "HEV_ORCHESTRATOR"} {
		t.Setenv(k, "")
	}
}

func TestTraceIDsAndRefs(t *testing.T) {
	id, span := NewTraceID(), NewSpanID()
	if !ValidTraceID(id) || !ValidSpanID(span) || id == NewTraceID() {
		t.Fatalf("ids %q %q", id, span)
	}
	if ValidTraceID(strings.Repeat("0", 32)) || ValidSpanID(strings.Repeat("0", 16)) {
		t.Fatal("accepted an all-zero id")
	}
	tp := Traceparent(id, span)
	if !regexp.MustCompile(`^00-[0-9a-f]{32}-[0-9a-f]{16}-01$`).MatchString(tp) {
		t.Fatalf("traceparent %q", tp)
	}
	if got, parent, err := ParseTraceRef(tp); err != nil || got != id || parent != span {
		t.Fatalf("parse traceparent: %q %q %v", got, parent, err)
	}
	if got, parent, err := ParseTraceRef(strings.ToUpper(id)); err != nil || got != id || parent != "" {
		t.Fatalf("parse id: %q %q %v", got, parent, err)
	}
	for _, bad := range []string{"", "example", id[:31], "01-" + id + "-" + span + "-01"} {
		if _, _, err := ParseTraceRef(bad); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
}

func TestStampTraceNewOrContinue(t *testing.T) {
	clearTraceEnv(t)
	caller := Caller{Session: "foh-test", Harness: "claude", HarnessSession: "caller-session", Host: "laptop"}

	// A plain dispatch starts a new trace with a root intake ask.
	var fresh Meta
	if err := StampTrace(&fresh, TraceStart{Caller: caller, Text: "Build the thing"}); err != nil {
		t.Fatal(err)
	}
	root := RootAsk(fresh)
	if !ValidTraceID(fresh.TraceID) || root == nil || root.Action != "dispatch" || root.ParentSpanID != "" || fresh.ParentSpanID != root.SpanID || root.Caller.HarnessSession != "caller-session" || root.Text != "Build the thing" {
		t.Fatalf("new trace: %+v", fresh)
	}

	// --trace continues it with a follow-up ask instead of minting a trace.
	var next Meta
	if err := StampTrace(&next, TraceStart{Ref: fresh.TraceID, Caller: caller}); err != nil {
		t.Fatal(err)
	}
	if next.TraceID != fresh.TraceID || RootAsk(next) != nil || len(next.Asks) != 1 || next.Asks[0].Kind != AskFollowUp || next.Asks[0].Action != "continue" {
		t.Fatalf("continued trace: %+v", next)
	}
	// A traceparent ref also names the span the ask continues from.
	var linked Meta
	if err := StampTrace(&linked, TraceStart{Ref: Traceparent(fresh.TraceID, root.SpanID)}); err != nil {
		t.Fatal(err)
	}
	if linked.TraceID != fresh.TraceID || linked.Asks[0].ParentSpanID != root.SpanID {
		t.Fatalf("traceparent ref: %+v", linked)
	}
	if err := StampTrace(&Meta{}, TraceStart{Ref: "not-a-trace"}); err == nil {
		t.Fatal("accepted an invalid --trace")
	}

	// A launch from inside a traced worker inherits that worker's context.
	workerSpan := NewSpanID()
	t.Setenv("TRACEPARENT", Traceparent(fresh.TraceID, workerSpan))
	var inherited Meta
	if err := StampTrace(&inherited, TraceStart{Orchestrator: "release-train"}); err != nil {
		t.Fatal(err)
	}
	if inherited.TraceID != fresh.TraceID || inherited.Asks[0].ParentSpanID != workerSpan || inherited.Untraced != "" {
		t.Fatalf("inherited: %+v", inherited)
	}

	// An orchestrator launch without context is untraced, never invented.
	t.Setenv("TRACEPARENT", "")
	orch := Meta{Order: Order{Name: "follow-on"}}
	if err := StampTrace(&orch, TraceStart{Orchestrator: "release-train"}); err != nil {
		t.Fatal(err)
	}
	if orch.TraceID != "" || len(orch.Asks) != 0 || !strings.Contains(orch.Untraced, "release-train") {
		t.Fatalf("orchestrator: %+v", orch)
	}
}

func TestTakeTraceKey(t *testing.T) {
	id := NewTraceID()
	text := strings.Replace(good, "repo: kit", "repo: kit\ntrace: "+id+" # continue", 1)
	if _, err := Parse(text); err != nil {
		t.Fatalf("Parse rejects trace: %v", err)
	}
	stripped, ref, err := TakeTraceKey(text)
	if err != nil || ref != id || strings.Contains(stripped, "trace:") {
		t.Fatalf("%q %q %v", stripped, ref, err)
	}
	if o, err := Parse(stripped); err != nil || o.Name != "example" {
		t.Fatal(o, err)
	}
	if _, _, err := TakeTraceKey(strings.Replace(good, "repo: kit", "repo: kit\ntrace: nope", 1)); err == nil {
		t.Fatal("accepted invalid trace key")
	}
	if _, err := Parse(strings.Replace(good, "repo: kit", "repo: kit\ntrace: nope", 1)); err == nil {
		t.Fatal("Parse accepted invalid trace key")
	}
	if got, ref, err := TakeTraceKey(good); err != nil || got != good || ref != "" {
		t.Fatal(got, ref, err)
	}
}

func TestOrderAskText(t *testing.T) {
	got := OrderAskText("---\nname: x\nrepo: y\n---\nBuild the trace view.\n\nMore detail.\n\n## Acceptance\n- done\n")
	if got != "Build the trace view.\n\nMore detail." {
		t.Fatalf("%q", got)
	}
}

// A record written by this version decodes with the previous release's Meta
// and Result (no trace fields), and a record without trace fields decodes
// here as untraced.
func TestTraceRecordCompatibility(t *testing.T) {
	type previousMeta struct {
		RequestedBy    string   `json:"requested_by,omitempty"`
		Sender         string   `json:"sender,omitempty"`
		HarnessWrapper []string `json:"harness_wrapper"`
		Parent         string   `json:"parent,omitempty"`
		Order
		Branch string    `json:"branch"`
		Start  time.Time `json:"start"`
	}
	m := Meta{Sender: "local:test", Order: Order{Name: "example", Repo: "kit", Harness: "codex", Timeout: 5}, Branch: "d/example", Start: time.Now().UTC()}
	if err := StampTrace(&m, TraceStart{Text: "ask"}); err != nil {
		t.Fatal(err)
	}
	beginAttempt(&m, "")
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var old previousMeta
	if err := json.Unmarshal(b, &old); err != nil || old.Name != "example" || old.Branch != "d/example" {
		t.Fatalf("previous reader: %+v %v", old, err)
	}
	// The previous Request decoder also accepts a request carrying a caller.
	type previousRequest struct {
		Action string       `json:"action"`
		Name   string       `json:"name"`
		Note   string       `json:"note,omitempty"`
		Meta   previousMeta `json:"meta"`
	}
	b, _ = json.Marshal(Request{Action: "rework", Name: "example", Note: "again", Meta: m, Caller: &Caller{Session: "foh"}})
	var oldRequest previousRequest
	if err := json.Unmarshal(b, &oldRequest); err != nil || oldRequest.Note != "again" {
		t.Fatalf("previous request reader: %+v %v", oldRequest, err)
	}
	// Previous-release bytes decode here as untraced.
	b, _ = json.Marshal(old)
	var current Meta
	if err := json.Unmarshal(b, &current); err != nil || current.TraceID != "" || len(current.Asks) != 0 {
		t.Fatalf("current reader: %+v %v", current, err)
	}
	if env := traceEnvironment(current); slices.ContainsFunc(env, func(s string) bool { return strings.HasPrefix(s, "TRACEPARENT=") }) {
		t.Fatalf("untraced run got trace env: %v", env)
	}
}

func TestSanitizeTrace(t *testing.T) {
	m := Meta{TraceID: "bogus", SpanID: "x", Asks: []AskSpan{{SpanID: NewSpanID()}}}
	sanitizeTrace(&m)
	if m.TraceID != "" || m.Asks != nil || m.SpanID != "" {
		t.Fatalf("%+v", m)
	}
	m = Meta{TraceID: NewTraceID(), SpanID: NewSpanID(), ParentSpanID: "zz", Asks: []AskSpan{{SpanID: NewSpanID()}, {SpanID: "bad"}}}
	sanitizeTrace(&m)
	if m.SpanID != "" || m.ParentSpanID != "" || len(m.Asks) != 1 {
		t.Fatalf("%+v", m)
	}
}

// traceEnvFixture is a traced run whose fake harness records its environment.
func traceEnvFixture(t *testing.T, name string) (Meta, string) {
	t.Helper()
	clearTraceEnv(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	bin, dump := t.TempDir(), t.TempDir()
	installEnvDump(t, bin, "claude")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DISPATCH_ENV_DUMP", dump)
	// A stale context in the supervisor's own environment must not leak.
	t.Setenv("TRACEPARENT", Traceparent(NewTraceID(), NewSpanID()))
	t.Setenv("HEV_TRACE_ID", NewTraceID())
	cfg := filepath.Join(home, "dispatch.toml")
	if e := os.WriteFile(cfg, []byte("[hosts.here]\nlocal = true\n"), 0600); e != nil {
		t.Fatal(e)
	}
	dir := filepath.Join(Root(), name)
	if e := os.MkdirAll(filepath.Join(dir, "wt"), 0700); e != nil {
		t.Fatal(e)
	}
	m := Meta{Start: time.Now().UTC(), ConfigPath: cfg, LaunchResolved: true, Order: Order{Name: name, Repo: "app", Harness: "claude", Timeout: 1}}
	if err := StampTrace(&m, TraceStart{Text: "ask"}); err != nil {
		t.Fatal(err)
	}
	beginAttempt(&m, "")
	if e := WriteJSON(filepath.Join(dir, "meta.json"), m); e != nil {
		t.Fatal(e)
	}
	return m, dump
}

// testWorkerTraceEnv runs the shared supervisor launch, the one both the
// Unix tmux and Windows detached-process paths execute, and checks the
// harness environment it produced.
func testWorkerTraceEnv(t *testing.T) {
	m, dump := traceEnvFixture(t, "trace-env")
	allowChildProbes(t)
	_ = superviseRound(m.Name)
	env := readEnvDump(t, dump, "claude")
	want := []string{"TRACEPARENT=" + Traceparent(m.TraceID, m.SpanID), "HEV_TRACE_ID=" + m.TraceID, "HEV_SPAN_ID=" + m.SpanID, "HEV_RUN=" + m.Name}
	for _, w := range want {
		if !slices.Contains(env, w) {
			t.Fatalf("worker env lacks %s", w)
		}
	}
	count := 0
	for _, e := range env {
		if k, _, _ := strings.Cut(e, "="); strings.EqualFold(k, "TRACEPARENT") || strings.EqualFold(k, "HEV_TRACE_ID") {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("worker env has %d trace variables, want exactly one of each", count)
	}
	var r Result
	if err := readJSON(filepath.Join(Root(), m.Name, "result.json"), &r); err != nil || r.TraceID != m.TraceID || r.SpanID != m.SpanID {
		t.Fatalf("result trace: %+v %v", r, err)
	}
}

func TestWorkerTraceEnv(t *testing.T) { testWorkerTraceEnv(t) }

func TestWorkerEnvironmentUntraced(t *testing.T) {
	env := workerEnvironment(Meta{Order: Order{Name: "old"}}, []string{"PATH=/bin", "TRACEPARENT=00-" + NewTraceID() + "-" + NewSpanID() + "-01"}, nil)
	if hasEnv(env, "TRACEPARENT") || hasEnv(env, "HEV_TRACE_ID") || !slices.Contains(env, "HEV_RUN=old") || !hasEnv(env, "PATH") {
		t.Fatalf("%v", env)
	}
}
