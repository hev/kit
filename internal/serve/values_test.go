package serve

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestValuesCountOtherFiltersOnly(t *testing.T) {
	s, _ := dashboardFixture(t)
	counts := func(url string) map[string]float64 {
		out := map[string]float64{}
		for _, raw := range requestJSON(t, s, url)["values"].([]any) {
			row := raw.(map[string]any)
			out[row["v"].(string)] = row["n"].(float64)
		}
		return out
	}
	// The facet's own selection is ignored: picking alpha still offers beta.
	if got := counts("/api/values?window=all&facet=project&project=alpha"); got["alpha"] != 3 || got["beta"] != 3 {
		t.Fatalf("project counts = %v", got)
	}
	// Other filters scope the counts: model-a traces are all alpha.
	if got := counts("/api/values?window=all&facet=project&model=model-a"); got["alpha"] != 3 || got["beta"] != 0 {
		t.Fatalf("scoped project counts = %v", got)
	}
	if got := counts("/api/values?window=all&facet=tool&host=mini"); got["Read"] != 3 || len(got) != 1 {
		t.Fatalf("tool counts = %v", got)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/values?facet=role", nil))
	if w.Code != 400 {
		t.Fatalf("unknown facet: %d", w.Code)
	}
}

func TestServesLayerUI(t *testing.T) {
	s, _ := dashboardFixture(t)
	for path, kind := range map[string]string{"/ui/list-select.mjs": "javascript", "/ui/list-select.css": "css", "/ui/theme.css": "css"} {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Type"), kind) {
			t.Fatalf("%s: %d %q", path, w.Code, w.Header().Get("Content-Type"))
		}
	}
}

// The in-memory picker path must count exactly what the store path counts.
func TestCachedValuesMatchStore(t *testing.T) {
	cached, _ := dashboardFixture(t)
	store, _ := dashboardFixture(t)
	store.WithCacheTTL(0)
	queries := []string{
		"window=all", "window=7d", "window=all&model=model-b", "window=all&project=alpha&project=beta",
		"window=all&tool=Read", "window=all&host=laptop&harness=claude_code", "window=all&tools_min=2&tools_max=4",
		"window=all&cost_min=2&wall_max=4&tokens_min=200", "window=all&weekday=" + weekdayOfFixture(t, store) + "&hour=12",
	}
	for _, q := range queries {
		for _, facet := range []string{"project", "model", "tool", "harness", "host"} {
			url := "/api/values?facet=" + facet + "&" + q
			a, b := requestJSON(t, cached, url), requestJSON(t, store, url)
			if fmt.Sprint(a["values"]) != fmt.Sprint(b["values"]) {
				t.Fatalf("%s: cached %v, store %v", url, a["values"], b["values"])
			}
			if b["cached"] != false {
				t.Fatalf("%s: store path reported cached", url)
			}
		}
	}
	if requestJSON(t, cached, "/api/values?window=all&facet=model")["cached"] != true {
		t.Fatal("repeat picker rescanned the archive")
	}
	// Grade predicates join eval rows, which only the store path does.
	if got := requestJSON(t, cached, "/api/values?window=all&facet=project&poor=true"); got["cached"] != false || len(got["values"].([]any)) != 1 {
		t.Fatalf("grade-scoped picker: %v", got)
	}
}

// weekdayOfFixture finds a weekday with a noon prompt in the fixture, so the
// heatmap predicate is exercised on real rows.
func weekdayOfFixture(t *testing.T, s *Server) string {
	t.Helper()
	for _, raw := range requestJSON(t, s, "/api/sessions?window=all")["sessions"].([]any) {
		start := int64(raw.(map[string]any)["start"].(float64))
		return fmt.Sprint(int(time.UnixMilli(start).UTC().Weekday()))
	}
	t.Fatal("empty fixture")
	return ""
}

// summary=true keeps summarized traces, on the list and in picker counts, on
// both the cached and the store path.
func TestSummaryFilter(t *testing.T) {
	for _, ttl := range []time.Duration{DefaultCacheTTL, 0} {
		s, f := dashboardFixture(t)
		s.WithCacheTTL(ttl)
		for _, row := range f.rows["test-sessions"] {
			if row["id"] == "trace-5" {
				row["summary"] = ""
			}
		}
		for q, want := range map[string]int{"summary=true": 5, "summary=false": 1, "": 6} {
			if got := len(requestJSON(t, s, "/api/sessions?window=all&"+q)["sessions"].([]any)); got != want {
				t.Fatalf("ttl %v, %s: %d sessions, want %d", ttl, q, got, want)
			}
		}
		got := map[string]float64{}
		for _, raw := range requestJSON(t, s, "/api/values?window=all&facet=project&summary=true")["values"].([]any) {
			got[raw.(map[string]any)["v"].(string)] = raw.(map[string]any)["n"].(float64)
		}
		if got["alpha"] != 3 || got["beta"] != 2 {
			t.Fatalf("ttl %v: summarized project counts = %v", ttl, got)
		}
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/sessions?summary=maybe", nil))
		if w.Code != 400 {
			t.Fatalf("summary=maybe: %d", w.Code)
		}
	}
}

// A stale archive answers at once and refreshes behind the request.
func TestStaleArchiveServesWhileRefreshing(t *testing.T) {
	s, _ := dashboardFixture(t)
	s.WithCacheTTL(time.Nanosecond)
	if requestJSON(t, s, "/api/values?window=all&facet=model")["cached"] != false {
		t.Fatal("first picker should fill the archive")
	}
	if requestJSON(t, s, "/api/values?window=all&facet=model")["cached"] != true {
		t.Fatal("stale archive was rescanned on the request path")
	}
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		s.archive.mu.Lock()
		done := !s.archive.refreshing
		s.archive.mu.Unlock()
		if done {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("background refresh never finished")
		}
	}
}
