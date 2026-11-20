package serve

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// Rule: OSS `hev serve` keeps its tabs (Stats, Traces, Asks) and adds no
// Evidence tab or route (RFC 0022, memo #551). Evidence is a folder plus
// `hev evidence`; its viewer is a Pro UI surface.
func TestNoEvidenceTab(t *testing.T) {
	tabs := regexp.MustCompile(`(?s)<div class="tabs" id="tabs">(.*?)</div>`).FindStringSubmatch(pageTemplate)
	if tabs == nil {
		t.Fatal("tab bar not found")
	}
	got := regexp.MustCompile(`data-v="([a-z-]+)"`).FindAllStringSubmatch(tabs[1], -1)
	var names []string
	for _, m := range got {
		names = append(names, m[1])
	}
	if strings.Join(names, ",") != "stats,traces,asks" {
		t.Fatalf("serve tabs %v; an Evidence tab belongs to the Pro UI", names)
	}
	if strings.Contains(strings.ToLower(tabs[1]), "evidence") || strings.Contains(pageTemplate, "/api/evidence") {
		t.Fatal("OSS serve exposes evidence")
	}
}

func TestNoEvidenceRoutes(t *testing.T) {
	// Unknown paths get 404 or the app page itself; none is an evidence API.
	h := New(&fakeReader{}).Handler()
	for _, path := range []string{"/api/evidence", "/api/evidence/search", "/evidence"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != http.StatusNotFound && !strings.Contains(w.Body.String(), `data-v="traces"`) {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
}
