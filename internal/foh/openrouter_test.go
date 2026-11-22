package foh

import (
	"context"
	"encoding/json"
	"errors"
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
)

const testKey = "sk-or-v1-0123456789abcdef0123456789abcdef"

// isolateHome points HOME (and the Windows profile) at a temp dir.
func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HEV_CONFIG", "")
	t.Setenv("HEV_FOH_SESSIONS_DIR", "")
	for _, k := range []string{"OPENROUTER_API_KEY", "HEV_FOH_API_KEY", "HEV_FOH_MODEL", "HEV_FOH_BASE_URL", "CODEX_HOME", "HEV_WORKSPACE", "CLAUDE_CONFIG_DIR", "HEV_FOH_CODEX_BASE_URL"} {
		t.Setenv(k, "")
	}
	return home
}

func sse(w http.ResponseWriter, chunks ...string) {
	w.Header().Set("Content-Type", "text/event-stream")
	for _, c := range chunks {
		fmt.Fprintf(w, "data: %s\n\n", c)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
	fmt.Fprint(w, "data: [DONE]\n\n")
}

func TestStreamTextDeltas(t *testing.T) {
	var gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		fmt.Fprint(w, ": OPENROUTER PROCESSING\n\n")
		sse(w,
			`{"model":"test/model","choices":[{"delta":{"content":"Hel"}}]}`,
			`{"choices":[{"delta":{"content":"lo"}}]}`,
			`not json`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
			`{"choices":[],"usage":{"prompt_tokens":11,"completion_tokens":2,"total_tokens":13}}`)
	}))
	defer srv.Close()
	c := NewClient(srv.URL, testKey)
	var deltas []string
	res, err := c.Stream(context.Background(), ChatRequest{Model: "test/model", Messages: []Message{{Role: "user", Content: "hi"}}}, func(d Delta) {
		deltas = append(deltas, d.Text)
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(deltas, "|") != "Hel|lo" || res.Content != "Hello" || res.FinishReason != "stop" {
		t.Fatalf("deltas=%q res=%+v", deltas, res)
	}
	if res.Usage.TotalTokens != 13 || res.Model != "test/model" {
		t.Fatalf("usage/model = %+v %q", res.Usage, res.Model)
	}
	if gotAuth != "Bearer "+testKey {
		t.Fatalf("auth header = %q", gotAuth)
	}
	var body map[string]any
	if json.Unmarshal([]byte(gotBody), &body) != nil || body["stream"] != true || body["model"] != "test/model" {
		t.Fatalf("request body = %s", gotBody)
	}
}

func TestStreamToolCallAssembly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sse(w,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"read_file","arguments":""}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":1,"id":"call_b","type":"function","function":{"name":"hev_status","arguments":"{}"}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"pa"}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"th\":\"a.txt\"}"}}]}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`)
	}))
	defer srv.Close()
	var started []string
	res, err := NewClient(srv.URL, testKey).Stream(context.Background(), ChatRequest{Model: "m"}, func(d Delta) {
		if d.ToolName != "" {
			started = append(started, d.ToolName)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.ToolCalls) != 2 {
		t.Fatalf("calls = %+v", res.ToolCalls)
	}
	a, b := res.ToolCalls[0], res.ToolCalls[1]
	if a.ID != "call_a" || a.Function.Name != "read_file" || a.Function.Arguments != `{"path":"a.txt"}` {
		t.Fatalf("call a = %+v", a)
	}
	if b.ID != "call_b" || b.Function.Name != "hev_status" || b.Function.Arguments != "{}" {
		t.Fatalf("call b = %+v", b)
	}
	if strings.Join(started, ",") != "read_file,hev_status" || res.FinishReason != "tool_calls" {
		t.Fatalf("started=%v finish=%q", started, res.FinishReason)
	}
}

func TestStreamTimeout(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)
	c := NewClient(srv.URL, testKey)
	c.Timeout = 150 * time.Millisecond
	start := time.Now()
	_, err := c.Stream(context.Background(), ChatRequest{Model: "m"}, nil)
	if err == nil || !ErrTimeout(err) {
		t.Fatalf("err = %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("timeout not enforced")
	}
	if strings.Contains(err.Error(), testKey) {
		t.Fatal("key in timeout error")
	}
}

func TestErrorRedaction(t *testing.T) {
	// The server echoes the key and its bearer header back in an error body.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `{"error":{"message":"bad key %s; header was %s; model foo/bar not found"}}`, testKey, r.Header.Get("Authorization"))
	}))
	defer srv.Close()
	_, err := NewClient(srv.URL, testKey).Stream(context.Background(), ChatRequest{Model: "m"}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	// Refused: the key and the bearer token.
	if strings.Contains(msg, testKey) || strings.Contains(msg, "0123456789abcdef") {
		t.Fatalf("secret leaked: %s", msg)
	}
	// Kept: status and the non-secret explanation.
	if !strings.Contains(msg, "401") || !strings.Contains(msg, "model foo/bar not found") || !strings.Contains(msg, redacted) {
		t.Fatalf("lost context: %s", msg)
	}
	// A mid-stream error chunk is scrubbed too.
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sse(w, fmt.Sprintf(`{"error":{"message":"upstream rejected Bearer %s for anthropic/x"}}`, testKey))
	}))
	defer srv2.Close()
	_, err = NewClient(srv2.URL, testKey).Stream(context.Background(), ChatRequest{Model: "m"}, nil)
	if err == nil || strings.Contains(err.Error(), testKey) || !strings.Contains(err.Error(), "anthropic/x") {
		t.Fatalf("stream error = %v", err)
	}
}

func TestScrubBothSides(t *testing.T) {
	for _, s := range []string{
		"Authorization: Bearer abc.def-ghi_jkl",
		"bearer eyJhbGciOiJIUzI1NiJ9.payload",
		"key sk-or-v1-aaaaaaaaaaaaaaaaaaaaaaaa",
		"sk-ant-api03-bbbbbbbbbbbbbbbbbbbbbbbb",
		"api_key=supersecretvalue123",
		`"apiKey": "x"` + " HEV_SECRET_VALUE",
	} {
		out := Scrub(s, "HEV_SECRET_VALUE")
		for _, leak := range []string{"abc.def-ghi_jkl", "eyJhbGciOiJIUzI1NiJ9", "aaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbb", "supersecretvalue123", "HEV_SECRET_VALUE"} {
			if strings.Contains(out, leak) {
				t.Errorf("Scrub(%q) = %q leaks %q", s, out, leak)
			}
		}
	}
	for _, keep := range []string{
		"anthropic/claude-sonnet-5.5 returned 404",
		"read ./docs/foh-harness.md: no such file",
		"the bearer of bad news",
		"tokens: 1234 prompt, 56 completion",
		"sk-short",
	} {
		if got := Scrub(keep); got != keep {
			t.Errorf("Scrub(%q) = %q, want unchanged", keep, got)
		}
	}
}

func catalogServer(t *testing.T, hits *atomic.Int32, fail *atomic.Bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			http.NotFound(w, r)
			return
		}
		hits.Add(1)
		if fail.Load() {
			http.Error(w, "down", http.StatusBadGateway)
			return
		}
		fmt.Fprint(w, `{"data":[
			{"id":"anthropic/claude-sonnet-5.5","name":"Claude Sonnet","context_length":200000,"pricing":{"prompt":"0.000003","completion":"0.000015"}},
			{"id":"openai/gpt-5","name":"GPT-5","context_length":400000,"pricing":{"prompt":"0.00000125","completion":"0.00001"}},
			{"id":"","name":"skipped"}]}`)
	}))
}

func TestCatalogFetchCacheAndFallback(t *testing.T) {
	isolateHome(t)
	var hits atomic.Int32
	var fail atomic.Bool
	srv := catalogServer(t, &hits, &fail)
	defer srv.Close()
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	cat := &Catalog{Client: NewClient(srv.URL, testKey), Now: func() time.Time { return now }}
	if cat.path() != filepath.Join(os.Getenv("HOME"), ".hev", "cache", "openrouter-models.json") {
		t.Fatalf("cache path = %s", cat.path())
	}

	models, src, err := cat.Models(context.Background(), false)
	if err != nil || src != SourceNetwork || len(models) != 2 || hits.Load() != 1 {
		t.Fatalf("fetch: %v %s %d hits=%d", err, src, len(models), hits.Load())
	}
	if m := models[0]; m.ContextLabel() != "200k" || m.PriceLabel() != "$3.00/$15.00" {
		t.Fatalf("labels = %s %s", m.ContextLabel(), m.PriceLabel())
	}

	// Within 24h: cache hit, no request.
	now = now.Add(23 * time.Hour)
	if _, src, err = cat.Models(context.Background(), false); err != nil || src != SourceCache || hits.Load() != 1 {
		t.Fatalf("cache hit: %v %s hits=%d", err, src, hits.Load())
	}
	// Past 24h: refetch.
	now = now.Add(2 * time.Hour)
	if _, src, _ = cat.Models(context.Background(), false); src != SourceNetwork || hits.Load() != 2 {
		t.Fatalf("expired: %s hits=%d", src, hits.Load())
	}
	// Offline past the TTL: stale cache with the error.
	now = now.Add(48 * time.Hour)
	fail.Store(true)
	models, src, err = cat.Models(context.Background(), false)
	if err == nil || src != SourceStale || len(models) != 2 {
		t.Fatalf("stale: %v %s %d", err, src, len(models))
	}
	// Offline with no cache at all: built-in starter list.
	cat.Path = filepath.Join(t.TempDir(), "none.json")
	models, src, err = cat.Models(context.Background(), false)
	if err == nil || src != SourceStarter || len(models) != len(StarterModels) {
		t.Fatalf("starter: %v %s %d", err, src, len(models))
	}
	if strings.Contains(err.Error(), testKey) {
		t.Fatal("key in catalog error")
	}
	// The cache file never contains the key.
	b, _ := os.ReadFile(CatalogPath())
	if strings.Contains(string(b), testKey) {
		t.Fatal("key in cache")
	}
}

func TestCatalogBoundedAndTimeout(t *testing.T) {
	isolateHome(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[`)
		for i := 0; i < MaxCatalogModels+50; i++ {
			if i > 0 {
				fmt.Fprint(w, ",")
			}
			fmt.Fprintf(w, `{"id":"p/m-%d","pricing":{"prompt":"x","completion":"0"}}`, i)
		}
		fmt.Fprint(w, `]}`)
	}))
	defer srv.Close()
	models, _, err := (&Catalog{Client: NewClient(srv.URL, "")}).Models(context.Background(), false)
	if err != nil || len(models) != MaxCatalogModels {
		t.Fatalf("bounded: %v %d", err, len(models))
	}
	if models[0].PriceLabel() != "?/free" {
		t.Fatalf("price label = %s", models[0].PriceLabel())
	}

	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer slow.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	cat := &Catalog{Client: NewClient(slow.URL, ""), Path: filepath.Join(t.TempDir(), "c.json")}
	_, src, err := cat.Models(ctx, true)
	if err == nil || !errors.Is(err, context.DeadlineExceeded) || src != SourceStarter {
		t.Fatalf("timeout: %v %s", err, src)
	}
}

func TestConfigAndKeyResolution(t *testing.T) {
	home := isolateHome(t)
	cfgPath := filepath.Join(home, ".hev", "config.toml")
	os.MkdirAll(filepath.Dir(cfgPath), 0o700)
	os.WriteFile(cfgPath, []byte("[foh]\nmodel = \"openai/gpt-5\"\nbase_url = \"http://local/v1/\"\napi_key_ref = \"op://vault/openrouter/credential\"\n"), 0o600)
	cfg, err := LoadConfig("")
	if err != nil || cfg.Model != "openai/gpt-5" {
		t.Fatalf("config = %+v %v", cfg, err)
	}
	if cfg.ResolvedBaseURL() != "http://local/v1" {
		t.Fatalf("base = %s", cfg.ResolvedBaseURL())
	}
	t.Setenv("HEV_FOH_BASE_URL", "http://env")
	if cfg.ResolvedBaseURL() != "http://env" {
		t.Fatal("env base url must win")
	}
	if cfg.ResolveModel("", []string{"fav/one"}, nil, "") != "openai/gpt-5" || cfg.ResolveModel("flag/m", nil, nil, "") != "flag/m" {
		t.Fatal("model precedence")
	}
	if (Config{}).ResolveModel("", []string{"fav/one"}, nil, "") != "fav/one" || (Config{}).ResolveModel("", nil, nil, "") != DefaultModel {
		t.Fatal("favorite / default model")
	}

	orig := ResolveSecretRef
	defer func() { ResolveSecretRef = orig }()
	ResolveSecretRef = func(_ context.Context, ref string) (string, error) {
		if ref != "op://vault/openrouter/credential" {
			t.Errorf("ref = %s", ref)
		}
		return "sk-or-v1-fromref0000000000000000", nil
	}
	if k, err := cfg.ResolveAPIKey(context.Background()); err != nil || k != "sk-or-v1-fromref0000000000000000" {
		t.Fatalf("ref key = %q %v", k, err)
	}
	t.Setenv("HEV_FOH_API_KEY", "envkey-hev")
	if k, _ := cfg.ResolveAPIKey(context.Background()); k != "envkey-hev" {
		t.Fatal("HEV_FOH_API_KEY must beat the ref")
	}
	t.Setenv("OPENROUTER_API_KEY", "envkey-or")
	if k, _ := cfg.ResolveAPIKey(context.Background()); k != "envkey-or" {
		t.Fatal("OPENROUTER_API_KEY must win")
	}
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("HEV_FOH_API_KEY", "")
	ResolveSecretRef = func(context.Context, string) (string, error) {
		return "", errors.New("op failed: Authorization: Bearer ops_tokenvalue123456")
	}
	_, err = cfg.ResolveAPIKey(context.Background())
	if err == nil || strings.Contains(err.Error(), "ops_tokenvalue123456") || !strings.Contains(err.Error(), "op failed") {
		t.Fatalf("ref error = %v", err)
	}
	if _, err := (Config{}).ResolveAPIKey(context.Background()); !errors.Is(err, ErrNoAPIKey) {
		t.Fatalf("no key = %v", err)
	}

	os.WriteFile(cfgPath, []byte("[foh]\nmodle = \"typo\"\n"), 0o600)
	if _, err := LoadConfig(""); err == nil || strings.Contains(err.Error(), home) {
		t.Fatalf("unknown field = %v", err)
	}
	os.WriteFile(cfgPath, []byte("[foh\n"), 0o600)
	if _, err := LoadConfig(""); err == nil || strings.Contains(err.Error(), home) {
		t.Fatalf("parse error = %v", err)
	}
	os.WriteFile(cfgPath, []byte("[metrics]\nenabled = true\n"), 0o600)
	if c, err := LoadConfig(""); err != nil || !reflect.DeepEqual(c, Config{}) {
		t.Fatalf("other tables must be ignored: %+v %v", c, err)
	}
}
