package serve

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
)

func TestCompactStatsMatchesOriginalSelectionAndCorpus(t *testing.T) {
	s, f := dashboardFixture(t)
	// Large text is deliberately present in the source, never in Stats transport.
	for _, row := range f.rows["test-sessions"] {
		row["first_prompt"] = strings.Repeat("sensitive prompt", 100)
		row["outcome_details"] = `{"unused":"outcome"}`
	}
	for _, query := range []string{"", "exclude_project=alpha", "exclude_project=https%3A%2F%2Fgithub.com%2Fexample%2Falpha", "exclude_host=laptop", "exclude_model=model-a", "exclude_harness=claude_code", "cost_max=0.5", "project=stale", "model=retired", "project=alpha&model=model-a", "poor=true", "poor=false", "poor=false&role=author", "mark_accuracy_max=2", "summary=true", "summary=false", "timezone=America/Denver", "timezone=UTC&weekday=0&hour=1"} {
		t.Run(query, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/stats?window=all&"+query, nil)
			// The pre-change selection path is an independent reference for the new
			// single projected scan, including legacy revisions and latest evals.
			reference := New(s.reader)
			rows, _, err := reference.rows(req)
			if err != nil {
				t.Fatal(err)
			}
			list := corpus(rows)
			got := requestJSON(t, s, req.URL.String())
			var spend, tokens, prompts, tools, api, wall, read, base float64
			for _, row := range list {
				spend += row["cost"].(float64)
				prompts += float64(row["prompts"].(int64))
				tools += float64(row["tools"].(int64))
				api += float64(row["api_ms"].(int64))
				wall += float64(row["wall_ms"].(int64))
				for _, raw := range row["usage_by_model"].(map[string]any) {
					u := raw.(map[string]int64)
					tokens += float64(u["input"] + u["output"] + u["cache_read"] + u["cache_write"])
					read += float64(u["cache_read"])
					base += float64(u["input"] + u["cache_read"] + u["cache_write"])
				}
			}
			want := map[string]float64{"spend": spend, "tokens": tokens, "prompts": prompts, "tools": tools, "api_hours": api / 3600000, "wall_hours": wall / 3600000, "traces": float64(len(list)), "cache_hit": read / max(1, base), "cost_per_prompt": spend / max(1, prompts)}
			for key, n := range want {
				if got[key] != n {
					t.Fatalf("%s=%v want %v", key, got[key], n)
				}
			}
			var pn, pt float64
			var pc float64
			for _, raw := range got["projects"].([]any) {
				p := raw.(map[string]any)
				pn += p["n"].(float64)
				pt += p["tok"].(float64)
				pc += p["cost"].(float64)
			}
			if pn != float64(len(rows)) || pt != tokens || pc != spend {
				t.Fatalf("project totals incomplete: %v", got["projects"])
			}
			var dn, dt, dc float64
			for _, raw := range got["daily"].([]any) {
				d := raw.(map[string]any)
				dn += d["traces"].(float64)
				dc += d["spend"].(float64)
				for _, m := range d["models"].(map[string]any) {
					dt += m.(map[string]any)["tokens"].(float64)
				}
			}
			if dn != float64(len(rows)) || dt != tokens || dc != spend {
				t.Fatalf("daily totals incomplete: %v", got["daily"])
			}
			checkCompact(t, got)
		})
	}
}

func checkCompact(t *testing.T, value any) {
	t.Helper()
	forbidden := map[string]bool{"sessions": true, "corpus": true, "id": true, "session_id": true, "summary": true, "first_prompt": true, "first_prompt_short": true, "prompt_ts": true, "text": true, "turns": true, "requests": true, "blocks": true, "evidence": true, "findings": true, "tool_counts": true, "usage_by_model": true, "eval": true, "outcome_details": true}
	var walk func(any)
	walk = func(value any) {
		switch v := value.(type) {
		case map[string]any:
			for key, item := range v {
				if forbidden[key] {
					t.Fatalf("Stats leaked trace field %s", key)
				}
				walk(item)
			}
		case []any:
			for _, item := range v {
				walk(item)
			}
		}
	}
	walk(value)
}

func TestCompactStatsProjectionAndSharedSnapshot(t *testing.T) {
	_, f := dashboardFixture(t)
	for _, endpoint := range []string{"/api/stats?window=all", "/api/sessions?window=all", "/api/sessions?window=all&include_stats=true", "/api/stats?window=all&summary=true"} {
		t.Run(endpoint, func(t *testing.T) {
			scans := 0
			wire := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" && strings.HasSuffix(r.URL.Path, "test-sessions/query") {
					scans++
					var q map[string]any
					if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
						t.Error(err)
					}
					encoded, _ := json.Marshal(q)
					r.Body = io.NopCloser(bytes.NewReader(encoded))
					attrs := map[string]bool{}
					for _, a := range q["include_attributes"].([]any) {
						attrs[a.(string)] = true
					}
					isStats := strings.HasPrefix(endpoint, "/api/stats")
					wantPrompts := isStats || strings.Contains(endpoint, "include_stats")
					if attrs["prompt_ts"] != wantPrompts || attrs["first_prompt"] || attrs["outcome_details"] {
						t.Errorf("unused payload projection: %v", attrs)
					}
					if isStats && (attrs["first_prompt_short"] || attrs["tool_counts"] || attrs["summary"] != strings.Contains(endpoint, "summary=")) {
						t.Errorf("Stats projection: %v", attrs)
					}
				}
				f.ServeHTTP(w, r)
			}))
			defer wire.Close()
			s := New(layer.New(wire.URL, "fixture", "test", ""))
			got := requestJSON(t, s, endpoint)
			if scans != 1 {
				t.Fatalf("%d scans", scans)
			}
			if strings.Contains(endpoint, "include_stats") {
				stats := got["stats"].(map[string]any)
				if stats["traces"] != float64(len(got["sessions"].([]any))) {
					t.Fatal("list/stats mismatch")
				}
				if got["timing"].(map[string]any)["rows"] != float64(10) {
					t.Fatalf("unexpected shared source rows %v", got["timing"])
				}
			}
		})
	}
}

func TestCompactStatsEmptyAndErrors(t *testing.T) {
	s, _ := dashboardFixture(t)
	empty := requestJSON(t, s, "/api/stats?window=all&project=missing")
	if empty["traces"] != float64(0) || empty["days"] != float64(1) || len(empty["daily"].([]any)) != 0 || len(empty["projects"].([]any)) != 0 {
		t.Fatal(empty)
	}
	for _, query := range []string{"timezone=invalid", "weekday=7&hour=0", "weekday=1", "summary=bad", "poor=bad", "cost_min=NaN", "mark_accuracy_max=bad", "since=2026-10-24&until=2026-10-23"} {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/stats?window=all&"+query, nil))
		if w.Code != 400 {
			t.Fatalf("%s: %d", query, w.Code)
		}
	}
	for _, fail := range []string{"schema", "-sessions/query", "-evals/query"} {
		t.Run(fail, func(t *testing.T) {
			_, f := dashboardFixture(t)
			wire := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, fail) {
					http.Error(w, "bounded fixture error", 502)
					return
				}
				f.ServeHTTP(w, r)
			}))
			defer wire.Close()
			s := New(layer.New(wire.URL, "fixture", "test", ""))
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/stats?window=all", nil))
			if w.Code == 200 || !strings.Contains(w.Body.String(), "error") || strings.Contains(w.Body.String(), "\"traces\"") {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
}

func TestStatsDeterministicBreakdownsAndRevisionTies(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	rows := []trace.SessionRow{}
	for i := 0; i < 24; i++ {
		rows = append(rows, trace.SessionRow{ID: fmt.Sprint(i), SessionID: fmt.Sprint(i), RepoURL: fmt.Sprintf("https://example.com/p%d", i), Start: now.UnixMilli(), End: now.UnixMilli(), Cost: 1, Model: fmt.Sprint(i % 6), InputTokens: 10})
	}
	rows = append(rows, trace.SessionRow{ID: "legacy", SessionID: "0", End: now.UnixMilli(), Cost: 99}) // canonical ID wins tie
	selected := latestSessions(rows)
	a := compactStats(selected, "all", time.UTC, now)
	if len(a["projects"].([]*statsProject)) != 24 || len(a["models"].(map[string]*statsModel)) != 6 || a["spend"] != float64(24) {
		t.Fatal(a)
	}
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	b := compactStats(latestSessions(rows), "all", time.UTC, now)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("unstable latest selection or group order")
	}
	// Every prompt instance is counted in its local hour, not just one per trace.
	ts := uint64(time.Date(2026, 9, 7, 1, 0, 0, 0, time.UTC).UnixMilli())
	denver, _ := time.LoadLocation("America/Denver")
	heat := compactStats([]trace.SessionRow{{PromptTS: []uint64{ts, ts}}}, "all", denver, now)
	grid := heat["prompt_grid"].([7][24]int)
	if grid[0][19] != 2 {
		t.Fatal(grid)
	}
	// The supplied zone is echoed for inspection and DST uses IANA rules.
	if heat["timezone"] != "America/Denver" {
		t.Fatal(heat["timezone"])
	}
}

func TestProjectedStatsPreservesMissingAndNullPredicates(t *testing.T) {
	s, f := dashboardFixture(t)
	newer := mapOf(trace.SessionRow{ID: "latest-missing", SessionID: "trace-0", End: time.Now().UnixMilli(), Start: 1})
	for _, key := range []string{"cost", "total_tokens", "tool_count", "wall_ms", "start", "repo_url", "model", "harness", "host"} {
		delete(newer, key)
	}
	newer["cost"] = nil
	f.rows["test-sessions"] = append(f.rows["test-sessions"], newer)
	for _, query := range []string{"cost_min=0", "cost_max=0", "tokens_min=0", "tools_max=0", "wall_max=0", "since=0", "until=1", "project=", "model=", "host=", "harness="} {
		t.Run(query, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/api/stats?window=all&"+query, nil)
			ref := New(s.reader)
			rows, _, err := ref.rows(r)
			if err != nil {
				t.Fatal(err)
			}
			got := requestJSON(t, s, r.URL.String())
			if got["traces"] != float64(len(rows)) {
				t.Fatalf("missing/null resurrected: query=%s got=%v reference=%d", query, got["traces"], len(rows))
			}
		})
	}
}

func TestStatsEmptySource(t *testing.T) {
	_, f := dashboardFixture(t)
	f.rows["test-sessions"] = []map[string]any{}
	wire := httptest.NewServer(f)
	defer wire.Close()
	s := New(layer.New(wire.URL, "fixture", "test", ""))
	data := requestJSON(t, s, "/api/stats?window=all")
	if data["traces"] != float64(0) || data["spend"] != float64(0) {
		t.Fatal(data)
	}
}

func TestStatsLegacyZeroPromptCardAndDST(t *testing.T) {
	now := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	// Both instants land in 01:30 on the fall DST transition in Denver.
	a := uint64(time.Date(2026, 11, 1, 7, 30, 0, 0, time.UTC).UnixMilli())
	b := uint64(time.Date(2026, 11, 1, 8, 30, 0, 0, time.UTC).UnixMilli())
	loc, _ := time.LoadLocation("America/Denver")
	data := compactStats([]trace.SessionRow{{Cost: 4, PromptTS: []uint64{a, b}}}, "all", loc, now)
	if data["cost_per_prompt"] != float64(4) {
		t.Fatal("legacy zero-prompt card changed", data)
	}
	grid := data["prompt_grid"].([7][24]int)
	if grid[0][1] != 2 {
		t.Fatal("DST prompt instances lost", grid)
	}
}
