package subscriptions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ClaudeUsageURL is the Claude Code OAuth usage endpoint. It is undocumented
// and rate-limits hard, so a read makes at most one request and the last good
// answer is cached.
const ClaudeUsageURL = "https://api.anthropic.com/api/oauth/usage"

// DefaultMinInterval is how long a cached Claude reading is served before
// the endpoint is asked again. Ten minutes keeps a watch loop or several
// agents running `hev subscriptions` to six requests an hour per machine,
// while a weekly meter rarely moves more than a point or two in that time.
const DefaultMinInterval = 10 * time.Minute

const claudeOAuthBeta = "oauth-2025-04-20"

// claudeCredential is what a read takes from the Claude Code login. The
// token stays in memory for one request.
type claudeCredential struct {
	token     string
	plan      string
	tier      string
	expiresAt time.Time
	source    string
}

func claudeConfigDir(env Env) string {
	if d := env.getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d
	}
	return filepath.Join(env.Home, ".claude")
}

// claudeLogin finds the Claude Code OAuth login: CLAUDE_CODE_OAUTH_TOKEN,
// else the keychain item, else ~/.claude/.credentials.json. ok is false when
// there is none; err reports one that exists but cannot be read.
func claudeLogin(ctx context.Context, env Env) (c claudeCredential, ok bool, err error) {
	if t := strings.TrimSpace(env.getenv("CLAUDE_CODE_OAUTH_TOKEN")); t != "" {
		return claudeCredential{token: t, source: "CLAUDE_CODE_OAUTH_TOKEN"}, true, nil
	}
	if env.Keychain != nil {
		if b, kerr := env.Keychain(ctx); kerr == nil && len(b) > 0 {
			c, err = parseClaudeCredential(b)
			c.source = "keychain"
			return c, err == nil, err
		}
	}
	path := filepath.Join(claudeConfigDir(env), ".credentials.json")
	b, ferr := os.ReadFile(path)
	if errors.Is(ferr, os.ErrNotExist) {
		return c, false, nil
	}
	if ferr != nil {
		return c, false, fmt.Errorf("credentials file unreadable")
	}
	c, err = parseClaudeCredential(b)
	c.source = "credentials file"
	return c, err == nil, err
}

func parseClaudeCredential(b []byte) (claudeCredential, error) {
	var raw struct {
		OAuth *struct {
			AccessToken      string `json:"accessToken"`
			ExpiresAt        int64  `json:"expiresAt"`
			SubscriptionType string `json:"subscriptionType"`
			RateLimitTier    string `json:"rateLimitTier"`
		} `json:"claudeAiOauth"`
	}
	// The decode error could quote credential bytes; never pass it on.
	if json.Unmarshal(b, &raw) != nil || raw.OAuth == nil || raw.OAuth.AccessToken == "" {
		return claudeCredential{}, fmt.Errorf("Claude Code credential has no OAuth login")
	}
	c := claudeCredential{token: raw.OAuth.AccessToken, plan: raw.OAuth.SubscriptionType, tier: raw.OAuth.RateLimitTier}
	if raw.OAuth.ExpiresAt > 0 {
		c.expiresAt = time.UnixMilli(raw.OAuth.ExpiresAt)
	}
	return c, nil
}

// readClaude reports the Claude Code login and its usage meters.
func readClaude(ctx context.Context, env Env) Status {
	s := Status{Source: "Claude OAuth usage"}
	now := env.now()
	cache, _ := loadClaudeCache(env)
	login, ok, err := claudeLogin(ctx, env)
	switch {
	case err != nil:
		s.State, s.StateReason = Unknown, err.Error()
		return fromCache(s, cache, "login unreadable")
	case !ok:
		s.State, s.StateReason = NotSignedIn, "no Claude Code login (CLAUDE_CODE_OAUTH_TOKEN, keychain or credentials file)"
		return s
	}
	plan := planName(login.plan, login.tier)
	if plan != "" {
		s.Plan = ptr(plan)
	} else if cache != nil && cache.Plan != "" {
		s.Plan = ptr(cache.Plan)
	} else if login.source == "CLAUDE_CODE_OAUTH_TOKEN" {
		s.PlanReason = "a token from CLAUDE_CODE_OAUTH_TOKEN does not carry the plan tier"
	}
	if !login.expiresAt.IsZero() && !login.expiresAt.After(now) {
		s.State = LoginExpired
		s.StateReason = "the access token expired at " + login.expiresAt.UTC().Format(time.RFC3339) + "; running Claude Code refreshes it"
		return fromCache(s, cache, "login expired")
	}
	// Serve the cache inside the minimum interval, after a success or a failure.
	if cache != nil && !env.Refresh {
		last := cache.LastAttempt()
		if !last.IsZero() && now.Sub(last) < env.minInterval() && now.Sub(last) >= 0 {
			s.State = SignedIn
			if cache.LastError != "" {
				if cache.LastStatus == http.StatusUnauthorized {
					s.State, s.StateReason = LoginExpired, "the usage endpoint answered 401 at "+last.UTC().Format(time.RFC3339)
				}
				return fromCache(s, cache, cache.LastError+"; next request after "+last.Add(env.minInterval()).UTC().Format(time.RFC3339))
			}
			s = fromCache(s, cache, "")
			s.Source = "Claude OAuth usage (cached)"
			return s
		}
	}
	code, body, err := fetchClaudeUsage(ctx, env, login.token)
	login.token = ""
	attempt := now.UTC()
	var reason string
	switch {
	case err != nil:
		reason = "usage request failed: " + err.Error()
	case code == http.StatusOK:
		meters, perr := parseClaudeUsage(body)
		if perr != nil {
			reason = "usage response unreadable: " + perr.Error()
			break
		}
		c := &claudeCache{Schema: claudeCacheSchema, ObservedAt: attempt, Plan: plan, Meters: meters, LastAttemptAt: attempt}
		if c.Plan == "" && cache != nil {
			c.Plan = cache.Plan
		}
		_ = saveClaudeCache(env, c)
		s.State = SignedIn
		return fromCache(s, c, "")
	case code == http.StatusUnauthorized:
		s.State, s.StateReason = LoginExpired, "the usage endpoint answered 401; sign in to Claude Code again"
		reason = "login expired (401)"
	case code == http.StatusTooManyRequests:
		reason = "rate limited (429)"
	default:
		reason = fmt.Sprintf("usage endpoint answered %d", code)
	}
	if s.State == "" {
		// The local login is present and unexpired; only the reading failed.
		s.State = SignedIn
	}
	// Record the failed attempt so the next read inside the interval does not
	// ask again, and keep the last good reading.
	failed := &claudeCache{Schema: claudeCacheSchema, Plan: plan, LastAttemptAt: attempt, LastError: reason, LastStatus: code}
	if cache != nil {
		failed.ObservedAt, failed.Meters = cache.ObservedAt, cache.Meters
		if failed.Plan == "" {
			failed.Plan = cache.Plan
		}
	}
	_ = saveClaudeCache(env, failed)
	return fromCache(s, failed, reason)
}

// fromCache fills a status from a cached reading. A non-empty staleReason
// marks it stale with that reason.
func fromCache(s Status, c *claudeCache, staleReason string) Status {
	if staleReason != "" {
		s.Stale, s.StaleReason = true, staleReason
	}
	if c == nil || c.ObservedAt.IsZero() {
		if staleReason != "" {
			s.ObservedReason = "no cached reading; " + staleReason
		}
		return s
	}
	at := c.ObservedAt
	s.ObservedAt = &at
	s.Meters = append([]Meter{}, c.Meters...)
	if s.Plan == nil && c.Plan != "" {
		s.Plan = ptr(c.Plan)
	}
	if staleReason != "" {
		s.Source = "Claude OAuth usage (cached)"
	}
	return s
}

func planName(sub, tier string) string {
	sub = strings.TrimSpace(sub)
	// rateLimitTier looks like default_claude_max_20x; keep the multiplier.
	if i := strings.LastIndex(tier, "_"); i >= 0 && sub != "" {
		if m := tier[i+1:]; strings.HasSuffix(m, "x") && len(m) > 1 && m[0] >= '0' && m[0] <= '9' {
			return sub + " " + m
		}
	}
	return sub
}

func fetchClaudeUsage(ctx context.Context, env Env, token string) (int, []byte, error) {
	u := env.ClaudeUsageURL
	if u == "" {
		u = ClaudeUsageURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, nil, fmt.Errorf("invalid usage URL")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-beta", claudeOAuthBeta)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "hev-kit")
	client := env.HTTP
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		// url.Error carries the URL, never the header.
		var ue *url.Error
		if errors.As(err, &ue) {
			return 0, nil, ue.Err
		}
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("reading the response: %w", err)
	}
	return resp.StatusCode, body, nil
}

// parseClaudeUsage reads the usage body. It accepts the limits[] list
// (kind, percent, resets_at, scope.model.display_name) and the keyed form
// (five_hour, seven_day, seven_day_<model> with utilization and resets_at),
// plus extra_usage in either.
func parseClaudeUsage(body []byte) ([]Meter, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("not a JSON object")
	}
	meters := []Meter{}
	if list, ok := raw["limits"]; ok {
		var limits []struct {
			Kind     string   `json:"kind"`
			Percent  *float64 `json:"percent"`
			ResetsAt *string  `json:"resets_at"`
			Scope    *struct {
				Model *struct {
					DisplayName string `json:"display_name"`
				} `json:"model"`
			} `json:"scope"`
		}
		if err := json.Unmarshal(list, &limits); err != nil {
			return nil, fmt.Errorf("limits is not a list")
		}
		for _, l := range limits {
			m := Meter{}
			switch l.Kind {
			case "session":
				m.Kind, m.Label, m.WindowMinutes = KindSession, "session (5h)", 300
			case "weekly_all":
				m.Kind, m.Label, m.WindowMinutes = KindWeekly, "weekly", 10080
			case "weekly_scoped":
				m.Kind, m.WindowMinutes = KindWeeklyScoped, 10080
				if l.Scope != nil && l.Scope.Model != nil {
					m.Scope = l.Scope.Model.DisplayName
				}
				m.Label = "weekly (" + nonEmpty(m.Scope, "scoped") + ")"
			default:
				m.Kind, m.Label = KindWindow, nonEmpty(l.Kind, "unnamed limit")
			}
			setPercent(&m, l.Percent)
			setReset(&m, l.ResetsAt)
			meters = append(meters, m)
		}
	} else {
		keys := []string{}
		for k := range raw {
			if k == "five_hour" || strings.HasPrefix(k, "seven_day") {
				keys = append(keys, k)
			}
		}
		sort.Slice(keys, func(i, j int) bool {
			return claudeKeyOrder(keys[i]) < claudeKeyOrder(keys[j]) || (claudeKeyOrder(keys[i]) == claudeKeyOrder(keys[j]) && keys[i] < keys[j])
		})
		for _, k := range keys {
			var w *struct {
				Utilization *float64 `json:"utilization"`
				ResetsAt    *string  `json:"resets_at"`
			}
			if json.Unmarshal(raw[k], &w) != nil || w == nil {
				continue
			}
			m := Meter{WindowMinutes: 10080}
			switch {
			case k == "five_hour":
				m.Kind, m.Label, m.WindowMinutes = KindSession, "session (5h)", 300
			case k == "seven_day":
				m.Kind, m.Label = KindWeekly, "weekly"
			default:
				scope := strings.TrimPrefix(k, "seven_day_")
				if len(scope) > 0 {
					scope = strings.ToUpper(scope[:1]) + scope[1:]
				}
				m.Kind, m.Scope, m.Label = KindWeeklyScoped, scope, "weekly ("+scope+")"
			}
			setPercent(&m, w.Utilization)
			setReset(&m, w.ResetsAt)
			meters = append(meters, m)
		}
	}
	if x, ok := raw["extra_usage"]; ok {
		if m, ok := parseExtraUsage(x); ok {
			meters = append(meters, m)
		}
	}
	if len(meters) == 0 {
		return nil, fmt.Errorf("no usage limits in the response")
	}
	return meters, nil
}

func claudeKeyOrder(k string) int {
	switch k {
	case "five_hour":
		return 0
	case "seven_day":
		return 1
	}
	return 2
}

func parseExtraUsage(b json.RawMessage) (Meter, bool) {
	var x *struct {
		Enabled     *bool    `json:"is_enabled"`
		Percent     *float64 `json:"percent"`
		Utilization *float64 `json:"utilization"`
		ResetsAt    *string  `json:"resets_at"`
	}
	if json.Unmarshal(b, &x) != nil || x == nil {
		return Meter{}, false
	}
	m := Meter{Kind: KindExtraUsage, Label: "extra usage"}
	if x.Enabled != nil && !*x.Enabled {
		m.Detail = "off"
		m.PercentReason = "extra usage is off"
		m.ResetReason = "extra usage is off"
		return m, true
	}
	if x.Enabled != nil {
		m.Detail = "on"
	}
	p := x.Percent
	if p == nil {
		p = x.Utilization
	}
	setPercent(&m, p)
	setReset(&m, x.ResetsAt)
	return m, true
}

func setPercent(m *Meter, p *float64) {
	switch {
	case p == nil:
		m.PercentReason = "the provider reported no percentage"
	case *p < 0 || *p > 100 || *p != *p:
		m.PercentReason = fmt.Sprintf("reported %v%% is out of range", *p)
	default:
		m.Percent = ptr(*p)
	}
}

func setReset(m *Meter, s *string) {
	if s == nil || *s == "" {
		m.ResetReason = "the provider reported no reset time"
		return
	}
	t, err := time.Parse(time.RFC3339Nano, *s)
	if err != nil {
		m.ResetReason = "reset time unreadable"
		return
	}
	m.ResetsAt = &t
}

func nonEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
