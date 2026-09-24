package serve

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
)

// wireFixture evaluates only the store operations used by these tests. Search
// scores are authored fixtures, not a substitute for Layer relevance testing.
type wireFixture struct {
	mu      sync.Mutex
	rows    map[string][]map[string]any
	filters []any
}

func mapOf(v any) map[string]any {
	raw, _ := json.Marshal(v)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}
func matches(row map[string]any, raw any) bool {
	if raw == nil {
		return true
	}
	f := raw.([]any)
	op := f[0].(string)
	if op == "And" || op == "Or" {
		for _, clause := range f[1].([]any) {
			match := matches(row, clause)
			if op == "And" && !match {
				return false
			}
			if op == "Or" && match {
				return true
			}
		}
		return op == "And"
	}
	value := row[op]
	target := f[2]
	op = f[1].(string)
	switch op {
	case "Eq":
		return reflect.DeepEqual(value, target)
	case "In":
		for _, v := range target.([]any) {
			if reflect.DeepEqual(value, v) {
				return true
			}
		}
		return false
	case "ContainsAny":
		if list, ok := value.([]any); ok {
			for _, v := range list {
				for _, want := range target.([]any) {
					if reflect.DeepEqual(v, want) {
						return true
					}
				}
			}
		}
		return false
	}
	if value == nil {
		return false
	}
	cmp := 0
	if n, ok := value.(float64); ok {
		other := target.(float64)
		if n < other {
			cmp = -1
		}
		if n > other {
			cmp = 1
		}
	} else {
		cmp = strings.Compare(value.(string), target.(string))
	}
	switch op {
	case "Gt":
		return cmp > 0
	case "Gte":
		return cmp >= 0
	case "Lt":
		return cmp < 0
	case "Lte":
		return cmp <= 0
	}
	panic("unsupported fixture filter: " + op)
}
func (f *wireFixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v2/namespaces/"), "/query")
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if upserts, ok := body["upsert_rows"].([]any); ok {
		for _, raw := range upserts {
			row := raw.(map[string]any)
			found := false
			for i, old := range f.rows[name] {
				if old["id"] == row["id"] {
					if matches(old, resolveNew(body["upsert_condition"], row)) {
						f.rows[name][i] = row
					}
					found = true
					break
				}
			}
			if !found {
				f.rows[name] = append(f.rows[name], row)
			}
		}
		writeJSON(w, map[string]any{"status": "OK", "rows_upserted": len(upserts)})
		return
	}
	if _, ok := f.rows[name]; !ok {
		http.Error(w, "namespace not found", 404)
		return
	}
	search := false
	query := ""
	if queries, ok := body["queries"].([]any); ok {
		search = true
		body = queries[1].(map[string]any)
		query = body["rank_by"].([]any)[2].(string)
	}
	f.filters = append(f.filters, body["filters"])
	rows := []map[string]any{}
	for _, row := range f.rows[name] {
		if matches(row, body["filters"]) && (!search || strings.Contains(strings.ToLower(fmt.Sprint(row["text"])), strings.ToLower(query))) {
			copy := map[string]any{}
			for k, v := range row {
				copy[k] = v
			}
			rows = append(rows, copy)
		}
	}
	rank := body["rank_by"].([]any)
	sort.Slice(rows, func(i, j int) bool {
		if search {
			return rows[i]["$dist"].(float64) > rows[j]["$dist"].(float64)
		}
		key := rank[0].(string)
		a, b := fmt.Sprint(rows[i][key]), fmt.Sprint(rows[j][key])
		if rank[1] == "desc" {
			return a > b
		}
		return a < b
	})
	if top := int(body["top_k"].(float64)); len(rows) > top {
		rows = rows[:top]
	}
	for _, row := range rows {
		if included, ok := body["include_attributes"].([]any); ok {
			keep := map[string]bool{"id": true, "$dist": true}
			for _, key := range included {
				keep[key.(string)] = true
			}
			for key := range row {
				if !keep[key] {
					delete(row, key)
				}
			}
		}
		if excluded, ok := body["exclude_attributes"].([]any); ok {
			for _, key := range excluded {
				delete(row, key.(string))
			}
		}
	}
	if search {
		writeJSON(w, map[string]any{"results": []any{map[string]any{"rows": rows}}})
	} else {
		writeJSON(w, map[string]any{"rows": rows})
	}
}
func dashboardFixture(t *testing.T) (*Server, *wireFixture) {
	t.Helper()
	now := time.Now().UTC()
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	base := midnight.Add(-48 * time.Hour).UnixMilli()
	f := &wireFixture{rows: map[string][]map[string]any{"test": {}, "test-sessions": {}, "test-blocks": {}, "test-evals": {}}}
	for i := 0; i < 6; i++ {
		id := fmt.Sprintf("trace-%d", i)
		project := "alpha"
		model := "model-a"
		host := "laptop"
		harness := "claude_code"
		tool := "Bash"
		if i%2 == 1 {
			project = "beta"
			model = "model-b"
			host = "mini"
			harness = "codex"
			tool = "Read"
		}
		start := base + int64(i%3)*86400000 + 12*3600000
		row := trace.SessionRow{ID: id, SessionID: id, Summary: fmt.Sprintf("Fixture %d: verify preflight behavior", i), FirstPrompt: "Check the preflight", FirstPromptShort: trace.ShortPrompt("Check the preflight"), RepoURL: "https://github.com/example/" + project, Model: model, Host: host, Harness: harness, Start: start, End: start + int64(i+1)*60000, WallMS: int64(i+1) * 60000, APIMS: 30000, PromptCount: 2, PromptTS: []uint64{uint64(start), uint64(start + 1000)}, ToolCount: int64(i + 1), ToolNames: []string{tool}, ToolCounts: trace.ToolCounts{tool: i + 1}, InputTokens: int64(i+1) * 100, TotalTokens: int64(i+1) * 100, Cost: float64(i + 1)}
		f.rows["test-sessions"] = append(f.rows["test-sessions"], mapOf(row))
		f.rows["test"] = append(f.rows["test"], mapOf(layer.Hit{ID: "chunk-" + id, SessionID: id, TurnUUID: "assistant-" + id, Text: "preflight transcript matching snippet", Role: "assistant", Dist: float64(10-i) / 100}))
		f.rows["test-blocks"] = append(f.rows["test-blocks"], mapOf(trace.BlockRow{ID: "prompt-" + id, SessionID: id, TurnUUID: "user-" + id, Role: "user", BlockType: "text", Seq: 0, Text: "Check the preflight", Start: start, End: start}), mapOf(trace.BlockRow{ID: "reply-" + id, SessionID: id, TurnUUID: "assistant-" + id, Role: "assistant", BlockType: "text", Seq: 1, Text: "preflight transcript matching snippet", Start: start, End: start + 1000, Model: model, RequestID: "request-" + id, InputTokens: 100}))
	}
	// Legacy duplicates have different mutable metadata, so filtering must select
	// only the newest physical row, not just fold whichever rows matched.
	old := mapOf(trace.SessionRow{ID: "legacy", SessionID: "trace-0", End: base, Cost: .1, RepoURL: "https://github.com/example/stale", Model: "retired", PromptCount: 1})
	f.rows["test-sessions"] = append(f.rows["test-sessions"], old)
	server := httptest.NewServer(f)
	t.Cleanup(server.Close)
	cl := layer.New(server.URL, "fixture", "test", "")
	evals := []trace.Eval{
		{Session: "trace-0", TS: now.Add(-time.Hour).Format(time.RFC3339), Role: "reviewer", Instance: "nightly", Host: "mini", Marks: map[string]int{"accuracy": 2, "clarity": 4, "coverage": 5, "evidence": 5}, Poor: true, Summary: "Needs stronger validation", Evidence: map[string]string{"accuracy": "Distinct evaluator phrase at Turn 1; assistant-trace-0"}, Findings: []string{"Add an empty-response regression test"}},
		{Session: "trace-1", TS: now.Add(-2 * time.Hour).Format(time.RFC3339), Role: "reviewer", Instance: "nightly", Marks: map[string]int{"accuracy": 1}, Poor: true},
		{Session: "trace-1", TS: now.Format(time.RFC3339), Role: "author", Instance: "manual", Marks: map[string]int{"accuracy": 5}, Poor: false},
	}
	if _, err := cl.WriteEvals(evals); err != nil {
		t.Fatal(err)
	}
	for _, row := range f.rows["test-evals"] {
		row["$dist"] = .2
	}
	return New(cl), f
}
func requestJSON(t *testing.T, s *Server, url string) map[string]any {
	t.Helper()
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest("GET", url, nil))
	if w.Code != 200 {
		t.Fatalf("%s: %d %s", url, w.Code, w.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}
func TestDashboardAcceptance(t *testing.T) {
	s, _ := dashboardFixture(t)
	all := requestJSON(t, s, "/api/sessions?window=7d")["sessions"].([]any)
	if len(all) != 6 {
		t.Fatalf("got %d rows", len(all))
	}
	ids := map[string]bool{}
	for _, raw := range all {
		r := raw.(map[string]any)
		id := r["id"].(string)
		if ids[id] {
			t.Fatal("duplicate", id)
		}
		ids[id] = true
		if _, present := r["prompt_ts"]; present {
			t.Fatal("list leaked prompt timestamps")
		}
	}
	t.Log("steps 1–2: rows=6 unique=6; list excludes prompt_ts")
	cases := []struct {
		query string
		want  int
	}{
		{"project=alpha&project=beta&model=model-a&tool=Bash&cost_min=1", 3},
		{"project=alpha&model=model-a&model=model-b&harness=claude_code&host=laptop&tool=Read&tool=Bash&tools_min=2&tools_max=5&tokens_min=200&tokens_max=500&cost_min=2&cost_max=5&wall_min=2&wall_max=5", 2},
		{"cost_max=0.5", 0}, {"model=retired", 0}, {"project=stale", 0}, {"project=unknown", 0},
		{"poor=true", 1}, {"poor=false", 5}, {"poor=false&project=alpha", 2}, {"poor=false&role=author", 1}, {"mark_accuracy_max=2", 1}, {"role=reviewer&instance=nightly", 1}, {"role=author&role=reviewer&instance=manual&instance=nightly", 2},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			list := requestJSON(t, s, "/api/sessions?window=all&"+tc.query)["sessions"].([]any)
			stats := requestJSON(t, s, "/api/stats?window=all&"+tc.query)
			if len(list) != tc.want || int(stats["traces"].(float64)) != tc.want {
				t.Fatalf("got list=%d stats=%v want=%d", len(list), stats["traces"], tc.want)
			}
			t.Logf("store-filtered list and stats count=%d", tc.want)
		})
	}
	hits := requestJSON(t, s, "/api/search?q=preflight&project=alpha")["sessions"].([]any)
	if len(hits) != 3 {
		t.Fatal(hits)
	}
	for _, raw := range hits {
		h := raw.(map[string]any)
		if h["project"] != "alpha" || h["snippet"] == "" || h["turn_uuid"] == "" {
			t.Fatal(h)
		}
	}
	if hits[0].(map[string]any)["id"] != "trace-0" {
		t.Fatal("store ranking changed", hits)
	}
	t.Log("step 4: 3 alpha search hits, each with snippet and turn UUID, descending Layer score")
	evalHits := requestJSON(t, s, "/api/search?q=Distinct%20evaluator%20phrase&poor=true")["sessions"].([]any)
	if len(evalHits) != 1 || evalHits[0].(map[string]any)["source"] != "eval" {
		t.Fatal(evalHits)
	}
	session := requestJSON(t, s, "/api/session/trace-0")
	evaluation := session["eval"].(map[string]any)
	if len(evaluation["marks"].(map[string]any)) != 4 || len(evaluation["findings"].([]any)) != 1 {
		t.Fatal(evaluation)
	}
	t.Log("step 6: newest eval only; poor trace has four marks, evidence, findings; evaluator phrase returns trace-0")
	for _, q := range []string{"cost_min=NaN", "tools_min=1.5", "wall_min=-1", "tokens_min=2&tokens_max=1", "since=nope", "poor=maybe", "mark_accuracy_max=nope"} {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/sessions?"+q, nil))
		if w.Code != 400 {
			t.Fatalf("invalid query %s: %d", q, w.Code)
		}
	}
}
func TestDashboardFixtureServer(t *testing.T) {
	addr := os.Getenv("HEV_DASHBOARD_FIXTURE_LISTEN")
	if addr == "" {
		t.Skip("set HEV_DASHBOARD_FIXTURE_LISTEN for the local browser fixture")
	}
	s, _ := dashboardFixture(t)
	t.Log("fixture listening on", addr)
	if err := http.ListenAndServe(addr, s.Handler()); err != nil {
		t.Fatal(err)
	}
}

func TestDashboardAllWindowPastTenThousand(t *testing.T) {
	s, f := dashboardFixture(t)
	for i := 0; i < 10000; i++ {
		id := fmt.Sprintf("large-%05d", i)
		f.rows["test-sessions"] = append(f.rows["test-sessions"], mapOf(trace.SessionRow{ID: id, SessionID: id, End: 1}))
	}
	list := requestJSON(t, s, "/api/sessions?window=all")["sessions"].([]any)
	stats := requestJSON(t, s, "/api/stats?window=all")
	if len(list) != 10006 || stats["traces"] != float64(10006) {
		t.Fatalf("list=%d stats=%v", len(list), stats["traces"])
	}
	t.Log("all window: 10006 unique sessions; stats traces=10006")
}
func TestDashboardDateBoundsAndEmptySearch(t *testing.T) {
	s, f := dashboardFixture(t)
	start := int64(f.rows["test-sessions"][0]["start"].(float64))
	q := fmt.Sprintf("window=all&since=%d&until=%d", start, start+86400000)
	if list := requestJSON(t, s, "/api/sessions?"+q)["sessions"].([]any); len(list) != 2 {
		t.Fatal(list)
	}
	if n := requestJSON(t, s, "/api/stats?"+q)["traces"]; n != float64(2) {
		t.Fatal(n)
	}
	hits := requestJSON(t, s, "/api/search?window=all&q=preflight&project=unknown")["sessions"].([]any)
	if len(hits) != 0 {
		t.Fatal(hits)
	}
	t.Log("since inclusive / until exclusive selects 2 traces; empty candidates produce no search hits")
}

func resolveNew(raw any, row map[string]any) any {
	if m, ok := raw.(map[string]any); ok {
		if key, ok := m["$ref_new"].(string); ok {
			return row[key]
		}
	}
	if a, ok := raw.([]any); ok {
		out := make([]any, len(a))
		for i, v := range a {
			out[i] = resolveNew(v, row)
		}
		return out
	}
	return raw
}
func TestDashboardReplayKeepsLatestState(t *testing.T) {
	s, f := dashboardFixture(t)
	cl := s.reader.(*layer.Client)
	old := trace.SessionRow{ID: "replay", SessionID: "replay", End: 1, Cost: 1}
	newer := old
	newer.End = 2
	newer.Cost = 5
	for _, row := range []trace.SessionRow{newer, old} {
		if _, err := cl.WriteSessions([]trace.SessionRow{row}); err != nil {
			t.Fatal(err)
		}
	}
	result := requestJSON(t, s, "/api/session/replay")
	if result["end"] != float64(2) || result["cost"] != float64(5) {
		t.Fatal(result)
	}
	newer.ToolNames = []string{"new-metadata"}
	if _, err := cl.WriteSessions([]trace.SessionRow{newer}); err != nil {
		t.Fatal(err)
	}
	detail := requestJSON(t, s, "/api/session/replay")
	if len(detail["tool_names"].([]any)) != 1 {
		t.Fatal("equal-end backfill was rejected")
	}
	e := f.rows["test-evals"][0]["ts"].(string)
	before := len(f.rows["test-evals"])
	if _, err := cl.WriteEvals([]trace.Eval{{Session: "trace-0", TS: e, Marks: map[string]int{"changed": 0}}}); err != nil {
		t.Fatal(err)
	}
	if len(f.rows["test-evals"]) != before || !strings.Contains(f.rows["test-evals"][0]["marks"].(string), "accuracy") {
		t.Fatal("eval replay mutated existing row")
	}
	t.Log("older session replay rejected; equal-end metadata backfill accepted; eval replay is insert-only")
}

// The initial identity scan may be slow; display selection must reach Layer
// before it completes. A legacy row matching the ceiling must still disappear
// when its newer physical row fails the same predicate.
func TestDashboardOverlapsSelectionWithoutResurrectingLegacyRows(t *testing.T) {
	_, fixture := dashboardFixture(t)
	selected := make(chan struct{})
	var once sync.Once
	wire := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		var query map[string]any
		if err := json.Unmarshal(body, &query); err != nil {
			t.Error(err)
		}
		if strings.Contains(r.URL.Path, "test-sessions") {
			if query["filters"] != nil {
				once.Do(func() { close(selected) })
			} else {
				select {
				case <-selected:
				case <-time.After(2 * time.Second):
					t.Error("display selection waited for the identity scan")
				}
			}
		}
		fixture.ServeHTTP(w, r)
	}))
	defer wire.Close()
	s := New(layer.New(wire.URL, "fixture", "test", ""))
	rows := requestJSON(t, s, "/api/sessions?window=30d&cost_max=0.5")["sessions"].([]any)
	if len(rows) != 0 {
		t.Fatalf("legacy row resurrected: %v", rows)
	}
}
