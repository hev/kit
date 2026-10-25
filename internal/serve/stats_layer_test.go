package serve

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
)

// This synthetic adapter exercises Kit's HTTP consumer, not the Rust executor.
// Its limited plans deliberately require valid nonnull end/start and string IDs.
// Complete Stats must never infer eligibility from this smaller supported mix.
func TestLatestAggregateFixtureConsumerParity(t *testing.T) {
	_, fixture := dashboardFixture(t)
	var source []trace.SessionRow
	for _, raw := range fixture.rows["test-sessions"] {
		encoded, _ := json.Marshal(raw)
		var row trace.SessionRow
		if err := json.Unmarshal(encoded, &row); err != nil {
			t.Fatal(err)
		}
		source = append(source, row)
	}
	// Tie at end: canonical ID wins over the lexically greater physical revision.
	canonical := source[0]
	loser := canonical
	loser.ID = "zz-legacy"
	loser.Cost = 999
	source = append(source, loser)
	selected := latestSessions(source)
	loc, err := time.LoadLocation("America/Denver")
	if err != nil {
		t.Fatal(err)
	}
	countCalls, sourceRows, sourceBytes, outputBytes := 0, 0, 0, 0
	sourceJSON, _ := json.Marshal(source)
	wire := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/namespaces/test-sessions/aggregate" || r.Header.Get("Authorization") != "Bearer fixture" {
			http.Error(w, "wrong tenant", 403)
			return
		}
		var q layer.LatestRequest
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			t.Error(err)
			http.Error(w, "bad request", 400)
			return
		}
		if !reflect.DeepEqual(q.LatestBy, layer.LatestSpec{Key: []string{"session_id"}, OrderBy: []layer.LatestOrder{{Field: "end", Direction: "desc", Nulls: "last"}, {Field: "id", Direction: "desc", Nulls: "last", CanonicalKey: "session_id"}}}) {
			t.Error("winner order changed")
		}
		if !reflect.DeepEqual(q.Filters, []any{"model", "Eq", "model-a"}) {
			t.Errorf("filter lost: %v", q.Filters)
		}
		countCalls++
		sourceRows += len(source)
		sourceBytes += len(sourceJSON)
		type entry struct {
			key     []map[string]any
			metrics map[string]float64
		}
		groups := map[string]map[string]*entry{}
		totals := map[string]float64{"n": 0, "spend": 0, "tokens": 0}
		for _, row := range selected {
			if row.Model != "model-a" {
				continue
			}
			totals["n"]++
			totals["spend"] += row.Cost
			totals["tokens"] += float64(row.InputTokens)
			for name, plan := range q.Grouped {
				if groups[name] == nil {
					groups[name] = map[string]*entry{}
				}
				var instances [][]map[string]any
				switch name {
				case "models":
					instances = append(instances, []map[string]any{{"type": "string", "value": row.Model}})
				case "days":
					instances = append(instances, []map[string]any{{"type": "string", "value": time.UnixMilli(row.Start).UTC().Format("2006-01-02")}})
				case "heat":
					if plan.Expand == nil || plan.Expand.Kind != "timestamps" || plan.Expand.Field != "prompt_ts" || plan.Expand.Timezone != loc.String() {
						t.Error("timestamp expansion lost")
					}
					for _, ts := range row.PromptTS {
						d := time.UnixMilli(int64(ts)).In(loc)
						instances = append(instances, []map[string]any{{"type": "number", "value": fmt.Sprint((int(d.Weekday()) + 6) % 7)}, {"type": "number", "value": fmt.Sprint(d.Hour())}})
					}
				default:
					t.Errorf("unexpected plan %s", name)
				}
				for _, key := range instances {
					encoded, _ := json.Marshal(key)
					id := string(encoded)
					if groups[name][id] == nil {
						groups[name][id] = &entry{key: key, metrics: map[string]float64{}}
					}
					for metric, spec := range plan.Metrics {
						value := float64(1)
						if spec.Kind == "sum" {
							switch spec.Field {
							case "cost":
								value = row.Cost
							case "input_tokens":
								value = float64(row.InputTokens)
							default:
								t.Errorf("unsupported sum %s", spec.Field)
							}
						}
						groups[name][id].metrics[metric] += value
					}
				}
			}
		}
		pages := map[string]any{}
		for name, plan := range q.Grouped {
			ids := []string{}
			for id := range groups[name] {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			offset := 0
			if plan.Cursor != "" {
				if _, err := fmt.Sscanf(plan.Cursor, "fixture-%d", &offset); err != nil {
					t.Error(err)
				}
			}
			end := min(len(ids), offset+plan.PageSize)
			out := []any{}
			for _, id := range ids[offset:end] {
				g := groups[name][id]
				out = append(out, map[string]any{"key": g.key, "aggregates": g.metrics})
			}
			next := ""
			if end < len(ids) {
				next = fmt.Sprintf("fixture-%d", end)
			}
			pages[name] = map[string]any{"groups": out, "next_cursor": next, "complete": next == "", "total_groups": len(ids), "binding": name + "-synthetic", "relation": map[string]any{"complete": true, "consistency": "observed_scan"}}
		}
		raw, _ := json.Marshal(map[string]any{"aggregations": totals, "grouped": pages, "relation": map[string]any{"complete": true, "consistency": "observed", "read_boundary": "observed_pages", "source_rows": len(source), "source_pages": 1, "source_bytes": len(sourceJSON), "selected_rows": len(selected), "matching_rows": 3, "plan": "bounded_source_scan"}})
		outputBytes += len(raw)
		_, _ = w.Write(raw)
	}))
	defer wire.Close()
	metrics := map[string]layer.LatestMetric{"n": {Kind: "count"}, "spend": {Kind: "sum", Field: "cost"}, "tokens": {Kind: "sum", Field: "input_tokens"}}
	request := layer.LatestRequest{
		LatestBy: layer.LatestSpec{Key: []string{"session_id"}, OrderBy: []layer.LatestOrder{{Field: "end", Direction: "desc", Nulls: "last"}, {Field: "id", Direction: "desc", Nulls: "last", CanonicalKey: "session_id"}}},
		Scan:     layer.LatestScan{MaxRows: 100, MaxBytes: 1000000, Consistency: "observed"}, Filters: []any{"model", "Eq", "model-a"}, AggregateBy: map[string][]string{"n": {"Count"}, "spend": {"Sum", "cost"}, "tokens": {"Sum", "input_tokens"}},
		Grouped: map[string]layer.LatestGroupPlan{
			"models": {GroupBy: []layer.LatestDimension{{Kind: "field", Field: "model"}}, Metrics: metrics, PageSize: 1},
			"days":   {GroupBy: []layer.LatestDimension{{Kind: "utc_day", Field: "start"}}, Metrics: metrics, PageSize: 1},
			"heat":   {Expand: &layer.LatestExpansion{Kind: "timestamps", Field: "prompt_ts", Timezone: loc.String()}, GroupBy: []layer.LatestDimension{{Kind: "weekday"}, {Kind: "hour"}}, Metrics: map[string]layer.LatestMetric{"n": {Kind: "count"}}, PageSize: 1},
		},
	}
	result, err := layer.New(wire.URL, "fixture", "test-sessions", "").AggregateLatest(request)
	if err != nil {
		t.Fatal(err)
	}
	rows := []trace.SessionRow{}
	for _, row := range selected {
		if row.Model == "model-a" {
			rows = append(rows, row)
		}
	}
	reference := compactStats(rows, "all", loc, time.Now())
	for metric, key := range map[string]string{"n": "traces", "spend": "spend", "tokens": "tokens"} {
		number, err := result.Aggregations[metric].Float64()
		if err != nil {
			t.Fatal(err)
		}
		want := float64(0)
		switch v := reference[key].(type) {
		case int:
			want = float64(v)
		case int64:
			want = float64(v)
		case float64:
			want = v
		}
		if number != want {
			t.Errorf("%s got %v want %v", metric, number, want)
		}
	}
	if len(result.Grouped["models"]) != 1 || len(result.Grouped["days"]) != 3 {
		t.Fatalf("lost grouped output: %+v", result.Grouped)
	}
	grid := [7][24]int{}
	for _, g := range result.Grouped["heat"] {
		var day, hour string
		_ = json.Unmarshal(g.Key[0].Value, &day)
		_ = json.Unmarshal(g.Key[1].Value, &hour)
		var d, h int
		_, _ = fmt.Sscan(day, &d)
		_, _ = fmt.Sscan(hour, &h)
		n, _ := g.Aggregates["n"].Int64()
		grid[(d+1)%7][h] = int(n)
	}
	if grid != reference["prompt_grid"] {
		t.Fatalf("prompt instances differ: %v vs %v", grid, reference["prompt_grid"])
	}
	if countCalls != 3 || len(result.Executions) != 3 || sourceRows != len(source)*3 {
		t.Fatalf("rescan/replay accounting calls=%d rows=%d", countCalls, sourceRows)
	}
	t.Logf("synthetic cold supported partial-contract consumer: requests=%d mock_full_source_rows=%d mock_decoded_source_bytes=%d HTTP_output_bytes=%d; each continuation repeats all source work; not complete Stats or backend-native performance", countCalls, sourceRows, sourceBytes, outputBytes)
}

func TestStatsCompleteContractRetainsSingleScan(t *testing.T) {
	original, fixture := dashboardFixture(t)
	// Include incomplete prompt arrays: coverage is per trace with any timestamps,
	// not prompt_count and not expanded instance count.
	fixture.rows["test-sessions"][0]["prompt_count"] = float64(20)
	fixture.rows["test-sessions"][0]["prompt_ts"] = []any{float64(time.Now().UnixMilli())}
	for _, query := range []string{"window=all", "window=7d&timezone=America/Denver", "window=all&model=model-a", "window=all&tool=Bash", "window=all&summary=true", "window=all&poor=false", "window=all&timezone=America/Denver&weekday=0&hour=1"} {
		t.Run(query, func(t *testing.T) {
			aggregateCalls, scans := 0, 0
			wire := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/aggregate") {
					aggregateCalls++
					http.Error(w, "aggregate must not precede complete fallback", 500)
					return
				}
				if strings.HasSuffix(r.URL.Path, "-sessions/query") {
					scans++
				}
				fixture.ServeHTTP(w, r)
			}))
			defer wire.Close()
			candidate := New(layer.New(wire.URL, "fixture", "test", ""))
			got := requestJSON(t, candidate, "/api/stats?"+query)
			want := requestJSON(t, original, "/api/stats?"+query)
			for _, value := range []map[string]any{got, want} {
				delete(value, "generated_at")
				delete(value, "timing")
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatal("complete fallback response changed")
			}
			if aggregateCalls != 0 || scans != 1 {
				t.Fatalf("double scan regression aggregate=%d projected=%d", aggregateCalls, scans)
			}
		})
	}
}
