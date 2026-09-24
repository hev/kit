package layer

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hev/kit/internal/trace"
)

// The legacy* builders are frozen copies of the chunk schema, eval text field
// and search body as they stood on main at 2fc2c07, before the capability
// seam. A hosted config must still put exactly these bytes on the wire.
func legacySchema(model string) map[string]any {
	str := func(fts bool) map[string]any {
		m := map[string]any{"type": "string"}
		if fts {
			m["full_text_search"] = true
		}
		return m
	}
	return map[string]any{
		"text": map[string]any{
			"type":             "string",
			"full_text_search": true,
			"embed":            map[string]any{"model": model},
		},
		"session_id": str(false), "turn_uuid": str(false), "parent_uuid": str(false),
		"seq": map[string]any{"type": "int"}, "block": map[string]any{"type": "int"},
		"part": map[string]any{"type": "int"},
		"ts":   str(false), "role": str(false), "block_type": str(false),
		"tier": str(false), "tool_name": str(false),
		"workdir": str(true), "branch": str(false), "harness": str(false),
		"source_path": str(false), "host": str(false),
		"instance": str(false), "plan": str(false), "rfc": str(false),
		"issue": str(false), "pr": str(false),
	}
}

func legacySearchBody(query string, topK int, filter any, attrs []string) map[string]any {
	leg := func(rankBy any) map[string]any {
		q := map[string]any{"rank_by": rankBy, "top_k": topK, "include_attributes": attrs}
		if filter != nil {
			q["filters"] = filter
		}
		return q
	}
	return map[string]any{
		"queries": []map[string]any{
			leg([]any{"text", "ANN", []any{"Embed", query}}),
			leg([]any{"text", "BM25", query}),
		},
		"rerank_by": []any{"RRF"},
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// capture serves canned replies and records every request body by path.
func captureAll(t *testing.T, reply string) (*httptest.Server, *[]string, *[]string) {
	t.Helper()
	var paths, bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		paths = append(paths, r.URL.Path)
		bodies = append(bodies, string(raw))
		io.WriteString(w, reply)
	}))
	t.Cleanup(srv.Close)
	return srv, &paths, &bodies
}

func TestHostedWireIsByteIdenticalToMain(t *testing.T) {
	filter := []any{"plan", "Eq", "scoped-key-leak"}
	for _, kind := range []string{"", StoreTurbopuffer} {
		srv, _, bodies := captureAll(t, `{"status":"OK","results":[{"rows":[]}]}`)
		cl, err := New(srv.URL, "k", "ns", "").WithStore(kind)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := mustJSON(t, cl.schema()), mustJSON(t, legacySchema(DefaultModel)); got != want {
			t.Fatalf("kind %q schema\n got %s\nwant %s", kind, got, want)
		}
		if _, err := cl.Search("why did it fail", 5, filter); err != nil {
			t.Fatal(err)
		}
		attrs := []string{"text", "session_id", "turn_uuid", "ts", "role", "block_type", "harness", "workdir", "plan", "pr"}
		if got, want := (*bodies)[0], mustJSON(t, legacySearchBody("why did it fail", 5, filter, attrs)); got != want {
			t.Fatalf("kind %q search body\n got %s\nwant %s", kind, got, want)
		}

		rows := []Row{{ID: "a", Text: "t"}}
		if _, err := cl.Write(rows); err != nil {
			t.Fatal(err)
		}
		want := mustJSON(t, map[string]any{"upsert_rows": rows, "distance_metric": "cosine_distance", "schema": legacySchema(DefaultModel)})
		if got := (*bodies)[1]; got != want {
			t.Fatalf("kind %q write body\n got %s\nwant %s", kind, got, want)
		}

		if _, err := cl.WriteEvals([]trace.Eval{{Session: "s", TS: "2026-09-01T00:00:00Z", Marks: map[string]int{"outcome": 2}}}); err != nil {
			t.Fatal(err)
		}
		var eval struct {
			Schema    map[string]json.RawMessage `json:"schema"`
			Condition json.RawMessage            `json:"upsert_condition"`
		}
		if err := json.Unmarshal([]byte((*bodies)[2]), &eval); err != nil {
			t.Fatal(err)
		}
		if got, want := string(eval.Schema["text"]), `{"embed":{"model":"`+DefaultModel+`"},"filterable":false,"full_text_search":true,"type":"string"}`; got != want {
			t.Fatalf("eval text field %s, want %s", got, want)
		}
		if got := string(eval.Condition); got != `["id","Eq",null]` {
			t.Fatalf("eval condition %s", got)
		}

		if _, err := cl.WriteSessions([]trace.SessionRow{{ID: "s"}}); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains((*bodies)[3], `"upsert_condition":["Or",`) || !strings.Contains((*bodies)[3], `"prompt_ts":{"type":"[]uint"}`) {
			t.Fatalf("session write lost its condition or schema: %s", (*bodies)[3])
		}
	}
}

func TestLocalLaneSchemaAndRoute(t *testing.T) {
	srv, paths, bodies := captureAll(t, `{"status":"OK","rows":[{"id":"a","text":"hit","session_id":"s"}],"hybrid":{"fuzziness":0},"next_cursor":null}`)
	cl, err := New(srv.URL, "local", "ns", "").WithStore(StorePgvector)
	if err != nil {
		t.Fatal(err)
	}

	schema := mustJSON(t, cl.schema())
	if strings.Contains(schema, "embed") {
		t.Fatalf("schema declares embed on a store that cannot: %s", schema)
	}
	if n := strings.Count(schema, "full_text_search"); n != 1 {
		t.Fatalf("%d full-text fields, want 1: %s", n, schema)
	}
	if got := mustJSON(t, cl.schema()["workdir"]); got != `{"type":"string"}` {
		t.Fatalf("workdir = %s, want a plain filterable string", got)
	}

	hits, err := cl.Search("why did it fail", 5, []any{"workdir", "Eq", "/w"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Text != "hit" {
		t.Fatalf("hits = %+v", hits)
	}
	want := `{"filters":["workdir","Eq","/w"],"include_attributes":["text","session_id","turn_uuid","ts","role","block_type","harness","workdir","plan","pr"],"rank_by":["text","HybridText","why did it fail",{"fuzziness":0}],"top_k":5}`
	if got := (*bodies)[0]; got != want {
		t.Fatalf("search body\n got %s\nwant %s", got, want)
	}
	for _, banned := range []string{"cursor", "temporal_filter", "queries", "rerank_by", "ANN", "Embed"} {
		if strings.Contains((*bodies)[0], banned) {
			t.Fatalf("local search body carries %q: %s", banned, (*bodies)[0])
		}
	}

	if _, err := cl.SearchEvals("q", 3, nil); err != nil {
		t.Fatal(err)
	}
	if (*paths)[1] != "/v2/namespaces/ns-evals/query" || !strings.Contains((*bodies)[1], `"HybridText","q",{"fuzziness":0}`) {
		t.Fatalf("eval search took %s %s", (*paths)[1], (*bodies)[1])
	}

	if _, err := cl.WriteEvals([]trace.Eval{{Session: "s", TS: "2026-09-01T00:00:00Z", Marks: map[string]int{"outcome": 2}}}); err != nil {
		t.Fatal(err)
	}
	if body := (*bodies)[2]; strings.Contains(body, "embed") || strings.Contains(body, "upsert_condition") {
		t.Fatalf("eval write carries embed or a condition: %s", body)
	}

	// Rows only an ordered scan can read back are not written at all.
	before := len(*bodies)
	if _, err := cl.WriteSessions([]trace.SessionRow{{ID: "s"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := cl.WriteBlocks([]trace.BlockRow{{ID: "b"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := cl.PatchSessionSummaries([]trace.SessionRow{{ID: "s"}}); err != nil {
		t.Fatal(err)
	}
	if len(*bodies) != before {
		t.Fatalf("read-side rows written to a store that cannot read them: %v", (*paths)[before:])
	}
}

// When the store says it can embed and index more fields, the declarations
// come back with no client change: that is LYR-87 and LYR-88 landing.
func TestSchemaFollowsTheAnswerNotTheStoreKind(t *testing.T) {
	caps, _ := StaticCapabilities(StorePgvector)
	caps.SchemaLimits.Embed.Support = Supported
	caps.SchemaLimits.MaxFullTextSearchFields = nil
	cl := New("http://x", "k", "ns", "")
	cl.Caps = caps
	if got, want := mustJSON(t, cl.schema()), mustJSON(t, legacySchema(DefaultModel)); got != want {
		t.Fatalf("schema\n got %s\nwant %s", got, want)
	}
}

func TestRuntimeAnswerFillsTheSeam(t *testing.T) {
	// The expected RFC 0117 read for a store that serves only HybridText, in
	// full: no fuzziness restriction, so none is sent.
	runtime := &Capabilities{
		Declared: true,
		Store:    StoreRef{Name: "main", Kind: "search"},
		HybridRoutes: []HybridRouteCoverage{
			{Route: RouteHybridText, Support: Supported},
			{Route: RouteMultiQuery, Support: Unsupported},
		},
	}
	caps, err := ResolveCapabilities(runtime, StorePgvector)
	if err != nil {
		t.Fatal(err)
	}
	if route, _ := caps.SearchRoute(); route != RouteHybridText {
		t.Fatalf("route = %s", route)
	}
	if opts := caps.HybridTextOptions(); opts != nil {
		t.Fatalf("options = %v, want gateway defaults", opts)
	}
}

func TestUndeclaredFallsBackToTheStaticTable(t *testing.T) {
	undeclared := &Capabilities{
		Declared: false,
		HybridRoutes: []HybridRouteCoverage{
			{Route: RouteHybridText, Support: Undeclared},
			{Route: RouteMultiQuery, Support: Undeclared},
		},
		SchemaLimits: SchemaLimits{Embed: Coverage{Support: Undeclared}},
	}
	caps, err := ResolveCapabilities(undeclared, StorePgvector)
	if err != nil {
		t.Fatal(err)
	}
	if route, _ := caps.SearchRoute(); route != RouteHybridText || caps.CanEmbed() {
		t.Fatalf("fallback did not use the pgvector table: %+v", caps)
	}

	// Taken at face value, an undeclared answer is never a yes.
	if undeclared.CanEmbed() || undeclared.ReadSide() || undeclared.WriteCondition("c") != nil {
		t.Fatal("undeclared read as supported")
	}
	if _, err := undeclared.SearchRoute(); err == nil {
		t.Fatal("undeclared store produced a search route")
	}
	if Support("someday").usable() {
		t.Fatal("a value outside the closed set read as usable")
	}
}

func TestUnknownStoreKindIsAnError(t *testing.T) {
	if _, err := New("http://x", "k", "ns", "").WithStore("sqlite"); err == nil {
		t.Fatal("unknown store kind accepted")
	}
}

// A store's refusal is recognised by status and typed fields. The message text
// is in flux upstream and is never matched.
func TestUnsupportedByStoreIsAStatusNotAMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		io.WriteString(w, `{"error":"UnsupportedByStore","message":"text that will change","store":"pgvector","route":"/v2/namespaces/ns/query"}`)
	}))
	defer srv.Close()
	_, err := New(srv.URL, "k", "ns", "").Search("q", 1, nil)
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.Status != http.StatusUnprocessableEntity {
		t.Fatalf("err = %v", err)
	}
}
