package store

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hev/kit/internal/board/layer"
)

func TestLayerBoardsCountAcrossBatches(t *testing.T) {
	const count = 10_001
	const newest = "2026-11-21T09:00:00Z"
	rows := make([]layer.Row, 0, count+1)
	// Equal update times force the keyset to advance by thread ID.
	for id := count; id >= 1; id-- {
		row := layer.Row{"kind": "thread", "thread_id": id, "updated": newest, "board": "tooling"}
		if id == 1 {
			row["replies"] = 2
			row["last_author"] = "last"
			row["last_created"] = newest
			row["last_n"] = 3
		}
		rows = append(rows, row)
	}
	rows = append(rows, layer.Row{"kind": "deleted", "thread_id": 0, "updated": newest, "board": "tooling"})
	st := boardReadFixture(t, rows)
	got, err := st.Board(t.Context(), "tooling")
	if err != nil {
		t.Fatal(err)
	}
	if got.Threads != count || got.Posts != count+2 || got.Last == nil || got.Last.ThreadID != 1 || got.Last.N != 3 {
		t.Fatalf("board totals must include every live thread and its replies: %+v", got)
	}
}

func TestLayerPostsReadAllRepliesInOrder(t *testing.T) {
	const replies = 10_000
	rows := []layer.Row{{"kind": "thread", "thread_id": 1, "n": 1, "body": "opening"}}
	for n := 2; n <= replies+1; n++ {
		rows = append(rows, layer.Row{"kind": "post", "thread_id": 1, "n": n, "text": fmt.Sprintf("reply %d", n)})
	}
	rows = append(rows, layer.Row{"kind": "post", "thread_id": 2, "n": replies + 2, "text": "another thread"})
	st := boardReadFixture(t, rows)
	got, err := st.Posts(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != replies+1 || got[0].Body != "opening" {
		t.Fatalf("want opening post and %d replies, got %d messages", replies, len(got))
	}
	for i, post := range got[1:] {
		n := i + 2
		if post.ThreadID != 1 || post.N != n || post.Body != fmt.Sprintf("reply %d", n) {
			t.Fatalf("message %d is missing, out of order or from another thread: %+v", n, post)
		}
	}
}

// Rows are provided in backend rank order. The fixture applies filters before
// enforcing top_k, so it cannot return an extra row beyond the query limit.
func boardReadFixture(t *testing.T, rows []layer.Row) *Store {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			Filter any `json:"filters"`
			TopK   int `json:"top_k"`
		}
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil || q.TopK <= 0 || q.TopK > 10_000 {
			t.Errorf("invalid query or batch size: %+v, %v", q, err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		matches := []layer.Row{}
		for _, row := range rows {
			if matchBoardReadFilter(t, row, q.Filter) {
				matches = append(matches, row)
				if len(matches) == q.TopK {
					break
				}
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"rows": matches})
	}))
	t.Cleanup(srv.Close)
	return New(layer.New(srv.URL, "key", "test", ""))
}

func matchBoardReadFilter(t *testing.T, row layer.Row, filter any) bool {
	if filter == nil {
		return true
	}
	f := filter.([]any)
	switch f[0] {
	case "And", "Or":
		for _, clause := range f[1].([]any) {
			if matchBoardReadFilter(t, row, clause) == (f[0] == "Or") {
				return f[0] == "Or"
			}
		}
		return f[0] == "And"
	}
	key := f[0].(string)
	switch f[1] {
	case "Eq":
		if value, ok := f[2].(string); ok {
			return str(row, key) == value
		}
		return num(row, key) == int64(f[2].(float64))
	case "Lt":
		if value, ok := f[2].(string); ok {
			return str(row, key) < value
		}
		return num(row, key) < int64(f[2].(float64))
	case "Gt":
		return num(row, key) > int64(f[2].(float64))
	default:
		t.Errorf("unexpected Board read filter: %v", f)
		return false
	}
}
