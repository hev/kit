package layer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hev/kit/internal/trace"
)

func TestTimingIsolatedConcurrentRequestsAndFailures(t *testing.T) {
	store := &sessionSourceStore{rows: map[string]map[string]any{"s": {"id": "s", "session_id": "s"}}, schema: map[string]any{}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "-sessions/query") {
			var b map[string]any
			json.NewDecoder(r.Body).Decode(&b)
			if b["filters"] == nil {
				fmt.Fprint(w, `{"rows":[{"id":"s","session_id":"s"}]}`)
				return
			}
			raw, _ := json.Marshal(b)
			r.Body = io.NopCloser(bytes.NewReader(raw))
		}
		if strings.Contains(r.URL.Path, "-sessions") {
			store.handle(w, r)
			return
		}
		http.Error(w, "store unavailable", 502)
	}))
	defer server.Close()
	client := New(server.URL, "", "test", "")
	var wg sync.WaitGroup
	collectors := make([]*Timing, 8)
	for i := range collectors {
		collectors[i] = &Timing{}
		wg.Add(1)
		go func(timing *Timing) {
			defer wg.Done()
			c := client.WithTiming(timing)
			if _, err := c.ListSlimSessionRows(-1, nil); err != nil {
				t.Error(err)
			}
			if _, err := c.WriteSessions([]trace.SessionRow{{ID: "s"}, {ID: "t"}}); err != nil {
				t.Error(err)
			}
			if _, err := c.ListBlockRows("s"); err == nil {
				t.Error("expected store failure")
			}
		}(collectors[i])
	}
	wg.Wait()
	for _, got := range collectors {
		// Each source row has a read, conditional write and persisted readback.
		if got.Queries != 9 || got.Rows < 6 || got.Rows > 7 || got.LayerMS <= 0 {
			t.Fatalf("cross-request timing or dropped call: %+v", got)
		}
	}
}

func TestSlimProjectionOmitsUndeclaredLargeAttributes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["include_attributes"] != nil {
			t.Error("slim read includes all attributes")
		}
		got, _ := json.Marshal(body["exclude_attributes"])
		if string(got) != `["first_prompt","vector"]` {
			t.Error(string(got))
		}
		fmt.Fprint(w, `{"rows":[{"id":"legacy","session_id":"legacy"}]}`)
	}))
	defer server.Close()
	if _, err := New(server.URL, "", "test", "").ListSlimSessionRows(-1, nil); err != nil {
		t.Fatal(err)
	}
}
