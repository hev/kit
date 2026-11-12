package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
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
		Orders []string       `json:"orders"`
	}
	getAPI(t, h, "/api/asks?window=all", &list)
	if len(list.Traces) != 3 {
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
	if multi == nil || len(multi.Orders) != 2 || multi.Runs != 4 || multi.Sessions != 4 || multi.Asks != 4 || !multi.RootHere || multi.Cost <= 0 || multi.State != "done" || len(multi.Hosts) != 2 || multi.Caller == nil || multi.Caller.Session != "foh-hev-3" {
		t.Fatalf("multi-order trace: %+v", multi)
	}
	if untraced == nil || !untraced.Untraced || untraced.Sessions != 1 {
		t.Fatalf("untraced run must be listed and labelled: %+v", untraced)
	}
	// The order filter narrows the list to traces containing that order.
	getAPI(t, h, "/api/asks?window=all&order="+tracefixture.OrderSearch, &list)
	if len(list.Traces) != 1 || list.Traces[0].ID != info.Trace {
		t.Fatalf("order filter: %+v", list.Traces)
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
	if c == nil || c.Trace != info.Trace || c.Launch != "validate" || c.Order != tracefixture.OrderNav || c.Run != tracefixture.Validation || c.Kind != "validation" {
		t.Fatalf("trace context: %+v", c)
	}
	getAPI(t, h, "/api/session/11111111-0000-4000-8000-000000000006", &session)
	if session.TraceContext == nil || !session.TraceContext.Untraced {
		t.Fatalf("untraced context: %+v", session.TraceContext)
	}
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
	getAPI(t, h, "/api/asks", &list)
	if len(list.Traces) != 1 || !list.Traces[0].Untraced || list.Traces[0].ID != "run:old-run" {
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
		"/api/asks?window=30d":                        getAPI(t, h, "/api/asks?window=30d", nil),
		"/api/asks/" + info.Trace:                     getAPI(t, h, "/api/asks/"+info.Trace, nil),
		"/api/asks/run%3A" + tracefixture.OrderLegacy: getAPI(t, h, "/api/asks/run:"+tracefixture.OrderLegacy, nil),
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
