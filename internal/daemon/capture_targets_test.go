package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hev/kit/internal/index"
	"github.com/hev/kit/internal/secrets"
	"github.com/hev/kit/internal/trace"
)

func captureTranscript(t *testing.T, home, id, dir string) {
	t.Helper()
	root := filepath.Join(home, ".claude", "projects", "fixture")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	unit := fmt.Sprintf(`{"type":"user","uuid":%q,"sessionId":%q,"timestamp":"2026-11-22T10:00:00Z","cwd":%q,"message":{"role":"user","content":"capture test prompt"}}`+"\n", id, id, dir)
	if err := os.WriteFile(filepath.Join(root, id+".jsonl"), []byte(unit), 0600); err != nil {
		t.Fatal(err)
	}
}

func awaitCaptureStatus(t *testing.T, done <-chan error, accept func(Status) bool) Status {
	t.Helper()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	ticks := time.NewTicker(10 * time.Millisecond)
	defer ticks.Stop()
	var last Status
	for {
		select {
		case err := <-done:
			t.Fatalf("capture stopped before status was published: %v", err)
		case <-timer.C:
			t.Fatalf("timed out waiting for capture status: %+v", last)
		case <-ticks.C:
			if s, err := ReadStatus(); err == nil {
				last = s
				if accept(s) {
					return s
				}
			}
		}
	}
}

func startCaptureFixture(t *testing.T, cfg *Config, home string, resolve captureKeyResolver, logger *slog.Logger) (<-chan error, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		defer close(done)
		done <- runCapture(ctx, cfg, home, Target{}, resolve, logger)
	}()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Errorf("stop capture: %v", err)
				}
			case <-time.After(10 * time.Second):
				t.Error("capture did not stop")
			}
		})
	}
	t.Cleanup(stop)
	return done, stop
}

func TestCaptureTargetsIsolateWritesAndDiagnostics(t *testing.T) {
	const (
		firstKey       = "fixture-primary-token"
		mirrorKey      = "fixture-mirror-token"
		slowKey        = "fixture-slow-token"
		brokenRef      = "op://private-vault/private-item/private-field"
		credentialPath = "/private/local/credential-path"
	)
	wantKeys := map[string]string{"primary-traces": firstKey, "mirror-traces": mirrorKey, "slow-traces": slowKey}
	slowInWrite, releaseSlow := make(chan struct{}), make(chan struct{})
	var block, releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseSlow) }) }
	var writesMu sync.Mutex
	sessionRows := map[string][]json.RawMessage{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) < 4 {
			http.NotFound(w, r)
			return
		}
		ns := parts[3]
		base := strings.TrimSuffix(strings.TrimSuffix(ns, "-blocks"), "-sessions")
		if key := wantKeys[base]; key == "" || r.Header.Get("Authorization") != "Bearer "+key {
			t.Errorf("wrong credential or namespace for capture request")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var body struct {
			Filters []json.RawMessage `json:"filters"`
			Rows    []json.RawMessage `json:"upsert_rows"`
		}
		if r.Method == http.MethodPost {
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error("invalid capture request body")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}
		switch {
		case r.Method == http.MethodGet:
			io.WriteString(w, "{}")
		case strings.HasSuffix(r.URL.Path, "/query"):
			if ns == "slow-traces-sessions" {
				// Wait inside the session writer, while both index locks used
				// to be held for all destinations.
				block.Do(func() { close(slowInWrite) })
				select {
				case <-releaseSlow:
				case <-r.Context().Done():
					return
				}
				http.Error(w, strings.Join([]string{firstKey, mirrorKey, slowKey, brokenRef, credentialPath}, " "), http.StatusForbidden)
				return
			}
			var rows []json.RawMessage
			if len(body.Filters) > 0 && string(body.Filters[0]) == `"id"` {
				writesMu.Lock()
				rows = sessionRows[ns]
				writesMu.Unlock()
			}
			json.NewEncoder(w).Encode(map[string]any{"rows": rows})
		default:
			if strings.HasSuffix(ns, "-sessions") {
				if ns == "primary-traces-sessions" {
					select {
					case <-slowInWrite:
					case <-r.Context().Done():
						return
					}
				}
				writesMu.Lock()
				sessionRows[ns] = append(sessionRows[ns], body.Rows...)
				writesMu.Unlock()
			}
			io.WriteString(w, `{"status":"OK","rows_upserted":1,"rows_affected":1}`)
		}
	}))
	defer server.Close()
	defer release()

	cfg, root := isolate(t, fmt.Sprintf(`
[layer]
endpoint = %q
namespace = "primary-traces"
api_key = %q

[[capture.targets]]
name = "mirror"
endpoint = %q
namespace = "mirror-traces"
api_key_ref = "env://MIRROR_CAPTURE_KEY"

[[capture.targets]]
name = "slow"
endpoint = %q
namespace = "slow-traces"
api_key_ref = "env://SLOW_CAPTURE_KEY"

[[capture.targets]]
name = "broken"
endpoint = %q
namespace = "broken-traces"
api_key_ref = %q
`, server.URL, firstKey, server.URL, server.URL, server.URL, brokenRef))
	t.Setenv("MIRROR_CAPTURE_KEY", mirrorKey)
	t.Setenv("SLOW_CAPTURE_KEY", slowKey)
	home, _ := os.UserHomeDir()
	captureTranscript(t, home, "fixture", home)
	var log bytes.Buffer
	resolve := func(_ context.Context, _ string, refs []secrets.Ref) (map[string]string, error) {
		if len(refs) != 1 || refs[0].URI != brokenRef {
			t.Error("unexpected key lookup")
		}
		return nil, fmt.Errorf("provider read failed for %s: %s %s", brokenRef, credentialPath, firstKey)
	}
	done, stop := startCaptureFixture(t, cfg, home, resolve, slog.New(slog.NewJSONHandler(&log, nil)))
	defer stop() // Stop requests before the fixture server closes, even on failure.
	s := awaitCaptureStatus(t, done, func(s Status) bool {
		return len(s.Targets) == 4 && s.Targets[0].UnitsIndexed == 1 && s.Targets[1].UnitsIndexed == 1 && s.Targets[3].LastError != ""
	})
	if !s.Targets[2].LastRun.IsZero() || s.LastError != "" {
		t.Fatalf("healthy destinations did not complete independently of the slow one: %+v", s.Targets)
	}
	writesMu.Lock()
	firstWrites, mirrorWrites := len(sessionRows["primary-traces-sessions"]), len(sessionRows["mirror-traces-sessions"])
	writesMu.Unlock()
	if firstWrites != 1 || mirrorWrites != 1 {
		t.Fatalf("session writes = primary %d, mirror %d", firstWrites, mirrorWrites)
	}
	primary := index.LoadStateAt(filepath.Join(home, ".hev", "index-state.json"))
	mirror := index.LoadStateAt(filepath.Join(home, ".hev", "targets", "mirror", "state.json"))
	if len(primary.Units) == 0 || len(mirror.Units) == 0 {
		t.Fatal("successful targets did not each persist their own cursor")
	}
	for key := range primary.Units {
		if _, shared := mirror.Units[key]; shared {
			t.Fatal("primary and mirror share an index signature")
		}
	}
	release()
	s = awaitCaptureStatus(t, done, func(s Status) bool { return s.Targets[2].LastError != "" })
	stop()
	if s.Targets[3].LastError != "could not resolve API key reference" || s.Targets[2].LastError != "indexing failed in 1 operations or units" {
		t.Fatalf("missing target-specific errors: %+v", s.Targets)
	}
	for _, path := range []string{
		filepath.Join(root, "broken-traces"),
		filepath.Join(home, ".hev", "targets", "broken", "state.json"),
	} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("unresolved Layer reference created an archive or cursor: %v", err)
		}
	}
	raw, err := os.ReadFile(StatusPath())
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{firstKey, mirrorKey, slowKey, brokenRef, credentialPath, "MIRROR_CAPTURE_KEY", "SLOW_CAPTURE_KEY"} {
		if strings.Contains(string(raw), forbidden) || strings.Contains(log.String(), forbidden) {
			t.Fatal("credential or local error path was emitted")
		}
	}
	for _, kept := range []string{server.URL, "primary-traces", "mirror-traces", "1Password", "environment", "could not resolve API key reference"} {
		if !bytes.Contains(raw, []byte(kept)) {
			t.Errorf("status did not retain %q", kept)
		}
	}
	if !strings.Contains(log.String(), "capture target failed") || !strings.Contains(log.String(), "indexing failed") {
		t.Fatal("log omitted safe failure diagnostics")
	}
}

func TestWorkspaceCaptureKeepsOnlyOwnedSessions(t *testing.T) {
	cfg, _ := isolate(t, `
[workspaces.bcc]
dir = "~/clients"
archive_dir = "~/bcc-archive"
traces_namespace = "bcc-scoped"

[workspaces.cloud]
dir = "~/clients/cloud"
archive_dir = "~/cloud-archive"
traces_namespace = "cloud-scoped"
`)
	home, _ := os.UserHomeDir()
	owned := filepath.Join(home, "clients", "repo")
	nested := filepath.Join(home, "clients", "cloud", "repo")
	captureTranscript(t, home, "bcc-session", owned)
	captureTranscript(t, home, "cloud-session", nested)
	captureTranscript(t, home, "prefix-session", filepath.Join(home, "clients-other"))
	captureTranscript(t, home, "unknown-session", "")
	for _, target := range cfg.captureTargets {
		if target.Name != "bcc" {
			continue
		}
		mixed := []trace.Turn{
			{SessionID: "bcc", Workdir: owned}, {SessionID: "bcc"},
			{SessionID: "mixed", Workdir: owned}, {SessionID: "mixed", Workdir: nested},
			{SessionID: "unknown"}, {SessionID: "", Workdir: owned},
		}
		if got := target.filterTurns(mixed); len(got) != 2 || got[0].SessionID != "bcc" || got[1].SessionID != "bcc" {
			t.Fatalf("incorrect filtering of a multi-session unit: %+v", got)
		}
	}
	done, stop := startCaptureFixture(t, cfg, home, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	awaitCaptureStatus(t, done, func(s Status) bool {
		return len(s.Targets) == 3 && !s.Targets[0].LastRun.IsZero() && !s.Targets[1].LastRun.IsZero() && !s.Targets[2].LastRun.IsZero()
	})
	status, err := ReadStatus()
	if err != nil {
		t.Fatal(err)
	}
	stop()
	for i, name := range []string{"bcc", "cloud"} {
		if target := status.Targets[i+1]; target.Name != name || target.LastError != "" || target.UnitsIndexed != 1 {
			t.Fatalf("%s capture: %+v", name, target)
		}
		path := filepath.Join(home, name+"-archive", name+"-scoped", "index.jsonl")
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var row trace.SessionRow
		if err := json.Unmarshal(raw, &row); err != nil || row.SessionID != name+"-session" {
			t.Fatalf("%s archive must contain exactly its owned session: %v, %q", name, err, raw)
		}
	}
}
