package layer

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/hev/kit/internal/trace"
)

func capture(t *testing.T, reply string) (*Client, *map[string]any) {
	t.Helper()
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(b, &got); err != nil {
			t.Errorf("request body is not JSON: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, reply)
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, "k", "ns", ""), &got
}

// The embedding declaration is the one line that cannot be corrected later: a
// namespace created without it can never gain a vector column, and a namespace
// created with the wrong model can only be re-embedded by re-ingesting
// everything. It is asserted here so a refactor cannot quietly drop it.
func TestWriteDeclaresEmbeddingAndFullText(t *testing.T) {
	cl, got := capture(t, `{"status":"OK","rows_upserted":1,"performance":{"embedding_tokens":42}}`)

	res, err := cl.Write([]Row{{ID: "c1", Text: "hello"}})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if res.RowsUpserted != 1 || res.EmbeddingTokens != 42 {
		t.Errorf("result = %+v", res)
	}

	body := *got
	if body["distance_metric"] != "cosine_distance" {
		t.Errorf("distance_metric = %v; the store rejects a vector write without it", body["distance_metric"])
	}
	text, ok := body["schema"].(map[string]any)["text"].(map[string]any)
	if !ok {
		t.Fatalf("no text column in schema: %v", body["schema"])
	}
	if text["full_text_search"] != true {
		t.Error("text is not full-text indexed — the BM25 leg would have nothing to score")
	}
	embed, ok := text["embed"].(map[string]any)
	if !ok {
		t.Fatal("text carries no embed declaration — this namespace could never gain a vector")
	}
	if embed["model"] != DefaultModel {
		t.Errorf("embed model = %v, want %s", embed["model"], DefaultModel)
	}
}

// Hybrid is a multi-query fused server-side. The single-rank_by form shown in
// turbopuffer's embedding docs is rejected by the API; this test pins the form
// that actually works so nobody re-derives it from the docs.
func TestSearchIsMultiQueryFusedByRRF(t *testing.T) {
	cl, got := capture(t, `{"results":[{"rows":[{"id":"c1","$dist":0.03,"text":"found","harness":"codex"}]}]}`)

	hits, err := cl.Search("why did it fail", 5, nil)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != "c1" || hits[0].Text != "found" || hits[0].Harness != "codex" {
		t.Fatalf("hits = %+v", hits)
	}

	body := *got
	rerank, _ := body["rerank_by"].([]any)
	if len(rerank) != 1 || rerank[0] != "RRF" {
		t.Errorf("rerank_by = %v, want [RRF] — without it the two legs come back unfused", body["rerank_by"])
	}
	queries, _ := body["queries"].([]any)
	if len(queries) != 2 {
		t.Fatalf("queries = %d, want a semantic leg and a lexical leg", len(queries))
	}
	attrs := queries[0].(map[string]any)["include_attributes"].([]any)
	if !sliceContains(attrs, "harness") {
		t.Errorf("search does not request harness: %v", attrs)
	}

	ann := queries[0].(map[string]any)["rank_by"].([]any)
	if ann[0] != "text" || ann[1] != "ANN" {
		t.Errorf("first leg is not an ANN over text: %v", ann)
	}
	// The query text is embedded in the store; no vector crosses the wire.
	embed, ok := ann[2].([]any)
	if !ok || embed[0] != "Embed" || embed[1] != "why did it fail" {
		t.Errorf("semantic leg does not defer embedding to the store: %v", ann[2])
	}

	bm25 := queries[1].(map[string]any)["rank_by"].([]any)
	if bm25[0] != "text" || bm25[1] != "BM25" || bm25[2] != "why did it fail" {
		t.Errorf("second leg is not a BM25 over text: %v", bm25)
	}
}

func sliceContains(values []any, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// A filter has to reach both legs, or a scoped search silently returns
// unscoped semantic hits alongside scoped lexical ones.
func TestSearchFilterAppliesToBothLegs(t *testing.T) {
	cl, got := capture(t, `{"results":[{"rows":[]}]}`)
	if _, err := cl.Search("q", 5, []any{"plan", "Eq", "scoped-key-leak"}); err != nil {
		t.Fatalf("Search: %v", err)
	}
	for i, q := range (*got)["queries"].([]any) {
		if q.(map[string]any)["filters"] == nil {
			t.Errorf("leg %d carries no filter", i)
		}
	}
}

func TestListSessionsUsesGroupedAggregation(t *testing.T) {
	cl, got := capture(t, `{"aggregation_groups":[
		{"session_id":"s1","harness":"codex","workdir":"/w","ts":"2026-09-05T10:00:00Z","seq":0,"chunks":2},
		{"session_id":"s1","harness":"codex","workdir":"/w","ts":"2026-09-05T10:01:00Z","seq":1,"chunks":1},
		{"session_id":"s2","harness":"claude_code","workdir":"/x","ts":"2026-09-05T11:00:00Z","seq":0,"chunks":1}
	]}`)

	sessions, err := cl.ListSessions("2026-09-05T00:00:00Z", "")
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 2 || sessions[0].ID != "s2" || sessions[1].TurnCount != 2 {
		t.Fatalf("sessions = %+v", sessions)
	}
	body := *got
	if body["aggregate_by"] == nil || body["group_by"] == nil {
		t.Fatalf("not a grouped aggregation: %v", body)
	}
	if body["rank_by"] != nil || body["include_attributes"] != nil {
		t.Fatalf("aggregation accidentally became a ranked fetch: %v", body)
	}
}

func TestSessionRowsIsFilteredFetchWithoutEmbedding(t *testing.T) {
	cl, got := capture(t, `{"rows":[{"id":"c1","text":"hello","session_id":"abcdef","seq":1,"part":0}]}`)
	rows, err := cl.SessionRows("abc")
	if err != nil {
		t.Fatalf("SessionRows: %v", err)
	}
	if len(rows) != 1 || rows[0].SessionID != "abcdef" {
		t.Fatalf("rows = %+v", rows)
	}
	body := *got
	if body["queries"] != nil {
		t.Fatalf("fetch routed through hybrid Search: %v", body)
	}
	rank := body["rank_by"].([]any)
	if len(rank) != 2 {
		t.Fatalf("rank_by = %v", rank)
	}
	raw, _ := json.Marshal(body)
	if contains(string(raw), "Embed") || contains(string(raw), "BM25") {
		t.Fatalf("plain fetch would embed a query: %s", raw)
	}
}

func TestSessionRowsReadsNamespaceWithoutBlockCoordinate(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		b, _ := io.ReadAll(r.Body)
		if requests == 1 {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"error":"attribute \"block\" not found in schema"}`)
			return
		}
		if contains(string(b), `"block"`) {
			t.Errorf("compatibility retry still requested block: %s", b)
		}
		io.WriteString(w, `{"rows":[{"id":"c1","session_id":"old"}]}`)
	}))
	defer srv.Close()

	rows, err := New(srv.URL, "k", "ns", "").SessionRows("old")
	if err != nil || len(rows) != 1 || requests != 2 {
		t.Fatalf("rows=%+v requests=%d err=%v", rows, requests, err)
	}
}

func TestRowOfCarriesChunkCoordinates(t *testing.T) {
	r := RowOf(trace.Chunk{
		ID: "c1", Text: "t", SessionID: "s", TurnUUID: "u", Seq: 3, Block: 2, Part: 1,
		Tier: "text", Workdir: "/w", Harness: "claude_code",
	})
	if r.ID != "c1" || r.SessionID != "s" || r.Seq != 3 || r.Block != 2 || r.Part != 1 || r.Harness != "claude_code" {
		t.Errorf("row lost coordinates: %+v", r)
	}
	// Attribution is the caller's to supply; the parser never invents it.
	if r.Plan != "" || r.Instance != "" {
		t.Errorf("row invented factory attribution: %+v", r)
	}
}

func TestWriteSurfacesStoreErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"error":"💔 distance_metric must be specified"}`)
	}))
	defer srv.Close()

	_, err := New(srv.URL, "k", "ns", "").Write([]Row{{ID: "c1", Text: "x"}})
	if err == nil {
		t.Fatal("a 400 was reported as success")
	}
	// The store's own words are the useful part; a bare status code has sent
	// people looking in the wrong place before.
	if !contains(err.Error(), "distance_metric") {
		t.Errorf("error lost the store's message: %v", err)
	}
}

func TestReadSideSchemasHaveNoEmbedding(t *testing.T) {
	for name, schema := range map[string]map[string]any{"blocks": blockSchema(), "sessions": sessionSchema()} {
		for field, raw := range schema {
			if _, ok := raw.(map[string]any)["embed"]; ok {
				t.Fatalf("%s.%s unexpectedly embeds", name, field)
			}
		}
	}
	if blockSchema()["start"].(map[string]any)["type"] != "int" {
		t.Fatal("block start is not numeric")
	}
	if sessionSchema()["has_subagents"].(map[string]any)["type"] != "bool" {
		t.Fatal("has_subagents is not bool")
	}
}

func TestListSessionsUsesSessionsNamespace(t *testing.T) {
	cl, got := capture(t, `{"rows":[{"id":"row","session_id":"s","start":42}]}`)
	rows, err := cl.ListSessionRows(5, []any{"start", "Gte", 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].SessionID != "s" {
		t.Fatalf("rows=%+v", rows)
	}
	if (*got)["top_k"] != float64(5) || (*got)["filters"] == nil {
		t.Fatalf("query=%v", *got)
	}
}

func TestListBlockRowsUsesExactSessionAndSequence(t *testing.T) {
	cl, got := capture(t, `{"rows":[{"id":"b","session_id":"s","seq":2,"text":"whole"}]}`)
	rows, err := cl.ListBlockRows("s")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Text != "whole" {
		t.Fatalf("rows=%+v", rows)
	}
	if !reflect.DeepEqual((*got)["filters"], []any{"session_id", "Eq", "s"}) {
		t.Fatalf("filter=%#v", (*got)["filters"])
	}
	if !reflect.DeepEqual((*got)["rank_by"], []any{"seq", "asc"}) {
		t.Fatalf("rank_by=%#v", (*got)["rank_by"])
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestSessionWritePreservesLongDisplayText(t *testing.T) {
	cl, got := capture(t, `{"rows_upserted":1}`)
	prompt := strings.Repeat("long prompt ", 3000)
	_, err := cl.WriteSessions([]trace.SessionRow{{ID: "s", FirstPrompt: prompt, Summary: prompt}})
	if err != nil {
		t.Fatal(err)
	}
	body := *got
	for _, field := range []string{"first_prompt", "summary"} {
		if body["schema"].(map[string]any)[field].(map[string]any)["filterable"] != false {
			t.Fatalf("%s must not be filterable", field)
		}
		if body["upsert_rows"].([]any)[0].(map[string]any)[field] != prompt {
			t.Fatalf("%s was truncated", field)
		}
	}
}

func TestSessionToolCountsWireRoundTrip(t *testing.T) {
	cl, got := capture(t, `{"rows_upserted":1}`)
	_, err := cl.WriteSessions([]trace.SessionRow{{ID: "s", ToolCounts: trace.ToolCounts{"Read": 3, "Bash": 2}}})
	if err != nil {
		t.Fatal(err)
	}
	row := (*got)["upsert_rows"].([]any)[0].(map[string]any)
	raw, ok := row["tool_counts"].(string)
	if !ok {
		t.Fatalf("Layer attribute must be a string: %v", row)
	}
	cl, _ = capture(t, `{"rows":[{"id":"s","tool_counts":`+strconv.Quote(raw)+`}]}`)
	rows, err := cl.ListSessionRows(1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].ToolCounts["Read"] != 3 || rows[0].ToolCounts["Bash"] != 2 {
		t.Fatal(rows)
	}
}

func TestSessionIndexProjectionPreservesIdentityAndFacets(t *testing.T) {
	cl, got := capture(t, `{"rows":[{"id":"legacy","session_id":"trace","end":42,"repo_url":"repo","tool_names":["Bash"]}]}`)
	rows, err := cl.ListSessionIndexRows(-1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != "legacy" || rows[0].SessionID != "trace" || rows[0].End != 42 || len(rows[0].ToolNames) != 1 {
		t.Fatalf("identity/facets lost: %+v", rows)
	}
	want := []any{"session_id", "end", "repo_url", "model", "harness", "host", "tool_names"}
	if !reflect.DeepEqual((*got)["include_attributes"], want) {
		t.Fatalf("projection: %v", *got)
	}
}

func TestEvalMarksProjectionKeepsGradeWithoutProse(t *testing.T) {
	cl, got := capture(t, `{"rows":[{"id":"grade","session_id":"trace","marks":"{\"outcome\":3}","poor":true,"role":"reviewer"}]}`)
	rows, err := cl.ListEvalMarks(nil)
	if err != nil {
		t.Fatal(err)
	}
	e, err := rows[0].Eval()
	if err != nil || e.Marks["outcome"] != 3 || !e.Poor || e.Role != "reviewer" {
		t.Fatalf("grade lost: %+v %v", e, err)
	}
	want := []any{"session_id", "ts", "role", "instance", "host", "poor", "marks"}
	if !reflect.DeepEqual((*got)["include_attributes"], want) {
		t.Fatalf("projection: %v", *got)
	}
	if _, err := cl.ListEvalRows(nil); err != nil {
		t.Fatal(err)
	}
	attrs := (*got)["include_attributes"].([]any)
	for _, required := range []string{"text", "summary", "evidence", "findings"} {
		found := false
		for _, attr := range attrs {
			if attr == required {
				found = true
			}
		}
		if !found {
			t.Errorf("detail lost %s", required)
		}
	}
}

func TestSearchPhrasingsFusesEveryLegInOneRequest(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var body struct {
			Queries  []map[string]any `json:"queries"`
			RerankBy []any            `json:"rerank_by"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if len(body.Queries) != 6 || len(body.RerankBy) != 1 || body.RerankBy[0] != "RRF" {
			t.Errorf("want 6 legs fused by RRF, got %d legs rerank_by=%v", len(body.Queries), body.RerankBy)
		}
		for _, q := range body.Queries {
			if q["filters"] == nil {
				t.Errorf("leg lost the filter: %v", q)
			}
		}
		io.WriteString(w, `{"results":[{"rows":[{"id":"c1","session_id":"s"}]}]}`)
	}))
	defer srv.Close()

	hits, err := New(srv.URL, "k", "ns", "").SearchPhrasings([]string{"a", "b", "c"}, 5, []any{"harness", "Eq", "codex"})
	if err != nil || len(hits) != 1 || requests != 1 {
		t.Fatalf("hits=%+v requests=%d err=%v", hits, requests, err)
	}
	if _, err := New(srv.URL, "k", "ns", "").SearchPhrasings(make([]string, MaxPhrasings+1), 5, nil); err == nil {
		t.Error("more than MaxPhrasings phrasings should be refused")
	}
}
