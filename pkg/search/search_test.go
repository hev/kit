package search

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestBodyOneLegPairPerPhrasing(t *testing.T) {
	body, err := Query{Phrasings: []string{"port collides", " ", "5432"}, TopK: 5,
		Filter: []any{"board", "Eq", "tooling"}, Attrs: []string{"text"}}.Body()
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(body)
	want := `{"queries":[` +
		`{"filters":["board","Eq","tooling"],"include_attributes":["text"],"rank_by":["text","ANN",["Embed","port collides"]],"top_k":5},` +
		`{"filters":["board","Eq","tooling"],"include_attributes":["text"],"rank_by":["text","BM25","port collides"],"top_k":5},` +
		`{"filters":["board","Eq","tooling"],"include_attributes":["text"],"rank_by":["text","ANN",["Embed","5432"]],"top_k":5},` +
		`{"filters":["board","Eq","tooling"],"include_attributes":["text"],"rank_by":["text","BM25","5432"],"top_k":5}` +
		`],"rerank_by":["RRF"]}`
	if string(raw) != want {
		t.Fatalf("body\n got %s\nwant %s", raw, want)
	}
}

func TestPhrasingLimits(t *testing.T) {
	if _, err := (Query{}).Body(); err == nil {
		t.Fatal("no phrasings accepted")
	}
	if _, err := (Query{Phrasings: strings.Split("a b c d e f g h i", " ")}).Body(); err == nil {
		t.Fatal("nine phrasings accepted")
	}
	if _, err := (Query{Phrasings: []string{"a", "b"}}).HybridTextBody(nil); err == nil {
		t.Fatal("HybridText took two phrasings")
	}
}

func TestRowsBothEnvelopes(t *testing.T) {
	type row struct {
		ID string `json:"id"`
	}
	for _, raw := range []string{`{"results":[{"rows":[{"id":"a"}]}]}`, `{"rows":[{"id":"a"}]}`} {
		rows, err := Rows[row]([]byte(raw))
		if err != nil || len(rows) != 1 || rows[0].ID != "a" {
			t.Fatalf("%s: %v %v", raw, rows, err)
		}
	}
	if _, err := Rows[row]([]byte(`{"error":"nope"}`)); err == nil {
		t.Fatal("error envelope read as rows")
	}
}

func TestSince(t *testing.T) {
	for in, want := range map[string]time.Duration{"7d": 168 * time.Hour, "2w": 336 * time.Hour, "36h": 36 * time.Hour} {
		if got, err := ParseSince(in); err != nil || got != want {
			t.Errorf("ParseSince(%q) = %v %v", in, got, err)
		}
	}
	if _, err := ParseSince("soon"); err == nil {
		t.Error("ParseSince(soon) accepted")
	}
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	f, _ := Since("ts", "1d", time.RFC3339, now)
	if got, _ := json.Marshal(f); string(got) != `["ts","Gte","2026-09-25T12:00:00Z"]` {
		t.Errorf("Since = %s", got)
	}
	if And() != nil || And(nil, "x") != "x" {
		t.Error("And")
	}
}
