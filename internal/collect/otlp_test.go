package collect

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/layer"
)

// pb is a minimal protobuf writer for building OTLP fixtures.
type pb []byte

func (b pb) varint(v uint64) pb {
	for v >= 0x80 {
		b = append(b, byte(v)|0x80)
		v >>= 7
	}
	return append(b, byte(v))
}
func (b pb) tag(field, wire int) pb { return b.varint(uint64(field<<3 | wire)) }
func (b pb) bytes(field int, v []byte) pb {
	return append(b.tag(field, 2).varint(uint64(len(v))), v...)
}
func (b pb) str(field int, v string) pb { return b.bytes(field, []byte(v)) }
func (b pb) fixed64(field int, v uint64) pb {
	b = b.tag(field, 1)
	return binary.LittleEndian.AppendUint64(b, v)
}
func (b pb) uint(field int, v uint64) pb { return b.tag(field, 0).varint(v) }

func pbStringKV(k, v string) []byte { return pb{}.str(1, k).bytes(2, pb{}.str(1, v)) }

func TestDecodeProtoMatchesJSON(t *testing.T) {
	trace := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	spanID := []byte{0xa1, 0xa1, 0xa1, 0xa1, 0xa1, 0xa1, 0xa1, 0xa1}
	start := uint64(time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC).UnixNano())
	intKV := pb{}.str(1, "n").bytes(2, pb{}.uint(3, 42))
	dblKV := pb{}.str(1, "d").bytes(2, pb{}.fixed64(4, math.Float64bits(1.5)))
	boolKV := pb{}.str(1, "b").bytes(2, pb{}.uint(2, 1))
	arrKV := pb{}.str(1, "arr").bytes(2, pb{}.bytes(5, pb{}.bytes(1, pb{}.str(1, "x")).bytes(1, pb{}.str(1, "y"))))
	span := pb{}.bytes(1, trace).bytes(2, spanID).str(5, "ai.streamText").fixed64(7, start).fixed64(8, start+1000).
		bytes(9, pbStringKV("ai.telemetry.metadata.userId", "101")).
		bytes(9, pbStringKV("ai.telemetry.metadata.turnId", "ki-run-1")).
		bytes(9, intKV).bytes(9, dblKV).bytes(9, boolKV).bytes(9, arrKV).
		bytes(15, pb{}.str(2, "bad").uint(3, 2)).
		uint(99, 7) // an unknown field is skipped
	scope := pb{}.bytes(1, pb{}.str(1, "ai")).bytes(2, span)
	resource := pb{}.bytes(1, pbStringKV("service.name", "ki-agent"))
	req := pb{}.bytes(1, pb{}.bytes(1, resource).bytes(2, scope))

	spans, err := DecodeProto(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) != 1 {
		t.Fatalf("spans = %d", len(spans))
	}
	s := spans[0]
	if s.TraceID != "0102030405060708090a0b0c0d0e0f10" || s.SpanID != "a1a1a1a1a1a1a1a1" || s.Name != "ai.streamText" || s.Scope != "ai" {
		t.Fatalf("span ids = %+v", s)
	}
	if !s.Start.Equal(time.Unix(0, int64(start)).UTC()) || s.StatusCode != StatusError || s.StatusMessage != "bad" {
		t.Fatalf("span = %+v", s)
	}
	if s.Attrs["ai.telemetry.metadata.userId"] != "101" || s.Attrs["n"] != int64(42) || s.Attrs["d"] != 1.5 || s.Attrs["b"] != true {
		t.Fatalf("attrs = %v", s.Attrs)
	}
	if arr, _ := s.Attrs["arr"].([]any); len(arr) != 2 || arr[1] != "y" {
		t.Fatalf("array attr = %v", s.Attrs["arr"])
	}
	if s.Resource["service.name"] != "ki-agent" {
		t.Fatalf("resource = %v", s.Resource)
	}
	if _, err := DecodeProto([]byte{0x0a, 0xff}); err == nil {
		t.Fatal("truncated protobuf accepted")
	}
}

func TestDecodeJSONIDsAndNumbers(t *testing.T) {
	body := `{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"ki-agent"}}]},
	"scopeSpans":[{"scope":{"name":"ai"},"spans":[{"traceId":"AQIDBAUGBwgJCgsMDQ4PEA==","spanId":"oaGhoaGhoaE=","name":"ai.toolCall",
	"startTimeUnixNano":1763373600000000000,"endTimeUnixNano":"1763373600000001000",
	"attributes":[{"key":"i","value":{"intValue":7}},{"key":"kv","value":{"kvlistValue":{"values":[{"key":"a","value":{"boolValue":true}}]}}}],
	"status":{"code":"STATUS_CODE_ERROR","message":"x"}}]}]}]}`
	spans, err := DecodeJSON([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	s := spans[0]
	if s.TraceID != "0102030405060708090a0b0c0d0e0f10" || s.SpanID != "a1a1a1a1a1a1a1a1" {
		t.Fatalf("base64 ids = %s %s", s.TraceID, s.SpanID)
	}
	if s.Start.UnixNano() != 1763373600000000000 || s.End.UnixNano() != 1763373600000001000 || s.StatusCode != StatusError {
		t.Fatalf("span = %+v", s)
	}
	if s.Attrs["i"] != int64(7) || s.Attrs["kv"].(map[string]any)["a"] != true {
		t.Fatalf("attrs = %v", s.Attrs)
	}
}

// fakeGateway records the bodies LayerTurns sends and answers like Layer.
func fakeGateway(t *testing.T, rows []map[string]any) (*layer.Client, *[]map[string]any) {
	t.Helper()
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			// No runtime capability declaration: the client keeps the
			// static pgvector answer.
			_, _ = w.Write([]byte(`{"declared":false}`))
			return
		}
		raw, _ := io.ReadAll(r.Body)
		body := map[string]any{}
		_ = json.Unmarshal(raw, &body)
		bodies = append(bodies, body)
		if strings.HasSuffix(r.URL.Path, "/query") {
			_ = json.NewEncoder(w).Encode(map[string]any{"rows": rows})
			return
		}
		_, _ = w.Write([]byte(`{"status":"OK","rows_upserted":1}`))
	}))
	t.Cleanup(srv.Close)
	// A store that answers only the hybrid-text route, as the local Postgres
	// gateway #273 was written against did. v0.7 retired that store kind
	// (layer.StorePgvector), so the capabilities are declared here.
	cl := layer.New(srv.URL, "local", "app-turns", "")
	cl.Caps = layer.Capabilities{
		Declared:     true,
		HybridRoutes: []layer.HybridRouteCoverage{{Route: layer.RouteHybridText, Support: layer.Supported}},
		SchemaLimits: layer.SchemaLimits{Embed: layer.Coverage{Support: layer.Supported}},
	}
	return cl, &bodies
}

func TestLayerTurnsScopesEveryReadToNamespaceOwnerAndTenant(t *testing.T) {
	cl, bodies := fakeGateway(t, []map[string]any{{"namespace": "story-a", "owner": "101", "tenant": "story", "chat_id": "c", "turn_id": "ki-run-1", "ts": "2026-11-17T10:00:00.000000Z", "prompt": "p", "answer": "a", "tools": `[{"name":"t","ok":true}]`, "$dist": 0.25}})
	idx := &LayerTurns{Client: cl, Namespace: "story-a"}
	if err := idx.Upsert(context.Background(), []Turn{{Namespace: "story-b", Owner: "101", TurnID: "x"}}); err == nil {
		t.Fatal("story-a's index took a story-b turn")
	}
	turn := Turn{Namespace: "story-a", Owner: "101", Tenant: "story", ChatID: "c", TurnID: "ki-run-1", TS: time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC), Prompt: "p", Answer: "a", Tools: []Tool{{Name: "t", OK: true}}, Service: "ki-agent"}
	if err := idx.Upsert(context.Background(), []Turn{turn}); err != nil {
		t.Fatal(err)
	}
	hits, err := idx.Search(context.Background(), "101", "story", "heist", time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Turn.TurnID != "ki-run-1" || hits[0].Turn.Namespace != "story-a" || hits[0].Score != 0.75 || hits[0].Turn.Tools[0].Name != "t" {
		t.Fatalf("hits = %+v", hits)
	}
	turns, err := idx.Conversation(context.Background(), "101", "story", "c")
	if err != nil || len(turns) != 1 {
		t.Fatalf("conversation = %v %v", turns, err)
	}
	if len(*bodies) != 3 {
		t.Fatalf("requests = %d", len(*bodies))
	}
	write, search, conv := (*bodies)[0], (*bodies)[1], (*bodies)[2]
	row := write["upsert_rows"].([]any)[0].(map[string]any)
	if row["id"] != turn.DocID() || row["owner"] != "101" || row["namespace"] != "story-a" || row["text"] != "p\n\na" || row["ts"] != "2026-11-17T10:00:00.000000Z" {
		t.Fatalf("row = %v", row)
	}
	schema := write["schema"].(map[string]any)
	for _, f := range []string{"namespace", "owner", "tenant", "chat_id", "ts"} {
		if schema[f].(map[string]any)["filterable"] != true {
			t.Fatalf("%s is not filterable: %v", f, schema[f])
		}
	}
	for name, body := range map[string]map[string]any{"search": search, "conversation": conv} {
		filters, _ := json.Marshal(body["filters"])
		for _, want := range []string{`["namespace","Eq","story-a"]`, `["owner","Eq","101"]`, `["tenant","Eq","story"]`} {
			if !strings.Contains(string(filters), want) {
				t.Fatalf("%s filters %s lack %s", name, filters, want)
			}
		}
	}
	if f, _ := json.Marshal(search["filters"]); !strings.Contains(string(f), `["ts","Gte","2026-11-01T00:00:00.000000Z"]`) {
		t.Fatalf("search since filter = %s", f)
	}
	if f, _ := json.Marshal(conv["filters"]); !strings.Contains(string(f), `["chat_id","Eq","c"]`) {
		t.Fatalf("conversation filter = %s", f)
	}
}

func TestFileSinkPartitions(t *testing.T) {
	dir := t.TempDir()
	sink := &FileSink{Dir: dir}
	env := Envelope(Attributed{Span: Span{TraceID: "t", SpanID: "s", Name: "ai.streamText", Start: time.Date(2026, 11, 17, 1, 0, 0, 0, time.UTC), Attrs: map[string]any{}, Resource: map[string]any{"service.name": "ki-agent"}}, Owner: "101", Tenant: "story", TurnID: "ki-run-1"}, "h", RedactionPolicy)
	if err := sink.Write("story-a", []*daemon.TraceEnvelope{env, env}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "ns=story-a", "dt=2026-11-17", "harness=app_ki-agent", "envelopes.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(raw), "\n"); n != 2 {
		t.Fatalf("lines = %d", n)
	}
	var back daemon.TraceEnvelope
	if err := json.Unmarshal([]byte(strings.SplitN(string(raw), "\n", 2)[0]), &back); err != nil || back.Harness != "app:ki-agent" || back.TurnID != "ki-run-1" {
		t.Fatalf("envelope = %+v %v", back, err)
	}
}
