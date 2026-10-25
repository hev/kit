package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hev/kit/internal/layer"
)

var parallelStatsQueries = []string{
	"window=all", "window=7d", "window=30d", "window=90d", "window=07d", "window=all&cost_max=0.5", "window=all&project=stale", "window=all&model=retired",
	"window=all&project=alpha&model=model-a", "window=all&poor=true", "window=all&poor=false", "window=all&poor=false&role=author", "window=all&mark_accuracy_max=2", "window=all&role=reviewer&instance=nightly",
	"window=all&summary=true", "window=all&summary=false", "window=all&timezone=America/Denver", "window=all&timezone=America/New_York&weekday=0&hour=1",
	"window=all&tool=Bash&tokens_min=100&wall_max=10", "window=all&project=missing",
}

func equalStats(t *testing.T, reference, candidate *Server) {
	t.Helper()
	for _, query := range parallelStatsQueries {
		for _, path := range []string{"/api/stats?", "/api/sessions?include_stats=true&"} {
			a, b := requestJSON(t, reference, path+query), requestJSON(t, candidate, path+query)
			for _, obj := range []map[string]any{a, b} {
				delete(obj, "timing")
				delete(obj, "generated_at")
				if stats, ok := obj["stats"].(map[string]any); ok {
					delete(stats, "generated_at")
				}
			}
			if !reflect.DeepEqual(a, b) {
				t.Fatalf("complete output differs at %s%s\nreference=%v\ncandidate=%v", path, query, a, b)
			}
		}
	}
}

func TestStatsSelectsProjectedSourceAndPreservesCompleteOutput(t *testing.T) {
	reference, fixture := dashboardFixture(t)
	var scans, queries atomic.Int32
	wire := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "-sessions/query") {
			queries.Add(1)
		}
		if !strings.HasSuffix(r.URL.Path, "/scans/rows") {
			fixture.ServeHTTP(w, r)
			return
		}
		scans.Add(1)
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		attrs, ok := body["include_attributes"].([]any)
		if !ok || body["scan"] == nil || body["filters"] != nil {
			t.Errorf("bad source request %v", body)
		}
		for _, a := range attrs {
			if a == "first_prompt" || a == "outcome_details" || a == "vector" {
				t.Errorf("full trace projection %v", attrs)
			}
		}
		query, _ := json.Marshal(map[string]any{"include_attributes": attrs, "top_k": 1000000, "rank_by": []string{"id", "asc"}})
		recorder := httptest.NewRecorder()
		fixture.ServeHTTP(recorder, httptest.NewRequest("POST", "/v2/namespaces/test-sessions/query", bytes.NewReader(query)))
		var out map[string]any
		_ = json.Unmarshal(recorder.Body.Bytes(), &out)
		out["source"] = map[string]any{"rows": len(out["rows"].([]any)), "pages": 16, "bytes": recorder.Body.Len(), "complete": true, "consistency": "observed", "threads": 8}
		_ = json.NewEncoder(w).Encode(out)
	}))
	defer wire.Close()
	candidate := New(layer.New(wire.URL, "fixture", "test", ""))
	equalStats(t, reference, candidate)
	if scans.Load() != int32(2*len(parallelStatsQueries)) || queries.Load() != 0 {
		t.Fatalf("unused or duplicated source path scans=%d queries=%d", scans.Load(), queries.Load())
	}
}

func TestStatsProjectedSourceCancellation(t *testing.T) {
	_, fixture := dashboardFixture(t)
	entered, observed := make(chan struct{}), make(chan struct{})
	wire := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/scans/rows") {
			fixture.ServeHTTP(w, r)
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		close(entered)
		select {
		case <-r.Context().Done():
			close(observed)
		case <-time.After(5 * time.Second):
			t.Error("source ignored cancellation")
		}
	}))
	defer wire.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	w := httptest.NewRecorder()
	go func() {
		defer close(done)
		New(layer.New(wire.URL, "fixture", "test", "")).Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/stats?window=all", nil).WithContext(ctx))
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("parallel source unused")
	}
	cancel()
	select {
	case <-observed:
	case <-time.After(5 * time.Second):
		t.Fatal("cancellation not propagated")
	}
	<-done
	if w.Code == 200 {
		t.Fatal("cancelled stats succeeded")
	}
}

// Invoked by Layer's real Rust HTTP integration fixture. Export and consume the
// same rich fixture so the sequential reference and actual gateway are identical.
func TestStatsRustProjectedSource(t *testing.T) {
	if dir := os.Getenv("KIT_STATS_BRIDGE_DIR"); dir != "" {
		reference, f := dashboardFixture(t)
		f.rows["test-sessions"] = append(f.rows["test-sessions"], map[string]any{"id": "zero", "session_id": "zero", "end": nil, "start": 0, "cost": 2, "prompt_count": 3, "prompt_ts": []uint64{1636263000000, 1636266600000}, "repo_url": " trailing.git ", "model": "legacy"})
		for i, row := range f.rows["test-sessions"] {
			row["_hevlayer_shard"] = float64(i % 16)
		}
		upstream := httptest.NewServer(f)
		defer upstream.Close()
		if err := os.WriteFile(filepath.Join(dir, "upstream"), []byte(upstream.URL), 0600); err != nil {
			t.Fatal(err)
		}
		var endpoint []byte
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			var err error
			endpoint, err = os.ReadFile(filepath.Join(dir, "gateway"))
			if err == nil && len(endpoint) > 0 {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if len(endpoint) == 0 {
			t.Fatal("Rust gateway receipt absent")
		}
		equalStats(t, reference, New(layer.New(string(endpoint), "fixture", "test", "")))
		t.Log("actual Rust /scans/rows: complete Stats and paired-list parity passed")
		return
	}
	if path := os.Getenv("KIT_STATS_FIXTURE_EXPORT"); path != "" {
		_, f := dashboardFixture(t)
		f.rows["test-sessions"] = append(f.rows["test-sessions"], map[string]any{"id": "zero", "session_id": "zero", "end": nil, "start": 0, "cost": 2, "prompt_count": 3, "prompt_ts": []uint64{1636263000000, 1636266600000}, "repo_url": " trailing.git ", "model": "legacy"})
		raw, _ := json.Marshal(f.rows)
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		return
	}
	endpoint := os.Getenv("KIT_STATS_RUST_URL")
	if endpoint == "" {
		t.Skip("run through Layer projected_source_kit_http_bridge")
	}
	raw, err := os.ReadFile(os.Getenv("KIT_STATS_FIXTURE"))
	if err != nil {
		t.Fatal(err)
	}
	var rows map[string][]map[string]any
	if err = json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	f := &wireFixture{rows: rows}
	wire := httptest.NewServer(f)
	defer wire.Close()
	equalStats(t, New(layer.New(wire.URL, "fixture", "test", "")), New(layer.New(endpoint, "fixture", "test", "")))
	fmt.Println("actual Rust /scans/rows: complete Stats and paired-list parity passed")
}
