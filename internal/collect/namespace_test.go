package collect

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestParseNamespaces(t *testing.T) {
	got, err := ParseNamespaces(" story-prod, story-staging ,story-prod,")
	if err != nil || strings.Join(got, ",") != "story-prod,story-staging" {
		t.Fatalf("ParseNamespaces = %v %v", got, err)
	}
	for _, list := range []string{"", " ", ",,"} {
		if _, err := ParseNamespaces(list); err != ErrNoNamespaces {
			t.Fatalf("ParseNamespaces(%q) = %v, want ErrNoNamespaces", list, err)
		}
	}
	for _, bad := range []string{"Story", "-story", "story_prod", "story.prod", "story/prod", strings.Repeat("x", 49)} {
		if _, err := ParseNamespaces("story-prod," + bad); err == nil {
			t.Fatalf("ParseNamespaces took %q", bad)
		}
	}
	if !ValidNamespace(strings.Repeat("x", 48)) || !ValidNamespace("0") {
		t.Fatal("ValidNamespace refused a valid namespace")
	}
	if got := LayerNamespace("", "story-win-oven"); got != "hev-app-traces-story-win-oven" {
		t.Fatalf("LayerNamespace = %s", got)
	}
	if got := LayerNamespace("e2e-1", "story-a"); got != "e2e-1-story-a" {
		t.Fatalf("LayerNamespace = %s", got)
	}
}

// The same owner, in the same chat, has a turn in each of two namespaces.
// Each namespace's search and conversation fetch return only its own turn,
// and each turn lands only in its own namespace's index and envelope tree.
func TestNamespaceNeverSeesAnotherNamespacesTurns(t *testing.T) {
	h := newHarness(t)
	inA := userA
	inB := userA
	inB.ns, inB.turn, inB.trace = nsB, "ki-run-b", "f1f2f3f4f5f6f7f8f9fafbfcfdfeff00"
	inB.prompt, inB.answer = "Open a heist story in Lisbon, the staging draft", "Staging: the tram bell rang twice."
	h.export(inA)
	h.export(inB)

	for ns, want := range map[string]string{nsA: "ki-run-1", nsB: "ki-run-b"} {
		turns := h.indexes[ns].all()
		if len(turns) != 1 || turns[0].TurnID != want || turns[0].Namespace != ns {
			t.Fatalf("%s index = %+v, want only %s", ns, turns, want)
		}
		if n := len(h.sink.byNS[ns]); n != 3 {
			t.Fatalf("%s envelopes = %d, want 3", ns, n)
		}
		for _, e := range h.sink.byNS[ns] {
			var res map[string]any
			_ = json.Unmarshal([]byte(*e.Resource), &res)
			if res["namespace"] != ns || res[AttrNamespace] != ns || e.TurnID != want {
				t.Fatalf("%s envelope resource = %v turn %s", ns, res, e.TurnID)
			}
		}
	}

	for _, c := range []struct{ ns, want, not string }{{nsA, "ki-run-1", "ki-run-b"}, {nsB, "ki-run-b", "ki-run-1"}} {
		code, body := h.do("POST", "/v1/app/traces/search", testKey, map[string]any{"namespace": c.ns, "owner": "101", "tenant": "story", "query": "heist Lisbon tram"})
		var sr searchResp
		_ = json.Unmarshal([]byte(body), &sr)
		if code != 200 || len(sr.Hits) != 1 || sr.Hits[0].TurnID != c.want || strings.Contains(body, c.not) {
			t.Fatalf("%s search = %d %s, want only %s", c.ns, code, body, c.want)
		}
		code, body = h.do("GET", "/v1/app/traces/conversations/chat-shared?namespace="+c.ns+"&owner=101&tenant=story", testKey, nil)
		var cr convResp
		_ = json.Unmarshal([]byte(body), &cr)
		if code != 200 || len(cr.Turns) != 1 || cr.Turns[0].TurnID != c.want || strings.Contains(body, c.not) {
			t.Fatalf("%s conversation = %d %s, want only %s", c.ns, code, body, c.want)
		}
	}

	// Even if a store handed back another namespace's turn, the server's
	// second lock drops it.
	leaked := h.indexes[nsA].all()[0]
	h.indexes[nsB].mu.Lock()
	h.indexes[nsB].turns["leak"] = leaked
	h.indexes[nsB].mu.Unlock()
	_, body := h.do("POST", "/v1/app/traces/search", testKey, map[string]any{"namespace": nsB, "owner": "101", "tenant": "story", "query": "heist"})
	if strings.Contains(body, "ki-run-1") {
		t.Fatalf("story-b search returned story-a's turn from a leaky store: %s", body)
	}
	_, body = h.do("GET", "/v1/app/traces/conversations/chat-shared?namespace="+nsB+"&owner=101&tenant=story", testKey, nil)
	if strings.Contains(body, "ki-run-1") {
		t.Fatalf("story-b conversation returned story-a's turn from a leaky store: %s", body)
	}
}

type partialResp struct {
	PartialSuccess *struct {
		RejectedSpans string `json:"rejectedSpans"`
		ErrorMessage  string `json:"errorMessage"`
	} `json:"partialSuccess"`
}

// Spans with no namespace, or one not on the allowlist, are refused: counted
// in partial_success, written nowhere, indexed nowhere, and logged once per
// namespace without the key.
func TestRefusedNamespacesAreWrittenNowhere(t *testing.T) {
	h := newHarness(t)
	refused := []string{noNamespace, "story-prod", "Story_Bad", testKey}
	for round := 0; round < 2; round++ {
		for _, ns := range refused {
			s := userA
			s.ns = ns
			for _, b := range aiSDKBatches(s) {
				code, body := h.do("POST", "/v1/traces", testKey, b)
				var pr partialResp
				if err := json.Unmarshal([]byte(body), &pr); err != nil || code != 200 || pr.PartialSuccess == nil || pr.PartialSuccess.RejectedSpans != "1" {
					t.Fatalf("export with namespace %q = %d %s, want partialSuccess rejectedSpans 1", ns, code, body)
				}
				if strings.Contains(body, testKey) {
					t.Fatalf("response echoes the key: %s", body)
				}
			}
		}
	}
	for ns, idx := range h.indexes {
		if n := len(idx.all()); n != 0 {
			t.Fatalf("%s index holds %d refused turns", ns, n)
		}
	}
	if len(h.sink.envs) != 0 {
		t.Fatalf("refused spans were written: %d envelopes under %v", len(h.sink.envs), h.sink.byNS)
	}
	h.srv.writeOrphans(h.srv.agg.Drain())
	if len(h.sink.envs) != 0 {
		t.Fatal("refused spans were held and written on drain")
	}

	logs := h.logs.String()
	if strings.Contains(logs, testKey) {
		t.Fatalf("key in logs:\n%s", logs)
	}
	for _, want := range []string{"namespace missing", "namespace=(none)", "namespace=story-prod", "namespace=(withheld)", "namespace invalid"} {
		if !strings.Contains(logs, want) {
			t.Fatalf("logs lack %q:\n%s", want, logs)
		}
	}
	// Twelve refused exports per round, two rounds: four log lines, one per
	// refused namespace.
	if n := strings.Count(logs, "refused spans"); n != len(refused) {
		t.Fatalf("refusals logged %d times, want once per namespace (%d):\n%s", n, len(refused), logs)
	}

	// One request with an allowed and a refused resource: the allowed one
	// is ingested, the other counted.
	good := aiSDKBatches(userA)[2]
	bad := userA
	bad.ns = "story-prod"
	var a, b map[string]any
	_ = json.Unmarshal(good, &a)
	_ = json.Unmarshal(aiSDKBatches(bad)[2], &b)
	mixed, _ := json.Marshal(map[string]any{"resourceSpans": append(a["resourceSpans"].([]any), b["resourceSpans"].([]any)...)})
	code, body := h.do("POST", "/v1/traces", testKey, mixed)
	if code != 200 || !strings.Contains(body, `"rejectedSpans":"1"`) {
		t.Fatalf("mixed export = %d %s", code, body)
	}
	if turns := h.indexes[nsA].all(); len(turns) != 1 || turns[0].Namespace != nsA {
		t.Fatalf("story-a index = %+v", turns)
	}
	for ns := range h.sink.byNS {
		if ns != nsA {
			t.Fatalf("envelopes written under %q", ns)
		}
	}
	// A fully accepted export has no partial success.
	if code, body := h.do("POST", "/v1/traces", testKey, aiSDKBatches(userB)[0]); code != 200 || strings.TrimSpace(body) != "{}" {
		t.Fatalf("accepted export = %d %s", code, body)
	}
}

func TestRefusedSpansInProtobufPartialSuccess(t *testing.T) {
	h := newHarness(t)
	span := pb{}.bytes(1, bytes.Repeat([]byte{1}, 16)).bytes(2, bytes.Repeat([]byte{2}, 8)).str(5, "ai.streamText").
		fixed64(7, uint64(time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC).UnixNano())).
		bytes(9, pbStringKV("ai.telemetry.metadata.userId", "101")).bytes(9, pbStringKV("ai.telemetry.metadata.turnId", "ki-run-p"))
	scope := pb{}.bytes(1, pb{}.str(1, "ai")).bytes(2, span).bytes(2, span)
	resource := pb{}.bytes(1, pbStringKV("service.name", "ki-agent"))
	req := pb{}.bytes(1, pb{}.bytes(1, resource).bytes(2, scope))

	post := func(body []byte) []byte {
		r, _ := http.NewRequest("POST", h.http.URL+"/v1/traces", bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+testKey)
		r.Header.Set("Content-Type", "application/x-protobuf")
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "application/x-protobuf" {
			t.Fatalf("protobuf export = %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
		}
		raw, _ := io.ReadAll(resp.Body)
		return raw
	}

	raw := post(req)
	var rejected uint64
	var message string
	err := pbEach(raw, func(field, wire int, n uint64, b []byte) error {
		if field != 1 {
			return nil
		}
		return pbEach(b, func(field, wire int, n uint64, b []byte) error {
			switch field {
			case 1:
				rejected = n
			case 2:
				message = string(b)
			}
			return nil
		})
	})
	if err != nil || rejected != 2 || !strings.Contains(message, "hev.namespace") {
		t.Fatalf("partial success = %d %q (%v) from %x", rejected, message, err, raw)
	}
	if len(h.sink.envs) != 0 || len(h.index.all()) != 0 {
		t.Fatal("refused protobuf spans were written")
	}

	// With the namespace on the resource, the same spans are accepted and
	// the response is the empty message.
	resource = pb{}.bytes(1, pbStringKV("service.name", "ki-agent")).bytes(1, pbStringKV(AttrNamespace, nsA))
	if raw := post(pb{}.bytes(1, pb{}.bytes(1, resource).bytes(2, scope))); len(raw) != 0 {
		t.Fatalf("accepted protobuf export answered %x", raw)
	}
	if len(h.sink.byNS[nsA]) != 2 {
		t.Fatalf("story-a envelopes = %d", len(h.sink.byNS[nsA]))
	}
}

func TestAPINeedsAnAllowedNamespace(t *testing.T) {
	h := newHarness(t)
	h.export(userA)
	search := func(ns any) (int, string) {
		body := map[string]any{"owner": "101", "tenant": "story", "query": "heist"}
		if ns != nil {
			body["namespace"] = ns
		}
		return h.do("POST", "/v1/app/traces/search", testKey, body)
	}
	conv := func(q string) (int, string) {
		return h.do("GET", "/v1/app/traces/conversations/chat-shared?owner=101&tenant=story"+q, testKey, nil)
	}
	for _, c := range []struct {
		name string
		call func() (int, string)
		want int
	}{
		{"search without namespace", func() (int, string) { return search(nil) }, 400},
		{"search with empty namespace", func() (int, string) { return search("") }, 400},
		{"search with another namespace", func() (int, string) { return search("story-prod") }, 403},
		{"search with the archive's Layer namespace", func() (int, string) { return search("hev-traces") }, 403},
		{"search with a prefixed Layer namespace", func() (int, string) { return search("hev-app-traces-story-a") }, 403},
		{"conversation without namespace", func() (int, string) { return conv("") }, 400},
		{"conversation with another namespace", func() (int, string) { return conv("&namespace=story-prod") }, 403},
		{"conversation with the archive's Layer namespace", func() (int, string) { return conv("&namespace=hev-traces") }, 403},
	} {
		if code, body := c.call(); code != c.want || strings.Contains(body, "ki-run-1") {
			t.Errorf("%s = %d %s, want %d", c.name, code, body, c.want)
		}
	}
	if code, _ := search(nsA); code != 200 {
		t.Fatalf("search in story-a = %d", code)
	}
}
