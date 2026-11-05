package secrets

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

const sentinel = "fake-secret-never-log-this"

func TestMain(m *testing.M) {
	if os.Getenv("HEV_FAKE_OP") == "1" {
		if len(os.Args) > 1 && os.Args[1] == "inject" {
			if len(os.Args) != 2 {
				os.Exit(2)
			}
			b, _ := io.ReadAll(os.Stdin)
			if os.Getenv("HEV_FAKE_FAIL") == "1" {
				fmt.Fprintln(os.Stderr, "daily rate limit exceeded "+sentinel+string(b))
				os.Exit(1)
			}
			s := string(b)
			for {
				start := strings.Index(s, "{{ ")
				if start < 0 {
					break
				}
				end := strings.Index(s[start:], " }}")
				if end < 0 {
					os.Exit(2)
				}
				s = s[:start] + sentinel + "\nline two" + s[start+end+3:]
			}
			fmt.Print(s)
			os.Exit(0)
		}
		fmt.Print(`{"daily":{"limit":100,"remaining":50},"secret":"` + sentinel + `"}`)
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func fake(t *testing.T) *Cache {
	t.Helper()
	t.Setenv("HEV_FAKE_OP", "1")
	t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "fake-service-account-token")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	c := New()
	c.op = exe
	return c
}
func refs() []Ref {
	return []Ref{{"TOKEN", "op://example/item/token", time.Minute}, {"OTHER", "op://example/other/token", time.Minute}}
}
func TestConcurrentBatchAndTTL(t *testing.T) {
	c := fake(t)
	now := time.Now()
	c.now = func() time.Time { return now }
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, e := c.Resolve(context.Background(), refs())
			if e != nil || v["TOKEN"] != sentinel+"\nline two" {
				t.Errorf("resolution failed")
			}
		}()
	}
	wg.Wait()
	if len(c.reads) != 1 {
		t.Fatalf("got %d reads", len(c.reads))
	}
	now = now.Add(2 * time.Minute)
	if _, e := c.Resolve(context.Background(), refs()); e != nil {
		t.Fatal(e)
	}
	if len(c.reads) != 2 {
		t.Fatal("TTL did not expire")
	}
}
func TestRateLimitCachedFallbackAndCleanFailure(t *testing.T) {
	c := fake(t)
	now := time.Now()
	c.now = func() time.Time { return now }
	if _, e := c.Resolve(context.Background(), refs()); e != nil {
		t.Fatal(e)
	}
	t.Setenv("HEV_FAKE_FAIL", "1")
	fresh := []Ref{{"FRESH", "op://example/fresh/token", 0}}
	_, err := c.Resolve(context.Background(), fresh)
	if err == nil || !strings.Contains(err.Error(), "daily rate limit") || (strings.Contains(err.Error(), sentinel) || strings.Contains(err.Error(), "op://")) {
		t.Fatal("unsafe or missing rate error")
	}
	if _, err = c.Resolve(context.Background(), refs()); err != nil {
		t.Fatal("cached fallback failed")
	}
	for i := 0; i < 10; i++ {
		if _, e := c.Resolve(context.Background(), fresh); e == nil {
			t.Fatal("missing failure")
		}
	}
	if len(c.reads) != 2 {
		t.Fatal("retry storm")
	}
	now = c.status.Reset.Add(time.Second)
	if _, e := c.Resolve(context.Background(), refs()); e == nil {
		t.Fatal("expired cache served")
	}
}
func TestFreshAndValidation(t *testing.T) {
	c := fake(t)
	r := refs()[:1]
	r[0].TTL = 0
	for i := 0; i < 2; i++ {
		if _, e := c.Resolve(context.Background(), r); e != nil {
			t.Fatal(e)
		}
	}
	if len(c.reads) != 2 || len(c.entries) != 0 {
		t.Fatal("fresh value cached")
	}
	r[0].URI = "op://example/{{bad}}/field"
	if _, e := c.Resolve(context.Background(), r); e == nil {
		t.Fatal("template injection accepted")
	}
}
func TestStatusNoValues(t *testing.T) {
	c := fake(t)
	c.Resolve(context.Background(), refs())
	s := c.Status(context.Background())
	if s.ReadsDay != 1 || s.ReadsHour != 1 || strings.Contains(s.AccountLimits, sentinel) || !strings.Contains(s.AccountLimits, "remaining") {
		t.Fatal("unsafe or missing status")
	}
}
func TestEndpointAndFallback(t *testing.T) {
	c := fake(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // Unix socket paths must fit sockaddr_un.
	if len(endpoint()) > 100 && strings.HasPrefix(endpoint(), "/") {
		t.Skip("temporary home too long for Unix socket")
	}
	// Direct fallback uses PATH to find the fake executable; no real op is called.
	bin := filepath.Join(home, "bin")
	os.Mkdir(bin, 0700)
	src, _ := os.Executable()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"op", "op.exe"} {
		if err = os.WriteFile(filepath.Join(bin, name), data, 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	if _, err = Resolve(context.Background(), refs()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	close, err := Serve(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	defer close()
	if duplicate, e := Serve(ctx, c); e == nil {
		duplicate()
		t.Fatal("duplicate endpoint accepted")
	}
	if runtime.GOOS != "windows" {
		for p, want := range map[string]os.FileMode{endpoint(): 0600, filepath.Dir(endpoint()): 0700} {
			st, e := os.Stat(p)
			if e != nil || st.Mode().Perm() != want {
				t.Fatal("unsafe socket permissions")
			}
		}
	}
	if _, err = Resolve(ctx, refs()); err != nil {
		t.Fatal(err)
	}
	if _, err = Resolve(ctx, refs()); err != nil {
		t.Fatal(err)
	}
	if len(c.reads) != 1 {
		t.Fatal("IPC did not reuse cache")
	}
	if _, err = ReadStatus(ctx); err != nil {
		t.Fatal(err)
	}
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	if sameUser(a) {
		t.Fatal("unverified peer accepted")
	}
}

func TestShorterCallerTTL(t *testing.T) {
	c := fake(t)
	now := time.Now()
	c.now = func() time.Time { return now }
	r := refs()[:1]
	c.Resolve(context.Background(), r)
	now = now.Add(2 * time.Second)
	r[0].TTL = time.Second
	c.Resolve(context.Background(), r)
	if len(c.reads) != 2 {
		t.Fatal("caller freshness ignored")
	}
}

func TestNoCacheRateFailure(t *testing.T) {
	c := fake(t)
	t.Setenv("HEV_FAKE_FAIL", "1")
	_, err := c.Resolve(context.Background(), refs())
	if err == nil || strings.Contains(err.Error(), sentinel) || !strings.Contains(err.Error(), "retry after") {
		t.Fatal("missing sanitized cooldown")
	}
	if len(c.entries) != 0 {
		t.Fatal("failed result cached")
	}
}
func TestResolverArgumentsAndGenericError(t *testing.T) {
	c := fake(t) // TestMain rejects any inject arguments beyond the verb.
	if _, err := c.Resolve(context.Background(), refs()); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "")
	c = New()
	if _, err := c.Resolve(context.Background(), refs()); err == nil || (strings.Contains(err.Error(), sentinel) || strings.Contains(err.Error(), "op://")) {
		t.Fatal("service account not required")
	}
}
