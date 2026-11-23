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
	for _, k := range []string{"OPENROUTER_API_KEY", "HEV_API_KEY", "HEV_KIT_SERVICE", "HEV_FOH_MODEL", "HEV_FOH_BASE_URL", "CODEX_HOME", "HEV_WORKSPACE", "CLAUDE_CONFIG_DIR", "HEV_FOH_CODEX_BASE_URL"} {
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
	t.Setenv("HEV_API_KEY", "envkey-hev")
	if k, _ := cfg.ResolveAPIKey(context.Background()); k != "envkey-hev" {
		t.Fatal("HEV_API_KEY must beat the ref")
	}
	t.Setenv("OPENROUTER_API_KEY", "envkey-or")
	if k, _ := cfg.ResolveAPIKey(context.Background()); k != "envkey-or" {
		t.Fatal("OPENROUTER_API_KEY must win")
	}
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("HEV_API_KEY", "")
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

func TestScrubKitAndLayerTokensBothSides(t *testing.T) {
	hevPayload := strings.Repeat("a1B2", 8)
	hvlPayload := strings.Repeat("Z9y8", 8)
	hkPayload := strings.Repeat("Ab9_-1", 7) + "A"
	skOrPayload := strings.Repeat("deadbeef", 8)

	tokens := []struct {
		name, token, payload string
	}{
		{"hev", "hev_" + hevPayload, hevPayload},
		{"hvl", "hvl_" + hvlPayload, hvlPayload},
		{"hk", "hk_" + hkPayload, hkPayload},
		{"sk-or-v1", "sk-or-v1-" + skOrPayload, skOrPayload},
	}

	// Scrub must catch all token formats even when the raw key is NOT passed as an argument.
	for _, tc := range tokens {
		samples := []string{
			tc.token,
			"plain text error with " + tc.token + " inside",
			fmt.Sprintf(`{"error":{"message":"rejected %s"},"key":"%s"}`, tc.token, tc.token),
			fmt.Sprintf(`curl --key=%s -H "Authorization: Bearer %s" https://kit.hevmind.com/v1/inference`, tc.token, tc.token),
			fmt.Sprintf("https://kit.hevmind.com/v1/inference?token=%s&foo=bar", tc.token),
			fmt.Sprintf("HTTP 401: invalid credential %s", tc.token),
			"auth=Bearer%20" + tc.token,
			"q=a%3D" + tc.token + "%26b=1",
			"GET /x?a=1%26" + tc.token,
			"q=" + tc.token + "%26" + tc.token,
		}
		for _, in := range samples {
			out := Scrub(in)
			if strings.Contains(out, tc.token) || strings.Contains(out, tc.payload) {
				t.Errorf("Scrub(%q) = %q leaked %s", in, out, tc.name)
			}
			if !strings.Contains(out, redacted) {
				t.Errorf("Scrub(%q) = %q missing %s", in, out, redacted)
			}
			if Scrub(out) != out {
				t.Errorf("Scrub(%q) not idempotent: %q -> %q", in, out, Scrub(out))
			}
		}
		if got := Scrub("q=a%3D" + tc.token + "%26b=1"); got != "q=a%3D"+redacted+"%26b=1" {
			t.Errorf("Scrub pct delimiter %s = %q, want %q", tc.name, got, "q=a%3D"+redacted+"%26b=1")
		}
	}

	// Underscore-adjacent and back-to-back exact 32-char hev_/hvl_ tokens preserve surrounding delimiters.
	hevToken := "hev_" + hevPayload
	hvlToken := "hvl_" + hvlPayload
	for _, uc := range []struct{ in, want string }{
		{"X_" + hevToken, "X_" + redacted},
		{hevToken + "_x", redacted + "_x"},
		{"X_" + hevToken + "_x", "X_" + redacted + "_x"},
		{"X_" + hvlToken + "_y", "X_" + redacted + "_y"},
		{hevToken + "_" + hevToken, redacted + "_" + redacted},
		{hevToken + "_" + hvlToken, redacted + "_" + redacted},
		{hevToken + " " + hvlToken, redacted + " " + redacted},
	} {
		got := Scrub(uc.in)
		if got != uc.want {
			t.Errorf("Scrub(%q) = %q, want %q", uc.in, got, uc.want)
		}
		if Scrub(got) != got {
			t.Errorf("Scrub(%q) not idempotent", uc.in)
		}
	}

	// Permitted/preserved side: Prometheus metrics, tool names, env var names, and wordmark prose.
	for _, keep := range []string{
		"hev_cluster_agents_working",
		"hev_dispatch_runs_total",
		"hev_dispatch",
		"hev_board_post",
		"my_hev_metric",
		"HEV_API_KEY",
		"HEVLAYER_API_KEY",
		"hev_ kit",
		"hev_abcdefghijklmnopqrstuvwxyz_suffix",
		"hev_abcdefghijklmnopqrstuvwxyz_total",
		"hvl_abcdefghijklmnopqrstuvwxyz_metric",
		"foo%20hev_abcdefghijklmnopqrstuvwxyz_total",
		"foo%20hev_dispatch",
		"metrics: hev_cluster_agents_working and hev_dispatch_runs_total via hev_dispatch / hev_board_post",
		"set HEV_API_KEY or HEVLAYER_API_KEY in hev_ kit",
	} {
		if got := Scrub(keep); got != keep {
			t.Errorf("Scrub(%q) = %q, want unchanged", keep, got)
		}
	}
}

func TestClubKitEndpointResolutionAndGuards(t *testing.T) {
	hevKey := "hev_" + strings.Repeat("a1B2", 8)
	hvlKey := "hvl_" + strings.Repeat("Z9y8", 8)
	hkKey := "hk_" + strings.Repeat("Ab9_-1", 7) + "A"
	orKey := "sk-or-v1-" + strings.Repeat("01234567", 8)

	t.Run("load-kit-table-preserves-strict-foh-checks", func(t *testing.T) {
		home := isolateHome(t)
		cfgPath := filepath.Join(home, ".hev", "config.toml")
		os.MkdirAll(filepath.Dir(cfgPath), 0o700)
		os.WriteFile(cfgPath, []byte(fmt.Sprintf(`
[foh]
model = "anthropic/claude-sonnet-5.5"

[kit]
service = "https://kit.example.com/"
api = "https://api.kit.example.com/"
key = %q
author_id = "user_123"
author_name = "Adam"
`, hevKey)), 0o600)
		cfg, err := LoadConfig("")
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		if cfg.Kit.Service != "https://kit.example.com/" || cfg.Kit.API != "https://api.kit.example.com/" || cfg.Kit.Key != hevKey {
			t.Fatalf("kit = %+v", cfg.Kit)
		}
		// Unknown fields in [foh] and [foh.codex] must still fail closed.
		os.WriteFile(cfgPath, []byte("[kit]\nkey = \"x\"\n[foh]\nbogus = 1\n"), 0o600)
		if _, err := LoadConfig(""); err == nil {
			t.Fatal("unknown [foh] field accepted")
		}
		os.WriteFile(cfgPath, []byte("[kit]\nkey = \"x\"\n[foh.codex]\nbogus = 1\n"), 0o600)
		if _, err := LoadConfig(""); err == nil {
			t.Fatal("unknown [foh.codex] field accepted")
		}
	})

	t.Run("direct-openrouter-unchanged", func(t *testing.T) {
		isolateHome(t)
		t.Setenv("OPENROUTER_API_KEY", orKey)
		t.Setenv("HEV_API_KEY", hevKey)
		cfg := Config{Kit: KitConfig{Key: hevKey, API: "https://kit.example.com"}}
		ep, err := cfg.ResolveEndpoint(context.Background())
		if err != nil {
			t.Fatalf("ResolveEndpoint: %v", err)
		}
		if ep.Mode != InferenceModeOpenRouter || ep.BaseURL != DefaultBaseURL || ep.APIKey != orKey {
			t.Fatalf("ep = %+v", ep)
		}
		t.Setenv("HEV_FOH_BASE_URL", "https://proxy.example.com/v1/")
		ep, err = cfg.ResolveEndpoint(context.Background())
		if err != nil || ep.BaseURL != "https://proxy.example.com/v1" {
			t.Fatalf("override ep = %+v %v", ep, err)
		}
	})

	t.Run("hosted-url-precedence-hev-and-hvl", func(t *testing.T) {
		isolateHome(t)
		t.Setenv("HEV_API_KEY", hevKey)
		cfg := Config{}
		ep, err := cfg.ResolveEndpoint(context.Background())
		if err != nil || ep.Mode != InferenceModeClubKit || ep.BaseURL != DefaultKitServiceURL+"/v1/inference" || ep.APIKey != hevKey {
			t.Fatalf("default hosted = %+v %v", ep, err)
		}

		cfg.Kit.Service = "https://svc.kit.example.com/"
		ep, err = cfg.ResolveEndpoint(context.Background())
		if err != nil || ep.BaseURL != "https://svc.kit.example.com/v1/inference" {
			t.Fatalf("kit.service = %+v %v", ep, err)
		}

		cfg.Kit.API = "https://api.kit.example.com/"
		ep, err = cfg.ResolveEndpoint(context.Background())
		if err != nil || ep.BaseURL != "https://api.kit.example.com/v1/inference" {
			t.Fatalf("kit.api = %+v %v", ep, err)
		}

		t.Setenv("HEV_KIT_SERVICE", "https://env.kit.example.com/")
		ep, err = cfg.ResolveEndpoint(context.Background())
		if err != nil || ep.BaseURL != "https://env.kit.example.com/v1/inference" {
			t.Fatalf("HEV_KIT_SERVICE = %+v %v", ep, err)
		}

		cfg.BaseURL = "https://foh.kit.example.com/custom/"
		ep, err = cfg.ResolveEndpoint(context.Background())
		if err != nil || ep.BaseURL != "https://foh.kit.example.com/custom" {
			t.Fatalf("foh.base_url = %+v %v", ep, err)
		}

		t.Setenv("HEV_FOH_BASE_URL", "http://127.0.0.1:9999/v1/inference/")
		ep, err = cfg.ResolveEndpoint(context.Background())
		if err != nil || ep.BaseURL != "http://127.0.0.1:9999/v1/inference" {
			t.Fatalf("HEV_FOH_BASE_URL = %+v %v", ep, err)
		}

		// hvl_ key also routes to hosted inference.
		t.Setenv("HEV_FOH_BASE_URL", "")
		t.Setenv("HEV_API_KEY", hvlKey)
		cfg.BaseURL = ""
		ep, err = cfg.ResolveEndpoint(context.Background())
		if err != nil || ep.Mode != InferenceModeClubKit || ep.BaseURL != "https://env.kit.example.com/v1/inference" || ep.APIKey != hvlKey {
			t.Fatalf("hvl hosted = %+v %v", ep, err)
		}
	})

	t.Run("kit-key-and-api-key-ref-precedence", func(t *testing.T) {
		isolateHome(t)
		cfg := Config{Kit: KitConfig{Key: hevKey, Service: "https://kit.example.com"}}
		ep, err := cfg.ResolveEndpoint(context.Background())
		if err != nil || ep.Mode != InferenceModeClubKit || ep.APIKey != hevKey || ep.BaseURL != "https://kit.example.com/v1/inference" {
			t.Fatalf("kit.key = %+v %v", ep, err)
		}

		// [foh].api_key_ref overrides [kit].key when set.
		orig := ResolveSecretRef
		defer func() { ResolveSecretRef = orig }()
		ResolveSecretRef = func(context.Context, string) (string, error) { return hvlKey, nil }
		cfg.APIKeyRef = "op://vault/kit/key"
		ep, err = cfg.ResolveEndpoint(context.Background())
		if err != nil || ep.Mode != InferenceModeClubKit || ep.APIKey != hvlKey {
			t.Fatalf("api_key_ref hvl = %+v %v", ep, err)
		}
		ResolveSecretRef = func(context.Context, string) (string, error) { return orKey, nil }
		ep, err = cfg.ResolveEndpoint(context.Background())
		if err != nil || ep.Mode != InferenceModeOpenRouter || ep.BaseURL != DefaultBaseURL || ep.APIKey != orKey {
			t.Fatalf("api_key_ref openrouter = %+v %v", ep, err)
		}
	})

	t.Run("hev-kit-service-scoped-to-hev-api-key", func(t *testing.T) {
		isolateHome(t)
		cfg := Config{Kit: KitConfig{Key: hevKey, API: "https://prod.kit.example.com/"}}
		t.Setenv("HEV_KIT_SERVICE", "https://staging.kit.example.com/")

		// Refused side: stray HEV_KIT_SERVICE must NOT redirect a [kit].key from config.toml.
		ep, err := cfg.ResolveEndpoint(context.Background())
		if err != nil || ep.Mode != InferenceModeClubKit || ep.BaseURL != "https://prod.kit.example.com/v1/inference" || ep.APIKey != hevKey {
			t.Fatalf("[kit].key with stray HEV_KIT_SERVICE = %+v %v", ep, err)
		}
		if got := cfg.ResolvedKitServiceURL(); got != "https://prod.kit.example.com" {
			t.Fatalf("ResolvedKitServiceURL without HEV_API_KEY = %q", got)
		}

		// [foh].api_key_ref without HEV_API_KEY also ignores stray HEV_KIT_SERVICE.
		orig := ResolveSecretRef
		defer func() { ResolveSecretRef = orig }()
		ResolveSecretRef = func(context.Context, string) (string, error) { return hvlKey, nil }
		cfgRef := cfg
		cfgRef.APIKeyRef = "op://vault/kit/key"
		ep, err = cfgRef.ResolveEndpoint(context.Background())
		if err != nil || ep.Mode != InferenceModeClubKit || ep.BaseURL != "https://prod.kit.example.com/v1/inference" || ep.APIKey != hvlKey {
			t.Fatalf("api_key_ref with stray HEV_KIT_SERVICE = %+v %v", ep, err)
		}

		// Explicit HEV_FOH_BASE_URL still overrides for [kit].key.
		t.Setenv("HEV_FOH_BASE_URL", "https://explicit.foh.example.com/v1/inference/")
		ep, err = cfg.ResolveEndpoint(context.Background())
		if err != nil || ep.BaseURL != "https://explicit.foh.example.com/v1/inference" {
			t.Fatalf("explicit HEV_FOH_BASE_URL = %+v %v", ep, err)
		}
		t.Setenv("HEV_FOH_BASE_URL", "")

		// Permitted side: HEV_API_KEY + HEV_KIT_SERVICE routes to HEV_KIT_SERVICE + /v1/inference.
		t.Setenv("HEV_API_KEY", hvlKey)
		ep, err = cfg.ResolveEndpoint(context.Background())
		if err != nil || ep.Mode != InferenceModeClubKit || ep.BaseURL != "https://staging.kit.example.com/v1/inference" || ep.APIKey != hvlKey {
			t.Fatalf("HEV_API_KEY + HEV_KIT_SERVICE = %+v %v", ep, err)
		}
		if got := cfg.ResolvedKitServiceURL(); got != "https://staging.kit.example.com" {
			t.Fatalf("ResolvedKitServiceURL with HEV_API_KEY = %q", got)
		}
	})

	t.Run("scheme-safety-and-openrouter-hard-guard", func(t *testing.T) {
		isolateHome(t)
		t.Setenv("HEV_API_KEY", hevKey)

		// Loopback http:// and https:// are permitted.
		for _, allowed := range []string{
			"https://kit.hevmind.com/v1/inference",
			"http://localhost:8080/v1/inference",
			"http://127.0.0.1:8080/v1/inference",
			"http://[::1]:8080/v1/inference",
		} {
			t.Setenv("HEV_FOH_BASE_URL", allowed)
			if _, err := (Config{}).ResolveEndpoint(context.Background()); err != nil {
				t.Errorf("expected allowed URL %s, got %v", allowed, err)
			}
		}

		// Non-loopback http:// and unsupported schemes are refused.
		for _, refused := range []string{
			"http://kit.hevmind.com/v1/inference",
			"http://192.168.1.50:8080/v1/inference",
			"http://localhost.evil.example/v1/inference",
			"ftp://kit.hevmind.com/v1/inference",
		} {
			t.Setenv("HEV_FOH_BASE_URL", refused)
			if _, err := (Config{}).ResolveEndpoint(context.Background()); err == nil {
				t.Errorf("expected refused URL %s", refused)
			}
		}

		// Hard guard: never send hev_ or hvl_ to openrouter.ai even via explicit override or OPENROUTER_API_KEY.
		for _, badURL := range []string{
			"https://openrouter.ai/api/v1",
			"https://api.openrouter.ai/api/v1",
			"https://OPENROUTER.AI:443/api/v1",
		} {
			t.Setenv("HEV_FOH_BASE_URL", badURL)
			_, err := (Config{}).ResolveEndpoint(context.Background())
			if !errors.Is(err, ErrOpenRouterKeyMismatch) {
				t.Errorf("URL %s: err = %v, want ErrOpenRouterKeyMismatch", badURL, err)
			}
			if strings.Contains(err.Error(), hevKey) {
				t.Errorf("URL %s: error leaked key", badURL)
			}
		}
		t.Setenv("HEV_FOH_BASE_URL", "")
		t.Setenv("HEV_API_KEY", "")
		t.Setenv("OPENROUTER_API_KEY", hevKey)
		if _, err := (Config{}).ResolveEndpoint(context.Background()); !errors.Is(err, ErrOpenRouterKeyMismatch) {
			t.Fatalf("OPENROUTER_API_KEY=hev_: err = %v, want ErrOpenRouterKeyMismatch", err)
		}

		// Client itself refuses before any network call.
		c := NewClient(DefaultBaseURL, hevKey)
		if _, err := c.Stream(context.Background(), ChatRequest{Model: "m"}, nil); !errors.Is(err, ErrOpenRouterKeyMismatch) {
			t.Fatalf("Client.Stream openrouter guard = %v", err)
		}
		if _, _, err := (&Catalog{Client: c, Path: filepath.Join(t.TempDir(), "c.json")}).Models(context.Background(), true); !errors.Is(err, ErrOpenRouterKeyMismatch) {
			t.Fatalf("Catalog.Models openrouter guard = %v", err)
		}
		cPlain := NewClient("http://kit.example.com/v1/inference", hvlKey)
		if _, err := cPlain.Stream(context.Background(), ChatRequest{Model: "m"}, nil); err == nil || !strings.Contains(err.Error(), "https://") {
			t.Fatalf("Client.Stream plain http guard = %v", err)
		}
	})

	t.Run("legacy-hk-key-fails-closed", func(t *testing.T) {
		isolateHome(t)
		var hits atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		checkErr := func(label string, err error) {
			t.Helper()
			if !errors.Is(err, ErrLegacyKitKey) {
				t.Fatalf("%s: err = %v, want ErrLegacyKitKey", label, err)
			}
			msg := err.Error()
			if strings.Contains(msg, hkKey) || strings.Contains(msg, hkKey[3:]) {
				t.Fatalf("%s: leaked hk_ secret: %s", label, msg)
			}
			for _, want := range []string{"hk_", "hev login", "hev_", "OPENROUTER_API_KEY"} {
				if !strings.Contains(msg, want) {
					t.Errorf("%s: missing %q in %q", label, want, msg)
				}
			}
		}

		// Via explicit HEV_API_KEY (must NOT be wrapped as ImplicitLegacyKitKeyError).
		t.Setenv("HEV_API_KEY", hkKey)
		_, err := (Config{}).ResolveEndpoint(context.Background())
		checkErr("HEV_API_KEY", err)
		var implicitErr ImplicitLegacyKitKeyError
		if errors.As(err, &implicitErr) {
			t.Fatalf("explicit HEV_API_KEY must not return ImplicitLegacyKitKeyError: %T", err)
		}
		_, err = (Config{}).ResolveAPIKey(context.Background())
		checkErr("ResolveAPIKey", err)
		if errors.As(err, &implicitErr) {
			t.Fatalf("explicit ResolveAPIKey must not return ImplicitLegacyKitKeyError: %T", err)
		}

		// Via explicit OPENROUTER_API_KEY and [foh].api_key_ref (must remain immediately fatal).
		t.Setenv("HEV_API_KEY", "")
		t.Setenv("OPENROUTER_API_KEY", hkKey)
		_, err = (Config{}).ResolveEndpoint(context.Background())
		checkErr("OPENROUTER_API_KEY", err)
		if errors.As(err, &implicitErr) {
			t.Fatalf("explicit OPENROUTER_API_KEY must not return ImplicitLegacyKitKeyError: %T", err)
		}
		t.Setenv("OPENROUTER_API_KEY", "")
		origRef := ResolveSecretRef
		defer func() { ResolveSecretRef = origRef }()
		ResolveSecretRef = func(context.Context, string) (string, error) { return hkKey, nil }
		_, err = (Config{APIKeyRef: "op://vault/kit/legacy"}).ResolveEndpoint(context.Background())
		checkErr("[foh].api_key_ref", err)
		if errors.As(err, &implicitErr) {
			t.Fatalf("explicit api_key_ref must not return ImplicitLegacyKitKeyError: %T", err)
		}

		// Via implicit [kit].key (wraps ErrLegacyKitKey AND identifies itself as ImplicitLegacyKitKeyError).
		_, err = (Config{Kit: KitConfig{Key: hkKey, API: srv.URL}}).ResolveEndpoint(context.Background())
		checkErr("[kit].key", err)
		if !errors.As(err, &implicitErr) {
			t.Fatalf("implicit [kit].key ResolveEndpoint should return ImplicitLegacyKitKeyError, got %T", err)
		}
		_, err = (Config{Kit: KitConfig{Key: hkKey, API: srv.URL}}).ResolveAPIKey(context.Background())
		checkErr("[kit].key ResolveAPIKey", err)
		if !errors.As(err, &implicitErr) {
			t.Fatalf("implicit [kit].key ResolveAPIKey should return ImplicitLegacyKitKeyError, got %T", err)
		}

		// Via direct Client.Stream and Catalog.Models (must make zero network requests).
		c := NewClient(srv.URL+"/v1/inference", hkKey)
		_, err = c.Stream(context.Background(), ChatRequest{Model: "m"}, nil)
		checkErr("Client.Stream", err)
		_, _, err = (&Catalog{Client: c, Path: filepath.Join(t.TempDir(), "c.json")}).Models(context.Background(), true)
		checkErr("Catalog.Models", err)
		if hits.Load() != 0 {
			t.Fatalf("legacy hk_ key made %d network requests, want 0", hits.Load())
		}
	})
}

func TestClubKitHostedInferenceHTTPAndErrors(t *testing.T) {
	isolateHome(t)
	hevKey := "hev_" + strings.Repeat("a1B2", 8)

	t.Run("routes-chat-completions-and-models", func(t *testing.T) {
		var gotChatPath, gotModelsPath, gotChatAuth, gotModelsAuth string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/v1/inference/chat/completions":
				gotChatPath = r.URL.Path
				gotChatAuth = r.Header.Get("Authorization")
				sse(w, `{"model":"anthropic/claude-sonnet-5.5","choices":[{"delta":{"content":"hosted ok"},"finish_reason":"stop"}]}`)
			case "/v1/inference/models":
				gotModelsPath = r.URL.Path
				gotModelsAuth = r.Header.Get("Authorization")
				fmt.Fprint(w, `{"data":[{"id":"anthropic/claude-sonnet-5.5","name":"Claude Sonnet","context_length":200000,"pricing":{"prompt":"0.000003","completion":"0.000015"}}]}`)
			default:
				http.NotFound(w, r)
			}
		}))
		defer srv.Close()

		cfg := Config{Kit: KitConfig{API: srv.URL, Key: hevKey}}
		ep, err := cfg.ResolveEndpoint(context.Background())
		if err != nil {
			t.Fatalf("ResolveEndpoint: %v", err)
		}
		if ep.BaseURL != srv.URL+"/v1/inference" {
			t.Fatalf("baseURL = %s", ep.BaseURL)
		}

		c := NewClient(ep.BaseURL, ep.APIKey)
		res, err := c.Stream(context.Background(), ChatRequest{Model: "anthropic/claude-sonnet-5.5", Messages: []Message{{Role: "user", Content: "hi"}}}, nil)
		if err != nil || res.Content != "hosted ok" {
			t.Fatalf("Stream = %+v %v", res, err)
		}
		models, src, err := c.Models(context.Background(), true)
		if err != nil || src != SourceNetwork || len(models) != 1 || models[0].ID != "anthropic/claude-sonnet-5.5" {
			t.Fatalf("Models = %+v %s %v", models, src, err)
		}
		if gotChatPath != "/v1/inference/chat/completions" || gotChatAuth != "Bearer "+hevKey {
			t.Fatalf("chat path=%q auth ok=%v", gotChatPath, gotChatAuth == "Bearer "+hevKey)
		}
		if gotModelsPath != "/v1/inference/models" || gotModelsAuth != "Bearer "+hevKey {
			t.Fatalf("models path=%q auth ok=%v", gotModelsPath, gotModelsAuth == "Bearer "+hevKey)
		}
	})

	t.Run("refuses-redirects-without-leaking-bearer", func(t *testing.T) {
		var offOriginHits atomic.Int32
		offOrigin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			offOriginHits.Add(1)
			w.WriteHeader(http.StatusOK)
		}))
		defer offOrigin.Close()

		redirectSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, offOrigin.URL+"/steal", http.StatusFound)
		}))
		defer redirectSrv.Close()

		c := NewClient(redirectSrv.URL+"/v1/inference", hevKey)
		_, err := c.Stream(context.Background(), ChatRequest{Model: "m"}, nil)
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "redirect") {
			t.Fatalf("Stream redirect err = %v", err)
		}
		if strings.Contains(err.Error(), hevKey) {
			t.Fatalf("Stream redirect leaked key: %v", err)
		}
		_, _, err = (&Catalog{Client: c, Path: filepath.Join(t.TempDir(), "c.json")}).Models(context.Background(), true)
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "redirect") {
			t.Fatalf("Models redirect err = %v", err)
		}
		if offOriginHits.Load() != 0 {
			t.Fatalf("off-origin server received %d redirected requests, want 0", offOriginHits.Load())
		}
	})

	t.Run("surfaces-401-402-403-scrubbed", func(t *testing.T) {
		cases := []struct {
			name     string
			status   int
			body     string
			wantSub  []string
			keepText string
		}{
			{
				name:     "401-invalid-revoked-key",
				status:   http.StatusUnauthorized,
				body:     fmt.Sprintf(`{"error":{"message":"revoked token %s header Authorization: Bearer %s"}}`, hevKey, hevKey),
				wantSub:  []string{"401", "hev_", "hev login"},
				keepText: "revoked token",
			},
			{
				name:     "402-org-budget-exhausted",
				status:   http.StatusPaymentRequired,
				body:     fmt.Sprintf(`{"error":{"message":"org monthly cap reached for %s"}}`, hevKey),
				wantSub:  []string{"402", "budget"},
				keepText: "org monthly cap reached",
			},
			{
				name:     "403-org-inference-disabled",
				status:   http.StatusForbidden,
				body:     fmt.Sprintf(`{"error":{"message":"inference disabled on org_acme (%s)"}}`, hevKey),
				wantSub:  []string{"403", "disabled", "entitlement"},
				keepText: "inference disabled on org_acme",
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(tc.status)
					fmt.Fprint(w, tc.body)
				}))
				defer srv.Close()

				c := NewClient(srv.URL+"/v1/inference", hevKey)
				_, err := c.Stream(context.Background(), ChatRequest{Model: "m"}, nil)
				if err == nil {
					t.Fatal("expected error")
				}
				msg := err.Error()
				if strings.Contains(msg, hevKey) || strings.Contains(msg, hevKey[4:]) {
					t.Fatalf("error leaked key: %s", msg)
				}
				for _, want := range tc.wantSub {
					if !strings.Contains(strings.ToLower(msg), strings.ToLower(want)) {
						t.Errorf("error %q missing %q", msg, want)
					}
				}
				if !strings.Contains(msg, tc.keepText) {
					t.Errorf("error %q lost detail %q", msg, tc.keepText)
				}

				// /models surfaces the same scrubbed hosted status error.
				_, _, mErr := (&Catalog{Client: c, Path: filepath.Join(t.TempDir(), "c.json")}).Models(context.Background(), true)
				if mErr == nil || strings.Contains(mErr.Error(), hevKey) || !strings.Contains(mErr.Error(), tc.keepText) {
					t.Fatalf("Models err = %v", mErr)
				}
			})
		}
	})
}
