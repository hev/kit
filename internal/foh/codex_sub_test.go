package foh

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	testAccountID = "acct-7f3c9e21-codexsub"
	testRefresh   = "rt_refreshSECRET0123456789abcdef"
)

// fakeJWT builds an unsigned JWT with exp and the ChatGPT account claim.
func fakeJWT(t *testing.T, exp time.Time, tag string) string {
	t.Helper()
	enc := func(v any) string {
		b, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(b)
	}
	return enc(map[string]string{"alg": "none", "typ": "JWT"}) + "." +
		enc(map[string]any{"exp": exp.Unix(), "tag": tag, "https://api.openai.com/auth": map[string]string{"chatgpt_account_id": testAccountID}}) +
		"." + base64.RawURLEncoding.EncodeToString([]byte("signature-"+tag))
}

// codexHome isolates HOME and writes CODEX_HOME/auth.json with tokens.
func codexHome(t *testing.T, tokens map[string]string) string {
	t.Helper()
	isolateHome(t)
	dir := t.TempDir()
	t.Setenv("CODEX_HOME", dir)
	if tokens != nil {
		writeAuth(t, dir, map[string]any{"OPENAI_API_KEY": nil, "auth_mode": "chatgpt", "tokens": tokens, "last_refresh": "2026-11-01T00:00:00Z", "keep_me": 42})
	}
	return dir
}

func writeAuth(t *testing.T, dir string, doc any) {
	t.Helper()
	b, _ := json.Marshal(doc)
	if err := os.WriteFile(filepath.Join(dir, "auth.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func newCodex(t *testing.T, base string) *CodexSub {
	t.Helper()
	c, err := NewCodexSub(CodexConfig{BaseURL: base})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func codexSSE(w http.ResponseWriter, events ...string) {
	w.Header().Set("Content-Type", "text/event-stream")
	for _, e := range events {
		var probe struct {
			Type string `json:"type"`
		}
		json.Unmarshal([]byte(e), &probe)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", probe.Type, e)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
}

func TestCodexAuthDiscovery(t *testing.T) {
	access := fakeJWT(t, time.Now().Add(time.Hour), "a")
	dir := codexHome(t, map[string]string{"access_token": access, "refresh_token": testRefresh, "id_token": fakeJWT(t, time.Now().Add(time.Hour), "id")})
	c := newCodex(t, "")
	if c.Home != dir || !c.Available() || !c.Active() {
		t.Fatalf("CODEX_HOME login not found: home=%q", c.Home)
	}
	models, _, err := c.Models(context.Background(), false)
	if err != nil || len(models) != len(DefaultCodexModels) || models[0].ID != "codex/gpt-6-astra" || models[0].Provider != ProviderCodexSub {
		t.Fatalf("models = %v %v", models, err)
	}
	// [foh.codex].models overrides the list; a codex/ prefix is accepted.
	c2, _ := NewCodexSub(CodexConfig{Models: []string{"codex/gpt-5.4", "gpt-5.3-codex"}})
	m2, _, _ := c2.Models(context.Background(), false)
	if len(m2) != 2 || m2[0].ID != "codex/gpt-5.4" || m2[1].ID != "codex/gpt-5.3-codex" {
		t.Fatalf("configured models = %v", m2)
	}
	// [foh.codex].home beats CODEX_HOME.
	other := t.TempDir()
	if h, err := ResolveCodexHome(CodexConfig{Home: other}); err != nil || h != other {
		t.Fatalf("home override = %q %v", h, err)
	}
	// Without CODEX_HOME the default is ~/.codex under HOME.
	t.Setenv("CODEX_HOME", "")
	if h, _ := ResolveCodexHome(CodexConfig{}); h != filepath.Join(os.Getenv("HOME"), ".codex") {
		t.Fatalf("default home = %q", h)
	}
}

func TestCodexWorkspaceProfileHome(t *testing.T) {
	home := isolateHome(t)
	profile := filepath.Join(home, "codex-profile")
	cfg := filepath.Join(home, ".hev", "config.toml")
	os.MkdirAll(filepath.Dir(cfg), 0o700)
	os.WriteFile(cfg, []byte("[workspaces.client]\nlogin = \"own\"\n[workspaces.client.subscriptions]\ncodex_home = \""+filepath.ToSlash(profile)+"\"\n"), 0o600)
	t.Setenv("HEV_CONFIG", cfg)
	t.Setenv("HEV_WORKSPACE", "client")
	h, err := ResolveCodexHome(CodexConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if h != profile {
		t.Fatalf("workspace codex profile = %q, want %q", h, profile)
	}
	// CODEX_HOME still wins over the workspace profile.
	other := t.TempDir()
	t.Setenv("CODEX_HOME", other)
	if h, _ := ResolveCodexHome(CodexConfig{}); h != other {
		t.Fatalf("CODEX_HOME over workspace = %q", h)
	}
}

func TestCodexMissingAndExpiredAuth(t *testing.T) {
	ctx := context.Background()
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }))
	defer srv.Close()

	// No auth.json: not offered, and a turn fails cleanly without a path.
	dir := codexHome(t, nil)
	c := newCodex(t, srv.URL)
	if c.Available() || c.Active() {
		t.Fatal("missing auth must not be available")
	}
	if m, _, _ := c.Models(ctx, false); len(m) != 0 {
		t.Fatalf("missing auth lists models: %v", m)
	}
	_, err := c.Stream(ctx, ChatRequest{Model: "codex/gpt-5.4"}, nil)
	if !errors.Is(err, ErrNoCodexAuth) || strings.Contains(err.Error(), dir) {
		t.Fatalf("missing auth err = %v", err)
	}

	// An API-key-only auth.json is not a subscription login.
	writeAuth(t, dir, map[string]any{"OPENAI_API_KEY": "sk-proj-abcdefghijklmnopqrstuvwx"})
	if c.Available() {
		t.Fatal("API key auth must not count as a ChatGPT login")
	}
	// Garbage is reported, not panicked on.
	os.WriteFile(filepath.Join(dir, "auth.json"), []byte("{not json"), 0o600)
	if _, err := c.Stream(ctx, ChatRequest{Model: "codex/gpt-5.4"}, nil); err == nil || strings.Contains(err.Error(), dir) {
		t.Fatalf("invalid auth err = %v", err)
	}

	// Expired access token and no refresh token: not offered, clean error.
	expired := fakeJWT(t, time.Now().Add(-time.Hour), "old")
	writeAuth(t, dir, map[string]any{"tokens": map[string]string{"access_token": expired}})
	if c.Available() {
		t.Fatal("expired, unrefreshable auth must not be available")
	}
	_, err = c.Stream(ctx, ChatRequest{Model: "codex/gpt-5.4"}, nil)
	if !errors.Is(err, ErrCodexAuthExpired) || strings.Contains(err.Error(), expired) {
		t.Fatalf("expired err = %v", err)
	}
	if hit {
		t.Fatal("no request may be sent without a usable login")
	}
	// [foh.codex].enabled = true still lists the models; the turn explains.
	on := true
	c.Enabled = &on
	if m, _, _ := c.Models(ctx, false); len(m) == 0 {
		t.Fatal("enabled = true must list models")
	}
	off := false
	c.Enabled = &off
	writeAuth(t, dir, map[string]any{"tokens": map[string]string{"access_token": fakeJWT(t, time.Now().Add(time.Hour), "live")}})
	if c.Active() {
		t.Fatal("enabled = false must hide a valid login")
	}
}

func TestCodexStreamTextAndToolCalls(t *testing.T) {
	access := fakeJWT(t, time.Now().Add(time.Hour), "live")
	codexHome(t, map[string]string{"access_token": access, "refresh_token": testRefresh})
	var gotAuth, gotAccount, gotPath string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth, gotAccount = r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("ChatGPT-Account-Id")
		json.NewDecoder(r.Body).Decode(&body)
		codexSSE(w,
			`{"type":"response.created","response":{"id":"resp_1"}}`,
			`{"type":"response.output_text.delta","delta":"Look"}`,
			`{"type":"response.output_text.delta","delta":"ing."}`,
			`{"type":"response.output_item.added","output_index":1,"item":{"type":"function_call","id":"fc_1","call_id":"call_A","name":"glob","arguments":""}}`,
			`{"type":"response.function_call_arguments.delta","item_id":"fc_1","output_index":1,"delta":"{\"pattern\":"}`,
			`{"type":"response.function_call_arguments.delta","item_id":"fc_1","output_index":1,"delta":"\"*.md\"}"}`,
			`{"type":"response.output_item.added","output_index":2,"item":{"type":"function_call","id":"fc_2","call_id":"call_B","name":"read_file"}}`,
			`{"type":"response.output_item.done","output_index":2,"item":{"type":"function_call","id":"fc_2","call_id":"call_B","name":"read_file","arguments":"{\"path\":\"a.md\"}"}}`,
			`not json`,
			`{"type":"response.completed","response":{"usage":{"input_tokens":40,"output_tokens":9,"total_tokens":49}}}`)
	}))
	defer srv.Close()
	c := newCodex(t, srv.URL)
	var text strings.Builder
	var tools []string
	req := ChatRequest{Model: "codex/gpt-6-astra", Messages: []Message{
		{Role: "system", Content: "SYS"},
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "earlier", ToolCalls: []ToolCall{call("toolu_or", "bash", `{"command":"ls"}`)}},
		{Role: "tool", ToolCallID: "toolu_or", Name: "bash", Content: "README.md"},
		{Role: "user", Content: "now"},
	}, Tools: []ToolSpec{{Type: "function", Function: FunctionSpec{Name: "glob", Description: "g", Parameters: json.RawMessage(`{"type":"object"}`)}}}}
	res, err := c.Stream(context.Background(), req, func(d Delta) {
		text.WriteString(d.Text)
		if d.ToolName != "" {
			tools = append(tools, d.ToolName)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/responses" || gotAuth != "Bearer "+access || gotAccount != testAccountID {
		t.Fatalf("path=%q auth ok=%v account=%q", gotPath, gotAuth == "Bearer "+access, gotAccount)
	}
	if body["model"] != "gpt-6-astra" || body["instructions"] != "SYS" || body["stream"] != true || body["store"] != false {
		t.Fatalf("body = %v", body)
	}
	var kinds []string
	for _, it := range body["input"].([]any) {
		kinds = append(kinds, it.(map[string]any)["type"].(string))
	}
	if strings.Join(kinds, ",") != "message,message,function_call,function_call_output,message" {
		t.Fatalf("input kinds = %v", kinds)
	}
	if tl := body["tools"].([]any); len(tl) != 1 || tl[0].(map[string]any)["name"] != "glob" {
		t.Fatalf("tools = %v", body["tools"])
	}
	if res.Content != "Looking." || text.String() != "Looking." {
		t.Fatalf("content = %q / %q", res.Content, text.String())
	}
	if len(res.ToolCalls) != 2 || res.ToolCalls[0].ID != "call_A" || res.ToolCalls[0].Function.Arguments != `{"pattern":"*.md"}` ||
		res.ToolCalls[1].Function.Name != "read_file" || res.ToolCalls[1].Function.Arguments != `{"path":"a.md"}` {
		t.Fatalf("tool calls = %+v", res.ToolCalls)
	}
	if strings.Join(tools, ",") != "glob,read_file" || res.FinishReason != "tool_calls" {
		t.Fatalf("tool starts = %v finish=%q", tools, res.FinishReason)
	}
	if res.Usage.PromptTokens != 40 || res.Usage.CompletionTokens != 9 || res.Usage.TotalTokens != 49 {
		t.Fatalf("usage = %+v", res.Usage)
	}
}

func TestCodexRefreshExpiredToken(t *testing.T) {
	old := fakeJWT(t, time.Now().Add(-time.Minute), "old")
	fresh := fakeJWT(t, time.Now().Add(time.Hour), "fresh")
	const newRefresh = "rt_rotatedSECRET9876543210zyxwv"
	dir := codexHome(t, map[string]string{"access_token": old, "refresh_token": testRefresh, "account_id": testAccountID})
	var refreshes int
	var mu sync.Mutex
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		refreshes++
		mu.Unlock()
		var in map[string]string
		json.NewDecoder(r.Body).Decode(&in)
		if in["grant_type"] != "refresh_token" || in["refresh_token"] != testRefresh || in["client_id"] == "" {
			w.WriteHeader(400)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"access_token": fresh, "refresh_token": newRefresh})
	}))
	defer tokenSrv.Close()
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		codexSSE(w, `{"type":"response.output_text.delta","delta":"ok"}`, `{"type":"response.completed","response":{}}`)
	}))
	defer srv.Close()
	c := newCodex(t, srv.URL)
	c.TokenURL = tokenSrv.URL
	if !c.Available() {
		t.Fatal("expired token with a refresh token must be available")
	}
	res, err := c.Stream(context.Background(), ChatRequest{Model: "codex/gpt-5.4"}, nil)
	if err != nil || res.Content != "ok" {
		t.Fatalf("stream = %+v %v", res, err)
	}
	if refreshes != 1 || gotAuth != "Bearer "+fresh {
		t.Fatalf("refreshes=%d fresh token used=%v", refreshes, gotAuth == "Bearer "+fresh)
	}
	// The rotated tokens are written back; unknown fields survive.
	b, _ := os.ReadFile(filepath.Join(dir, "auth.json"))
	var doc map[string]any
	json.Unmarshal(b, &doc)
	tok := doc["tokens"].(map[string]any)
	if tok["access_token"] != fresh || tok["refresh_token"] != newRefresh || doc["keep_me"] != float64(42) || doc["auth_mode"] != "chatgpt" {
		t.Fatalf("auth.json after refresh = %s", b)
	}
	if st, _ := os.Stat(filepath.Join(dir, "auth.json")); st.Mode().Perm()&0o077 != 0 && os.PathSeparator == '/' {
		t.Fatalf("auth.json mode = %v", st.Mode())
	}
	// The next request uses the stored fresh token without refreshing again.
	c.Stream(context.Background(), ChatRequest{Model: "codex/gpt-5.4"}, nil)
	if refreshes != 1 {
		t.Fatalf("refreshed again: %d", refreshes)
	}
	// Every loaded or refreshed credential is known to the scrubber.
	secrets := strings.Join(c.Secrets(), " ")
	for _, s := range []string{old, fresh, testRefresh, newRefresh, testAccountID} {
		if !strings.Contains(secrets, s) {
			t.Fatalf("secret not tracked: %.12s…", s)
		}
	}
}

func TestCodexRefreshOn401AndBounded(t *testing.T) {
	live := fakeJWT(t, time.Now().Add(time.Hour), "revoked")
	fresh := fakeJWT(t, time.Now().Add(time.Hour), "fresh")
	codexHome(t, map[string]string{"access_token": live, "refresh_token": testRefresh})
	var refreshes, calls int
	failRefresh := false
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		refreshes++
		if failRefresh {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"error":"invalid_grant","refresh_token":%q}`, testRefresh)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"access_token": fresh})
	}))
	defer tokenSrv.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer "+fresh {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"detail":"token %s revoked"}`, live)
			return
		}
		codexSSE(w, `{"type":"response.output_text.delta","delta":"retried"}`)
	}))
	defer srv.Close()
	c := newCodex(t, srv.URL)
	c.TokenURL = tokenSrv.URL
	res, err := c.Stream(context.Background(), ChatRequest{Model: "codex/gpt-5.4"}, nil)
	if err != nil || res.Content != "retried" || refreshes != 1 || calls != 2 {
		t.Fatalf("401 retry: res=%q err=%v refreshes=%d calls=%d", res.Content, err, refreshes, calls)
	}

	// A failing refresh is reported once, then backed off; nothing leaks.
	c2 := newCodex(t, srv.URL)
	c2.TokenURL = tokenSrv.URL
	writeAuth(t, c2.Home, map[string]any{"tokens": map[string]string{"access_token": live, "refresh_token": testRefresh}})
	failRefresh, refreshes, calls = true, 0, 0
	for i := 0; i < 3; i++ {
		_, err = c2.Stream(context.Background(), ChatRequest{Model: "codex/gpt-5.4"}, nil)
		if err == nil || strings.Contains(err.Error(), testRefresh) || strings.Contains(err.Error(), live) {
			t.Fatalf("failed refresh err = %v", err)
		}
	}
	if refreshes != 1 {
		t.Fatalf("refresh not bounded: %d attempts", refreshes)
	}
}

func TestCodexNeverReadsAnthropicCredentials(t *testing.T) {
	home := isolateHome(t)
	claudeDir := filepath.Join(home, ".claude")
	os.MkdirAll(claudeDir, 0o700)
	const claudeToken = "sk-ant-oat01-CLAUDESUBSCRIPTIONTOKEN0123456789"
	cred := `{"claudeAiOauth":{"accessToken":"` + claudeToken + `","refreshToken":"sk-ant-ort01-xyz0123456789abcdef"}}`
	os.WriteFile(filepath.Join(claudeDir, ".credentials.json"), []byte(cred), 0o600)
	os.WriteFile(filepath.Join(claudeDir, "auth.json"), []byte(cred), 0o600)
	var reads []string
	onAuthRead = func(p string) { reads = append(reads, p) }
	t.Cleanup(func() { onAuthRead = nil })

	// Refused: CODEX_HOME, [foh.codex].home or CLAUDE_CONFIG_DIR pointing at
	// a Claude config directory.
	t.Setenv("CODEX_HOME", claudeDir)
	if _, err := NewCodexSub(CodexConfig{}); !errors.Is(err, ErrAnthropicCredentials) {
		t.Fatalf("CODEX_HOME=~/.claude err = %v", err)
	}
	t.Setenv("CODEX_HOME", "")
	if _, err := NewCodexSub(CodexConfig{Home: "~/.claude"}); !errors.Is(err, ErrAnthropicCredentials) {
		t.Fatalf("home=~/.claude err = %v", err)
	}
	custom := filepath.Join(home, "claude-cfg")
	os.MkdirAll(custom, 0o700)
	os.WriteFile(filepath.Join(custom, "auth.json"), []byte(cred), 0o600)
	t.Setenv("CLAUDE_CONFIG_DIR", custom)
	t.Setenv("CODEX_HOME", custom)
	if _, err := NewCodexSub(CodexConfig{}); !errors.Is(err, ErrAnthropicCredentials) {
		t.Fatalf("CODEX_HOME=CLAUDE_CONFIG_DIR err = %v", err)
	}
	// Refused: a Claude OAuth blob, whatever directory it sits in.
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	c, err := NewCodexSub(CodexConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if c.Available() {
		t.Fatal("a Claude OAuth blob must not count as a login")
	}
	_, err = c.Stream(context.Background(), ChatRequest{Model: "codex/gpt-5.4"}, nil)
	if !errors.Is(err, ErrAnthropicCredentials) || strings.Contains(err.Error(), claudeToken) {
		t.Fatalf("claude blob err = %v", err)
	}
	if strings.Contains(strings.Join(c.Secrets(), " "), claudeToken) {
		t.Fatal("Claude token was loaded")
	}
	// Refused: Anthropic models on the subscription provider.
	if _, err := codexUpstream("codex/claude-sonnet-5.5"); err == nil {
		t.Fatal("codex/claude-* must be refused")
	}

	// Kept: the normal ~/.codex discovery reads only Codex's auth.json, and a
	// directory that merely contains "claude" in its name is fine.
	t.Setenv("CODEX_HOME", "")
	reads = nil
	os.MkdirAll(filepath.Join(home, ".codex"), 0o700)
	writeAuth(t, filepath.Join(home, ".codex"), map[string]any{"tokens": map[string]string{"access_token": fakeJWT(t, time.Now().Add(time.Hour), "ok")}})
	c, _ = NewCodexSub(CodexConfig{})
	if !c.Available() {
		t.Fatal("~/.codex login not found")
	}
	okDir := filepath.Join(home, "not-claude-codex")
	os.MkdirAll(okDir, 0o700)
	if _, err := NewCodexSub(CodexConfig{Home: okDir}); err != nil {
		t.Fatalf("ordinary dir refused: %v", err)
	}
	if _, err := codexUpstream("codex/gpt-5.3-codex"); err != nil {
		t.Fatal(err)
	}
	for _, p := range reads {
		if strings.Contains(filepath.ToSlash(p), "/.claude") || strings.HasSuffix(p, ".credentials.json") || filepath.Base(p) != "auth.json" {
			t.Fatalf("read a non-Codex credential file: %s", p)
		}
	}
	if len(reads) == 0 {
		t.Fatal("expected the Codex auth file to be read")
	}
}

func TestCodexErrorRedaction(t *testing.T) {
	access := fakeJWT(t, time.Now().Add(time.Hour), "live")
	codexHome(t, map[string]string{"access_token": access, "refresh_token": testRefresh, "account_id": testAccountID})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = b
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"error":{"message":"model codex/gpt-5.4 rejected: Authorization: Bearer %s ChatGPT-Account-Id: %s refresh_token=%s"}}`, access, testAccountID, testRefresh)
	}))
	defer srv.Close()
	c := newCodex(t, srv.URL)
	_, err := c.Stream(context.Background(), ChatRequest{Model: "codex/gpt-5.4"}, nil)
	if err == nil {
		t.Fatal("want error")
	}
	msg := err.Error()
	for _, s := range []string{access, testAccountID, testRefresh} {
		if strings.Contains(msg, s) {
			t.Fatalf("secret leaked: %s", msg)
		}
	}
	if !strings.Contains(msg, "gpt-5.4 rejected") || !strings.Contains(msg, "codex-sub") {
		t.Fatalf("message lost: %s", msg)
	}
	// SSE failure events are scrubbed too.
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		codexSSE(w, fmt.Sprintf(`{"type":"response.failed","response":{"error":{"message":"quota for %s exhausted"}}}`, testAccountID))
	}))
	defer srv2.Close()
	c.BaseURL = srv2.URL
	_, err = c.Stream(context.Background(), ChatRequest{Model: "codex/gpt-5.4"}, nil)
	if err == nil || strings.Contains(err.Error(), testAccountID) || !strings.Contains(err.Error(), "quota for") {
		t.Fatalf("failed event err = %v", err)
	}
}

func TestScrubCodexShapes(t *testing.T) {
	jwt := fakeJWT(t, time.Now(), "x")
	in := `{"tokens":{"access_token":"` + jwt + `","refresh_token":"rt_abcdefgh12345678","id_token":"opaqueIDtoken123","account_id":"acct-123456"}} ChatGPT-Account-Id: acct-123456 bare ` + jwt
	out := Scrub(in)
	for _, s := range []string{jwt, "rt_abcdefgh12345678", "opaqueIDtoken123", "acct-123456"} {
		if strings.Contains(out, s) {
			t.Fatalf("not scrubbed %q: %s", s, out)
		}
	}
	// Kept: model IDs, provider names and prose.
	keep := `switched codex/gpt-5.3-codex (codex-sub) → google/gemini-4-neon via openrouter; the account page loads; tokens used: 49`
	if got := Scrub(keep); got != keep {
		t.Fatalf("over-scrubbed:\n%s\n%s", keep, got)
	}
}
