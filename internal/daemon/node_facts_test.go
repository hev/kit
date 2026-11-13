package daemon

import (
	"context"
	"encoding/json"
	"github.com/hev/kit/internal/secrets/fakeop"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/cluster"
	"github.com/hev/kit/internal/index"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/secrets"
)

func TestCredentialObservationsNeverProbe(t *testing.T) {
	t.Setenv("HEV_SECRETS_READ_LOG", "off")
	t.Setenv("HEV_SECRETS_STATE_DIR", t.TempDir())
	t.Setenv("HEV_CONFIG", t.TempDir()+"/absent")
	fake := fakeop.Install(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "fixture-token")
	t.Setenv("HEV_OP_TOKEN_FILE", filepath.Join(t.TempDir(), "absent"))
	t.Setenv("FAKE_OP_ERROR", "planted raw-error /private/path repo-private")
	for _, h := range []string{"claude", "codex", "pi", "opencode"} {
		if e := os.WriteFile(filepath.Join(fake.Bin, h), []byte("#!/bin/sh\necho harness-launched >> "+fake.Calls+"\n"), 0700); e != nil {
			t.Fatal(e)
		}
	}
	c := secrets.New()
	now := time.Now().UTC()
	refs := []secrets.Ref{{Name: "FIXTURE_NAME", URI: "op://private/reference/value", TTL: time.Minute}}
	if _, e := c.ResolveAs(context.Background(), "fixture", refs); e == nil {
		t.Fatal("expected fake failure")
	}
	f := credentialFacts(c.LocalStatus(), now)
	if f.Items["op"].State != "failing" {
		t.Fatal(f)
	}
	calls := fake.Count(t)
	for i := 0; i < 3; i++ {
		_ = collectNodeFacts(c, false, time.Now().UTC())
	}
	if fake.Count(t) != calls {
		t.Fatal("health made a request")
	}
	t.Setenv("FAKE_OP_ERROR", "")
	if _, e := c.ResolveAs(context.Background(), "fixture", refs); e != nil {
		t.Fatal(e)
	}
	f = credentialFacts(c.LocalStatus(), now)
	if f.Items["op"].State != "ok" {
		t.Fatal(f)
	}
	raw, _ := json.Marshal(f)
	for _, bad := range []string{"planted-session", "/private/path", "repo-private", "op://private/reference", "raw-error", "private-value"} {
		if strings.Contains(string(raw), bad) {
			t.Fatal("credential leak", bad)
		}
	}
}
func TestCaptureObservedFailureRecoveryAndBacklog(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	o := captureObservation{running: true}
	o.result(now, &layer.HTTPError{Status: 401, Message: "raw-error /private/path repo-private"}, true)
	if o.class != "auth" || o.layer.State != "failing" || !o.upload.IsZero() {
		t.Fatal(o.class, o.layer)
	}
	now = now.Add(time.Minute)
	o.result(now, nil, true)
	if o.class != "" || o.layer.State != "ok" || !o.upload.Equal(now) {
		t.Fatal(o.class, o.layer, o.upload)
	}
	o.result(now.Add(time.Minute), nil, false)
	if !o.upload.Equal(now) {
		t.Fatal("local file save invented an upload")
	}
	raw, _ := json.Marshal(cluster.CaptureFacts{At: now, Running: o.running, LastUpload: o.upload, Backlog: 0, Error: o.class})
	if strings.Contains(string(raw), "raw-error") || strings.Contains(string(raw), "/private/path") {
		t.Fatal("capture leak")
	}
}

func TestCaptureRealIndexUploadHealth(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HEV_CONFIG", filepath.Join(home, "absent"))
	root := filepath.Join(home, ".claude", "projects", "repo-private")
	if e := os.MkdirAll(root, 0700); e != nil {
		t.Fatal(e)
	}
	raw := `{"type":"user","uuid":"fixture","sessionId":"planted-session","message":{"role":"user","content":"fixture content"}}` + "\n"
	if e := os.WriteFile(filepath.Join(root, "private.jsonl"), []byte(raw), 0600); e != nil {
		t.Fatal(e)
	}
	fail := true
	rows := map[string]map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			io.WriteString(w, `{}`)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if strings.HasSuffix(r.URL.Path, "/query") {
			out := []any{}
			if strings.Contains(r.URL.Path, "-sessions/") {
				if filter, ok := body["filters"].([]any); ok && len(filter) == 3 && filter[0] == "id" && filter[1] == "In" {
					if ids, ok := filter[2].([]any); ok {
						for _, id := range ids {
							if row := rows[id.(string)]; row != nil {
								out = append(out, row)
							}
						}
					}
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"rows": out})
			return
		}
		if fail {
			w.WriteHeader(401)
			io.WriteString(w, `{"error":"raw-error /private/path repo-private"}`)
			return
		}
		if list, ok := body["upsert_rows"].([]any); ok && strings.HasSuffix(r.URL.Path, "-sessions") {
			for _, raw := range list {
				row := raw.(map[string]any)
				rows[row["id"].(string)] = row
			}
		}
		io.WriteString(w, `{"rows_upserted":1,"rows_affected":1}`)
	}))
	defer srv.Close()
	state := &index.State{Units: map[string]string{}}
	client := layer.New(srv.URL, "fixture-key", "fixture", "")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	runIndexCycle(client, state, logger)
	captureHealth.Lock()
	class, cred, pending := captureHealth.class, captureHealth.layer.State, captureHealth.backlog
	captureHealth.Unlock()
	if class != "auth" || cred != "failing" || pending != 1 {
		t.Fatal(class, cred, pending)
	}
	fail = false
	runIndexCycle(client, state, logger)
	captureHealth.Lock()
	class, cred, pending = captureHealth.class, captureHealth.layer.State, captureHealth.backlog
	uploaded := captureHealth.upload
	captureHealth.Unlock()
	if class != "" || cred != "ok" || pending != 0 || uploaded.IsZero() {
		t.Fatal(class, cred, pending, uploaded)
	}
}

func TestFactsPublisherCoalescesAndRefreshes(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	p := factsPublisher{}
	writes := 0
	write := func(cluster.NodeFacts) error { writes++; return nil }
	f := cluster.NodeFacts{Version: 1, At: now, Dispatch: &cluster.DispatchFacts{At: now, Cap: 2, Free: 2}}
	if e := p.publish(f, write); e != nil {
		t.Fatal(e)
	}
	f.At = now.Add(5 * time.Second)
	f.Dispatch.At = f.At
	_ = p.publish(f, write)
	if writes != 1 {
		t.Fatal("timestamp-only publication", writes)
	}
	f.At = now.Add(cluster.FactsRefresh)
	_ = p.publish(f, write)
	if writes != 2 {
		t.Fatal("missing refresh", writes)
	}
	f.At = f.At.Add(5 * time.Second)
	f.Dispatch.Queue = 1
	_ = p.publish(f, write)
	if writes != 3 {
		t.Fatal("missing change", writes)
	}
}
