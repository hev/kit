package daemon

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/secrets"
	"github.com/hev/kit/internal/secrets/fakeop"
)

// A child run of this test binary is one kit process that needs the Layer key.
func TestMain(m *testing.M) {
	if os.Getenv("KIT_TEST_LAYER_KEY_CHILD") == "1" {
		key, err := LayerKey()
		if err != nil {
			os.Stderr.WriteString("layer key failed\n")
			os.Exit(1)
		}
		if key == fakeop.Value {
			os.Stdout.WriteString("key-ok\n")
		}
		os.Exit(0)
	}
	if os.Getenv("KIT_TEST_HEVD_CHILD") == "1" {
		// One hevd start: serve the endpoint, then read the Layer key the way
		// Run does. The supervisor's stop arrives as the context deadline.
		ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
		defer cancel()
		stop, err := secrets.Serve(ctx, secrets.New())
		if err != nil {
			os.Exit(4)
		}
		_, err = layerKeyAfterCooldown(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)))
		stop()
		if errors.Is(err, context.DeadlineExceeded) {
			os.Exit(3) // stayed up waiting for the reset
		}
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func layerKeyEnv(t *testing.T) (fakeop.Fake, string) {
	t.Helper()
	f := fakeop.Install(t)
	home, err := os.MkdirTemp("/tmp", "kitlk") // short enough for a Unix socket path
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(home) })
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HEV_SECRETS_READ_LOG", filepath.Join(home, "op-reads.log"))
	t.Setenv("HEV_OP_TOKEN_FILE", filepath.Join(home, "no-token-file"))
	t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "fake-service-account-token")
	for _, k := range []string{"HEVLAYER_API_KEY", "LAYER_API_KEY", "TPUF_API_KEY", "LAYER_API_KEY_OP_REF"} {
		t.Setenv(k, "")
	}
	t.Setenv("HEVLAYER_API_KEY_OP_REF", "op://example/layer/credential")
	// These legacy routing tests intentionally exercise repeated direct reads.
	if err := os.WriteFile(filepath.Join(home, "secrets-policy.json"), []byte(`{"caller_minute":50}`), 0600); err != nil {
		t.Fatal(err)
	}
	return f, home
}

func runLayerKeyProcess(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^$")
	cmd.Env = append(os.Environ(), "KIT_TEST_LAYER_KEY_CHILD=1")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(out)) != "key-ok" {
		t.Fatalf("kit process did not get the Layer key: %v", err)
	}
	return stderr.String()
}

func TestLayerKeyTwoProcessesOneRequestWithDaemon(t *testing.T) {
	f, home := layerKeyEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop, err := secrets.Serve(ctx, secrets.New())
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	for i := 0; i < 2; i++ {
		if errs := runLayerKeyProcess(t); strings.Contains(errs, secrets.FallbackNotice) {
			t.Fatal("fallback notice printed while the daemon served")
		}
	}
	if calls := f.CallLines(t); len(calls) != 1 || calls[0] != "inject" {
		t.Fatalf("two kit processes made %d op requests (%q), want one inject", len(calls), calls)
	}
	log, err := os.ReadFile(filepath.Join(home, "op-reads.log"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(log), "\n") != 2 || !strings.Contains(string(log), "kit_layer-key") || strings.Contains(string(log), fakeop.Value) || strings.Contains(string(log), "op://") {
		t.Fatalf("read log should hold one labelled line without values")
	}
}

func TestLayerKeyWithoutDaemonReadsDirectlyAsBefore(t *testing.T) {
	f, home := layerKeyEnv(t)
	for i := 0; i < 2; i++ {
		// The fallback is explicit: one stderr notice per direct read.
		if errs := runLayerKeyProcess(t); strings.Count(errs, secrets.FallbackNotice) != 1 || strings.Contains(errs, fakeop.Value) {
			t.Fatalf("no explicit fallback notice: %q", errs)
		}
	}
	calls := f.CallLines(t)
	if len(calls) != 2 {
		t.Fatalf("got %d op requests without a daemon, want one per process", len(calls))
	}
	for _, c := range calls {
		if c != "read op://example/layer/credential" {
			t.Fatalf("direct path changed: op %q", c)
		}
	}
	log, _ := os.ReadFile(filepath.Join(home, "op-reads.log"))
	if strings.Count(string(log), "kind=read") != 2 || strings.Contains(string(log), fakeop.Value) {
		t.Fatal("direct reads should be logged once each, without values")
	}
}

// hevd whose Layer-key read is rate limited, started and then restarted five
// times inside the cooldown: one 1Password request in total. Each start stays
// up serving secrets until stopped instead of exiting into a restart loop.
func TestHevdRestartsInsideCooldownCostOneRequest(t *testing.T) {
	f, home := layerKeyEnv(t)
	reset := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	t.Setenv("FAKE_OP_RATE_LIMIT", reset)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		cmd := exec.Command(exe, "-test.run=^$")
		cmd.Env = append(os.Environ(), "KIT_TEST_HEVD_CHILD=1")
		var stderr strings.Builder
		cmd.Stderr = &stderr
		err := cmd.Run()
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 3 {
			t.Fatalf("start %d: hevd should wait for the reset, got %v", i, err)
		}
		if !strings.Contains(stderr.String(), "rate limit; retry after "+reset[:11]) || strings.Contains(stderr.String(), fakeop.Value) {
			t.Fatalf("start %d: want the rate limit and its reset on stderr", i)
		}
	}
	if calls := f.CallLines(t); len(calls) != 1 {
		t.Fatalf("six hevd starts inside the cooldown made %d op requests (%q), want 1", len(calls), calls)
	}
	b, err := os.ReadFile(filepath.Join(home, "op-cooldown.json"))
	if err != nil || !strings.Contains(string(b), `"limit":"daily rate limit"`) || !strings.Contains(string(b), reset[:11]) {
		t.Fatalf("cooldown not persisted beside the read log: %v", err)
	}
}

// The direct path (no daemon) honours and writes the same cooldown: a
// rate-limited `op read` is one request, and the next kit processes make none.
func TestLayerKeyDirectHonoursHostCooldown(t *testing.T) {
	f, home := layerKeyEnv(t)
	t.Setenv("FAKE_OP_RATE_LIMIT", time.Now().Add(time.Hour).UTC().Format(time.RFC3339))
	for i := 0; i < 5; i++ {
		if _, err := LayerKey(); !secrets.IsRateLimit(err) {
			t.Fatalf("want the rate limit, got %v", err)
		}
	}
	if n := len(f.CallLines(t)); n != 1 {
		t.Fatalf("five direct Layer-key reads in a cooldown made %d requests, want 1", n)
	}
	log, _ := os.ReadFile(filepath.Join(home, "op-reads.log"))
	if strings.Count(string(log), "outcome=rate_limited") != 1 {
		t.Fatal("the one rate-limited request should be logged once")
	}
}

// A separately supervised secrets-only owner outlives Capture. Each new
// Capture instance fails to bind that occupied endpoint harmlessly and reads
// its key from the same owner, with no provider call on restart.
func TestCaptureRestartsKeepSecretsOwnerCache(t *testing.T) {
	f, _ := layerKeyEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop, e := secrets.Serve(ctx, secrets.New())
	if e != nil {
		t.Fatal(e)
	}
	defer stop()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	for i := 0; i < 3; i++ {
		capture, cancelCapture := context.WithCancel(ctx)
		cleanup := startSecrets(capture, logger)
		runLayerKeyProcess(t)
		cancelCapture()
		cleanup()
	}
	if f.Count(t) != 1 {
		t.Fatalf("Capture restarts made %d requests", f.Count(t))
	}
}
