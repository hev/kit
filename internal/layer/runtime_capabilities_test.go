package layer

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Wire entries from layer-pro #801's contract, deliberately out of order.
const pgvectorDeclaration = `{"declared":true,"store":{"name":"main","kind":"pgvector"},"features":[{"id":"search_after","support":"unsupported","note":""},{"id":"conditional_writes","support":"supported","note":""},{"id":"ordered_scan","support":"supported","note":""}],"hybrid_routes":[{"route":"multi_query","support":"supported"}],"schema_limits":{"embed":{"support":"approximate"},"max_gateway_embed_attributes":1,"max_full_text_search_fields":null,"max_vector_fields":1}}`

func servePgvectorDeclaration(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != "GET" || !strings.HasSuffix(r.URL.Path, "/capabilities") {
		return false
	}
	io.WriteString(w, pgvectorDeclaration)
	return true
}

func TestPgvectorRuntimeDeclarationContract(t *testing.T) {
	static, _ := StaticCapabilities(StorePgvector)
	for _, row := range static.Features {
		if row.ID == FeatureOrderedScan || row.ID == FeatureConditionalWrites {
			t.Fatalf("static override remains: %+v", row)
		}
	}
	var runtime Capabilities
	if err := json.Unmarshal([]byte(pgvectorDeclaration), &runtime); err != nil {
		t.Fatal(err)
	}
	if runtime.SchemaLimits.MaxVectorFields == nil || *runtime.SchemaLimits.MaxVectorFields != 1 || runtime.HybridRoutes[0].Route != RouteMultiQuery {
		t.Fatalf("wire fields not decoded: %+v", runtime)
	}
	caps, err := ResolveCapabilities(&runtime, StorePgvector)
	if err != nil || !caps.ReadSide() || caps.WriteCondition("guard") != "guard" {
		t.Fatalf("contract: %+v, %v", caps, err)
	}
	// Unresolved seams keep their compatibility restrictions even if a report
	// is coarser than the actual route. Ordered scans never enable ranked cursors.
	if caps.route(RouteMultiQuery) != Unsupported || caps.HybridTextOptions()["fuzziness"] != 0 || caps.Feature("search_after").usable() || caps.Feature(FeaturePatchRows) != Supported || !caps.Arrays() || !caps.CanEmbed() {
		t.Fatalf("compatibility changed: %+v", caps)
	}
	runtime.Features[1] = FeatureCoverage{ID: FeatureConditionalWrites, Support: Unsupported, Note: "backend refuses conditions"}
	caps, _ = ResolveCapabilities(&runtime, StorePgvector)
	if !caps.ReadSide() || caps.WriteCondition("guard") != nil {
		t.Fatal("server refusal overridden")
	}
	found := false
	for _, row := range caps.Features {
		if row.ID == FeatureConditionalWrites {
			found = row.Note == "backend refuses conditions"
		}
	}
	if !found {
		t.Fatal("server refusal reason lost")
	}
}

func TestPgvectorCapabilityReadAndConservativeFallback(t *testing.T) {
	for _, tc := range []struct {
		name, reply string
		status      int
		enabled     bool
	}{
		{"current", pgvectorDeclaration, 200, true},
		{"older", `{"error":"not found"}`, 404, false},
		{"unavailable", `{}`, 503, false},
		{"malformed", `{`, 200, false},
		{"undeclared", strings.Replace(pgvectorDeclaration, `"declared":true`, `"declared":false`, 1), 200, false},
		{"missing features", `{"declared":true,"store":{"kind":"pgvector"}}`, 200, false},
		{"wrong store", strings.Replace(pgvectorDeclaration, `"kind":"pgvector"`, `"kind":"other"`, 1), 200, false},
		{"unknown support", strings.ReplaceAll(pgvectorDeclaration, `"supported"`, `"future"`), 200, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "GET" || r.URL.EscapedPath() != "/v2/namespaces/archive%2Fone/capabilities" || r.Header.Get("Authorization") != "Bearer fixture" {
					t.Errorf("request: %s %s", r.Method, r.URL)
				}
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.reply)
			}))
			defer srv.Close()
			cl, err := New(srv.URL, "fixture", "archive/one", "").WithStore(StorePgvector)
			if err != nil || calls != 1 {
				t.Fatalf("read: calls=%d err=%v", calls, err)
			}
			if cl.Caps.ReadSide() != tc.enabled || (cl.Caps.WriteCondition("guard") != nil) != tc.enabled {
				t.Fatalf("unsafe capability: %+v", cl.Caps)
			}
		})
	}
}
