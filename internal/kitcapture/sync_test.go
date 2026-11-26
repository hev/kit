package kitcapture

import (
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

	"github.com/hev/kit/pkg/kitapi"
)

type syncRow struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	End       int64  `json:"end"`
	Text      string `json:"text,omitempty"`
}

func syncOutbox(t *testing.T) *Outbox {
	t.Helper()
	o, err := OpenOutbox(t.TempDir(), "team_test")
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func ingestFixture(t *testing.T, ingest func(http.ResponseWriter, *http.Request, []syncRow)) *kitapi.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/whoami":
			fmt.Fprint(w, `{"team":{"id":"team_test"}}`)
		case "/v1/traces/chunks", "/v1/traces/sessions":
			body, err := io.ReadAll(io.LimitReader(r.Body, kitapi.MaxBatchBytes+1))
			var batch struct {
				Rows []syncRow `json:"rows"`
			}
			if err != nil || len(body) > kitapi.MaxBatchBytes || json.Unmarshal(body, &batch) != nil || len(batch.Rows) > kitapi.MaxBatchRows {
				t.Error("ingest body exceeded the published limits or was invalid")
				w.WriteHeader(413)
				return
			}
			ingest(w, r, batch.Rows)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	client := kitapi.New(srv.URL, "hk_fixture_only")
	client.Retries = 0
	return client
}

func TestDrainCoalescesNewestFirstAndBoundsConcurrentPosts(t *testing.T) {
	o := syncOutbox(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for i := 1; i <= 10; i++ {
		rows := make([]syncRow, kitapi.MaxBatchRows/2)
		for j := range rows {
			rows[j] = syncRow{ID: fmt.Sprintf("%d/%d", i, j), SessionID: fmt.Sprint(i), End: int64(11 - i)}
		}
		// Enqueue earlier activity last, as a newest-first scanner would.
		if err := EnqueueRows(o, KindChunks, rows); err != nil {
			t.Fatal(err)
		}
	}
	started := make(chan struct{})
	release := make(chan struct{})
	var calls, active, peak atomic.Int32
	client := ingestFixture(t, func(w http.ResponseWriter, r *http.Request, rows []syncRow) {
		if len(rows) != kitapi.MaxBatchRows {
			t.Errorf("got %d rows, want %d", len(rows), kitapi.MaxBatchRows)
		}
		call := calls.Add(1)
		pending := r.Header.Get("X-Kit-Sync-Pending-Batches")
		sessions := r.Header.Get("X-Kit-Sync-Pending-Sessions")
		if r.Header.Get("X-Kit-Sync-Total-Batches") != "10" || r.Header.Get("X-Kit-Sync-Scanning") != "false" {
			t.Errorf("unexpected progress headers")
		}
		if call == 1 {
			if len(rows) == kitapi.MaxBatchRows && (rows[0].SessionID != "1" || rows[len(rows)/2].SessionID != "2") {
				t.Error("first coalesced upload skipped newer sessions")
			}
			if pending != "10" || sessions != "10" {
				t.Error("first upload did not count all pending files and sessions")
			}
		} else {
			if pending != "8" || sessions != "8" {
				t.Error("in-flight upload counts excluded files awaiting acknowledgement")
			}
			n := active.Add(1)
			defer active.Add(-1)
			for prev := peak.Load(); n > prev && !peak.CompareAndSwap(prev, n); prev = peak.Load() {
			}
			if n == drainWorkers {
				close(started)
			}
			select {
			case <-release:
			case <-ctx.Done():
			}
		}
		fmt.Fprintf(w, `{"accepted":%d}`, len(rows))
	})
	result := make(chan error, 1)
	go func() {
		res, err := o.Drain(ctx, client, nil)
		if err == nil && (res.Sent != 10 || res.Pending != 0 || res.Rows != 5*kitapi.MaxBatchRows) {
			err = fmt.Errorf("bad drain result: %+v", res)
		}
		result <- err
	}()
	select {
	case <-started:
	case err := <-result:
		t.Fatalf("drain returned before opening the worker window: %v", err)
	case <-ctx.Done():
		t.Fatal("uploads did not fill the worker window")
	}
	if names, err := o.Pending(); err != nil || len(names) != 8 {
		t.Errorf("deleted files before HTTP confirmation: pending=%d err=%v", len(names), err)
	}
	close(release)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 5 || peak.Load() != drainWorkers {
		t.Fatalf("calls=%d peak=%d", calls.Load(), peak.Load())
	}
}

func TestDrainCoalescesWithinEncodedByteLimitAndRefusesOversizedFile(t *testing.T) {
	o := syncOutbox(t)
	for i, size := range []int{kitapi.MaxBatchBytes, 2 << 20, 1 << 20, 1 << 20} {
		body, err := json.Marshal(map[string][]syncRow{"rows": {{ID: fmt.Sprint(i), End: int64(i), Text: strings.Repeat("x", size)}}})
		if err != nil {
			t.Fatal(err)
		}
		if err := o.Enqueue(KindChunks, body); err != nil {
			t.Fatal(err)
		}
	}
	var sizes []int
	client := ingestFixture(t, func(w http.ResponseWriter, _ *http.Request, rows []syncRow) {
		sizes = append(sizes, len(rows))
		fmt.Fprintf(w, `{"accepted":%d}`, len(rows))
	})
	res, err := o.Drain(t.Context(), client, nil)
	if err != nil || res.Sent != 3 || res.Failed != 1 || !reflect.DeepEqual(sizes, []int{2, 1}) {
		t.Fatalf("drain=%+v requests=%v err=%v", res, sizes, err)
	}
}

func TestDrainIsolatesRejectedFilesInCoalescedUpload(t *testing.T) {
	for _, partial := range []bool{false, true} {
		t.Run(fmt.Sprintf("partial_2xx=%t", partial), func(t *testing.T) {
			o := syncOutbox(t)
			for i, id := range []string{"old", "refused", "new"} {
				if err := EnqueueRows(o, KindSessions, []syncRow{{ID: id, SessionID: id, End: int64(i + 1)}}); err != nil {
					t.Fatal(err)
				}
			}
			const sensitive = "hk_fixture_only /private/transcripts/secret.jsonl"
			var accepted []string
			var coalesced bool
			client := ingestFixture(t, func(w http.ResponseWriter, _ *http.Request, rows []syncRow) {
				coalesced = coalesced || len(rows) == 3
				rejected := -1
				for i, row := range rows {
					if row.ID == "refused" {
						rejected = i
					}
				}
				if rejected >= 0 && !partial {
					http.Error(w, sensitive, 400)
					return
				}
				for i, row := range rows {
					if i != rejected {
						accepted = append(accepted, row.ID)
					}
				}
				if rejected >= 0 {
					fmt.Fprintf(w, `{"accepted":%d,"rejected":[{"index":%d,"error":%q}]}`, len(rows)-1, rejected, sensitive)
				} else {
					fmt.Fprintf(w, `{"accepted":%d}`, len(rows))
				}
			})
			res, err := o.Drain(t.Context(), client, nil)
			if err != nil || res.Pending != 0 || res.Sent != 2 || res.Failed != 1 || !coalesced ||
				!reflect.DeepEqual(accepted, []string{"new", "old"}) {
				t.Fatalf("drain=%+v accepted=%v combined=%v err=%v", res, accepted, coalesced, err)
			}
			dir := filepath.Join(o.Dir(), "failed")
			files, err := os.ReadDir(dir)
			if err != nil || len(files) != 2 {
				t.Fatalf("failed files=%d err=%v", len(files), err)
			}
			body, err := os.ReadFile(filepath.Join(dir, files[0].Name()))
			if err != nil || !strings.Contains(string(body), `"id":"refused"`) {
				t.Fatalf("failed outbox did not retain the refused source")
			}
			note, err := os.ReadFile(filepath.Join(dir, files[1].Name()))
			if err != nil || strings.Contains(string(note), "hk_fixture") || strings.Contains(string(note), "/private/") {
				t.Fatal("failure diagnostic missing or exposed sensitive data")
			}
		})
	}
}

func TestDrainPreservesVersionOrderForTheSameSession(t *testing.T) {
	o := syncOutbox(t)
	for _, content := range []string{"original", "updated"} {
		if err := EnqueueRows(o, KindSessions, []syncRow{{ID: "same", SessionID: "same", Text: content}}); err != nil {
			t.Fatal(err)
		}
	}
	var versions []string
	client := ingestFixture(t, func(w http.ResponseWriter, _ *http.Request, rows []syncRow) {
		if len(rows) != 1 {
			t.Error("combined conflicting updates")
		}
		versions = append(versions, rows[0].Text)
		fmt.Fprint(w, `{"accepted":1}`)
	})
	if _, err := o.Drain(t.Context(), client, nil); err != nil || !reflect.DeepEqual(versions, []string{"original", "updated"}) {
		t.Fatalf("versions=%v err=%v", versions, err)
	}
}
