package secrets

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// These tests run on every OS; native Windows CI runs this package, so they
// cover the named-pipe endpoint there and the Unix socket here.

// isolatedEndpoint returns a fresh endpoint no other test or daemon uses.
func isolatedEndpoint(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		return fmt.Sprintf(`\\.\pipe\hev-secrets-test-%d-%016x`, os.Getpid(), rand.Uint64())
	}
	dir, err := os.MkdirTemp("", "hs") // short: sockaddr_un limits the path
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "test.sock")
}

func serveOn(t *testing.T, ep string, c *Cache) {
	t.Helper()
	t.Setenv(EndpointEnv, ep)
	ctx, cancel := context.WithCancel(context.Background())
	stop, err := Serve(ctx, c)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { stop(); cancel() })
}

// fakeCalls counts the fake op's invocations (verb only, never stdin).
func fakeCalls(t *testing.T) func() []string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "calls")
	t.Setenv("HEV_FAKE_CALLS", p)
	return func() []string {
		b, _ := os.ReadFile(p)
		return strings.Fields(string(b))
	}
}

// fakeOnPath puts copies of the test binary named op and op.exe first on PATH,
// so a direct fallback runs the fake, never a real op.
func fakeOnPath(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"op", "op.exe"} {
		if err := os.WriteFile(filepath.Join(bin, n), data, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
}

func TestEndpointOverrideServesAndDialsIsolatedEndpoint(t *testing.T) {
	c := fake(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv(EndpointEnv, "")
	def := Endpoint()
	if def != defaultEndpoint() {
		t.Fatal("unset override changed the default endpoint")
	}
	a, b := isolatedEndpoint(t), isolatedEndpoint(t)
	serveOn(t, a, c)
	if Endpoint() != a {
		t.Fatal("override not honoured")
	}
	if err := Ping(context.Background()); err != nil {
		t.Fatalf("client did not reach the overridden endpoint: %v", err)
	}
	if runtime.GOOS != "windows" {
		if _, err := os.Stat(a); err != nil {
			t.Fatal("server did not listen on the overridden socket")
		}
		if _, err := os.Stat(def); err == nil {
			t.Fatal("server also created the default socket")
		}
	}
	if dup, err := Serve(context.Background(), New()); err == nil {
		dup()
		t.Fatal("second server on the same endpoint accepted")
	}
	// A second, independent daemon on its own endpoint.
	c2 := fake(t)
	serveOn(t, b, c2)
	if err := Ping(context.Background()); err != nil {
		t.Fatal("second isolated endpoint unreachable")
	}
	t.Setenv(EndpointEnv, isolatedEndpoint(t))
	if err := Ping(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unused endpoint answered: %v", err)
	}
	bad := "relative.sock"
	if runtime.GOOS == "windows" {
		bad = `C:\not-a-pipe`
	}
	t.Setenv(EndpointEnv, bad)
	if stop, err := Serve(context.Background(), New()); err == nil {
		stop()
		t.Fatal("invalid endpoint accepted by the server")
	}
	if err := Ping(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatal("invalid endpoint dialled")
	}
}

func TestResolveResultReportsPerReferenceDisposition(t *testing.T) {
	c := fake(t)
	calls := fakeCalls(t)
	serveOn(t, isolatedEndpoint(t), c)
	ctx := context.Background()
	var notice bytes.Buffer
	opts := Options{Caller: "disposition-test", Notice: &notice}
	r, err := ResolveResult(ctx, refs(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if r.Via != ViaDaemon || r.Sources["TOKEN"] != SourceRead || r.Sources["OTHER"] != SourceRead {
		t.Fatalf("first resolution: via=%s sources=%v", r.Via, r.Sources)
	}
	mixed := append(refs()[:1], Ref{"NEW", "op://example/new/token", 0})
	r, err = ResolveResult(ctx, mixed, opts)
	if err != nil {
		t.Fatal(err)
	}
	if r.Sources["TOKEN"] != SourceCache || r.Sources["NEW"] != SourceRead || r.Via != ViaDaemon {
		t.Fatalf("mixed resolution: %v", r.Sources)
	}
	r, err = ResolveResult(ctx, refs(), opts)
	if err != nil || r.Sources["TOKEN"] != SourceCache || r.Sources["OTHER"] != SourceCache {
		t.Fatalf("hit resolution: %v %v", r.Sources, err)
	}
	if n := len(calls()); n != 2 {
		t.Fatalf("want 2 real requests, got %d", n)
	}
	if notice.Len() != 0 {
		t.Fatal("notice printed while the daemon served")
	}
	for k, v := range r.Sources {
		if strings.Contains(v, sentinel) || strings.Contains(k, sentinel) {
			t.Fatal("value in dispositions")
		}
	}
	lines, _ := os.ReadFile(os.Getenv("HEV_SECRETS_READ_LOG"))
	if n := strings.Count(string(lines), "\n"); n != 2 || bytes.Contains(lines, []byte(sentinel)) {
		t.Fatalf("read log: %d lines", n)
	}
}

func TestFallbackIsExplicitAndPingIsReadFree(t *testing.T) {
	c := fake(t)
	calls := fakeCalls(t)
	fakeOnPath(t)
	ep := isolatedEndpoint(t)
	t.Setenv(EndpointEnv, ep)
	ctx := context.Background()
	if err := Ping(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("ping with no daemon: %v", err)
	}
	if n := len(calls()); n != 0 {
		t.Fatal("ping made a request")
	}
	var notice bytes.Buffer
	if _, err := ResolveResult(ctx, refs(), Options{CacheOnly: true, Notice: &notice}); !errors.Is(err, ErrUnavailable) || notice.Len() != 0 || len(calls()) != 0 {
		t.Fatal("cache-only must refuse quietly with no request")
	}
	r, err := ResolveResult(ctx, refs(), Options{Caller: "fallback-test", Notice: &notice})
	if err != nil {
		t.Fatal(err)
	}
	if r.Via != ViaDirect || r.Sources["TOKEN"] != SourceRead || r.Values["TOKEN"] == "" {
		t.Fatalf("fallback not distinguishable: via=%s sources=%v", r.Via, r.Sources)
	}
	if !strings.HasPrefix(notice.String(), FallbackNotice) || !strings.Contains(notice.String(), ep) || strings.Contains(notice.String(), sentinel) {
		t.Fatalf("fallback notice: %q", notice.String())
	}
	if n := len(calls()); n != 1 {
		t.Fatalf("direct fallback made %d requests", n)
	}

	// With the daemon up: ping and --no-quota status are free; full status
	// is the call that may ask for the quota.
	serveOn(t, ep, c)
	if err := Ping(ctx); err != nil {
		t.Fatal(err)
	}
	s, err := ReadLocalStatus(ctx)
	if err != nil || !s.Daemon {
		t.Fatalf("local status: %v", err)
	}
	if n := len(calls()); n != 1 {
		t.Fatalf("ping or local status made a request (%d calls)", n)
	}
	if _, err := ReadStatus(ctx); err != nil {
		t.Fatal(err)
	}
	if got := calls(); len(got) != 2 || got[1] != "service-account" {
		t.Fatalf("full status should be the only extra request: %v", got)
	}
}
