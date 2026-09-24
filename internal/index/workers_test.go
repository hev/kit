package index

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
)

func TestReadSideWorkersBoundWritesAndRetryFailedUnits(t *testing.T) {
	root := t.TempDir()
	for i := range 6 {
		data := fmt.Sprintf(`{"type":"user","uuid":"u%d","sessionId":"s%d","timestamp":"2026-09-05T10:00:00Z","message":{"role":"user","content":"hello"}}`, i, i)
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("%d.jsonl", i)), []byte(data+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var mu sync.Mutex
	active, peak, writes := 0, 0, 0
	fail := true
	gate := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		active++
		writes++
		peak = max(peak, active)
		if active == 3 {
			select {
			case <-gate:
			default:
				close(gate)
			}
		}
		mu.Unlock()
		defer func() { mu.Lock(); active--; mu.Unlock() }()
		select {
		case <-gate:
		case <-time.After(3 * time.Second):
			t.Error("writes did not overlap")
		}
		var body struct {
			Rows []struct {
				SessionID string `json:"session_id"`
			} `json:"upsert_rows"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if !strings.HasSuffix(r.URL.Path, "-blocks") && !strings.HasSuffix(r.URL.Path, "-sessions") {
			t.Errorf("embedding path contacted: %s", r.URL.Path)
		}
		if fail && strings.HasSuffix(r.URL.Path, "-blocks") && body.Rows[0].SessionID == "s2" {
			http.Error(w, "fixture write failure", 500)
			return
		}
		fmt.Fprint(w, `{"status":"OK","rows_upserted":1}`)
	}))
	defer srv.Close()
	src := &trace.ClaudeSource{Root: root}
	st := &State{Units: map[string]string{}}
	cl := layer.New(srv.URL, "fixture", "test", "")
	done := 0
	opt := Options{ReadSide: true, Workers: 3, Progress: func(n, total int, _ string) {
		if n != done+1 || total != 6 {
			t.Errorf("progress %d/%d after %d", n, total, done)
		}
		done = n
	}}
	rep, err := Run(src, cl, st, opt)
	if err != nil {
		t.Fatal(err)
	}
	if peak != 3 || rep.UnitsIndexed != 5 || rep.BlockRowsUpserted != 5 || rep.SessionRowsUpserted != 5 || len(rep.Errors) != 1 || rep.RowsUpserted != 0 || rep.EmbeddingTokens != 0 || len(st.Units) != 5 {
		t.Fatalf("peak=%d state=%d report=%+v", peak, len(st.Units), rep)
	}
	if _, ok := st.Units[filepath.Join(root, "2.jsonl")]; ok {
		t.Fatal("failed unit marked complete")
	}
	fail = false
	done = 0
	before := writes
	rep, err = Run(src, cl, st, opt)
	if err != nil {
		t.Fatal(err)
	}
	if rep.UnitsSkipped != 5 || rep.UnitsIndexed != 1 || len(rep.Errors) != 0 || writes-before != 2 || len(st.Units) != 6 {
		t.Fatalf("retry: %+v writes=%d", rep, writes-before)
	}
}

func TestWorkersRequireBoundedReadSideMode(t *testing.T) {
	for _, opt := range []Options{{Workers: 2}, {ReadSide: true, Workers: 9}, {ReadSide: true, Workers: -1}} {
		if _, err := Run(nil, nil, nil, opt); err == nil {
			t.Fatalf("accepted %+v", opt)
		}
	}
}
