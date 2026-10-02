package serve

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/hev/kit/internal/layer"
)

// Exercises Server -> real Client -> HTTP JSON wire, with store-evaluated
// filters and ordered scans rather than a mock phrase-search method.
func TestPhraseSearchBeforeDedupAndLimit(t *testing.T) {
	s, f := dashboardFixture(t)
	f.hybridAll = true
	f.rows["test"] = []map[string]any{
		mapOf(layer.Hit{ID: "a", SessionID: "trace-0", Text: "own irrelevant semantic ANN match", Dist: 10}),
		mapOf(layer.Hit{ID: "b", SessionID: "trace-0", Text: "lower QUARTZ\n\tAmber exact hit", Dist: 1}),
		mapOf(layer.Hit{ID: "c", SessionID: "trace-1", Text: "amber quartz reversed", Dist: 9}),
		mapOf(layer.Hit{ID: "d", SessionID: "trace-2", Text: "quartz blue amber noncontiguous", Dist: 8}),
		mapOf(layer.Hit{ID: "e", SessionID: "foreign-session", Text: "quartz amber foreign", Dist: 20}),
	}
	hybrid := requestJSON(t, s, "/api/search?window=all&q=quartz+amber&top=1")
	hits := hybrid["sessions"].([]any)
	if hybrid["mode"] != "hybrid" || len(hits) != 1 || hits[0].(map[string]any)["snippet"] != "own irrelevant semantic ANN match" {
		t.Fatal(hybrid)
	}
	phrase := requestJSON(t, s, "/api/search?window=all&q=quartz+amber&mode=phrase&top=1")
	hits = phrase["sessions"].([]any)
	if len(hits) != 1 || hits[0].(map[string]any)["snippet"] != "lower QUARTZ\n\tAmber exact hit" || hits[0].(map[string]any)["score"] != float64(0) {
		t.Fatal(phrase)
	}
	if phrase["candidate_limit"] != float64(10000) || phrase["candidate_truncated"].(map[string]any)["transcript"] != false {
		t.Fatal(phrase)
	}
	for _, filter := range []string{"project=beta", "model=model-b", "host=mini", "harness=codex", "poor=false", "project=unknown", "since=2999-01-01"} {
		got := requestJSON(t, s, "/api/search?window=all&q=quartz+amber&mode=phrase&"+filter)
		if len(got["sessions"].([]any)) != 0 {
			t.Fatalf("filter %s: %v", filter, got)
		}
	}
	for _, filter := range []string{"project=alpha", "model=model-a", "host=laptop", "harness=claude_code", "poor=true", "role=reviewer&instance=nightly"} {
		got := requestJSON(t, s, "/api/search?window=all&q=quartz+amber&mode=phrase&"+filter)
		if len(got["sessions"].([]any)) != 1 {
			t.Fatalf("filter %s: %v", filter, got)
		}
	}
}

func TestPhraseEvalTextAndInvalidMode(t *testing.T) {
	s, f := dashboardFixture(t)
	// Existing latest-eval selection remains unchanged.
	for _, r := range f.rows["test-evals"] {
		if r["session_id"] == "trace-1" && r["role"] == "reviewer" {
			r["text"] = "older exact eval"
		}
	}
	// High nonmatching transcript candidates must not suppress an exact eval.
	f.hybridAll = true
	got := requestJSON(t, s, "/api/search?window=all&q=distinct+EVALUATOR+phrase&mode=phrase&poor=true&top=1")
	hits := got["sessions"].([]any)
	if len(hits) != 1 || hits[0].(map[string]any)["source"] != "eval" {
		t.Fatal(got)
	}
	for _, q := range []string{"phrase+evaluator+distinct", "distinct+phrase", "Distinct%2C+evaluator+phrase"} {
		got = requestJSON(t, s, "/api/search?window=all&q="+q+"&mode=phrase")
		if len(got["sessions"].([]any)) != 0 {
			t.Fatal(got)
		}
	}
	got = requestJSON(t, s, "/api/search?window=all&q=older+exact+eval&mode=phrase")
	if len(got["sessions"].([]any)) != 0 {
		t.Fatal(got)
	}
	for _, mode := range []string{"invalid", "PHRASE", "ann"} {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/search?q=x&mode="+mode, nil))
		if w.Code != 400 {
			t.Fatalf("mode %s: %d", mode, w.Code)
		}
	}
}

func TestPhraseSearchReportsIncompleteScan(t *testing.T) {
	s, f := dashboardFixture(t)
	f.rows["test"] = nil
	for i := 0; i < 10001; i++ {
		text := "nonmatching text"
		if i == 10000 {
			text = "exact overflow phrase"
		}
		f.rows["test"] = append(f.rows["test"], mapOf(layer.Hit{ID: fmt.Sprintf("%05d", i), SessionID: "trace-0", Text: text}))
	}
	got := requestJSON(t, s, "/api/search?window=all&q=exact+overflow+phrase&mode=phrase&top=1")
	if len(got["sessions"].([]any)) != 0 || got["candidate_truncated"].(map[string]any)["transcript"] != true {
		t.Fatal(got)
	}
}
