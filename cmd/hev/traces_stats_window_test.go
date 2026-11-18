package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/layer"
	tracepkg "github.com/hev/kit/internal/trace"
)

// A run first launched before the window and reworked inside it is not a
// run started in the window, even when the span query only returns attempts
// that start inside it.
func TestHostedSpanWindowKeepsPreWindowLaunch(t *testing.T) {
	store, _, _ := statsFixture(t)
	now := time.Now()
	from := now.Add(-5 * 24 * time.Hour).UnixMilli()
	all := []tracepkg.SpanRow{
		{ID: "old", Kind: "attempt", RunKey: "same", Host: "h1", Start: from - 1000},
		{ID: "rework", Kind: "attempt", RunKey: "same", Host: "h1", Start: from + 1000},
		{ID: "fresh", Kind: "attempt", RunKey: "new", Host: "h2", Start: from + 2000},
	}
	windowed := func(_ traceStore, lower int64) ([]tracepkg.SpanRow, error) {
		var out []tracepkg.SpanRow
		for _, s := range all {
			if s.Start >= lower {
				out = append(out, s)
			}
		}
		return out, nil
	}
	for _, withHistory := range []bool{false, true} {
		rt := statsRuntime{Store: func(string) (traceStore, error) { return store, nil }, Now: func() time.Time { return now }, Spans: windowed}
		if withHistory {
			rt.RunHistory = func(traceStore, []tracepkg.SpanRow) ([]tracepkg.SpanRow, error) { return all, nil }
		}
		rep, err := buildTracesStats(rt, statsOptions{since: "5d", by: "kind"}, io.Discard)
		if err != nil {
			t.Fatal(err)
		}
		if rep.Dispatch.Runs != 1 || len(rep.Dispatch.ByHost) != 1 || rep.Dispatch.ByHost[0].Host != "h2" {
			t.Fatalf("history=%v: dispatch %+v, want only the run launched in the window", withHistory, rep.Dispatch)
		}
	}
}

// The default runtime against a hosted archive: the windowed span query
// misses the original launch, and the run-history query by run directory
// (the filterable attribute) brings it back.
func TestHostedSpanQueryFetchesRunHistory(t *testing.T) {
	now := time.Now()
	from := now.Add(-5 * 24 * time.Hour).UnixMilli()
	all := []tracepkg.SpanRow{
		{ID: "old", Kind: "attempt", RunKey: "k1", Run: "r1", Publisher: "p", Host: "h1", Start: from - 1000},
		{ID: "rework", Kind: "attempt", RunKey: "k1", Run: "r1", Publisher: "p", Host: "h1", Start: from + 1000},
		{ID: "legacy-old", Kind: "attempt", Run: "r2", Publisher: "p", Host: "h1", Start: from - 5000},
		{ID: "legacy-rework", Kind: "attempt", Run: "r2", Publisher: "p", Host: "h1", Start: from + 5000},
		{ID: "fresh", Kind: "attempt", RunKey: "k3", Run: "r3", Publisher: "p", Host: "h2", Start: from + 2000},
		// The same directory name on another host, long ago: another run.
		{ID: "other-host", Kind: "attempt", RunKey: "k9", Run: "r3", Publisher: "q", Host: "h3", Start: from - 9000},
	}
	var filters []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Filters any `json:"filters"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f, _ := json.Marshal(body.Filters)
		if !strings.HasSuffix(r.URL.Path, "-spans/query") {
			_, _ = w.Write([]byte(`{"rows":[]}`))
			return
		}
		filters = append(filters, string(f))
		var out []tracepkg.SpanRow
		if !strings.Contains(string(f), `"id","Gt"`) {
			for _, s := range all {
				switch {
				case strings.Contains(string(f), `"start","Gte"`):
					if s.Start >= from {
						out = append(out, s)
					}
				case strings.Contains(string(f), `"run","In"`):
					if strings.Contains(string(f), `"`+s.Run+`"`) {
						out = append(out, s)
					}
				}
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID }) // rank_by id asc
		_ = json.NewEncoder(w).Encode(map[string]any{"rows": out})
	}))
	defer srv.Close()
	lc := layer.New(srv.URL, "key", "archive", "")
	rt := defaultStatsRuntime()
	rt.Store = func(string) (traceStore, error) { return lc, nil }
	rt.Now = func() time.Time { return now }
	rep, err := buildTracesStats(rt, statsOptions{since: "5d", by: "kind"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Dispatch.Runs != 1 || rep.Dispatch.Attempts != 1 || len(rep.Dispatch.ByHost) != 1 || rep.Dispatch.ByHost[0].Host != "h2" {
		t.Fatalf("dispatch %+v\nqueries %v", rep.Dispatch, filters)
	}
	var byRun bool
	for _, f := range filters {
		byRun = byRun || strings.HasPrefix(f, `["run","In",`) && strings.Contains(f, `"r1"`) && strings.Contains(f, `"r2"`) && strings.Contains(f, `"r3"`)
	}
	if !byRun {
		t.Fatalf("history queries missing: %v", filters)
	}
}
