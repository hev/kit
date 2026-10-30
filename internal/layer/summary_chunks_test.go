package layer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSummaryUserChunksIncludesPart(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var query struct {
			Include []string `json:"include_attributes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
			t.Error(err)
		}
		found := false
		for _, attr := range query.Include {
			if attr == "part" {
				found = true
			}
		}
		if !found {
			t.Error("summary chunks query omitted part")
		}
		w.Write([]byte(`{"rows":[{"session_id":"s","seq":1,"part":2,"role":"user","block_type":"text","text":"continuation"}]}`))
	}))
	defer srv.Close()
	rows, err := New(srv.URL, "key", "test", "").SummaryUserChunks(context.Background(), []string{"s"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Part != 2 {
		t.Fatalf("chunk parts not decoded: %+v", rows)
	}
}
