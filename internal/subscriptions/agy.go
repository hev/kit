package subscriptions

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/hev/kit/internal/platformops"
	_ "modernc.org/sqlite"
)

// DefaultAgyProbeTimeout bounds `agy models` subprocess probes so a hung
// helper or slow network never stalls `hev subscriptions`.
const DefaultAgyProbeTimeout = 5 * time.Second

// Antigravity local read bounds.
const (
	agyActivityTimeout         = 3 * time.Second
	agyMaxConfigBytes          = 1 << 20
	agyMaxTokenBytes           = 64 << 10
	agyMaxProbeBytes           = 256 << 10
	agyMaxTranscriptFiles      = 64
	agyMaxTranscriptBytes      = 8 << 20
	agyMaxTotalTranscriptBytes = 32 << 20
	agyMaxLineBytes            = 2 << 20
	agySessionMinutes          = 300
	agyWeeklyMinutes           = 10080
)

const (
	agyNoQuotaPercentReason = "Antigravity does not expose quota percentages outside its interactive TUI"
	agyNoQuotaResetReason   = "Antigravity does not expose window reset times outside its interactive TUI"
)

var (
	agyModelIDRe    = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,80}$`)
	agyPlanSafeRe   = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,31}$`)
	agyANSIEscapeRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

	// Secret, credential, identity and local-path patterns scrubbed from every
	// status, cache and error string as defense-in-depth.
	agySecretTokenRe = regexp.MustCompile(`(?i)(?:ya29\.[A-Za-z0-9._-]+|1//[A-Za-z0-9._-]+|AIza[A-Za-z0-9_-]{8,}|eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+|Bearer\s+\S+|sk-[A-Za-z0-9_-]{8,}|(?:op|azkv|secret)://\S+)`)
	agyJSONSecretRe  = regexp.MustCompile(`(?i)"(?:access_token|accessToken|refresh_token|refreshToken|id_token|idToken|api_key|apiKey|GEMINI_API_KEY)"\s*:\s*"[^"]*"`)
	agyEmailRe       = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
	agyUnixPathRe    = regexp.MustCompile(`(?:/Users/|/home/|/private/var/|/private/tmp/|/var/folders/|/tmp/|/etc/|/opt/)[^\s"':;)\]]+`)
	agyWinPathRe     = regexp.MustCompile(`(?:[A-Za-z]:\\|\\\\[A-Za-z0-9._-]+\\)[^\s"':;)\]]+`)
	agyDotGeminiRe   = regexp.MustCompile(`[^\s"':;)\]]*(?:\.gemini[/\\]antigravity-cli|antigravity-oauth-token|jetski-standalone-oauth-token)[^\s"':;)\]]*`)
)

type limitedWriter struct {
	w   *bytes.Buffer
	max int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	orig := len(p)
	if rem := l.max - l.w.Len(); rem > 0 {
		if len(p) > rem {
			p = p[:rem]
		}
		_, _ = l.w.Write(p)
	}
	return orig, nil
}

func agyDataDir(env Env) string {
	if d := strings.TrimSpace(agyGetenv(env, "HEV_AGY_DATA_DIR")); d != "" {
		return d
	}
	return filepath.Join(env.Home, ".gemini", "antigravity-cli")
}

func agyDispatchConfigPath(env Env) string {
	if strings.TrimSpace(env.DispatchConfig) != "" {
		return env.DispatchConfig
	}
	if p := strings.TrimSpace(agyGetenv(env, "HEV_DISPATCH_CONFIG")); p != "" {
		return p
	}
	return filepath.Join(env.Home, ".config", "hev", "dispatch.toml")
}

func agyGetenv(env Env, key string) string {
	if v := env.getenv(key); v != "" {
		return v
	}
	if lower := strings.ToLower(key); lower != key {
		if v := env.getenv(lower); v != "" {
			return v
		}
	}
	return ""
}

// readAgy reports Antigravity CLI (agy) sign-in mode, plan tier and bounded
// local host activity without exposing credentials or fabricating percentages.
func readAgy(ctx context.Context, env Env) Status {
	now := env.now()
	dataDir := agyDataDir(env)
	dispatchCfgPath := agyDispatchConfigPath(env)
	var redactPaths []string
	for _, p := range []string{dataDir, dispatchCfgPath, env.Home, env.StateDir} {
		if strings.TrimSpace(p) != "" {
			redactPaths = append(redactPaths, p)
		}
	}

	dispatchInfo := readAgyDispatchConfig(dispatchCfgPath)

	// 1. Explicit Gemini API-key mode overrides OAuth subscription sign-in.
	if apiWhy, ok := agyAPIKeyMode(env, dataDir, dispatchInfo); ok {
		s := Status{
			Plan:         ptr("api-key"),
			State:        NotSignedIn,
			StateReason:  "Antigravity is configured for Gemini API-key billing (" + apiWhy + "), not an OAuth subscription",
			Source:       "Antigravity Gemini API key",
			WeeklyReason: "API-key billing has no subscription weekly window",
		}
		act := readAgyActivity(ctx, dataDir, now)
		if act.hasAny {
			s.Source = "Antigravity Gemini API key + local transcripts"
			at := act.newestAt
			s.ObservedAt = &at
			s.Activity = act.toActivity()
			s.Meters = []Meter{{
				Kind:          KindWindow,
				Label:         "local activity (7d)",
				WindowMinutes: agyWeeklyMinutes,
				PercentReason: "API-key billing is pay-as-you-go, not a subscription window",
				ResetReason:   "API-key billing has no subscription reset",
				Detail:        act.weeklySummary(),
			}}
		} else {
			s.ObservedReason = "API-key billing has no subscription usage window"
		}
		return redactAgyStatus(s, redactPaths...)
	}

	configuredTier := agyConfiguredTier(env, dispatchInfo)
	oauth := readAgyOAuthFile(dataDir, now)
	act := readAgyActivity(ctx, dataDir, now)

	if oauth.corrupt {
		s := Status{
			State:       Unknown,
			StateReason: "Antigravity OAuth credential file is unreadable",
			Source:      "Antigravity local state",
		}
		if configuredTier != "" {
			s.Plan = ptr(configuredTier)
		}
		if act.hasAny {
			at := act.newestAt
			s.ObservedAt = &at
			s.Activity = act.toActivity()
		}
		return redactAgyStatus(s, redactPaths...)
	}

	if oauth.expired {
		s := Status{
			State:       LoginExpired,
			StateReason: "the access token expired at " + oauth.expiredAt.UTC().Format(time.RFC3339) + " and has no refresh token; run agy to sign in again",
			Source:      "Antigravity OAuth",
		}
		if configuredTier != "" {
			s.Plan = ptr(configuredTier)
		}
		if act.hasAny {
			at := act.newestAt
			s.ObservedAt = &at
			s.Activity = act.toActivity()
		}
		return redactAgyStatus(s, redactPaths...)
	}

	keychainOK := false
	if !oauth.valid && env.AgyKeychain != nil {
		if ok, err := env.AgyKeychain(ctx); err == nil && ok {
			keychainOK = true
		}
	}

	localSignedIn := oauth.valid || keychainOK

	// 2. Bounded `agy models` probe (cached for env.minInterval() in agy.json).
	probeOutcome := probeAgyCached(ctx, env, localSignedIn, oauth.valid, oauth.authMethod)

	switch {
	case probeOutcome.ran && probeOutcome.expired:
		s := Status{
			State:       LoginExpired,
			StateReason: "Antigravity OAuth session expired; run agy to sign in again",
			Source:      "Antigravity OAuth",
		}
		if configuredTier != "" {
			s.Plan = ptr(configuredTier)
		} else if probeOutcome.plan != "" {
			s.Plan = ptr(probeOutcome.plan)
		}
		if act.hasAny {
			at := act.newestAt
			s.ObservedAt = &at
			s.Activity = act.toActivity()
		}
		return redactAgyStatus(s, redactPaths...)

	case probeOutcome.ran && probeOutcome.signedOut:
		s := Status{Source: "Antigravity OAuth"}
		if localSignedIn {
			s.State = LoginExpired
			s.StateReason = "stored Antigravity OAuth session was rejected; run agy to sign in again"
			if configuredTier != "" {
				s.Plan = ptr(configuredTier)
			} else if probeOutcome.plan != "" {
				s.Plan = ptr(probeOutcome.plan)
			}
			if act.hasAny {
				at := act.newestAt
				s.ObservedAt = &at
				s.Activity = act.toActivity()
			}
		} else {
			s.State = NotSignedIn
			s.StateReason = "not signed in to Antigravity"
		}
		return redactAgyStatus(s, redactPaths...)

	case !localSignedIn && (!probeOutcome.signedIn):
		s := Status{
			State:  NotSignedIn,
			Source: "Antigravity local state",
		}
		if probeOutcome.notInstalled && !dirExists(dataDir) {
			s.StateReason = "agy is not installed"
		} else {
			s.StateReason = "no Antigravity OAuth login"
		}
		if probeOutcome.staleReason != "" {
			s.Stale, s.StaleReason = true, probeOutcome.staleReason
		}
		return redactAgyStatus(s, redactPaths...)
	}

	// 3. Signed-in OAuth subscription.
	s := Status{
		State:  SignedIn,
		Source: "Antigravity OAuth",
	}
	if probeOutcome.fromCache {
		s.Source = "Antigravity OAuth (cached)"
	}
	if act.hasAny {
		s.Source += " + local transcripts"
	}
	if probeOutcome.staleReason != "" {
		s.Stale = true
		s.StaleReason = probeOutcome.staleReason
	}

	plan := configuredTier
	if plan == "" {
		plan = probeOutcome.plan
	}
	if plan == "" && oauth.authMethod == "enterprise" {
		plan = "enterprise"
	}
	if plan == "" {
		plan = inferAgyPlanFromTranscriptModels(act.models)
	}
	if plan != "" {
		s.Plan = ptr(plan)
	} else {
		s.PlanReason = "plan tier requires agy models or a plan annotation in subscriptions.toml"
	}

	s.Activity = act.toActivity()
	s.Meters = buildAgySubscriptionMeters(plan, act)

	var observed time.Time
	if !probeOutcome.observedAt.IsZero() {
		observed = probeOutcome.observedAt
	}
	if act.hasAny && act.newestAt.After(observed) {
		observed = act.newestAt
	}
	if !observed.IsZero() {
		s.ObservedAt = &observed
	} else {
		s.ObservedReason = "signed in; no local Antigravity transcripts or model probe reading yet"
	}

	return redactAgyStatus(s, redactPaths...)
}

func buildAgySubscriptionMeters(plan string, act agyActivityScan) []Meter {
	meters := []Meter{}
	if plan != "standard" {
		meters = append(meters, Meter{
			Kind:          KindSession,
			Label:         "session (5h)",
			WindowMinutes: agySessionMinutes,
			PercentReason: agyNoQuotaPercentReason,
			ResetReason:   agyNoQuotaResetReason,
			Detail:        act.sessionSummary(),
		})
	}
	meters = append(meters, Meter{
		Kind:          KindWeekly,
		Label:         "weekly",
		WindowMinutes: agyWeeklyMinutes,
		PercentReason: agyNoQuotaPercentReason,
		ResetReason:   agyNoQuotaResetReason,
		Detail:        act.weeklySummary(),
	})
	return meters
}

type agyDispatchInfo struct {
	modelProvider string
	hasGeminiRef  bool
	plan          string
}

func readAgyDispatchConfig(path string) agyDispatchInfo {
	b, err := readBoundedFile(path, agyMaxConfigBytes)
	if err != nil {
		return agyDispatchInfo{}
	}
	var raw struct {
		Harnesses map[string]struct {
			EnvRefs       map[string]string `toml:"env_refs"`
			ModelProvider string            `toml:"model_provider"`
			Plan          string            `toml:"plan"`
		} `toml:"harnesses"`
	}
	if toml.Unmarshal(b, &raw) != nil {
		return agyDispatchInfo{}
	}
	for name, h := range raw.Harnesses {
		if !strings.EqualFold(name, "agy") {
			continue
		}
		hasRef := false
		for k, v := range h.EnvRefs {
			if strings.EqualFold(strings.TrimSpace(k), "GEMINI_API_KEY") && strings.TrimSpace(v) != "" {
				hasRef = true
				break
			}
		}
		return agyDispatchInfo{
			modelProvider: strings.TrimSpace(h.ModelProvider),
			hasGeminiRef:  hasRef,
			plan:          normalizeAgyPlan(h.Plan),
		}
	}
	return agyDispatchInfo{}
}

func agyAPIKeyMode(env Env, dataDir string, d agyDispatchInfo) (string, bool) {
	if b, err := readBoundedFile(filepath.Join(dataDir, "settings.json"), agyMaxConfigBytes); err == nil {
		var settings struct {
			ModelProvider string `json:"modelProvider"`
			Auth          struct {
				Type string `json:"type"`
			} `json:"auth"`
		}
		if json.Unmarshal(b, &settings) == nil {
			if strings.EqualFold(strings.TrimSpace(settings.ModelProvider), "gemini") {
				return "settings.json modelProvider=gemini", true
			}
			if strings.EqualFold(strings.TrimSpace(settings.Auth.Type), "gemini-api-key") {
				return "settings.json auth.type=gemini-api-key", true
			}
		}
	}
	if strings.EqualFold(d.modelProvider, "gemini") {
		return "dispatch.toml harnesses.agy.model_provider=gemini", true
	}
	if d.modelProvider == "" && d.hasGeminiRef {
		return "dispatch.toml harnesses.agy.env_refs.GEMINI_API_KEY", true
	}
	if strings.TrimSpace(agyGetenv(env, "GEMINI_API_KEY")) != "" {
		return "GEMINI_API_KEY environment variable", true
	}
	return "", false
}

func agyConfiguredTier(env Env, d agyDispatchInfo) string {
	for _, c := range env.Plans {
		if strings.EqualFold(c.Harness, "agy") {
			if p := normalizeAgyPlan(c.Tier); p != "" && p != "api-key" {
				return p
			}
		}
	}
	if d.plan != "" && d.plan != "api-key" {
		return d.plan
	}
	return ""
}

func normalizeAgyPlan(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	switch s {
	case "":
		return ""
	case "ultra", "google-ai-ultra", "google ai ultra", "ai-ultra", "ai ultra":
		return "ultra"
	case "pro", "google-ai-pro", "google ai pro", "ai-pro", "ai pro":
		return "pro"
	case "standard", "free", "plus", "google-ai-plus", "google ai plus":
		return "standard"
	case "enterprise", "gemini-enterprise", "gemini enterprise":
		return "enterprise"
	case "api-key", "apikey", "api_key", "gemini-api-key":
		return "api-key"
	}
	if agyPlanSafeRe.MatchString(s) {
		return s
	}
	return ""
}

type agyOAuthState struct {
	valid      bool
	expired    bool
	expiredAt  time.Time
	authMethod string
	corrupt    bool
}

func readAgyOAuthFile(dataDir string, now time.Time) agyOAuthState {
	for _, name := range []string{"antigravity-oauth-token", "jetski-standalone-oauth-token"} {
		b, err := readBoundedFile(filepath.Join(dataDir, name), agyMaxTokenBytes)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return agyOAuthState{corrupt: true}
		}
		return parseAgyOAuthToken(b, now)
	}
	return agyOAuthState{}
}

func parseAgyOAuthToken(b []byte, now time.Time) agyOAuthState {
	var raw struct {
		Token *struct {
			AccessToken       string          `json:"access_token"`
			AccessTokenCamel  string          `json:"accessToken"`
			RefreshToken      string          `json:"refresh_token"`
			RefreshTokenCamel string          `json:"refreshToken"`
			Expiry            json.RawMessage `json:"expiry"`
			ExpiresAt         json.RawMessage `json:"expires_at"`
			ExpiresAtCamel    json.RawMessage `json:"expiresAt"`
		} `json:"token"`
		AccessToken       string          `json:"access_token"`
		AccessTokenCamel  string          `json:"accessToken"`
		RefreshToken      string          `json:"refresh_token"`
		RefreshTokenCamel string          `json:"refreshToken"`
		Expiry            json.RawMessage `json:"expiry"`
		ExpiresAt         json.RawMessage `json:"expires_at"`
		ExpiresAtCamel    json.RawMessage `json:"expiresAt"`
		AuthMethod        string          `json:"auth_method"`
		AuthMethodCamel   string          `json:"authMethod"`
	}
	if json.Unmarshal(b, &raw) != nil {
		return agyOAuthState{corrupt: true}
	}
	hasAccess := strings.TrimSpace(raw.AccessToken) != "" || strings.TrimSpace(raw.AccessTokenCamel) != ""
	hasRefresh := strings.TrimSpace(raw.RefreshToken) != "" || strings.TrimSpace(raw.RefreshTokenCamel) != ""
	expiry := parseFlexibleTime(raw.Expiry, raw.ExpiresAt, raw.ExpiresAtCamel)
	if raw.Token != nil {
		hasAccess = hasAccess || strings.TrimSpace(raw.Token.AccessToken) != "" || strings.TrimSpace(raw.Token.AccessTokenCamel) != ""
		hasRefresh = hasRefresh || strings.TrimSpace(raw.Token.RefreshToken) != "" || strings.TrimSpace(raw.Token.RefreshTokenCamel) != ""
		if t := parseFlexibleTime(raw.Token.Expiry, raw.Token.ExpiresAt, raw.Token.ExpiresAtCamel); !t.IsZero() {
			expiry = t
		}
	}
	// Clear credential fields before returning.
	authRaw := strings.ToLower(strings.TrimSpace(nonEmpty(raw.AuthMethod, raw.AuthMethodCamel)))
	raw = struct {
		Token *struct {
			AccessToken       string          `json:"access_token"`
			AccessTokenCamel  string          `json:"accessToken"`
			RefreshToken      string          `json:"refresh_token"`
			RefreshTokenCamel string          `json:"refreshToken"`
			Expiry            json.RawMessage `json:"expiry"`
			ExpiresAt         json.RawMessage `json:"expires_at"`
			ExpiresAtCamel    json.RawMessage `json:"expiresAt"`
		} `json:"token"`
		AccessToken       string          `json:"access_token"`
		AccessTokenCamel  string          `json:"accessToken"`
		RefreshToken      string          `json:"refresh_token"`
		RefreshTokenCamel string          `json:"refreshToken"`
		Expiry            json.RawMessage `json:"expiry"`
		ExpiresAt         json.RawMessage `json:"expires_at"`
		ExpiresAtCamel    json.RawMessage `json:"expiresAt"`
		AuthMethod        string          `json:"auth_method"`
		AuthMethodCamel   string          `json:"authMethod"`
	}{}

	if !hasAccess && !hasRefresh {
		return agyOAuthState{corrupt: true}
	}
	method := ""
	switch authRaw {
	case "enterprise", "oauth-business", "gcp", "business":
		method = "enterprise"
	case "consumer", "oauth-personal", "oauth", "google":
		method = "consumer"
	}
	if !expiry.IsZero() && !expiry.After(now) && !hasRefresh {
		return agyOAuthState{expired: true, expiredAt: expiry, authMethod: method}
	}
	return agyOAuthState{valid: true, authMethod: method}
}

func parseFlexibleTime(candidates ...json.RawMessage) time.Time {
	for _, raw := range candidates {
		raw = bytes.TrimSpace(raw)
		if len(raw) == 0 || string(raw) == "null" {
			continue
		}
		var s string
		if json.Unmarshal(raw, &s) == nil {
			s = strings.TrimSpace(s)
			if i := strings.Index(s, " m="); i > 0 {
				s = s[:i]
			}
			for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05.999999999 -0700 MST", "2006-01-02 15:04:05"} {
				if t, err := time.Parse(layout, s); err == nil {
					return t
				}
			}
			continue
		}
		var n int64
		if json.Unmarshal(raw, &n) == nil && n > 0 {
			if n > 1e12 {
				return time.UnixMilli(n)
			}
			return time.Unix(n, 0)
		}
	}
	return time.Time{}
}

type agyProbeOutcome struct {
	ran          bool
	fromCache    bool
	notInstalled bool
	signedIn     bool
	signedOut    bool
	expired      bool
	plan         string
	observedAt   time.Time
	staleReason  string
}

func probeAgyCached(ctx context.Context, env Env, localSignedIn, usedLocalFile bool, authMethod string) agyProbeOutcome {
	if env.AgyProbe == nil {
		return agyProbeOutcome{}
	}
	now := env.now()
	cache, _ := loadAgyCache(env)
	path := AgyCachePath(env)

	var unlock func()
	if env.StateDir != "" && os.MkdirAll(filepath.Dir(path), 0o700) == nil {
		u, err := platformops.LockPath(path+".lock", true)
		if err != nil {
			if platformops.IsLockBusy(err) {
				return agyOutcomeFromCache(cache, "another process is reading Antigravity model availability")
			}
			return agyOutcomeFromCache(cache, "cannot lock the Antigravity subscription cache")
		}
		unlock = u
		defer unlock()
		now = env.now()
		cache, _ = loadAgyCache(env)
	}

	if cache != nil && agyCacheConsistent(cache, localSignedIn, usedLocalFile) {
		last := cache.LastAttempt()
		if !last.IsZero() && now.Sub(last) < env.minInterval() {
			if cache.LastError != "" {
				next := last.Add(env.minInterval()).UTC().Format(time.RFC3339)
				return agyOutcomeFromCache(cache, cache.LastError+"; next check after "+next)
			}
			out := agyOutcomeFromCache(cache, "")
			out.fromCache = true
			return out
		}
	}

	attempt := now.UTC()
	if env.StateDir != "" {
		pending := agyCache{Schema: agyCacheSchema, UsedLocalFile: usedLocalFile}
		if cache != nil {
			pending = *cache
			pending.UsedLocalFile = usedLocalFile
		}
		pending.LastAttemptAt = attempt
		pending.LastError = "previous Antigravity model probe did not finish"
		_ = saveAgyCache(env, &pending)
	}

	pctx, cancel := context.WithTimeout(ctx, env.agyProbeTimeout())
	res, err := env.AgyProbe(pctx)
	timedOut := errors.Is(err, context.DeadlineExceeded) || errors.Is(pctx.Err(), context.DeadlineExceeded)
	cancel()

	if err == nil && !res.Installed {
		return agyProbeOutcome{ran: true, notInstalled: true}
	}
	if err != nil {
		reason := "Antigravity model probe failed"
		if timedOut {
			reason = fmt.Sprintf("Antigravity model probe timed out after %s", humanDuration(env.agyProbeTimeout()))
		}
		failed := &agyCache{
			Schema:        agyCacheSchema,
			UsedLocalFile: usedLocalFile,
			LastAttemptAt: attempt,
			LastError:     reason,
		}
		if cache != nil {
			failed.ObservedAt = cache.ObservedAt
			failed.Plan = cache.Plan
			failed.Models = cache.Models
			failed.LoginState = cache.LoginState
		}
		_ = saveAgyCache(env, failed)
		return agyOutcomeFromCache(failed, reason)
	}

	models := sanitizeAgyModels(res.Models)
	switch {
	case res.Expired:
		saved := &agyCache{
			Schema:        agyCacheSchema,
			LoginState:    LoginExpired,
			UsedLocalFile: usedLocalFile,
			LastAttemptAt: attempt,
		}
		if cache != nil && cache.Plan != "" {
			saved.Plan = cache.Plan
		}
		_ = saveAgyCache(env, saved)
		return agyProbeOutcome{ran: true, expired: true, plan: saved.Plan}

	case res.SignedOut:
		state := NotSignedIn
		if localSignedIn {
			state = LoginExpired
		}
		saved := &agyCache{
			Schema:        agyCacheSchema,
			LoginState:    state,
			UsedLocalFile: usedLocalFile,
			LastAttemptAt: attempt,
		}
		_ = saveAgyCache(env, saved)
		return agyProbeOutcome{ran: true, signedOut: true}

	default:
		plan := inferAgyPlanFromModels(models, authMethod)
		signedIn := localSignedIn || len(models) > 0
		loginState := NotSignedIn
		if signedIn {
			loginState = SignedIn
		}
		saved := &agyCache{
			Schema:        agyCacheSchema,
			ObservedAt:    attempt,
			Plan:          plan,
			LoginState:    loginState,
			UsedLocalFile: usedLocalFile,
			Models:        models,
			LastAttemptAt: attempt,
		}
		_ = saveAgyCache(env, saved)
		return agyProbeOutcome{
			ran:        true,
			signedIn:   signedIn,
			plan:       plan,
			observedAt: attempt,
		}
	}
}

func agyCacheConsistent(c *agyCache, localSignedIn, usedLocalFile bool) bool {
	if c == nil {
		return false
	}
	if localSignedIn && c.LoginState == NotSignedIn {
		return false
	}
	if !localSignedIn && c.UsedLocalFile && c.LoginState == SignedIn {
		return false
	}
	return true
}

func agyOutcomeFromCache(c *agyCache, staleReason string) agyProbeOutcome {
	out := agyProbeOutcome{staleReason: staleReason}
	if c == nil {
		return out
	}
	out.plan = c.Plan
	out.observedAt = c.ObservedAt
	switch c.LoginState {
	case SignedIn:
		out.signedIn = true
	case LoginExpired:
		out.ran = true
		out.expired = true
	case NotSignedIn:
		if staleReason == "" {
			out.ran = true
			out.signedOut = true
		}
	}
	return out
}

// newDefaultAgyProbe returns a probe that runs `agy models` non-interactively
// against home with bounded output and returns sanitized model IDs or auth
// status flags.
func newDefaultAgyProbe(home string) func(ctx context.Context) (AgyProbeResult, error) {
	return func(ctx context.Context) (AgyProbeResult, error) {
		exe, err := exec.LookPath("agy")
		if err != nil {
			if errors.Is(err, exec.ErrNotFound) {
				return AgyProbeResult{Installed: false}, nil
			}
			return AgyProbeResult{}, errors.New("agy executable lookup failed")
		}
		cmd := exec.CommandContext(ctx, exe, "models")
		cmd.Stdin = nil
		cmd.WaitDelay = 2 * time.Second
		cmdEnv := append(os.Environ(), "BROWSER=false", "NO_COLOR=1", "TERM=dumb")
		if strings.TrimSpace(home) != "" {
			cmdEnv = append(cmdEnv, "HOME="+home, "USERPROFILE="+home)
		}
		cmd.Env = cmdEnv
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &limitedWriter{w: &stdout, max: agyMaxProbeBytes}
		cmd.Stderr = &limitedWriter{w: &stderr, max: agyMaxProbeBytes}
		runErr := cmd.Run()
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return AgyProbeResult{Installed: true}, context.DeadlineExceeded
		}
		return parseAgyProbeOutput(stdout.String(), stderr.String(), runErr)
	}
}

// parseAgyProbeOutput classifies `agy models` output without keeping raw text.
func parseAgyProbeOutput(stdout, stderr string, runErr error) (AgyProbeResult, error) {
	combined := stripAgyTerminalNoise(stdout + "\n" + stderr)
	lower := strings.ToLower(combined)
	switch {
	case strings.Contains(lower, "not logged into antigravity"),
		strings.Contains(lower, "not logged in"),
		strings.Contains(lower, "not signed in"),
		strings.Contains(lower, "authentication required"),
		strings.Contains(lower, "please sign in"):
		return AgyProbeResult{Installed: true, SignedOut: true}, nil
	case strings.Contains(lower, "session expired"),
		strings.Contains(lower, "token expired"),
		strings.Contains(lower, "invalid_grant"),
		strings.Contains(lower, "re-authenticate"):
		return AgyProbeResult{Installed: true, Expired: true}, nil
	}

	var models []string
	for _, line := range strings.Split(stripAgyTerminalNoise(stdout), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		id := strings.ToLower(fields[0])
		if validAgyModelID(id) {
			models = append(models, id)
		}
	}
	models = sanitizeAgyModels(models)
	if runErr != nil && len(models) == 0 {
		return AgyProbeResult{Installed: true}, errors.New("agy models returned an error")
	}
	return AgyProbeResult{Installed: true, Models: models}, nil
}

func stripAgyTerminalNoise(s string) string {
	s = agyANSIEscapeRe.ReplaceAllString(s, "")
	// Spinner updates overwrite the current line with \r; keep the final segment.
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if idx := strings.LastIndex(l, "\r"); idx >= 0 {
			lines[i] = l[idx+1:]
		}
	}
	return strings.Join(lines, "\n")
}

func validAgyModelID(id string) bool {
	if !agyModelIDRe.MatchString(id) {
		return false
	}
	// Refuse anything that looks like a token or key rather than a model id.
	if strings.HasPrefix(id, "ya29.") || strings.HasPrefix(id, "aiza") || strings.HasPrefix(id, "sk-") {
		return false
	}
	return strings.HasPrefix(id, "gemini-") || strings.HasPrefix(id, "claude-") || strings.HasPrefix(id, "gpt-") || strings.Contains(id, "-orchestrator")
}

func sanitizeAgyModels(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, m := range in {
		id := strings.ToLower(strings.TrimSpace(m))
		if !validAgyModelID(id) || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
		if len(out) >= agyMaxCachedModels {
			break
		}
	}
	return out
}

// inferAgyPlanFromModels maps a complete `agy models` list to its Google AI
// plan tier according to Antigravity's model availability matrix:
//   - Gemini 4 Carbon / Orchestrator (Carbon) -> ultra (unless enterprise-only Flash/Pro models also appear with enterprise auth)
//   - Gemini 3.1 Pro / 3.7 Flash / 3.6 Flash -> enterprise
//   - Gemini 4 Neon / Argon / Hydrogen / Claude 5.5 (without Carbon) -> pro
//   - Gemini 3.8 Flash / standard models only -> standard
func inferAgyPlanFromModels(models []string, authMethod string) string {
	hasCarbon := false
	hasEnterpriseOnly := false
	hasProTier := false
	for _, raw := range models {
		m := strings.ToLower(strings.TrimSpace(raw))
		switch {
		case strings.HasPrefix(m, "gemini-4-carbon"), m == "gemini-pro-orchestrator":
			hasCarbon = true
		case strings.HasPrefix(m, "gemini-3.1-pro"), strings.HasPrefix(m, "gemini-3.7-flash"), strings.HasPrefix(m, "gemini-3.6-flash"):
			hasEnterpriseOnly = true
		case strings.HasPrefix(m, "gemini-4-neon"),
			strings.HasPrefix(m, "gemini-4-argon"),
			strings.HasPrefix(m, "gemini-4-hydrogen"),
			strings.HasPrefix(m, "gemini-pro-orchestrator-"),
			strings.HasPrefix(m, "claude-opus-5-5"),
			strings.HasPrefix(m, "claude-sonnet-5-5"):
			hasProTier = true
		}
	}
	if authMethod == "enterprise" || (hasEnterpriseOnly && authMethod != "consumer") {
		return "enterprise"
	}
	if hasCarbon {
		return "ultra"
	}
	if hasProTier {
		return "pro"
	}
	if len(models) > 0 {
		return "standard"
	}
	return ""
}

// inferAgyPlanFromTranscriptModels uses passive transcript history cautiously:
// using Carbon proves Ultra (or Enterprise), whereas using Flash/Neon alone is
// only a lower bound and does not rule out a higher tier.
func inferAgyPlanFromTranscriptModels(models []string) string {
	for _, raw := range models {
		m := strings.ToLower(strings.TrimSpace(raw))
		if strings.HasPrefix(m, "gemini-4-carbon") || m == "gemini-pro-orchestrator" || strings.Contains(m, "gemini 4 carbon") || strings.Contains(m, "orchestrator (carbon)") {
			return "ultra"
		}
	}
	return ""
}

type agyActivityScan struct {
	hasAny   bool
	newestAt time.Time
	session  WindowActivity
	weekly   WindowActivity
	models   []string
}

func (a agyActivityScan) toActivity() *Activity {
	s := a.session
	w := a.weekly
	return &Activity{Session: &s, Weekly: &w}
}

func (a agyActivityScan) sessionSummary() string {
	return formatWindowActivity(a.session, "5h")
}

func (a agyActivityScan) weeklySummary() string {
	return formatWindowActivity(a.weekly, "7d")
}

func formatWindowActivity(w WindowActivity, label string) string {
	if w.Sessions == 0 && w.Turns == 0 && w.TotalTokens == 0 {
		return "0 sessions on this host in last " + label
	}
	sessWord := "sessions"
	if w.Sessions == 1 {
		sessWord = "session"
	}
	turnWord := "turns"
	if w.Turns == 1 {
		turnWord = "turn"
	}
	if w.TotalTokens == 0 {
		if w.Turns == 0 {
			return fmt.Sprintf("%d %s on this host in last %s", w.Sessions, sessWord, label)
		}
		return fmt.Sprintf("%d %s, %d %s on this host", w.Sessions, sessWord, w.Turns, turnWord)
	}
	return fmt.Sprintf("%d %s, %d %s, %s tokens (%s in, %s out, %s cached) on this host",
		w.Sessions, sessWord, w.Turns, turnWord,
		humanTokens(w.TotalTokens), humanTokens(w.InputTokens), humanTokens(w.OutputTokens), humanTokens(w.CacheReadTokens))
}

func humanTokens(n int64) string {
	switch {
	case n < 1000:
		return fmt.Sprintf("%d", n)
	case n < 1_000_000:
		return trimFloat1(float64(n)/1e3) + "k"
	case n < 1_000_000_000:
		return trimFloat1(float64(n)/1e6) + "M"
	default:
		return trimFloat1(float64(n)/1e9) + "B"
	}
}

func trimFloat1(v float64) string {
	s := fmt.Sprintf("%.1f", v)
	return strings.TrimSuffix(s, ".0")
}

func readAgyActivity(ctx context.Context, dataDir string, now time.Time) agyActivityScan {
	ctx, cancel := context.WithTimeout(ctx, agyActivityTimeout)
	defer cancel()

	scan := agyActivityScan{
		session: WindowActivity{WindowMinutes: agySessionMinutes},
		weekly:  WindowActivity{WindowMinutes: agyWeeklyMinutes},
	}
	weekCut := now.Add(-7 * 24 * time.Hour)
	sessCut := now.Add(-5 * time.Hour)

	type candidate struct {
		cid  string
		path string
		mod  time.Time
		size int64
	}
	var files []candidate
	seenCIDs := map[string]bool{}

	if entries, err := os.ReadDir(filepath.Join(dataDir, "brain")); err == nil {
		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			cid := e.Name()
			logsDir := filepath.Join(dataDir, "brain", cid, ".system_generated", "logs")
			p := filepath.Join(logsDir, "transcript_full.jsonl")
			info, err := os.Stat(p)
			if err != nil {
				p = filepath.Join(logsDir, "transcript.jsonl")
				info, err = os.Stat(p)
			}
			if err != nil || info.IsDir() || info.Size() == 0 || info.ModTime().Before(weekCut) {
				continue
			}
			files = append(files, candidate{cid: cid, path: p, mod: info.ModTime(), size: info.Size()})
		}
	}
	sort.Slice(files, func(i, j int) bool {
		if !files[i].mod.Equal(files[j].mod) {
			return files[i].mod.After(files[j].mod)
		}
		return files[i].cid < files[j].cid
	})
	if len(files) > agyMaxTranscriptFiles {
		files = files[:agyMaxTranscriptFiles]
	}

	modelSeen := map[string]bool{}
	var totalBytes int64
	for _, f := range files {
		if ctx.Err() != nil || totalBytes >= agyMaxTotalTranscriptBytes {
			break
		}
		budget := min(int64(agyMaxTranscriptBytes), agyMaxTotalTranscriptBytes-totalBytes)
		readBytes, sess5h, sess7d := scanAgyTranscript(f.path, f.size, budget, f.mod, now, sessCut, weekCut, &scan, modelSeen)
		totalBytes += readBytes
		if sess7d {
			seenCIDs[f.cid] = true
			scan.weekly.Sessions++
			scan.hasAny = true
		}
		if sess5h {
			scan.session.Sessions++
		}
	}

	// Supplement from conversation_summaries.db for recent sessions that do not
	// have JSONL transcripts under brain/.
	scanAgySummaryDB(ctx, filepath.Join(dataDir, "conversation_summaries.db"), now, sessCut, weekCut, seenCIDs, &scan)

	return scan
}

func scanAgyTranscript(path string, size, maxBytes int64, mod, now, sessCut, weekCut time.Time, scan *agyActivityScan, modelSeen map[string]bool) (readBytes int64, active5h, active7d bool) {
	f, err := os.Open(path)
	if err != nil {
		return 0, false, false
	}
	defer f.Close()

	skipPartial := false
	if size > maxBytes && maxBytes > 0 {
		if _, err := f.Seek(size-maxBytes, io.SeekStart); err == nil {
			skipPartial = true
		}
	}
	lr := io.LimitReader(f, maxBytes)
	r := bufio.NewReaderSize(lr, 64<<10)
	if skipPartial {
		if b, err := r.ReadBytes('\n'); err != nil {
			return int64(len(b)), false, false
		} else {
			readBytes += int64(len(b))
		}
	}

	for {
		line, tooLong, err := readBoundedLine(r, agyMaxLineBytes)
		readBytes += int64(len(line))
		if !tooLong && len(bytes.TrimSpace(line)) > 0 {
			var step struct {
				Type            string `json:"type"`
				Status          string `json:"status"`
				CreatedAt       string `json:"created_at"`
				Model           string `json:"model"`
				InputTokens     int64  `json:"input_tokens"`
				OutputTokens    int64  `json:"output_tokens"`
				CacheReadTokens int64  `json:"cache_read_tokens"`
			}
			if json.Unmarshal(line, &step) == nil && !agyStepRunning(step.Status) {
				ts, terr := time.Parse(time.RFC3339Nano, strings.TrimSpace(step.CreatedAt))
				if terr != nil {
					ts = mod
				}
				if ts.After(now) {
					ts = now
				}
				if !ts.Before(weekCut) {
					active7d = true
					if ts.After(scan.newestAt) {
						scan.newestAt = ts
					}
					in5h := !ts.Before(sessCut)
					if in5h {
						active5h = true
					}
					if strings.EqualFold(strings.TrimSpace(step.Type), "PLANNER_RESPONSE") {
						inTok := max(int64(0), step.InputTokens)
						outTok := max(int64(0), step.OutputTokens)
						cacheTok := max(int64(0), step.CacheReadTokens)
						tot := inTok + outTok + cacheTok

						scan.weekly.Turns++
						scan.weekly.InputTokens += inTok
						scan.weekly.OutputTokens += outTok
						scan.weekly.CacheReadTokens += cacheTok
						scan.weekly.TotalTokens += tot

						if in5h {
							scan.session.Turns++
							scan.session.InputTokens += inTok
							scan.session.OutputTokens += outTok
							scan.session.CacheReadTokens += cacheTok
							scan.session.TotalTokens += tot
						}
						if m := strings.ToLower(strings.TrimSpace(step.Model)); m != "" && len(scan.models) < agyMaxCachedModels && !modelSeen[m] {
							modelSeen[m] = true
							scan.models = append(scan.models, m)
						}
					}
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
	}
	return readBytes, active5h, active7d
}

func scanAgySummaryDB(ctx context.Context, dbPath string, now, sessCut, weekCut time.Time, seenCIDs map[string]bool, scan *agyActivityScan) {
	if _, err := os.Stat(dbPath); err != nil {
		return
	}
	_, walErr := os.Stat(dbPath + "-wal")
	dsn := agySQLiteDSN(dbPath, os.IsNotExist(walErr))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	rows, err := db.QueryContext(ctx, `SELECT conversation_id, step_count, last_modified_time FROM conversation_summaries ORDER BY last_modified_time DESC LIMIT 100`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var cid string
		var steps int
		var rawTime string
		if rows.Scan(&cid, &steps, &rawTime) != nil {
			continue
		}
		cid = strings.TrimSpace(cid)
		if cid == "" || seenCIDs[cid] {
			continue
		}
		b, _ := json.Marshal(rawTime)
		ts := parseFlexibleTime(b)
		if ts.IsZero() || ts.Before(weekCut) {
			continue
		}
		if ts.After(now) {
			ts = now
		}
		seenCIDs[cid] = true
		scan.hasAny = true
		scan.weekly.Sessions++
		if !ts.Before(sessCut) {
			scan.session.Sessions++
		}
		if ts.After(scan.newestAt) {
			scan.newestAt = ts
		}
	}
}

func agySQLiteDSN(path string, immutable bool) string {
	p := filepath.ToSlash(path)
	if strings.HasPrefix(path, `\\`) {
		p = strings.ReplaceAll(path, `\`, "/")
	} else if len(path) >= 3 && ((path[0] >= 'A' && path[0] <= 'Z') || (path[0] >= 'a' && path[0] <= 'z')) && path[1] == ':' && (path[2] == '\\' || path[2] == '/') {
		p = "/" + strings.ReplaceAll(path, `\`, "/")
	}
	q := "mode=ro&_pragma=busy_timeout(1000)&_pragma=query_only(1)"
	if immutable {
		q = "mode=ro&immutable=1&_pragma=query_only(1)"
	}
	return (&url.URL{Scheme: "file", Path: p, RawQuery: q}).String()
}

func agyStepRunning(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "RUNNING", "PENDING", "IN_PROGRESS", "STREAMING":
		return true
	}
	return false
}

func readBoundedLine(r *bufio.Reader, maxBytes int) ([]byte, bool, error) {
	var buf []byte
	tooLong := false
	for {
		frag, isPrefix, err := r.ReadLine()
		if !tooLong {
			if len(buf)+len(frag) > maxBytes {
				tooLong = true
				buf = nil
			} else if !isPrefix && len(buf) == 0 {
				return frag, false, err
			} else {
				buf = append(buf, frag...)
			}
		}
		if !isPrefix || err != nil {
			return buf, tooLong, err
		}
	}
}

func readBoundedFile(path string, maxBytes int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > maxBytes {
		return nil, fmt.Errorf("file exceeds %d bytes", maxBytes)
	}
	return b, nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// hasWindowsCredentialTarget checks `cmdkey /list` output for a stored target
// matching prefix. Separated from keychain_windows.go so it is unit-tested on
// every OS.
func hasWindowsCredentialTarget(out, prefix string) bool {
	lowerPrefix := strings.ToLower(strings.TrimSpace(prefix))
	if lowerPrefix == "" {
		return false
	}
	for _, line := range strings.Split(out, "\n") {
		l := strings.ToLower(strings.TrimSpace(line))
		if strings.Contains(l, "none") && !strings.Contains(l, lowerPrefix) {
			continue
		}
		if (strings.Contains(l, "target:") || strings.Contains(l, "cible") || strings.Contains(l, "ziel:")) && strings.Contains(l, lowerPrefix) {
			return true
		}
		if strings.HasPrefix(l, lowerPrefix+":") || l == lowerPrefix {
			return true
		}
	}
	return false
}

// redactAgyText scrubs OAuth tokens, refresh tokens, API keys, secret URIs,
// email addresses and local paths from any text surfaced by the agy provider.
func redactAgyText(s string, extraPaths ...string) string {
	if s == "" {
		return ""
	}
	s = agyJSONSecretRe.ReplaceAllString(s, `"[redacted]"`)
	s = agySecretTokenRe.ReplaceAllString(s, "[redacted]")
	s = agyEmailRe.ReplaceAllString(s, "[redacted]")
	for _, p := range extraPaths {
		p = strings.TrimSpace(p)
		if len(p) >= 4 && p != "/" && p != `\` {
			s = strings.ReplaceAll(s, p, "[redacted]")
			if slash := filepath.ToSlash(p); slash != p {
				s = strings.ReplaceAll(s, slash, "[redacted]")
			}
		}
	}
	s = agyDotGeminiRe.ReplaceAllString(s, "[redacted]")
	s = agyUnixPathRe.ReplaceAllString(s, "[redacted]")
	s = agyWinPathRe.ReplaceAllString(s, "[redacted]")
	return s
}

func redactAgyStatus(s Status, extraPaths ...string) Status {
	if s.Plan != nil {
		clean := normalizeAgyPlan(*s.Plan)
		if clean == "" {
			s.Plan = nil
		} else {
			s.Plan = &clean
		}
	}
	s.PlanReason = redactAgyText(s.PlanReason, extraPaths...)
	s.StateReason = redactAgyText(s.StateReason, extraPaths...)
	s.WeeklyReason = redactAgyText(s.WeeklyReason, extraPaths...)
	s.Source = redactAgyText(s.Source, extraPaths...)
	s.ObservedReason = redactAgyText(s.ObservedReason, extraPaths...)
	s.StaleReason = redactAgyText(s.StaleReason, extraPaths...)
	for i := range s.Meters {
		s.Meters[i].Label = redactAgyText(s.Meters[i].Label, extraPaths...)
		s.Meters[i].Scope = redactAgyText(s.Meters[i].Scope, extraPaths...)
		s.Meters[i].PercentReason = redactAgyText(s.Meters[i].PercentReason, extraPaths...)
		s.Meters[i].ResetReason = redactAgyText(s.Meters[i].ResetReason, extraPaths...)
		s.Meters[i].Detail = redactAgyText(s.Meters[i].Detail, extraPaths...)
	}
	return s
}
