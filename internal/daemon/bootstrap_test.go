package daemon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hev/kit/internal/platformops"
	"github.com/hev/kit/internal/secrets/fakeop"
	"github.com/hev/kit/internal/testenv"
)

func TestHEVLayerBootstrapRotationAndRevocation(t *testing.T) {
	const oldKey = "fixture-primary-before"
	const newKey = "fixture-primary-after"
	const sentinel = "fixture-reflected-private-response"
	var revoked atomic.Bool
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("Authorization") {
		case "Bearer " + oldKey:
			if !revoked.Load() {
				io.WriteString(w, "{}")
				return
			}
		case "Bearer " + newKey:
			io.WriteString(w, "{}")
			return
		}
		http.Error(w, sentinel, http.StatusUnauthorized)
	}))
	defer gateway.Close()

	isolate(t, "")
	dir, err := filepath.EvalSymlinks(testenv.SocketDir(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := platformops.SecureDir(dir); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "config.toml")
	config := fmt.Sprintf("[layer]\nendpoint = %q\napi_key_file = \"layer.key\"\n", gateway.URL)
	if err := platformops.WriteFileAtomic(configPath, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEV_CONFIG", configPath)
	t.Setenv("PATH", t.TempDir()) // no provider executable
	t.Setenv("HEV_SECRETS_POLICY", "")
	t.Setenv("HEV_SECRETS_READ_LOG", filepath.Join(dir, "reads.jsonl"))
	t.Setenv("HEVLAYER_API_KEY_REF", "op://example/should-not-read/key")
	t.Setenv("HEVLAYER_API_KEY", "fixture-environment-must-not-win")
	path := filepath.Join(dir, "layer.key")
	write := func(key string) {
		t.Helper()
		if err := platformops.WriteFileAtomic(path, []byte(key), 0600); err != nil {
			t.Fatal(err)
		}
	}
	open := func() (Target, error) {
		t.Helper()
		conf, err := loadConfig(true)
		if err != nil {
			return Target{}, err
		}
		return openPrimary(conf, slog.New(slog.NewTextHandler(io.Discard, nil)))
	}
	auth := func(ctx context.Context, target Target) string {
		if err := target.Layer.WithContext(ctx).ReconcileQueryFilters(); err != nil {
			return captureError("archive authentication", err)
		}
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := open(); err == nil || !strings.Contains(err.Error(), "api_key_file is unreadable") || strings.Contains(err.Error(), path) {
		t.Fatalf("expected redacted missing-file diagnostic, got %v", err)
	}
	write(oldKey + "\n")
	old, err := open()
	if err != nil {
		t.Fatal(err)
	}
	defer old.Layer.HTTP.CloseIdleConnections()
	if got := auth(ctx, old); got != "" {
		t.Fatal(got)
	}
	write(newKey)
	revoked.Store(true)
	if got := auth(ctx, old); got != "archive authentication: HTTP 401" {
		t.Fatalf("revoked daemon authentication = %q", got)
	}
	next, err := open()
	if err != nil {
		t.Fatal(err)
	}
	defer next.Layer.HTTP.CloseIdleConnections()
	if got := auth(ctx, next); got != "" {
		t.Fatal(got)
	}
	write(oldKey)
	stale, err := open()
	if err != nil {
		t.Fatal(err)
	}
	defer stale.Layer.HTTP.CloseIdleConnections()
	if got := auth(ctx, stale); got != "archive authentication: HTTP 401" {
		t.Fatalf("revoked key after rollback = %q", got)
	}
	write("op://example/" + sentinel)
	if _, err := open(); err == nil || strings.Contains(err.Error(), sentinel) || strings.Contains(err.Error(), path) {
		t.Fatalf("expected redacted invalid-key refusal, got %v", err)
	}

	canceled, stop := context.WithCancel(ctx)
	stop()
	if got := auth(canceled, next); got != "archive authentication: canceled" {
		t.Fatalf("canceled auth = %q", got)
	}
	expired, stop := context.WithDeadline(ctx, time.Now().Add(-time.Second))
	defer stop()
	if got := auth(expired, next); got != "archive authentication: timed out" {
		t.Fatalf("expired auth = %q", got)
	}
	gateway.Close()
	if got := auth(ctx, next); got != "archive authentication: network error" {
		t.Fatalf("unavailable gateway = %q", got)
	}
	for _, name := range []string{"reads.jsonl", "budget.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("primary bootstrap wrote downstream accounting state")
		}
	}
}

func TestHEVLayerBootstrapReferenceOnlyFailsClosed(t *testing.T) {
	fake := fakeop.Install(t)
	cfg, _ := isolate(t, "")
	t.Setenv("HEVLAYER_API_KEY_OP_REF", "op://example/never-read/key")
	target, err := openPrimary(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil || !strings.Contains(err.Error(), "api_key_file") || strings.Contains(err.Error(), "never-read") ||
		target.Layer != nil || target.Files != nil || fake.Count(t) != 0 {
		t.Fatal("reference-only primary did not fail closed before secret resolution")
	}
}

func TestHEVLayerBootstrapEmptyFileSettingFailsClosed(t *testing.T) {
	cfg, _ := isolate(t, "[layer]\napi_key_file = \"\"\n")
	t.Setenv("HEVLAYER_API_KEY", "fixture-must-not-be-used")
	target, err := openPrimary(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil || !strings.Contains(err.Error(), "api_key_file is empty") || target.Layer != nil || target.Files != nil {
		t.Fatal("explicit empty bootstrap path selected a fallback")
	}
	t.Setenv("HEVLAYER_API_KEY", "")
	if target, err := OpenTarget(cfg, ""); err == nil || target.Files != nil {
		t.Fatal("CLI selected a file archive on a hevd Layer host")
	}
}
