package serve

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/trace"
)

func TestChromeDoesNotReadLayer(t *testing.T) {
	s, f := dashboardFixture(t)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/?session=trace-0", nil))
	if w.Code != 200 || len(f.filters) != 0 || strings.Contains(w.Body.String(), "Fixture 0: verify") {
		t.Fatalf("page read Layer or embedded corpus: status=%d queries=%d", w.Code, len(f.filters))
	}
	if !strings.Contains(w.Body.String(), ">Loading…</div>") {
		t.Fatal("chrome lacks initial loading state")
	}
}

func TestSlimRowsAndDetail(t *testing.T) {
	s, f := dashboardFixture(t)
	full := strings.Repeat("界", 10000)
	for _, row := range f.rows["test-sessions"] {
		row["tool_counts"] = `{"Bash":7,"Read":3}`
		row["first_prompt"] = full
		row["first_prompt_short"] = strings.Repeat("界", 600)
	}
	for _, endpoint := range []string{"/api/sessions?window=all", "/api/search?window=all&q=preflight"} {
		data := requestJSON(t, s, endpoint)
		for _, raw := range data["sessions"].([]any) {
			row := raw.(map[string]any)
			for _, key := range []string{"first_prompt", "prompt_ts", "tool_names"} {
				if _, found := row[key]; found {
					t.Fatalf("%s leaked %s", endpoint, key)
				}
			}
			if len([]rune(row["first_prompt_short"].(string))) != 600 {
				t.Fatal("short prompt lost")
			}
			counts := row["tool_counts"].(map[string]any)
			if counts["Bash"] != float64(7) || counts["Read"] != float64(3) {
				t.Fatalf("%s lost tool counts: %v", endpoint, counts)
			}
			if e, ok := row["eval"].(map[string]any); ok {
				if _, found := e["evidence"]; found {
					t.Fatal("list carries evaluation prose")
				}
			}
		}
	}
	detail := requestJSON(t, s, "/api/session/trace-0")
	if detail["first_prompt"] != full || len(detail["prompt_ts"].([]any)) != 2 || len(detail["tool_names"].([]any)) != 1 {
		t.Fatal("detail lost full data")
	}
}

func TestServerHeatmapMatchesDetailsAndSelection(t *testing.T) {
	s, _ := dashboardFixture(t)
	for _, zone := range []string{"UTC", "America/Denver"} {
		loc, _ := time.LoadLocation(zone)
		stats := requestJSON(t, s, "/api/stats?window=all&timezone="+zone)
		coverage := stats["prompt_coverage"].(map[string]any)
		var want [7][24]int
		list := requestJSON(t, s, "/api/sessions?window=all")["sessions"].([]any)
		with := 0
		for _, raw := range list {
			row := requestJSON(t, s, "/api/session/"+raw.(map[string]any)["id"].(string))
			times := row["prompt_ts"].([]any)
			if len(times) > 0 {
				with++
			}
			for _, raw := range times {
				d := time.UnixMilli(int64(raw.(float64))).In(loc)
				want[d.Weekday()][d.Hour()]++
			}
		}
		if coverage["with"] != float64(with) || coverage["total"] != float64(len(list)) {
			t.Fatal(coverage)
		}
		encoded, _ := json.Marshal(want)
		got, _ := json.Marshal(stats["prompt_grid"])
		if string(encoded) != string(got) {
			t.Fatalf("grid %s differs from %s", got, encoded)
		}
	}
	for _, query := range []string{"weekday=7&hour=1", "weekday=0", "hour=0", "weekday=0&hour=-1", "timezone=invalid"} {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/sessions?window=all&"+query, nil))
		if w.Code != 400 {
			t.Fatalf("%s: %d", query, w.Code)
		}
	}
	// Same instant falls on Sunday in Denver and Monday in UTC.
	ts := uint64(time.Date(2026, 9, 7, 1, 0, 0, 0, time.UTC).UnixMilli())
	s2 := New(&fakeReader{sessions: []trace.SessionRow{{ID: "one", SessionID: "one", PromptTS: []uint64{ts, ts}}, {ID: "two", SessionID: "two"}}})
	rows := requestJSON(t, s2, "/api/sessions?window=all&timezone=America/Denver&weekday=0&hour=19")["sessions"].([]any)
	if len(rows) != 1 {
		t.Fatalf("selection duplicated/lost trace: %v", rows)
	}
}

func TestAPITimingAndHead(t *testing.T) {
	s, _ := dashboardFixture(t)
	for _, endpoint := range []string{"/api/sessions?window=all", "/api/stats?window=all", "/api/search?window=all&q=preflight", "/api/session/trace-0", "/api/search"} {
		for _, method := range []string{"GET", "HEAD"} {
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, httptest.NewRequest(method, endpoint, nil))
			if !strings.HasPrefix(w.Header().Get("Server-Timing"), "layer;dur=") {
				t.Fatal("missing timing header")
			}
			var data map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
				t.Fatal(err)
			}
			timing, ok := data["timing"].(map[string]any)
			if !ok {
				t.Fatal("missing JSON timing")
			}
			if endpoint == "/api/search" {
				if timing["queries"] != float64(0) {
					t.Fatal(timing)
				}
			} else if timing["queries"].(float64) < 1 || timing["rows"].(float64) < 1 || timing["layer_ms"].(float64) <= 0 {
				t.Fatal(timing)
			}
		}
	}
}
