package serve

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/index"
	"github.com/hev/kit/internal/trace"
	"github.com/hev/kit/internal/tracefixture"
)

func asksFixture(t *testing.T) (http.Handler, tracefixture.Info) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	info, err := tracefixture.Write(home, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	sessions, blocks, err := index.ReadSide(&trace.ClaudeSource{Root: info.Transcripts}, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	return New(&MemoryReader{Sessions: sessions, Blocks: blocks}).Handler(), info
}

func getAPI(t *testing.T, h http.Handler, path string, out any) json.RawMessage {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
	}
	if out != nil {
		if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
			t.Fatal(err)
		}
	}
	return json.RawMessage(w.Body.Bytes())
}

func TestAsksReadOneTraceAcrossOrders(t *testing.T) {
	h, info := asksFixture(t)
	var list struct {
		Traces []TraceSummary `json:"traces"`
	}
	getAPI(t, h, "/api/asks?window=all&pretrace=1", &list)
	if len(list.Traces) != 4 {
		t.Fatalf("traces: %+v", list.Traces)
	}
	var multi, untraced *TraceSummary
	for i := range list.Traces {
		switch list.Traces[i].ID {
		case info.Trace:
			multi = &list.Traces[i]
		case "run:" + tracefixture.OrderLegacy:
			untraced = &list.Traces[i]
		}
	}
	if multi == nil || len(multi.Orders) != 2 || multi.Runs != 4 || multi.Sessions != 4 || multi.Asks != 4 || !multi.RootHere || multi.Cost <= 0 || multi.State != "done" || len(multi.Hosts) != 2 || multi.Caller == nil || multi.Caller.Session != "foh-hev-3" ||
		strings.Join(multi.AskHosts, ",") != "laptop" || strings.Join(multi.Hosts, ",") != "mini,oven" || multi.User != "local:hev" || strings.Join(multi.Projects, ",") != "docs" {
		t.Fatalf("multi-order trace: %+v", multi)
	}
	if untraced == nil || !untraced.Untraced || !untraced.PreTrace || untraced.Reason != "" || untraced.Sessions != 1 || untraced.User != "local:hev" {
		t.Fatalf("pre-tracing run must be listed and labelled: %+v", untraced)
	}

	var detail struct {
		Trace TraceSummary `json:"trace"`
		Spans []Span       `json:"spans"`
	}
	getAPI(t, h, "/api/asks/"+info.Trace, &detail)
	kinds := map[string]int{}
	byID := map[string]Span{}
	for _, s := range detail.Spans {
		kinds[s.Kind]++
		byID[s.ID] = s
	}
	if kinds["ask"] != 4 || kinds["order"] != 2 || kinds["run"] != 3 || kinds["validation"] != 1 || kinds["session"] != 4 {
		t.Fatalf("span kinds: %v", kinds)
	}
	for _, s := range detail.Spans {
		switch s.Kind {
		case "session":
			if run := byID[s.Parent]; run.Kind != "run" && run.Kind != "validation" || !s.Indexed {
				t.Fatalf("session %s parent %+v", s.SessionID, run)
			}
		case "run", "validation":
			if byID[s.Parent].Kind != "order" || byID[s.LaunchedBy].Kind != "ask" {
				t.Fatalf("attempt %+v", s)
			}
		}
	}
	if v := byID["c1c1c1c1c1c1c1c1"]; v.Kind != "validation" || v.State != "pass" || v.Action != "validate" || byID[v.Parent].Name != tracefixture.OrderNav {
		t.Fatalf("validation span: %+v", v)
	}
	if r := byID["b2b2b2b2b2b2b2b2"]; r.Attempt != 2 || r.Action != "rework" {
		t.Fatalf("rework attempt: %+v", r)
	}
	// The detail panel's content comes from the run records: each attempt's
	// own report (the first one archived by the rework), the order's intent
	// and Acceptance, and the validation verdict.
	if r := byID["b1b1b1b1b1b1b1b1"]; !strings.Contains(r.Report, "Regrouped the sidebar") || strings.Contains(r.Report, "rework") || r.Outcome != "done" || r.Branch != "d/"+tracefixture.OrderNav || r.Head == "" || r.Queued == 0 {
		t.Fatalf("attempt 1 detail: %+v", r)
	}
	if r := byID["b2b2b2b2b2b2b2b2"]; !strings.Contains(r.Report, "(rework)") || r.PR == "" {
		t.Fatalf("attempt 2 detail: %+v", r)
	}
	if v := byID["c1c1c1c1c1c1c1c1"]; v.Verdict == nil || v.Verdict.Verdict != "pass" || len(v.Verdict.Items) != 3 || !strings.Contains(v.Report, "Verdict: pass") {
		t.Fatalf("validation detail: %+v", v)
	}
	for _, s := range detail.Spans {
		if s.Kind == "order" && s.Name == tracefixture.OrderNav && (!strings.Contains(s.Intent, "two clicks") || strings.Contains(s.Intent, "## Acceptance") || len(s.Acceptance) != 3 || s.Revisions != 1) {
			t.Fatalf("order detail: %+v", s)
		}
		// A session is never labelled by the dispatch brief.
		if s.Kind == "session" && strings.HasPrefix(s.Summary, "Read ../order.md") {
			t.Fatalf("session summary is the dispatch brief: %+v", s)
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/asks/"+"ffffffffffffffffffffffffffffffff", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown trace: %d", w.Code)
	}

	// A worker session leads back to its trace, launching ask and order.
	var session struct {
		TraceContext *TraceContext `json:"trace_context"`
	}
	getAPI(t, h, "/api/session/11111111-0000-4000-8000-000000000003", &session)
	c := session.TraceContext
	if c == nil || c.Trace != info.Trace || c.Launch != "validate" || c.LaunchSpan != "a3a3a3a3a3a3a3a3" || c.Order != tracefixture.OrderNav || c.Run != tracefixture.Validation || c.RunSpan != "c1c1c1c1c1c1c1c1" || c.Kind != "validation" {
		t.Fatalf("trace context: %+v", c)
	}
	getAPI(t, h, "/api/session/11111111-0000-4000-8000-000000000006", &session)
	if session.TraceContext == nil || !session.TraceContext.Untraced || !session.TraceContext.PreTrace {
		t.Fatalf("pre-tracing context: %+v", session.TraceContext)
	}
	// Decode afresh: a reused struct would keep the last response's fields.
	session.TraceContext = nil
	getAPI(t, h, "/api/session/11111111-0000-4000-8000-000000000007", &session)
	if session.TraceContext == nil || !session.TraceContext.Untraced || session.TraceContext.PreTrace {
		t.Fatalf("deliberately untraced context: %+v", session.TraceContext)
	}
}

type asksList struct {
	Traces         []TraceSummary `json:"traces"`
	PreTraceHidden int            `json:"pretrace_hidden"`
	Users          []AskFacet     `json:"users"`
	Hosts          []AskFacet     `json:"hosts"`
	Projects       []AskFacet     `json:"projects"`
	Orders         any            `json:"orders"`
}

func (l asksList) ids() string {
	var out []string
	for _, t := range l.Traces {
		out = append(out, t.ID)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

func facetValues(fs []AskFacet) string {
	var out []string
	for _, f := range fs {
		out = append(out, fmt.Sprintf("%s=%d", f.Value, f.Count))
	}
	return strings.Join(out, ",")
}

// checkAsksFilters runs the list filters against a dashboard over the
// fixture, wherever its records come from (this host's disk or the archive).
func checkAsksFilters(t *testing.T, h http.Handler, info tracefixture.Info) {
	t.Helper()
	nav, other := info.Trace, info.OtherTrace
	pre, nightly := "run:"+info.PreTrace, "run:"+info.Deliberate
	ids := func(v ...string) string { sort.Strings(v); return strings.Join(v, ",") }
	var l asksList
	// Runs from before tracing are hidden by default, and counted.
	getAPI(t, h, "/api/asks?window=all", &l)
	if l.ids() != ids(nav, other, nightly) || l.PreTraceHidden != 1 || l.Orders != nil {
		t.Fatalf("default list: %s hidden %d orders %v", l.ids(), l.PreTraceHidden, l.Orders)
	}
	// The choices narrow real data: two users, asking and execution hosts,
	// two projects.
	if got := facetValues(l.Users); got != "alex=1,local:hev=2" {
		t.Fatalf("users: %s", got)
	}
	if got := facetValues(l.Hosts); got != "laptop=1,mini=2,oven=2,studio=1" {
		t.Fatalf("hosts: %s", got)
	}
	if got := facetValues(l.Projects); got != "docs=1,kit=2" {
		t.Fatalf("projects: %s", got)
	}
	for _, tr := range l.Traces {
		switch tr.ID {
		case other:
			if tr.User != "alex" || strings.Join(tr.AskHosts, ",") != "studio" || strings.Join(tr.Hosts, ",") != "oven" || strings.Join(tr.Projects, ",") != "kit" {
				t.Fatalf("teammate trace: %+v", tr)
			}
		case nightly:
			if !tr.Untraced || tr.PreTrace || tr.Reason != tracefixture.NightlyReason || len(tr.AskHosts) != 0 || strings.Join(tr.Hosts, ",") != "mini" || tr.User != "local:hev" {
				t.Fatalf("deliberately untraced run: %+v", tr)
			}
		}
	}
	getAPI(t, h, "/api/asks?window=all&pretrace=1", &l)
	if l.ids() != ids(nav, other, nightly, pre) || l.PreTraceHidden != 0 {
		t.Fatalf("with pre-tracing runs: %s", l.ids())
	}
	for _, c := range []struct{ query, want string }{
		{"user=alex", ids(other)},
		{"user=local:hev", ids(nav, nightly)},
		{"user=local:hev&pretrace=1", ids(nav, nightly, pre)},
		{"host=laptop", ids(nav)},        // asking host
		{"host=studio", ids(other)},      // asking host on another machine
		{"host=mini", ids(nav, nightly)}, // execution host, validation included
		{"host=oven", ids(nav, other)},   // execution host
		{"host=studio&host=laptop", ids(nav, other)},
		{"project=kit", ids(other, nightly)},
		{"project=docs", ids(nav)},
		{"project=kit&user=local:hev", ids(nightly)},
		{"project=kit&host=oven", ids(other)},
		{"user=alex&project=docs", ""},
		{"user=local:hev&host=oven&project=docs", ids(nav)},
	} {
		getAPI(t, h, "/api/asks?window=all&"+c.query, &l)
		if l.ids() != c.want {
			t.Fatalf("%s: got %s want %s", c.query, l.ids(), c.want)
		}
	}
	// Each filter's choices are counted with the other filters applied, and
	// a chosen value stays offered.
	getAPI(t, h, "/api/asks?window=all&project=kit&user=nobody", &l)
	if got := facetValues(l.Users); got != "alex=1,local:hev=1,nobody=0" {
		t.Fatalf("users under project=kit: %s", got)
	}
	if got := facetValues(l.Projects); got != "kit=0" {
		t.Fatalf("projects under user=nobody: %s", got)
	}
}

func TestAsksListFilters(t *testing.T) {
	h, info := asksFixture(t)
	checkAsksFilters(t, h, info)
}

// A dispatch root with no records, or one written before tracing, still serves.
func TestAsksEmptyAndPreTraceRecords(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	h := New(&MemoryReader{}).Handler()
	var list struct {
		Traces []TraceSummary `json:"traces"`
	}
	getAPI(t, h, "/api/asks", &list)
	if len(list.Traces) != 0 {
		t.Fatal(list.Traces)
	}
	dir := filepath.Join(home, "dispatch", "old-run")
	os.MkdirAll(dir, 0o700)
	os.WriteFile(filepath.Join(dir, "meta.json"), []byte(`{"name":"old-run","host":"oven","repo":"kit","base":"main","harness":"codex","validation":"none","timeout":5,"branch":"d/old-run","start":"`+time.Now().UTC().Format(time.RFC3339)+`","harness_wrapper":null}`), 0o600)
	os.WriteFile(filepath.Join(dir, "result.json"), []byte(`{"exit_code":0,"start":"`+time.Now().UTC().Format(time.RFC3339)+`","end":"`+time.Now().UTC().Format(time.RFC3339)+`","branch":"d/old-run","head_sha":"x","timeout":false,"session_id":"old-session"}`), 0o600)
	// A record with no trace fields and no reason is from before tracing:
	// hidden by default, counted, and listed on request.
	var l asksList
	getAPI(t, h, "/api/asks", &l)
	if len(l.Traces) != 0 || l.PreTraceHidden != 1 {
		t.Fatalf("%+v", l)
	}
	getAPI(t, h, "/api/asks?pretrace=1", &list)
	if len(list.Traces) != 1 || !list.Traces[0].Untraced || !list.Traces[0].PreTrace || list.Traces[0].ID != "run:old-run" {
		t.Fatalf("%+v", list.Traces)
	}
}

func TestAsksViewRendersFixture(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	h, info := asksFixture(t)
	api := map[string]json.RawMessage{
		"/api/asks/" + info.Trace:                      getAPI(t, h, "/api/asks/"+info.Trace, nil),
		"/api/asks/run%3A" + tracefixture.OrderLegacy:  getAPI(t, h, "/api/asks/run:"+tracefixture.OrderLegacy, nil),
		"/api/asks/run%3A" + tracefixture.OrderNightly: getAPI(t, h, "/api/asks/run:"+tracefixture.OrderNightly, nil),
	}
	// The list requests the page makes as filters are chosen.
	for _, q := range []string{"", "&pretrace=1", "&user=alex", "&user=local%3Ahev", "&user=local%3Ahev&host=mini", "&user=local%3Ahev&host=mini&project=kit", "&user=alex&project=docs"} {
		api["/api/asks?window=30d"+q] = getAPI(t, h, "/api/asks?window=30d"+q, nil)
	}
	for id := range info.Sessions {
		api["/api/session/"+id] = getAPI(t, h, "/api/session/"+id, nil)
	}
	api["session"] = api["/api/session/11111111-0000-4000-8000-000000000003"]
	traceID, _ := json.Marshal(info.Trace)
	api["trace"] = traceID
	b, _ := json.Marshal(api)
	path := filepath.Join(t.TempDir(), "api.json")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(node, filepath.Join("testdata", "asks-view.cjs"))
	cmd.Env = append(os.Environ(), "TEMPLATE=template.html", "API="+path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	t.Log(string(out))
}

// TestAsksLayoutInBrowser drives the Asks list and trace page in a real
// headless browser at 390px and 1440px: no sideways scrolling with the
// filters shown, the filters narrow the list and round-trip through the URL,
// and both labels render. It needs Playwright, so it runs only when
// PLAYWRIGHT names a playwright module (CHROMIUM may name the browser, and
// ASKS_LAYOUT_OUT a directory for screenshots).
func TestAsksLayoutInBrowser(t *testing.T) {
	module := os.Getenv("PLAYWRIGHT")
	if module == "" {
		t.Skip("PLAYWRIGHT is not set")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	h, info := asksFixture(t)
	srv := httptest.NewServer(h)
	defer srv.Close()
	cmd := exec.Command(node, filepath.Join("testdata", "asks-layout.cjs"))
	cmd.Env = append(os.Environ(), "BASE="+srv.URL+"/", "TRACE="+info.Trace, "OUT="+os.Getenv("ASKS_LAYOUT_OUT"))
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatal(err)
	}
}
