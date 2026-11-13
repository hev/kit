// Package secrets resolves service-account references without persisting values.
package secrets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Ref struct {
	Name string
	URI  string
	TTL  time.Duration
}
type entry struct {
	value   string
	expires time.Time
	created time.Time
}
type Status struct {
	Accounts         []AccountStatus `json:"accounts,omitempty"`
	QuotaUnavailable string          `json:"quota_unavailable,omitempty"`
	Budget           BudgetStatus    `json:"budget"`
	Stale            int             `json:"stale"`
	NotifyError      string          `json:"notify_error,omitempty"`
	MeterNote        string          `json:"meter_note"`
	Hits             int             `json:"hits"`
	Misses           int             `json:"misses"`
	ReadsHour        int             `json:"reads_hour"`
	ReadsDay         int             `json:"reads_day"`
	// LastLimit and Reset are 1Password's cooldown, kept at the top level for
	// existing readers; every provider's own is in Providers.
	LastLimit string    `json:"last_limit,omitempty"`
	Reset     time.Time `json:"reset,omitempty"`
	// AccountLimits keeps the earlier flattened numeric view of the quota.
	AccountLimits string `json:"account_limits"`
	// StartedAt and Requests count every real request (1Password inject
	// batches and quota checks, Key Vault secret reads) this process made
	// since it started.
	StartedAt   time.Time `json:"started_at"`
	Requests    int       `json:"requests"`
	RateLimited int       `json:"rate_limited"`
	// Quota holds the 1Password account's rate-limit rows as last reported by
	// op, checked at most once every quotaEvery.
	Quota           []QuotaRow `json:"quota,omitempty"`
	QuotaAgeSeconds int64      `json:"quota_age_seconds,omitempty"`
	QuotaCheckedAt  time.Time  `json:"quota_checked_at,omitempty"`
	// HostReadsDay counts real requests in the host's read log over the last
	// 24 hours, from every process on the host, not just this one.
	HostReadsDay *int   `json:"host_reads_day,omitempty"`
	ReadLog      string `json:"read_log,omitempty"`
	Daemon       bool   `json:"daemon"`
	// Providers has one row per registered provider, configured or not.
	Providers []ProviderStatus `json:"providers,omitempty"`
	// Manager is this host's secret manager for secret:// references, read
	// by the client from its own setting (a daemon of any age omits it).
	Manager *ManagerStatus `json:"manager,omitempty"`
}

// ProviderStatus is one provider's view in status. It names the credential
// type only and never holds a value, token or reference.
type ProviderStatus struct {
	Scheme     string `json:"scheme"`
	Name       string `json:"name"`
	Configured bool   `json:"configured"`
	// Credential is the credential type in use ("service-account token",
	// "client-secret", "managed-identity", ...).
	Credential string `json:"credential,omitempty"`
	// Setup says what to set when the provider is not configured.
	Setup string `json:"setup,omitempty"`
	// Reachable is "yes" or "no" from the last real request, or "unknown"
	// before the first one.
	Reachable   string    `json:"reachable"`
	LastOutcome string    `json:"last_outcome,omitempty"`
	LastAt      time.Time `json:"last_at,omitempty"`
	Hits        int       `json:"hits"`
	Misses      int       `json:"misses"`
	Requests    int       `json:"requests"`
	ReadsHour   int       `json:"reads_hour"`
	ReadsDay    int       `json:"reads_day"`
	RateLimited int       `json:"rate_limited"`
	LastLimit   string    `json:"last_limit,omitempty"`
	Reset       time.Time `json:"reset,omitempty"`
	// TokenRequests counts access-token requests (Key Vault only).
	TokenRequests int `json:"token_requests,omitempty"`
	// Quota says where the provider's quota is shown, or that it has none.
	Quota string `json:"quota"`
}

// QuotaRow is one row of `op service-account ratelimit`. Only whitelisted
// labels, numbers and a reset time are kept; raw output is discarded.
type QuotaRow struct {
	Type      string `json:"type"`
	Action    string `json:"action"`
	Limit     *int64 `json:"limit,omitempty"`
	Used      *int64 `json:"used,omitempty"`
	Remaining *int64 `json:"remaining,omitempty"`
	Reset     string `json:"reset,omitempty"`
}

// pstate is one provider's counters and cooldown, guarded by Cache.mu except
// fetch, which serializes that provider's misses.
type pstate struct {
	fetch       sync.Mutex
	hits        int
	misses      int
	requests    int
	rateLimited int
	reads       []time.Time
	lastLimit   string
	reset       time.Time
	lastOutcome string
	lastAt      time.Time
	// unclassified counts consecutive rate limits with no class, so their
	// cooldown doubles instead of retrying every minute all day.
	unclassified int
}

type Cache struct {
	cluster atomic.Pointer[clusterBinding]
	mu      sync.Mutex
	entries map[string]entry
	reads   []time.Time
	status  Status
	now     func() time.Time
	op      string
	// providers in registration order; state by scheme.
	providers []Provider
	state     map[string]*pstate
	// quotaMu serializes quota checks; the fields below it are guarded by mu.
	quotaMu        sync.Mutex
	quota          []QuotaRow
	quotaFlat      string
	quotaAt        time.Time
	notifyError    string
	quotaError     string
	quotaSuccessAt time.Time
	monitorEvery   time.Duration
}

// quotaEvery bounds how often status may ask op for the account's quota.
const quotaEvery = 5 * time.Minute

func New() *Cache {
	c := &Cache{entries: map[string]entry{}, now: time.Now, op: "op", state: map[string]*pstate{}}
	for _, sp := range registry {
		c.providers = append(c.providers, sp.build(c))
		c.state[sp.scheme] = &pstate{}
	}
	c.monitorEvery = time.Minute
	c.status.StartedAt = c.now()
	return c
}

func (c *Cache) provider(scheme string) Provider {
	for _, p := range c.providers {
		if p.Scheme() == scheme {
			return p
		}
	}
	return nil
}

var namePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Validate checks names, TTLs and that every reference is well formed for
// its provider (op:// or azkv://). A secret:// reference is expanded first
// (ExpandRefs); Validate itself accepts explicit references only.
func Validate(refs []Ref) error {
	seen := map[string]bool{}
	if len(refs) > 128 {
		return errors.New("too many secret references")
	}
	for _, r := range refs {
		if !namePattern.MatchString(r.Name) || seen[r.Name] || !ValidRef(r.URI) || r.TTL < 0 || r.TTL > 24*time.Hour {
			return errors.New("invalid secret reference, name or TTL (maximum 24h)")
		}
		seen[r.Name] = true
	}
	return nil
}

// Holding a provider's fetch lock serializes its misses, deduplicating
// concurrent batches; one provider's slow or throttled store never holds
// another's. Stale values are served only after provider rate limits; callers
// may shorten, but never extend, the stored freshness TTL.
func (c *Cache) Resolve(ctx context.Context, refs []Ref) (map[string]string, error) {
	return c.ResolveAs(ctx, "", refs)
}

// Per-reference dispositions. They name where a value came from, never the
// value, so a wrapper can keep an accurate label-only log of hits and reads.
const (
	// SourceCache: served from memory; no request.
	SourceCache = "cache"
	// SourceRead: part of a real request this call made.
	SourceRead = "read"
	// SourceStale is a held value served only during provider rate limiting.
	SourceStale = "stale"
)

// ResolveAs resolves refs and attributes any real request to caller in the
// host's read log.
func (c *Cache) ResolveAs(ctx context.Context, caller string, refs []Ref) (map[string]string, error) {
	values, _, err := c.ResolveSources(ctx, caller, refs)
	return values, err
}

// ResolveSources is ResolveAs plus each reference's disposition by name
// (SourceCache or SourceRead). References are grouped by provider; each
// group is one batch for its provider. A request fails as a whole: before any
// request is made, every provider with a miss must be configured and out of
// cooldown, and no value is returned unless every reference resolved.
func (c *Cache) ResolveSources(ctx context.Context, caller string, refs []Ref) (map[string]string, map[string]string, error) {
	// A secret:// reference becomes its explicit form before anything else,
	// so it shares the explicit form's cache entry, batch, cooldown and log.
	refs, err := ExpandRefs(refs)
	if err != nil {
		return nil, nil, err
	}
	if err := Validate(refs); err != nil {
		return nil, nil, err
	}
	groups := map[string][]Ref{}
	for _, r := range refs {
		s := schemeOf(r.URI)
		groups[s] = append(groups[s], r)
	}
	// Atomic precheck: no provider is asked while another would refuse.
	c.mu.Lock()
	now := c.now()
	for _, p := range c.providers {
		g := groups[p.Scheme()]
		if len(g) == 0 || !c.anyMissLocked(now, g) {
			continue
		}
		// The host's cooldown file carries a rate limit another process (or
		// an earlier run of this one) was answered with.
		ps := c.state[p.Scheme()]
		c.adoptCooldownLocked(now, p, ps)
		if now.Before(ps.reset) && !c.canStaleLocked(g) {
			c.mu.Unlock()
			return nil, nil, providerCooldownError(p.Name(), ps.lastLimit, ps.reset)
		}
	}
	c.mu.Unlock()
	for _, p := range c.providers {
		g := groups[p.Scheme()]
		if len(g) == 0 {
			continue
		}
		c.mu.Lock()
		miss := c.anyMissLocked(c.now(), g)
		c.mu.Unlock()
		if !miss {
			continue
		}
		if _, err := p.Credential(); err != nil {
			return nil, nil, err
		}
	}
	out := map[string]string{}
	sources := map[string]string{}
	for _, p := range c.providers {
		g := groups[p.Scheme()]
		if len(g) == 0 {
			continue
		}
		if err := c.resolveGroup(ctx, caller, p, g, out, sources); err != nil {
			return nil, nil, err
		}
	}
	return out, sources, nil
}

// anyMissLocked reports whether any reference would need a real request now.
func (c *Cache) anyMissLocked(now time.Time, g []Ref) bool {
	for _, r := range g {
		e, ok := c.entries[r.URI]
		if !ok || r.TTL <= 0 || !now.Before(e.created.Add(r.TTL)) || !now.Before(e.expires) {
			return true
		}
	}
	return false
}

func (c *Cache) resolveGroup(ctx context.Context, caller string, p Provider, refs []Ref, out, sources map[string]string) error {
	ps := c.state[p.Scheme()]
	ps.fetch.Lock()
	defer ps.fetch.Unlock()
	c.mu.Lock()
	now := c.now()
	for uri, e := range c.entries {
		if now.Sub(e.created) > 24*time.Hour {
			delete(c.entries, uri)
		}
	}
	missing := []Ref{}
	for _, r := range refs {
		if e, ok := c.entries[r.URI]; ok && r.TTL > 0 && now.Before(e.expires) && now.Before(e.created.Add(r.TTL)) {
			out[r.Name] = e.value
			sources[r.Name] = SourceCache
			c.status.Hits++
			ps.hits++
			LogReadProvider(now, p.Scheme(), caller, "cache", []string{Label(r.Name, r.URI)}, "ok")
		} else {
			missing = append(missing, r)
			c.status.Misses++
			ps.misses++
		}
	}
	if len(missing) == 0 {
		c.mu.Unlock()
		return nil
	}
	c.adoptCooldownLocked(now, p, ps)
	if now.Before(ps.reset) {
		if c.staleLocked(now, caller, p, refs, out, sources) {
			c.mu.Unlock()
			return nil
		}
		c.mu.Unlock()
		return providerCooldownError(p.Name(), ps.lastLimit, ps.reset)
	}
	c.mu.Unlock()
	if _, err := p.Credential(); err != nil {
		return err
	}
	unique := []string{}
	indices := map[string]int{}
	labels := []string{}
	for _, r := range missing {
		if _, ok := indices[r.URI]; !ok {
			indices[r.URI] = len(unique)
			unique = append(unique, r.URI)
			labels = append(labels, Label(r.Name, r.URI))
		}
	}
	cost := 1
	if p.Scheme() != "op" {
		cost = len(unique)
	}
	if err := c.reserve(now, caller, p.Scheme(), cost); err != nil {
		return err
	}
	c.warnings(ctx, c.LocalStatus())
	c.mu.Lock()
	c.reads = append(c.reads, now)
	ps.reads = append(ps.reads, now)
	c.mu.Unlock()
	values, n, err := p.Read(ctx, unique)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status.Requests += n
	ps.requests += n
	ps.lastAt = now
	if err != nil {
		var rl *RateLimit
		if errors.As(err, &rl) {
			c.status.RateLimited++
			ps.rateLimited++
			ps.lastOutcome = "rate_limited"
			reset, limit := c.cooldownLocked(now, p, ps, rl)
			LogReadProvider(now, p.Scheme(), caller, p.Kind(), labels, "rate_limited")
			if c.staleLocked(now, caller, p, refs, out, sources) {
				return nil
			}
			return providerCooldownError(p.Name(), limit, reset)
		}
		for _, r := range refs {
			delete(c.entries, r.URI)
		}
		ps.lastOutcome = "error"
		LogReadProvider(now, p.Scheme(), caller, p.Kind(), labels, "error")
		return err
	}
	ps.lastOutcome = "ok"
	LogReadProvider(now, p.Scheme(), caller, p.Kind(), labels, "ok")
	ps.unclassified = 0
	clearCooldownFor(p.Scheme(), now)
	for _, r := range missing {
		value := values[indices[r.URI]]
		out[r.Name] = value
		sources[r.Name] = SourceRead
		if r.TTL > 0 {
			expiry := now.Add(r.TTL)
			if old, ok := c.entries[r.URI]; ok && now.Before(old.expires) && old.expires.Before(expiry) {
				expiry = old.expires
			}
			c.entries[r.URI] = entry{value: value, expires: expiry, created: now}
		}
	}
	return nil
}

// cooldownLocked records one cooldown for provider p from a rate-limit
// answer. A protocol Retry-After is honoured as given; otherwise the class
// sets a conservative delay, an explicit reset or "try again in" hint within
// the class's bound may lengthen it, and unclassified answers back off
// exponentially. Caller holds c.mu.
func (c *Cache) cooldownLocked(now time.Time, p Provider, ps *pstate, rl *RateLimit) (time.Time, string) {
	c.adoptCooldownLocked(now, p, ps)
	limit := rl.Class
	if limit == "" {
		limit = "rate limit"
	}
	var reset time.Time
	if rl.After > 0 {
		reset = now.Add(min(max(rl.After, time.Second), 24*time.Hour))
	} else {
		delay, bound := time.Minute, 24*time.Hour
		switch limit {
		case "hourly rate limit":
			delay, bound = time.Hour, time.Hour
		case "daily rate limit":
			delay = 24 * time.Hour
		default:
			// An unclassified limit may be the daily account quota. Back off
			// exponentially across consecutive answers, capped at one hour.
			delay = min(time.Minute<<min(ps.unclassified, 6), time.Hour)
			ps.unclassified++
		}
		jitter := time.Duration(rand.IntN(30)) * time.Second
		reset = now.Add(delay + jitter)
		// Accept only explicit reset/retry times, bounded by the limit class.
		if !rl.Reset.IsZero() {
			if rl.Reset.After(now) && !rl.Reset.After(now.Add(bound)) {
				reset = rl.Reset.Add(jitter)
			}
		} else if d := rl.RetryIn; d > 0 && d <= bound && d > delay {
			reset = now.Add(d + jitter)
		}
	}
	ps.lastLimit, ps.reset = limit, reset
	if p.Scheme() == "op" {
		c.status.LastLimit, c.status.Reset = limit, reset
	}
	if c.clusterUsage() != nil {
		c.clusterUsage().PublishCooldown(p.Scheme(), reset)
	}
	writeCooldownFor(p.Scheme(), now, Cooldown{Limit: limit, Reset: reset, Strikes: ps.unclassified})
	return reset, limit
}

// rateLimit classifies a sanitized, lower-cased op diagnostic and, when it is
// a rate limit, records 1Password's cooldown, in memory and in the host's
// cooldown file. Caller holds c.mu.
func (c *Cache) rateLimit(now time.Time, lower string) (time.Time, string, bool) {
	rl := opRateLimit(lower)
	if rl == nil {
		return time.Time{}, "", false
	}
	p := c.provider("op")
	reset, limit := c.cooldownLocked(now, p, c.state["op"], rl)
	return reset, limit, true
}

// adoptCooldownLocked takes the provider's persisted cooldown into this cache
// when it ends later than the one in memory. Caller holds c.mu.
func (c *Cache) adoptCooldownLocked(now time.Time, p Provider, ps *pstate) {
	cd, ok := readCooldownFor(p.Scheme(), now)
	if c.clusterUsage() != nil {
		reset, node := c.clusterUsage().PeerCooldown(p.Scheme())
		if reset.After(cd.Reset) && reset.After(ps.reset) && now.Before(reset) {
			cd, ok = Cooldown{Limit: "learned from peer " + node + " rate limit", Reset: reset}, true
			LogReadProvider(now, p.Scheme(), "hevd cluster peer "+node, "cooldown", nil, "peer")
		}
	}
	if !ok {
		return
	}
	ps.unclassified = max(ps.unclassified, cd.Strikes)
	if now.Before(cd.Reset) && cd.Reset.After(ps.reset) {
		ps.lastLimit, ps.reset = cd.Limit, cd.Reset
		if p.Scheme() == "op" {
			c.status.LastLimit, c.status.Reset = cd.Limit, cd.Reset
		}
	}
}

// LocalStatus returns the counters and the last known quota rows without
// asking 1Password anything.
func (c *Cache) LocalStatus() Status { c.loadMeter(); return c.withQuota(c.counters()) }

func (c *Cache) counters() Status {
	c.mu.Lock()
	now := c.now()
	for _, p := range c.providers {
		c.adoptCooldownLocked(now, p, c.state[p.Scheme()])
	}
	s := c.status
	keep := c.reads[:0]
	for _, t := range c.reads {
		if now.Sub(t) < 24*time.Hour {
			keep = append(keep, t)
			s.ReadsDay++
			if now.Sub(t) < time.Hour {
				s.ReadsHour++
			}
		}
	}
	c.reads = keep
	for _, p := range c.providers {
		ps := c.state[p.Scheme()]
		row := ProviderStatus{Scheme: p.Scheme(), Name: p.Name(), Reachable: "unknown",
			LastOutcome: ps.lastOutcome, LastAt: ps.lastAt, Hits: ps.hits, Misses: ps.misses,
			Requests: ps.requests, RateLimited: ps.rateLimited}
		keep := ps.reads[:0]
		for _, t := range ps.reads {
			if now.Sub(t) < 24*time.Hour {
				keep = append(keep, t)
				row.ReadsDay++
				if now.Sub(t) < time.Hour {
					row.ReadsHour++
				}
			}
		}
		ps.reads = keep
		switch ps.lastOutcome {
		case "ok":
			row.Reachable = "yes"
		case "rate_limited":
			row.Reachable = "yes (throttled)"
		case "error":
			row.Reachable = "no"
		}
		if now.Before(ps.reset) {
			row.LastLimit, row.Reset = ps.lastLimit, ps.reset
		}
		s.Providers = append(s.Providers, row)
	}
	c.mu.Unlock()
	// Credential checks read only the environment and files, never the network.
	for i, p := range c.providers {
		row := &s.Providers[i]
		if cred, err := p.Credential(); err == nil {
			row.Configured, row.Credential = true, cred
		} else {
			row.Setup = err.Error()
		}
		row.Quota = "none"
		if q, ok := p.(interface{ quotaNote() string }); ok {
			row.Quota = q.quotaNote()
		}
		if t, ok := p.(interface{ tokenRequests() int }); ok {
			row.TokenRequests = t.tokenRequests()
		}
	}
	s.Budget = budgetStatus(now)
	if c.clusterUsage() != nil {
		for _, provider := range []string{"op", "azkv"} {
			a := c.clusterUsage().Usage(provider, now)
			if p, err := hostPolicy(); err == nil {
				a.Daily = p.AccountDaily[provider]
				a.Hourly = p.AccountHourly[provider]
			}
			s.Accounts = append(s.Accounts, a)
		}
	}
	c.mu.Lock()
	s.NotifyError = c.notifyError
	c.mu.Unlock()
	s.MeterNote = "meter costs one request under provider CLI request-count rules; scheduled polling off by default; designated meter host defaults to two hours; status is free unless --refresh-quota"
	s.ReadLog = ReadLogPath()
	return s
}

// Status returns the counters and, at most once every quotaEvery and never in
// a cooldown, asks op for the account's quota rows (one real request).
func (c *Cache) Status(ctx context.Context) Status {
	c.loadMeter()
	defer func() { c.warnings(ctx, c.LocalStatus()) }()
	s := c.counters()
	now := c.now()
	// Parse and expose only whitelisted quota fields, never raw CLI output.
	// The quota is asked at most once every quotaEvery and never in cooldown;
	// a concurrent status call reuses the last answer instead of waiting.
	if !c.quotaMu.TryLock() {
		return c.withQuota(s)
	}
	defer c.quotaMu.Unlock()
	token := serviceToken()
	c.mu.Lock()
	recent := !c.quotaAt.IsZero() && now.Sub(c.quotaAt) < quotaEvery
	c.mu.Unlock()
	if token == "" || now.Before(s.Reset) || recent {
		c.mu.Lock()
		if token == "" {
			c.quotaError = "no service-account credential configured"
		} else if now.Before(s.Reset) {
			c.quotaError = "provider cooldown; meter skipped"
		}
		c.mu.Unlock()
		return c.withQuota(s)
	}
	if err := c.reserve(now, "hev secrets meter", "op", 1); err != nil {
		c.mu.Lock()
		c.quotaError = err.Error()
		c.mu.Unlock()
		return c.withQuota(s)
	}
	c.mu.Lock()
	c.status.Requests++
	s.Requests = c.status.Requests
	if ps := c.state["op"]; ps != nil {
		ps.requests++
		ps.lastAt = now
		for i := range s.Providers {
			if s.Providers[i].Scheme == "op" {
				s.Providers[i].Requests = ps.requests
			}
		}
	}
	c.mu.Unlock()
	cmd := exec.CommandContext(ctx, c.op, "service-account", "ratelimit", "--format=json")
	cmd.Env = opEnv(token)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	b, err := cmd.Output()
	outcome := "error"
	c.mu.Lock()
	c.quotaAt = now
	c.quotaError = "meter command failed (unsupported CLI or provider error)"
	if err != nil {
		// A quota check is a request too: a rate-limit answer starts the
		// same host-wide cooldown as a read.
		lower := strings.ToLower(refPattern.ReplaceAllString(stderr.String(), ""))
		if _, _, ok := c.rateLimit(now, lower); ok {
			c.status.RateLimited++
			c.state["op"].rateLimited++
			outcome = "rate_limited"
		}
	} else {
		clearCooldown(now)
		var raw any
		if json.Unmarshal(b, &raw) == nil {
			outcome = "ok"
			safe := map[string]float64{}
			collectNumbers("", raw, safe)
			v, _ := json.Marshal(safe)
			c.quotaFlat = string(v)
			c.quota = quotaRows(raw, now)
			c.quotaSuccessAt = now
			c.quotaError = ""
			if len(c.quota) == 0 {
				c.quotaError = "meter returned no supported account/token rows"
			}
		}
	}
	c.mu.Unlock()
	c.saveMeter()
	LogReadProvider(now, "op", "hev secrets status", "ratelimit", nil, outcome)
	return c.withQuota(s)
}

func (c *Cache) withQuota(s Status) Status {
	s.Budget = budgetStatus(c.now())
	c.mu.Lock()
	defer c.mu.Unlock()
	s.AccountLimits = "unavailable"
	if c.quotaFlat != "" {
		s.AccountLimits = c.quotaFlat
	}
	s.Quota = append([]QuotaRow(nil), c.quota...)
	s.QuotaCheckedAt = c.quotaSuccessAt
	if !s.QuotaCheckedAt.IsZero() {
		s.QuotaAgeSeconds = max(0, int64(c.now().Sub(s.QuotaCheckedAt).Seconds()))
	}
	s.QuotaUnavailable = c.quotaError
	if len(s.Quota) == 0 && s.QuotaUnavailable == "" {
		s.QuotaUnavailable = "meter not checked yet"
	}
	return s
}

var safePhrase = regexp.MustCompile(`^[A-Za-z0-9 :./-]{1,40}$`)

// quotaRows keeps only rows whose type and action are known labels. A numeric
// reset is read as seconds from now; an RFC3339 reset is kept; any other reset
// is kept only as a short phrase made of safe characters ("N/A",
// "23 hours from now").
func quotaRows(raw any, now time.Time) []QuotaRow {
	list, ok := raw.([]any)
	if !ok {
		return nil
	}
	known := map[string]bool{"token": true, "account": true, "read": true, "write": true, "read_write": true}
	num := func(v any) *int64 {
		if f, ok := v.(float64); ok {
			n := int64(f)
			return &n
		}
		return nil
	}
	rows := []QuotaRow{}
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		typ, _ := m["type"].(string)
		act, _ := m["action"].(string)
		if !known[typ] || !known[act] {
			continue
		}
		r := QuotaRow{Type: typ, Action: act, Limit: num(m["limit"]), Used: num(m["used"]), Remaining: num(m["remaining"])}
		switch v := m["reset"].(type) {
		case float64:
			if v > 0 && v <= 7*24*3600 {
				r.Reset = now.Add(time.Duration(v) * time.Second).UTC().Format(time.RFC3339)
			}
		case string:
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				r.Reset = t.UTC().Format(time.RFC3339)
			} else if safePhrase.MatchString(v) {
				r.Reset = v
			}
		}
		rows = append(rows, r)
	}
	return rows
}

func collectNumbers(prefix string, v any, out map[string]float64) {
	switch x := v.(type) {
	case []any:
		for i, row := range x {
			collectNumbers(fmt.Sprintf("row%d.", i), row, out)
		}
	case map[string]any:
		for k, v := range x { // Only known quota labels are exposed.
			switch k {
			case "limit", "remaining", "used", "reset", "hourly", "daily", "account", "token":
				collectNumbers(prefix+k+".", v, out)
			}
		}
	case float64:
		out[strings.TrimSuffix(prefix, ".")] = x
	}
}

// Expired values stay in RAM only. Any non-rate error evicts them. A stale
// result is atomic: every requested reference must already be held.
func (c *Cache) canStaleLocked(refs []Ref) bool {
	p, err := hostPolicy()
	if err != nil || p.DisableStale {
		return false
	}
	for _, r := range refs {
		if _, ok := c.entries[r.URI]; !ok {
			return false
		}
	}
	return true
}
func (c *Cache) staleLocked(now time.Time, caller string, p Provider, refs []Ref, out, sources map[string]string) bool {
	if !c.canStaleLocked(refs) {
		return false
	}
	labels := []string{}
	for _, r := range refs {
		out[r.Name] = c.entries[r.URI].value
		sources[r.Name] = SourceStale
		labels = append(labels, Label(r.Name, r.URI))
	}
	c.status.Stale++
	LogReadProvider(now, p.Scheme(), caller, "stale", labels, "stale")
	return true
}
