package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/collect"
	"github.com/hev/kit/internal/layerenv"
	"github.com/hev/kit/internal/redact"
	"github.com/hev/kit/internal/storagemode"
)

// collectNoKeyEnv gives the test a fresh HOME, its own config path and
// archive root, and no Layer key anywhere in the environment.
func collectNoKeyEnv(t *testing.T) (base, home, config, root string) {
	t.Helper()
	base = t.TempDir()
	home, config, root = filepath.Join(base, "home"), filepath.Join(base, "cfg", "config.toml"), filepath.Join(base, "archive")
	for _, name := range layerenv.Names() {
		t.Setenv(name, "")
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HEV_CONFIG", config)
	t.Setenv("HEV_ARCHIVE_DIR", root)
	return base, home, config, root
}

func otlpTurn(ns, user, chat, turn, prompt, answer string, at time.Time) []byte {
	kv := func(pairs ...string) []map[string]any {
		var out []map[string]any
		for i := 0; i+1 < len(pairs); i += 2 {
			out = append(out, map[string]any{"key": pairs[i], "value": map[string]any{"stringValue": pairs[i+1]}})
		}
		return out
	}
	msgs, _ := json.Marshal(map[string]any{"messages": []any{map[string]any{"role": "user", "content": prompt}}})
	ts := fmt.Sprint(at.UnixNano())
	raw, _ := json.Marshal(map[string]any{"resourceSpans": []any{map[string]any{
		"resource": map[string]any{"attributes": kv("service.name", "ki-agent", "hev.namespace", ns)},
		"scopeSpans": []any{map[string]any{"scope": map[string]any{"name": "ai"}, "spans": []any{map[string]any{
			"traceId": fmt.Sprintf("%032x", at.UnixNano()), "spanId": "a1a1a1a1a1a1a1a1", "name": "ai.streamText",
			"startTimeUnixNano": ts, "endTimeUnixNano": ts, "status": map[string]any{},
			"attributes": kv("ai.telemetry.metadata.userId", user, "ai.telemetry.metadata.chatId", chat,
				"ai.telemetry.metadata.turnId", turn, "ai.telemetry.metadata.tenant", "story",
				"ai.operationId", "ai.streamText", "ai.prompt", string(msgs), "ai.response.text", answer),
		}}}},
	}}})
	return raw
}

// With no key, hev collect keeps turns, app boards and envelopes in the file
// archive: one archive namespace per app namespace, named as the Layer
// namespace would be, and nothing outside the archive root but the kit
// config file redaction keeps its salt in.
func TestCollectNoKeyUsesTheFileArchive(t *testing.T) {
	base, _, config, root := collectNoKeyEnv(t)
	st, err := openCollectStores([]string{"story-a", "story-b"}, collect.DefaultTracesPrefix, "hev-app-boards", "")
	if err != nil {
		t.Fatal(err)
	}
	if st.mode != storagemode.Filesystem {
		t.Fatalf("mode = %s, want filesystem", st.mode)
	}
	for _, ns := range []string{"story-a", "story-b"} {
		if _, ok := st.indexes[ns].(*collect.FileTurns); !ok {
			t.Fatalf("%s index = %T", ns, st.indexes[ns])
		}
	}
	if want := []string{"hev-app-traces-story-a", "hev-app-traces-story-b"}; strings.Join(st.turns, ",") != strings.Join(want, ",") {
		t.Fatalf("turn namespaces = %v, want %v", st.turns, want)
	}
	scrubber, err := redact.Load()
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	srv, err := collect.New(collect.Config{Key: "svc-key-files-test", Indexes: st.indexes, Boards: st.boards,
		Sink: &collect.FileSink{Dir: st.envelopes}, Scrubber: scrubber, Host: "test", Logger: slog.New(slog.NewTextHandler(&logs, nil))})
	if err != nil {
		t.Fatal(err)
	}
	hs := httptest.NewServer(srv.Handler())
	defer hs.Close()
	call := func(method, path string, body []byte) (int, string) {
		req, _ := http.NewRequest(method, hs.URL+path, bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer svc-key-files-test")
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	at := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	if code, body := call("POST", "/v1/traces", otlpTurn("story-a", "101", "chat-1", "turn-1", "Draft a heist in Lisbon", "The tram bell rang.", at)); code != 200 {
		t.Fatalf("export = %d %s", code, body)
	}
	code, body := call("POST", "/v1/app/traces/search", []byte(`{"namespace":"story-a","owner":"101","tenant":"story","query":"heist Lisbon"}`))
	if code != 200 || !strings.Contains(body, `"turnId":"turn-1"`) {
		t.Fatalf("search = %d %s", code, body)
	}
	if code, body := call("POST", "/v1/app/boards/story-memory/posts", []byte(`{"author":"101","title":"Present tense","body":"Ana wants present tense.","tags":["job:123"]}`)); code != 200 {
		t.Fatalf("board post = %d %s", code, body)
	}
	srv.Run(canceled()) // writes the envelopes still waiting

	absRoot, _ := filepath.Abs(root)
	for _, dir := range []string{"hev-app-traces-story-a", "hev-app-traces-story-b", "hev-app-boards", collectEnvelopesName} {
		if _, err := os.Stat(filepath.Join(absRoot, dir)); err != nil {
			t.Errorf("%s: %v", dir, err)
		}
	}
	if st.envelopes != filepath.Join(absRoot, collectEnvelopesName) {
		t.Fatalf("envelopes = %s", st.envelopes)
	}
	var outside []string
	_ = filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if rel, _ := filepath.Rel(absRoot, p); !strings.HasPrefix(rel, "..") {
			return nil
		}
		if p == config || strings.HasPrefix(p, config+".") {
			return nil // the redaction salt and its lock, in kit's config
		}
		outside = append(outside, p)
		return nil
	})
	if len(outside) > 0 {
		t.Fatalf("written outside the archive root %s: %v", absRoot, outside)
	}
}

// A key that is configured but cannot be read is an error, never a fall
// back to files, and the failed start leaves no archive behind.
func TestCollectUnreadableKeyIsAnError(t *testing.T) {
	for _, c := range []struct {
		name string
		set  func(t *testing.T)
	}{
		{"blank key", func(t *testing.T) { t.Setenv("HEVLAYER_API_KEY", "   ") }},
		{"failing reference", func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("fake op is a shell script")
			}
			bin := t.TempDir()
			if err := os.WriteFile(filepath.Join(bin, "op"), []byte("#!/bin/sh\necho 'op: not signed in' >&2\nexit 1\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "")
			t.Setenv("HEVLAYER_API_KEY_REF", "op://vault/item/field")
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, _, _, root := collectNoKeyEnv(t)
			c.set(t)
			st, err := openCollectStores([]string{"story-a"}, collect.DefaultTracesPrefix, "hev-app-boards", "")
			if err == nil {
				t.Fatalf("opened %s stores with an unreadable key", st.mode)
			}
			if _, statErr := os.Stat(root); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("an archive was created after %v (stat %v)", err, statErr)
			}
		})
	}
}

// A machine set up for hev layer (a Layer namespace in kit's config) with no
// key gets hevd's NoKeyError, not an empty file archive beside it.
func TestCollectNoKeyOnALayerMachineIsAnError(t *testing.T) {
	_, _, config, root := collectNoKeyEnv(t)
	if err := os.MkdirAll(filepath.Dir(config), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte("[layer]\nnamespace = \"hev-traces\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := openCollectStores([]string{"story-a"}, collect.DefaultTracesPrefix, "hev-app-boards", "")
	var nk *storagemode.NoKeyError
	if !errors.As(err, &nk) {
		t.Fatalf("err = %v, want NoKeyError", err)
	}
	if _, statErr := os.Stat(root); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("an archive was created (stat %v)", statErr)
	}
}

func canceled() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}
