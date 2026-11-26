package index

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/hev/kit/internal/filearchive"
	"github.com/hev/kit/internal/kitcapture"
	"github.com/hev/kit/internal/trace"
	"github.com/hev/kit/pkg/kitapi"
)

type gatedSource struct {
	trace.Source
	before func(trace.Unit)
}

func (s gatedSource) Read(u trace.Unit) ([]trace.Turn, error) {
	s.before(u)
	return s.Source.Read(u)
}

func TestNewestScanUploadsBeforeOlderSourceCompletes(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	latest := make(chan struct{}, 1)
	finished := make(chan struct{}, 1)
	started := make(chan struct{}, 1)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/whoami":
			fmt.Fprint(w, `{"team":{"id":"team_test"}}`)
		case "/v1/sync/status":
			scanning, pending := r.Header.Get("X-Kit-Sync-Scanning"), r.Header.Get("X-Kit-Sync-Pending-Batches")
			switch {
			case scanning == "true":
				select {
				case started <- struct{}{}:
				default:
				}
			case scanning == "false" && pending == "0":
				select {
				case finished <- struct{}{}:
				default:
				}
			}
			http.NotFound(w, r) // Optional on older servers.
		case "/v1/traces/sessions":
			var body struct {
				Rows []trace.SessionRow `json:"rows"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if len(body.Rows) == 1 && body.Rows[0].SessionID == "recent" {
				if r.Header.Get("X-Kit-Sync-Scanning") != "true" || r.Header.Get("X-Kit-Sync-Pending-Sessions") != "1" {
					t.Error("the recent upload did not carry live scan progress")
				}
				latest <- struct{}{}
			}
			fmt.Fprintf(w, `{"accepted":%d}`, len(body.Rows))
		default:
			fmt.Fprint(w, `{"accepted":1}`)
		}
	}))
	var workers sync.WaitGroup
	t.Cleanup(func() { cancel(); workers.Wait(); api.Close() })
	o, err := kitcapture.OpenOutbox(t.TempDir(), "team_test")
	if err != nil {
		t.Fatal(err)
	}
	files, err := filearchive.Open(t.TempDir(), "test")
	if err != nil {
		t.Fatal(err)
	}
	a := kitcapture.New(files, o)
	d := &kitcapture.Drainer{Outbox: o, Client: kitapi.New(api.URL, "hk_fixture")}
	o.Notify = d.Kick
	finishScan := o.BeginScan()
	defer finishScan()
	workers.Go(func() { d.Run(ctx) })

	write := func(root, id string, modified time.Time) {
		t.Helper()
		path := filepath.Join(root, "session.jsonl")
		raw := fmt.Sprintf(`{"type":"user","uuid":"u-%s","sessionId":"%s","timestamp":%q,"message":{"role":"user","content":"hello %s"}}`, id, id, modified.Format(time.RFC3339), id)
		if err := os.WriteFile(path, []byte(raw+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, modified, modified); err != nil {
			t.Fatal(err)
		}
	}
	old, recent := t.TempDir(), t.TempDir()
	now := time.Now().UTC()
	write(old, "older", now.Add(-time.Hour))
	write(recent, "recent", now)
	continueScan := make(chan struct{})
	scanResult := make(chan error, 1)
	workers.Go(func() {
		st := &State{Units: map[string]string{}}
		opt := Options{Context: ctx, RepoURL: func(string) string { return "" }}
		rep, err := RunNewest([]trace.Source{
			gatedSource{Source: &trace.ClaudeSource{Root: old}, before: func(trace.Unit) {
				select {
				case <-continueScan:
				case <-ctx.Done():
				}
			}},
			&trace.ClaudeSource{Root: recent},
		}, a, st, opt)
		finishScan()
		if err == nil && (rep.UnitsIndexed != 2 || len(rep.Errors) != 0) {
			err = fmt.Errorf("unexpected scan: %d indexed, %d errors", rep.UnitsIndexed, len(rep.Errors))
		}
		scanResult <- err
	})
	select {
	case <-latest:
	case <-ctx.Done():
		t.Fatal("the recent trace did not upload while the older source was blocked")
	}
	select {
	case <-started:
	default:
		t.Error("scan start heartbeat was absent")
	}
	close(continueScan)
	if err := <-scanResult; err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	case <-ctx.Done():
		t.Fatal("final heartbeat did not report the completed scan and empty queue")
	}
}
