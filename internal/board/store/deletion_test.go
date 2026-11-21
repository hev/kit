package store

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hev/kit/internal/board/layer"
)

func TestLayerDeleteRecoversReplyCleanup(t *testing.T) {
	row := layer.Row{"id": "t:1", "kind": "thread", "author": "owner", "body": "private opening"}
	replies := true
	cleanupAttempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Upserts []layer.Row `json:"upsert_rows"`
			Delete  any         `json:"delete_by_filter"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		switch {
		case len(body.Upserts) != 0:
			row = body.Upserts[0]
		case body.Delete != nil:
			cleanupAttempts++
			if cleanupAttempts == 1 {
				http.Error(w, "cleanup failed", 400)
				return
			}
			replies = false
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"rows": []layer.Row{row}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "OK"})
	}))
	defer srv.Close()
	st := New(layer.New(srv.URL, "key", "test", "")).Backend.(*layerStore)
	ctx := t.Context()
	if err := st.DeleteThread(ctx, 1, "owner", false); err == nil {
		t.Fatal("cleanup failure must reach the caller")
	}
	if _, err := st.Thread(ctx, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("pending deletion must hide the thread: %v", err)
	}
	if err := st.DeleteThread(ctx, 1, "other", false); !errors.Is(err, ErrForbidden) {
		t.Fatalf("pending deletion must still check ownership: %v", err)
	}
	if err := st.DeleteThread(ctx, 1, "owner", false); err != nil {
		t.Fatalf("owner must be able to retry cleanup: %v", err)
	}
	if err := st.DeleteThread(ctx, 1, "owner", false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("completed deletion must report missing: %v", err)
	}
	if replies || cleanupAttempts != 2 || row["kind"] != "deleted" || row["author"] != nil || row["body"] != nil {
		t.Fatalf("deletion left data or allowed an extra cleanup: %v %d %v", replies, cleanupAttempts, row)
	}
}
