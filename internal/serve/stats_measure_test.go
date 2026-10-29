package serve

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
)

// Opt-in, synthetic localhost comparison; no credentials or live archive reads.
// Baseline uses the assigned source's selection path and list+Stats payloads;
// it conservatively excludes the old browser's five eager value-picker reads.
func TestMeasureStatsSynthetic(t *testing.T) {
	if os.Getenv("HEV_STATS_MEASURE") != "1" {
		t.Skip("set HEV_STATS_MEASURE=1 for bounded synthetic measurement")
	}
	_, fixture := dashboardFixture(t)
	now := time.Now()
	for i := 6; i < 1200; i++ {
		row := trace.SessionRow{ID: fmt.Sprintf("s-%04d", i), SessionID: fmt.Sprintf("s-%04d", i), Summary: strings.Repeat("synthetic summary ", 20), FirstPrompt: strings.Repeat("full unused prompt ", 100), FirstPromptShort: strings.Repeat("short prompt ", 30), RepoURL: fmt.Sprintf("https://example.com/p%d", i%12), Model: fmt.Sprintf("m%d", i%6), Host: "synthetic", Harness: "codex", Start: now.Add(-time.Duration(i%28) * 24 * time.Hour).UnixMilli(), End: now.UnixMilli(), Cost: float64(i % 10), PromptCount: 10, PromptTS: []uint64{uint64(now.UnixMilli()), uint64(now.Add(-time.Hour).UnixMilli())}, InputTokens: 100, OutputTokens: 100, CacheReadTokens: 800, TotalTokens: 1000, WallMS: 60000, APIMS: 30000, ToolCount: 5, ToolNames: []string{"Bash"}}
		row.OutcomeChecked = now.UnixMilli()
		row.OutcomeDetails = `{"unused":"` + strings.Repeat("outcome ", 100) + `"}`
		fixture.rows["test-sessions"] = append(fixture.rows["test-sessions"], mapOf(row))
		if i%10 == 0 {
			row.ID += "-legacy"
			row.End--
			row.Cost = 0
			fixture.rows["test-sessions"] = append(fixture.rows["test-sessions"], mapOf(row))
		}
	}
	type sourceMetrics struct{ Calls, Scans, Rows, Bytes int }
	var mu sync.Mutex
	source := sourceMetrics{}
	wire := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := httptest.NewRecorder()
		fixture.ServeHTTP(rec, r)
		var result struct {
			Rows []json.RawMessage `json:"rows"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &result)
		mu.Lock()
		source.Calls++
		source.Rows += len(result.Rows)
		source.Bytes += rec.Body.Len()
		if strings.HasSuffix(r.URL.Path, "-sessions/query") {
			source.Scans++
		}
		mu.Unlock()
		for k, vs := range rec.Header() {
			w.Header()[k] = vs
		}
		w.WriteHeader(rec.Code)
		_, _ = w.Write(rec.Body.Bytes())
	}))
	defer wire.Close()
	for _, mode := range []string{"baseline_list_then_stats", "candidate_stats_only", "candidate_list_with_stats"} {
		client := layer.New(wire.URL, "fixture", "test", "")
		s := New(client)
		for run := 0; run < 4; run++ {
			mu.Lock()
			source = sourceMetrics{}
			mu.Unlock()
			start := time.Now()
			var responses [][]byte
			if mode == "baseline_list_then_stats" {
				for _, kind := range []string{"sessions", "stats"} {
					timing := &layer.Timing{}
					request := New(client.WithTiming(timing))
					r := httptest.NewRequest("GET", "/api/"+kind+"?window=30d&timezone=UTC", nil)
					rows, window, err := request.rows(r)
					if err != nil {
						t.Fatal(err)
					}
					var payload map[string]any
					if kind == "sessions" {
						list, err := request.joinedCorpus(rows)
						if err != nil {
							t.Fatal(err)
						}
						fc, err := request.archiveFacets()
						if err != nil {
							t.Fatal(err)
						}
						payload = map[string]any{"window": window, "sessions": list, "facets": fc}
					} else {
						payload = legacyStatsPayload(rows, window)
					}
					payload["timing"] = timing
					raw, err := json.Marshal(payload)
					if err != nil {
						t.Fatal(err)
					}
					responses = append(responses, append(raw, '\n'))
				}
			} else {
				path := "/api/stats?window=30d&timezone=UTC"
				if mode == "candidate_list_with_stats" {
					path = "/api/sessions?window=30d&timezone=UTC&include_stats=true"
				}
				w := httptest.NewRecorder()
				s.Handler().ServeHTTP(w, httptest.NewRequest("GET", path, nil))
				if w.Code != 200 {
					t.Fatal(w.Body.String())
				}
				responses = append(responses, w.Body.Bytes())
			}
			duration := time.Since(start)
			rawBytes, gzipBytes := 0, 0
			for _, body := range responses {
				rawBytes += len(body)
				var b bytes.Buffer
				gz := gzip.NewWriter(&b)
				_, _ = gz.Write(body)
				_ = gz.Close()
				gzipBytes += b.Len()
			}
			mu.Lock()
			metrics := source
			mu.Unlock()
			temperature := "cold"
			if run > 0 {
				temperature = "warm_transport"
			}
			t.Logf("mode=%s run=%d temperature=%s elapsed_ms=%.3f source_calls=%d session_scan_pages=%d source_rows=%d source_bytes=%d response_bytes=%d response_gzip_bytes=%d", mode, run, temperature, float64(duration)/float64(time.Millisecond), metrics.Calls, metrics.Scans, metrics.Rows, metrics.Bytes, rawBytes, gzipBytes)
		}
	}
	t.Logf("fixture physical_session_rows=%d logical_sessions=1200; no server data reuse; warm means same HTTP client/server, not cached aggregates", len(fixture.rows["test-sessions"]))
}

// Assigned-source Stats response, retained only to measure that baseline.
func legacyStatsPayload(rows []trace.SessionRow, window string) map[string]any {
	var grid [7][24]int
	coverage := 0
	var cost float64
	var tokens, prompts, tools, apiMS, wallMS, cacheRead, cacheBase int64
	for _, r := range rows {
		if len(r.PromptTS) > 0 {
			coverage++
		}
		for _, ts := range r.PromptTS {
			d := time.UnixMilli(int64(ts)).UTC()
			grid[d.Weekday()][d.Hour()]++
		}
		cost += r.Cost
		prompts += r.PromptCount
		tools += r.ToolCount
		apiMS += r.APIMS
		wallMS += r.WallMS
		cacheRead += r.CacheReadTokens
		cacheBase += r.InputTokens + r.CacheReadTokens + r.CacheCreationTokens
		tokens += r.InputTokens + r.OutputTokens + r.CacheReadTokens + r.CacheCreationTokens
	}
	hit := 0.0
	if cacheBase > 0 {
		hit = float64(cacheRead) / float64(cacheBase)
	}
	perPrompt := 0.0
	if prompts > 0 {
		perPrompt = cost / float64(prompts)
	}
	return map[string]any{"window": window, "spend": cost, "tokens": tokens, "traces": len(rows), "api_hours": float64(apiMS) / 3600000, "wall_hours": float64(wallMS) / 3600000, "cache_hit": hit, "cost_per_prompt": perPrompt, "prompts": prompts, "tools": tools, "prompt_grid": grid, "prompt_coverage": map[string]int{"with": coverage, "total": len(rows)}}
}
