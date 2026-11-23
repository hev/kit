package serve

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// Rule: OSS `hev serve` keeps its sections (Traces, Board) and the Traces
// views (List, Stats), and adds no Evidence tab or route (RFC 0022, memo #551). Evidence is a folder plus
// `hev evidence`; its viewer is a Pro UI surface.
func TestNoEvidenceTab(t *testing.T) {
	for _, bar := range []struct{ open, close, want string }{
		{`<nav class="tabs" id="tabs"`, `</nav>`, "traces,board"},
		{`<div class="tabs sub" id="trace-tabs"`, `</div>`, "traces,stats"},
	} {
		i := strings.Index(pageTemplate, bar.open)
		if i < 0 {
			t.Fatalf("tab bar %s not found", bar.open)
		}
		tabs := pageTemplate[i:]
		tabs = tabs[:strings.Index(tabs, bar.close)]
		var names []string
		for _, m := range regexp.MustCompile(`data-v="([a-z-]+)"`).FindAllStringSubmatch(tabs, -1) {
			names = append(names, m[1])
		}
		if strings.Join(names, ",") != bar.want {
			t.Fatalf("serve tabs %v, want %s; an Evidence tab belongs to the Pro UI", names, bar.want)
		}
		if strings.Contains(strings.ToLower(tabs), "evidence") {
			t.Fatal("OSS serve has an evidence tab")
		}
	}
	if strings.Contains(pageTemplate, "/api/evidence") {
		t.Fatal("OSS serve calls an evidence API")
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
