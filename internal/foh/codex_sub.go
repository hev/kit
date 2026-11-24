package foh

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hev/kit/internal/workspace"
)

// The codex-sub provider runs hev foh on the operator's OpenAI/Codex ChatGPT
// subscription: it reuses the login `codex login` saved in CODEX_HOME's
// auth.json and talks to the ChatGPT Codex Responses backend.
//
// Policy boundary (memo #446): this is the only subscription provider. hev foh
// never reads Anthropic/Claude OAuth credentials; Claude models route through
// OpenRouter with an API key, and Claude subscriptions stay in Claude Code.

const (
	// DefaultCodexBaseURL is the ChatGPT Codex backend; requests go to
	// BASE/responses.
	DefaultCodexBaseURL = "https://chatgpt.com/backend-api/codex"
	// DefaultCodexTokenURL refreshes an expired ChatGPT OAuth token.
	DefaultCodexTokenURL = "https://auth.openai.com/oauth/token"
	// codexClientID is the public OAuth client the Codex CLI signs in with;
	// a refresh must name the client that issued the token.
	codexClientID = "app_EMoamEEZ73f0CkXaXp7hrann"

	codexAuthFileName   = "auth.json"
	maxCodexAuthBytes   = 1 << 20
	codexRefreshTimeout = 30 * time.Second
	// codexRefreshSkew refreshes a token this long before it expires.
	codexRefreshSkew = 60 * time.Second
	// codexRefreshBackoff stops a failing refresh from being retried on
	// every request.
	codexRefreshBackoff = time.Minute
	maxCodexSecrets     = 32
)

// DefaultCodexModels are the upstream model names offered when
// [foh.codex].models is unset. The picker shows them as codex/NAME.
var DefaultCodexModels = []string{"gpt-6-astra", "gpt-5.4", "gpt-5.3-codex"}

// SourceConfig marks a catalog that comes from configuration, not a fetch.
const SourceConfig CatalogSource = "config"

var (
	// ErrNoCodexAuth means no usable ChatGPT login was found. The text names
	// no path.
	ErrNoCodexAuth = errors.New("codex-sub: no ChatGPT subscription login found for Codex (run `codex login`)")
	// ErrCodexAuthExpired means the access token expired and can't be refreshed.
	ErrCodexAuthExpired = errors.New("codex-sub: the Codex ChatGPT login has expired (run `codex login`)")
	// ErrAnthropicCredentials refuses any Anthropic/Claude credential location.
	ErrAnthropicCredentials = errors.New("codex-sub: refusing to read Anthropic/Claude credentials: Claude subscriptions stay in Claude Code (policy memo #446)")
)

// onAuthRead, when set, observes every credential file the provider opens.
// Tests use it to prove Anthropic files are never read.
var onAuthRead func(path string)

// ResolveCodexHome finds the Codex home holding auth.json: [foh.codex].home,
// then CODEX_HOME, then the active workspace's codex profile
// (HEV_WORKSPACE), then ~/.codex. Anthropic/Claude locations are refused.
func ResolveCodexHome(cfg CodexConfig) (string, error) {
	dir := ""
	for _, v := range []string{cfg.Home, os.Getenv("CODEX_HOME")} {
		if v = strings.TrimSpace(v); v != "" {
			dir = workspace.Expand(v)
			break
		}
	}
	if dir == "" {
		if ws := strings.TrimSpace(os.Getenv("HEV_WORKSPACE")); ws != "" {
			if sel, err := workspace.Named(ws); err == nil {
				dir = sel.Profile("codex")
			}
		}
	}
	if dir == "" {
		dir = filepath.Join(homeDir(), ".codex")
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	if isAnthropicPath(dir) {
		return "", ErrAnthropicCredentials
	}
	return dir, nil
}

// isAnthropicPath reports whether p is, or is inside, a Claude Code config
// directory: any .claude element, CLAUDE_CONFIG_DIR, or a .credentials.json.
func isAnthropicPath(p string) bool {
	clean := filepath.Clean(p)
	for _, el := range strings.Split(filepath.ToSlash(clean), "/") {
		if strings.EqualFold(el, ".claude") || strings.EqualFold(el, ".credentials.json") {
			return true
		}
	}
	if d := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR")); d != "" {
		d = filepath.Clean(d)
		if clean == d || strings.HasPrefix(clean, d+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// codexAuth is the ChatGPT login read from auth.json.
type codexAuth struct {
	AccessToken  string
	RefreshToken string
	IDToken      string
	AccountID    string
	Expires      time.Time // zero when the token carries no exp claim
}

func (a codexAuth) usable() bool { return a.AccessToken != "" || a.RefreshToken != "" }

// CodexSub is the codex-sub Provider.
type CodexSub struct {
	// Home is the Codex home; auth.json is read from it on every request so a
	// refresh by the Codex CLI is picked up.
	Home     string
	BaseURL  string
	TokenURL string
	// ModelNames are upstream names; Models lists them as codex/NAME.
	ModelNames []string
	// Enabled forces the provider on or off; nil means on when logged in.
	Enabled *bool
	HTTP    *http.Client
	Timeout time.Duration
	Now     func() time.Time
	// OnSecret is told every credential value the provider loads, so session
	// logs and tool output can scrub it.
	OnSecret func(string)

	mu          sync.Mutex
	secrets     []string
	lastRefresh time.Time
	refreshErr  error
}

// NewCodexSub builds the provider from [foh.codex]. HEV_FOH_CODEX_BASE_URL
// overrides the backend.
func NewCodexSub(cfg CodexConfig) (*CodexSub, error) {
	home, err := ResolveCodexHome(cfg)
	if err != nil {
		return nil, err
	}
	base := DefaultCodexBaseURL
	for _, v := range []string{os.Getenv("HEV_FOH_CODEX_BASE_URL"), cfg.BaseURL} {
		if v = strings.TrimSpace(v); v != "" {
			base = v
			break
		}
	}
	var names []string
	for _, n := range cfg.Models {
		n = strings.TrimPrefix(strings.TrimSpace(n), CodexModelPrefix)
		if n != "" {
			names = append(names, n)
		}
	}
	return &CodexSub{Home: home, BaseURL: strings.TrimRight(base, "/"), ModelNames: names,
		Enabled: cfg.Enabled, HTTP: &http.Client{}}, nil
}

// ID is ProviderCodexSub.
func (c *CodexSub) ID() string { return ProviderCodexSub }

func (c *CodexSub) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *CodexSub) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

func (c *CodexSub) timeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return DefaultRequestTimeout
}

func (c *CodexSub) authPath() (string, error) {
	if c.Home == "" {
		return "", ErrNoCodexAuth
	}
	p := filepath.Join(c.Home, codexAuthFileName)
	if isAnthropicPath(p) {
		return "", ErrAnthropicCredentials
	}
	return p, nil
}

// readAuthRaw reads auth.json as raw JSON fields so a refresh can write it
// back without dropping fields kit doesn't know.
func (c *CodexSub) readAuthRaw() (map[string]json.RawMessage, error) {
	p, err := c.authPath()
	if err != nil {
		return nil, err
	}
	if onAuthRead != nil {
		onAuthRead(p)
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, ErrNoCodexAuth
	}
	defer f.Close()
	var raw map[string]json.RawMessage
	if json.NewDecoder(io.LimitReader(f, maxCodexAuthBytes)).Decode(&raw) != nil {
		return nil, errors.New("codex-sub: the Codex auth file is not valid JSON (run `codex login`)")
	}
	// A Claude credentials blob is never used, whatever the file is called.
	if _, ok := raw["claudeAiOauth"]; ok {
		return nil, ErrAnthropicCredentials
	}
	return raw, nil
}

type codexTokens struct {
	IDToken      string `json:"id_token"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	AccountID    string `json:"account_id"`
}

// loadAuth reads the ChatGPT login. An API-key-only file is not a
// subscription login and reports ErrNoCodexAuth.
func (c *CodexSub) loadAuth() (codexAuth, error) {
	raw, err := c.readAuthRaw()
	if err != nil {
		return codexAuth{}, err
	}
	var tok codexTokens
	if t, ok := raw["tokens"]; !ok || json.Unmarshal(t, &tok) != nil {
		return codexAuth{}, ErrNoCodexAuth
	}
	a := codexAuth{AccessToken: tok.AccessToken, RefreshToken: tok.RefreshToken, IDToken: tok.IDToken, AccountID: tok.AccountID}
	if !a.usable() {
		return codexAuth{}, ErrNoCodexAuth
	}
	a.Expires = jwtExpiry(a.AccessToken)
	if a.AccountID == "" {
		a.AccountID = jwtAccountID(a.IDToken)
	}
	if a.AccountID == "" {
		a.AccountID = jwtAccountID(a.AccessToken)
	}
	c.remember(a.AccessToken, a.RefreshToken, a.IDToken, a.AccountID)
	return a, nil
}

func (c *CodexSub) expired(a codexAuth) bool {
	if a.AccessToken == "" {
		return true
	}
	return !a.Expires.IsZero() && !c.now().Add(codexRefreshSkew).Before(a.Expires)
}

// Available reports whether a ChatGPT login is usable now: a live access
// token, or a refresh token to get one.
func (c *CodexSub) Available() bool {
	a, err := c.loadAuth()
	if err != nil {
		return false
	}
	return !c.expired(a) || a.RefreshToken != ""
}

// Active reports whether the provider should be offered: [foh.codex].enabled
// when set, otherwise whether a login is Available.
func (c *CodexSub) Active() bool {
	if c.Enabled != nil {
		return *c.Enabled
	}
	return c.Available()
}

// Models lists codex/NAME for each configured model when the provider is
// Active, and nothing otherwise.
func (c *CodexSub) Models(context.Context, bool) ([]Model, CatalogSource, error) {
	if !c.Active() {
		return nil, SourceConfig, nil
	}
	names := c.ModelNames
	if len(names) == 0 {
		names = DefaultCodexModels
	}
	out := make([]Model, 0, len(names))
	for _, n := range names {
		out = append(out, Model{ID: CodexModelPrefix + n, Provider: ProviderCodexSub,
			Name: "Codex (ChatGPT subscription): " + n, PromptPrice: -1, CompletionPrice: -1})
	}
	return out, SourceConfig, nil
}

func (c *CodexSub) remember(vals ...string) {
	c.mu.Lock()
	var added []string
	for _, v := range vals {
		if len(v) < 4 {
			continue
		}
		dup := false
		for _, s := range c.secrets {
			if s == v {
				dup = true
				break
			}
		}
		if !dup {
			c.secrets = append(c.secrets, v)
			added = append(added, v)
		}
	}
	if len(c.secrets) > maxCodexSecrets {
		c.secrets = c.secrets[len(c.secrets)-maxCodexSecrets:]
	}
	cb := c.OnSecret
	c.mu.Unlock()
	if cb != nil {
		for _, v := range added {
			cb(v)
		}
	}
}

// Secrets lists every credential value loaded so far.
func (c *CodexSub) Secrets() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.secrets...)
}

func (c *CodexSub) scrub(err error) error { return scrubErr(err, c.Secrets()...) }

// auth returns a usable login, refreshing it once when it has expired (or
// when force is set after a 401).
func (c *CodexSub) auth(ctx context.Context, force bool) (codexAuth, error) {
	a, err := c.loadAuth()
	if err != nil {
		return codexAuth{}, err
	}
	if !force && !c.expired(a) {
		return a, nil
	}
	if a.RefreshToken == "" {
		return codexAuth{}, ErrCodexAuthExpired
	}
	return c.refresh(ctx, a)
}

// refresh exchanges the refresh token for new tokens, bounded by a timeout
// and a backoff after failure, and writes them back to auth.json so the
// Codex CLI keeps working with the rotated refresh token.
func (c *CodexSub) refresh(ctx context.Context, a codexAuth) (codexAuth, error) {
	c.mu.Lock()
	if c.refreshErr != nil && c.now().Sub(c.lastRefresh) < codexRefreshBackoff {
		err := c.refreshErr
		c.mu.Unlock()
		return codexAuth{}, err
	}
	c.lastRefresh = c.now()
	c.mu.Unlock()
	na, err := c.doRefresh(ctx, a)
	c.mu.Lock()
	c.refreshErr = err
	c.mu.Unlock()
	return na, err
}

func (c *CodexSub) doRefresh(ctx context.Context, a codexAuth) (codexAuth, error) {
	ctx, cancel := context.WithTimeout(ctx, codexRefreshTimeout)
	defer cancel()
	tokenURL := c.TokenURL
	if tokenURL == "" {
		tokenURL = DefaultCodexTokenURL
	}
	body, _ := json.Marshal(map[string]string{
		"client_id": codexClientID, "grant_type": "refresh_token",
		"refresh_token": a.RefreshToken, "scope": "openid profile email",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, bytes.NewReader(body))
	if err != nil {
		return codexAuth{}, c.scrub(fmt.Errorf("codex-sub: refresh: %w", err))
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return codexAuth{}, c.scrub(fmt.Errorf("codex-sub: refresh: %w", err))
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxErrorBody))
		return codexAuth{}, fmt.Errorf("codex-sub: token refresh failed (%s); run `codex login`", resp.Status)
	}
	var tok codexTokens
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxCodexAuthBytes)).Decode(&tok); err != nil || tok.AccessToken == "" {
		return codexAuth{}, errors.New("codex-sub: token refresh returned no access token; run `codex login`")
	}
	c.remember(tok.AccessToken, tok.RefreshToken, tok.IDToken)
	na := a
	na.AccessToken = tok.AccessToken
	if tok.RefreshToken != "" {
		na.RefreshToken = tok.RefreshToken
	}
	if tok.IDToken != "" {
		na.IDToken = tok.IDToken
		if id := jwtAccountID(tok.IDToken); id != "" {
			na.AccountID = id
		}
	}
	na.Expires = jwtExpiry(na.AccessToken)
	// A failed write-back still leaves a working token for this session.
	_ = c.saveAuth(na)
	return na, nil
}

// saveAuth writes refreshed tokens into auth.json, keeping every other field.
func (c *CodexSub) saveAuth(a codexAuth) error {
	raw, err := c.readAuthRaw()
	if err != nil {
		return err
	}
	var tokens map[string]json.RawMessage
	if t, ok := raw["tokens"]; !ok || json.Unmarshal(t, &tokens) != nil || tokens == nil {
		tokens = map[string]json.RawMessage{}
	}
	set := func(k, v string) {
		if v != "" {
			b, _ := json.Marshal(v)
			tokens[k] = b
		}
	}
	set("access_token", a.AccessToken)
	set("refresh_token", a.RefreshToken)
	set("id_token", a.IDToken)
	set("account_id", a.AccountID)
	tb, err := json.Marshal(tokens)
	if err != nil {
		return err
	}
	raw["tokens"] = tb
	lr, _ := json.Marshal(c.now().UTC().Format(time.RFC3339Nano))
	raw["last_refresh"] = lr
	p, err := c.authPath()
	if err != nil {
		return err
	}
	return writeFileAtomic(p, raw)
}

// jwtClaims decodes a JWT payload without verifying it; the server verifies.
func jwtClaims(tok string) map[string]json.RawMessage {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return nil
	}
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return nil
	}
	var claims map[string]json.RawMessage
	if json.Unmarshal(b, &claims) != nil {
		return nil
	}
	return claims
}

func jwtExpiry(tok string) time.Time {
	var exp float64
	if c := jwtClaims(tok); c != nil && json.Unmarshal(c["exp"], &exp) == nil && exp > 0 {
		return time.Unix(int64(exp), 0)
	}
	return time.Time{}
}

func jwtAccountID(tok string) string {
	c := jwtClaims(tok)
	if c == nil {
		return ""
	}
	var auth struct {
		AccountID string `json:"chatgpt_account_id"`
	}
	if json.Unmarshal(c["https://api.openai.com/auth"], &auth) == nil {
		return auth.AccountID
	}
	return ""
}

// codexUpstream maps codex/NAME to NAME and refuses Anthropic models, which
// never route through a ChatGPT subscription.
func codexUpstream(model string) (string, error) {
	name := strings.TrimPrefix(strings.TrimSpace(model), CodexModelPrefix)
	if name == "" {
		return "", errors.New("codex-sub: empty model name")
	}
	low := strings.ToLower(name)
	if strings.Contains(low, "claude") || strings.Contains(low, "anthropic") {
		return "", fmt.Errorf("codex-sub: %s is not a Codex model; Anthropic models route through OpenRouter", name)
	}
	return name, nil
}

// responsesInput converts the chat thread to Responses API input items and
// instructions. The system prompt becomes instructions.
func responsesInput(msgs []Message) (string, []map[string]any) {
	var instructions []string
	var items []map[string]any
	for _, m := range msgs {
		switch m.Role {
		case "system":
			instructions = append(instructions, m.Content)
		case "user":
			content := make([]map[string]any, 0, len(m.Images)+1)
			if m.Content != "" || len(m.Images) == 0 {
				content = append(content, map[string]any{"type": "input_text", "text": m.Content})
			}
			for _, img := range m.Images {
				content = append(content, map[string]any{"type": "input_image", "image_url": img.DataURL()})
			}
			items = append(items, map[string]any{"type": "message", "role": "user", "content": content})
		case "assistant":
			if m.Content != "" {
				items = append(items, map[string]any{"type": "message", "role": "assistant",
					"content": []map[string]any{{"type": "output_text", "text": m.Content}}})
			}
			for _, tc := range m.ToolCalls {
				args := tc.Function.Arguments
				if args == "" {
					args = "{}"
				}
				items = append(items, map[string]any{"type": "function_call", "call_id": tc.ID,
					"name": tc.Function.Name, "arguments": args})
			}
		case "tool":
			items = append(items, map[string]any{"type": "function_call_output", "call_id": m.ToolCallID, "output": m.Content})
		}
	}
	return strings.Join(instructions, "\n\n"), items
}

func responsesTools(specs []ToolSpec) []map[string]any {
	out := make([]map[string]any, 0, len(specs))
	for _, s := range specs {
		params := s.Function.Parameters
		if len(params) == 0 {
			params = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		out = append(out, map[string]any{"type": "function", "name": s.Function.Name,
			"description": s.Function.Description, "parameters": params, "strict": false})
	}
	return out
}

type responsesEvent struct {
	Type        string `json:"type"`
	Delta       string `json:"delta"`
	ItemID      string `json:"item_id"`
	OutputIndex int    `json:"output_index"`
	Item        *struct {
		Type      string `json:"type"`
		ID        string `json:"id"`
		CallID    string `json:"call_id"`
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
		Content   []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"item"`
	Response *struct {
		Model string `json:"model"`
		Usage *struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		IncompleteDetails *struct {
			Reason string `json:"reason"`
		} `json:"incomplete_details"`
	} `json:"response"`
	Message string `json:"message"`
	Error   *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Stream sends req to the Codex Responses backend and assembles the SSE
// reply. An expired login is refreshed first; a 401 refreshes once and
// retries once.
func (c *CodexSub) Stream(ctx context.Context, req ChatRequest, onDelta func(Delta)) (ChatResult, error) {
	upstream, err := codexUpstream(req.Model)
	if err != nil {
		return ChatResult{}, err
	}
	if c.Enabled != nil && !*c.Enabled {
		return ChatResult{}, errors.New("codex-sub: disabled by [foh.codex].enabled = false")
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()
	a, err := c.auth(ctx, false)
	if err != nil {
		return ChatResult{}, c.scrub(err)
	}
	instructions, input := responsesInput(req.Messages)
	payload := map[string]any{
		"model": upstream, "instructions": instructions, "input": input,
		"tool_choice": "auto", "parallel_tool_calls": true,
		"store": false, "stream": true,
	}
	if len(req.Tools) > 0 {
		payload["tools"] = responsesTools(req.Tools)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return ChatResult{}, err
	}
	resp, err := c.post(ctx, a, body)
	if err != nil {
		return ChatResult{}, err
	}
	if resp.StatusCode == http.StatusUnauthorized && a.RefreshToken != "" {
		resp.Body.Close()
		if a, err = c.auth(ctx, true); err != nil {
			return ChatResult{}, c.scrub(err)
		}
		if resp, err = c.post(ctx, a, body); err != nil {
			return ChatResult{}, err
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return ChatResult{}, c.statusError(resp)
	}
	return c.readStream(ctx, resp.Body, req.Model, onDelta)
}

func (c *CodexSub) post(ctx context.Context, a codexAuth, body []byte) (*http.Response, error) {
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/responses", bytes.NewReader(body))
	if err != nil {
		return nil, c.scrub(fmt.Errorf("codex-sub: %w", err))
	}
	hreq.Header.Set("Authorization", "Bearer "+a.AccessToken)
	if a.AccountID != "" {
		hreq.Header.Set("ChatGPT-Account-Id", a.AccountID)
	}
	hreq.Header.Set("Content-Type", "application/json")
	hreq.Header.Set("Accept", "text/event-stream")
	hreq.Header.Set("OpenAI-Beta", "responses=experimental")
	hreq.Header.Set("originator", "hev_foh")
	resp, err := c.httpClient().Do(hreq)
	if err != nil {
		return nil, c.scrub(fmt.Errorf("codex-sub: %w", err))
	}
	return resp, nil
}

func (c *CodexSub) statusError(resp *http.Response) error {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
	msg := strings.TrimSpace(string(b))
	var e struct {
		Detail any `json:"detail"`
		Error  struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(b, &e) == nil {
		if e.Error.Message != "" {
			msg = e.Error.Message
		} else if s, ok := e.Detail.(string); ok && s != "" {
			msg = s
		}
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		msg += " (run `codex login`)"
	}
	return c.scrub(fmt.Errorf("codex-sub: %s: %s", resp.Status, msg))
}

func (c *CodexSub) readStream(ctx context.Context, body io.Reader, model string, onDelta func(Delta)) (ChatResult, error) {
	res := ChatResult{Model: model}
	var text, doneText strings.Builder
	calls := map[int]*ToolCall{}
	byItem := map[string]int{}
	sc := bufio.NewScanner(io.LimitReader(body, maxResponseBytes))
	sc.Buffer(make([]byte, 64<<10), maxSSELine)
	callFor := func(idx int) *ToolCall {
		call := calls[idx]
		if call == nil {
			call = &ToolCall{Type: "function"}
			calls[idx] = call
		}
		return call
	}
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var ev responsesEvent
		if json.Unmarshal([]byte(data), &ev) != nil {
			continue
		}
		switch ev.Type {
		case "response.output_text.delta":
			if ev.Delta != "" {
				text.WriteString(ev.Delta)
				if onDelta != nil {
					onDelta(Delta{Text: ev.Delta})
				}
			}
		case "response.output_item.added":
			if ev.Item != nil && ev.Item.Type == "function_call" {
				call := callFor(ev.OutputIndex)
				call.ID, call.Function.Name = ev.Item.CallID, ev.Item.Name
				call.Function.Arguments += ev.Item.Arguments
				if ev.Item.ID != "" {
					byItem[ev.Item.ID] = ev.OutputIndex
				}
				if onDelta != nil && call.Function.Name != "" {
					onDelta(Delta{ToolName: call.Function.Name})
				}
			}
		case "response.function_call_arguments.delta":
			idx := ev.OutputIndex
			if i, ok := byItem[ev.ItemID]; ok {
				idx = i
			}
			callFor(idx).Function.Arguments += ev.Delta
		case "response.output_item.done":
			if ev.Item == nil {
				break
			}
			switch ev.Item.Type {
			case "function_call":
				call := callFor(ev.OutputIndex)
				if ev.Item.CallID != "" {
					call.ID = ev.Item.CallID
				}
				if ev.Item.Name != "" {
					call.Function.Name = ev.Item.Name
				}
				if ev.Item.Arguments != "" {
					call.Function.Arguments = ev.Item.Arguments
				}
			case "message":
				for _, part := range ev.Item.Content {
					if part.Type == "output_text" {
						doneText.WriteString(part.Text)
					}
				}
			}
		case "response.completed", "response.incomplete":
			if r := ev.Response; r != nil {
				if r.Usage != nil {
					res.Usage = Usage{PromptTokens: r.Usage.InputTokens, CompletionTokens: r.Usage.OutputTokens, TotalTokens: r.Usage.TotalTokens}
				}
				if r.IncompleteDetails != nil && r.IncompleteDetails.Reason != "" {
					res.FinishReason = r.IncompleteDetails.Reason
				}
			}
		case "response.failed", "error", "response.error":
			msg := ev.Message
			if ev.Error != nil && ev.Error.Message != "" {
				msg = ev.Error.Message
			}
			if ev.Response != nil && ev.Response.Error != nil && ev.Response.Error.Message != "" {
				msg = ev.Response.Error.Message
			}
			if msg == "" {
				msg = "the response failed"
			}
			return res, c.scrub(fmt.Errorf("codex-sub: %s", msg))
		}
	}
	if err := sc.Err(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return res, c.scrub(fmt.Errorf("codex-sub: stream: %w", ctxErr))
		}
		return res, c.scrub(fmt.Errorf("codex-sub: stream: %w", err))
	}
	if err := ctx.Err(); err != nil {
		return res, c.scrub(fmt.Errorf("codex-sub: stream: %w", err))
	}
	res.Content = text.String()
	if res.Content == "" && doneText.Len() > 0 {
		res.Content = doneText.String()
		if onDelta != nil {
			onDelta(Delta{Text: res.Content})
		}
	}
	idx := make([]int, 0, len(calls))
	for i := range calls {
		idx = append(idx, i)
	}
	sort.Ints(idx)
	for n, i := range idx {
		call := *calls[i]
		if call.Function.Name == "" {
			continue
		}
		if call.ID == "" {
			call.ID = fmt.Sprintf("call_%d", n)
		}
		if call.Function.Arguments == "" {
			call.Function.Arguments = "{}"
		}
		res.ToolCalls = append(res.ToolCalls, call)
	}
	if res.FinishReason == "" {
		res.FinishReason = "stop"
		if len(res.ToolCalls) > 0 {
			res.FinishReason = "tool_calls"
		}
	}
	return res, nil
}
