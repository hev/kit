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
	"strconv"
	"strings"
	"sync"
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
	Hits      int       `json:"hits"`
	Misses    int       `json:"misses"`
	ReadsHour int       `json:"reads_hour"`
	ReadsDay  int       `json:"reads_day"`
	LastLimit string    `json:"last_limit,omitempty"`
	Reset     time.Time `json:"reset,omitempty"`
	// AccountLimits keeps the earlier flattened numeric view of the quota.
	AccountLimits string `json:"account_limits"`
	// StartedAt and Requests count every real 1Password request (inject
	// batches and quota checks) this process made since it started.
	StartedAt   time.Time `json:"started_at"`
	Requests    int       `json:"requests"`
	RateLimited int       `json:"rate_limited"`
	// Quota holds the account's rate-limit rows as last reported by op,
	// checked at most once every quotaEvery.
	Quota          []QuotaRow `json:"quota,omitempty"`
	QuotaCheckedAt time.Time  `json:"quota_checked_at,omitempty"`
	// HostReadsDay counts real requests in the host's read log over the last
	// 24 hours, from every process on the host, not just this one.
	HostReadsDay *int   `json:"host_reads_day,omitempty"`
	ReadLog      string `json:"read_log,omitempty"`
	Daemon       bool   `json:"daemon"`
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
type Cache struct {
	mu      sync.Mutex
	entries map[string]entry
	reads   []time.Time
	status  Status
	now     func() time.Time
	op      string
	// unclassified counts consecutive rate limits with no class, so their
	// cooldown doubles instead of retrying every minute all day.
	unclassified int
	// quotaMu serializes quota checks; the fields below it are guarded by mu.
	quotaMu   sync.Mutex
	quota     []QuotaRow
	quotaFlat string
	quotaAt   time.Time
}

// quotaEvery bounds how often status may ask op for the account's quota.
const quotaEvery = 5 * time.Minute

func New() *Cache {
	c := &Cache{entries: map[string]entry{}, now: time.Now, op: "op"}
	c.status.StartedAt = c.now()
	return c
}

var namePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func Validate(refs []Ref) error {
	seen := map[string]bool{}
	if len(refs) > 128 {
		return errors.New("too many secret references")
	}
	for _, r := range refs {
		if !namePattern.MatchString(r.Name) || seen[r.Name] || !strings.HasPrefix(r.URI, "op://") || strings.ContainsAny(r.URI, "\r\n{}\x00") || r.TTL < 0 || r.TTL > 24*time.Hour {
			return errors.New("invalid secret reference, name or TTL (maximum 24h)")
		}
		seen[r.Name] = true
	}
	return nil
}

// Holding the resolver lock serializes misses, deduplicating concurrent batches.
// No stale values are served: callers may shorten, but never extend, stored TTL.
func (c *Cache) Resolve(ctx context.Context, refs []Ref) (map[string]string, error) {
	return c.ResolveAs(ctx, "", refs)
}

// Per-reference dispositions. They name where a value came from, never the
// value, so a wrapper can keep an accurate label-only log of hits and reads.
const (
	// SourceCache: served from memory; no 1Password request.
	SourceCache = "cache"
	// SourceRead: part of the one real 1Password request this call made.
	SourceRead = "read"
)

// ResolveAs resolves refs and attributes any real request to caller in the
// host's read log.
func (c *Cache) ResolveAs(ctx context.Context, caller string, refs []Ref) (map[string]string, error) {
	values, _, err := c.ResolveSources(ctx, caller, refs)
	return values, err
}

// ResolveSources is ResolveAs plus each reference's disposition by name
// (SourceCache or SourceRead).
func (c *Cache) ResolveSources(ctx context.Context, caller string, refs []Ref) (map[string]string, map[string]string, error) {
	if err := Validate(refs); err != nil {
		return nil, nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	out := map[string]string{}
	sources := map[string]string{}
	missing := []Ref{}
	for k, e := range c.entries {
		if !now.Before(e.expires) {
			delete(c.entries, k)
		}
	}
	for _, r := range refs {
		if e, ok := c.entries[r.URI]; ok && r.TTL > 0 && now.Before(e.created.Add(r.TTL)) {
			out[r.Name] = e.value
			sources[r.Name] = SourceCache
			c.status.Hits++
		} else {
			missing = append(missing, r)
			c.status.Misses++
		}
	}
	if len(missing) == 0 {
		return out, sources, nil
	}
	// The host's cooldown file carries a rate limit another process (or an
	// earlier run of this one) was answered with.
	c.adoptCooldown(now)
	if now.Before(c.status.Reset) {
		return nil, nil, cooldownError(c.status.LastLimit, c.status.Reset)
	}
	// A random boundary frames arbitrary multiline values without escaping.
	// The template and injected result travel only through anonymous pipes.
	token := serviceToken()
	if token == "" {
		return nil, nil, errors.New("1Password service-account token is required; desktop unlock disabled")
	}
	boundary := fmt.Sprintf("HEV-%016x-%016x", rand.Uint64(), rand.Uint64())
	var template strings.Builder
	unique := []Ref{}
	indices := map[string]int{}
	for _, r := range missing {
		if _, ok := indices[r.URI]; !ok {
			indices[r.URI] = len(unique)
			unique = append(unique, r)
			template.WriteString("{{ " + r.URI + " }}" + boundary)
		}
	}
	labels := make([]string, len(unique))
	for i, r := range unique {
		labels[i] = Label(r.Name, r.URI)
	}
	c.reads = append(c.reads, now)
	c.status.Requests++
	opCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(opCtx, c.op, "inject")
	cmd.Env = opEnv(token)
	cmd.Stdin = strings.NewReader(template.String())
	var stderr strings.Builder
	cmd.Stderr = &stderr
	data, err := cmd.Output()
	if err != nil {
		// Never return raw op output: it can echo injected values or references.
		lower := strings.ToLower(refPattern.ReplaceAllString(stderr.String(), ""))
		if reset, limit, ok := c.rateLimit(now, lower); ok {
			c.status.RateLimited++
			LogRead(now, caller, "inject", labels, "rate_limited")
			return nil, nil, fmt.Errorf("1Password %s; retry after %s", limit, reset.UTC().Format(time.RFC3339))
		}
		LogRead(now, caller, "inject", labels, "error")
		return nil, nil, errors.New("1Password secret resolution failed (service account required; desktop unlock disabled)")
	}
	LogRead(now, caller, "inject", labels, "ok")
	c.unclassified = 0
	clearCooldown(now)
	parts := strings.Split(string(data), boundary)
	if len(parts) != len(unique)+1 || strings.TrimSpace(parts[len(parts)-1]) != "" {
		return nil, nil, errors.New("1Password returned invalid batch framing")
	}
	for _, r := range missing {
		value := parts[indices[r.URI]]
		out[r.Name] = value
		sources[r.Name] = SourceRead
		if r.TTL > 0 {
			expiry := now.Add(r.TTL)
			if old, ok := c.entries[r.URI]; ok && old.expires.Before(expiry) {
				expiry = old.expires
			}
			c.entries[r.URI] = entry{value: value, expires: expiry, created: now}
		}
	}
	return out, sources, nil
}

var (
	resetStamp = regexp.MustCompile(`(?i)(?:reset|retry)(?: at| after| time)?[:= ]+(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z)`)
	retryIn    = regexp.MustCompile(`(?:try again|retry) in (\d{1,6}) ?(seconds?|secs?|s|minutes?|mins?|m|hours?|h)\b`)
)

// rateLimit classifies a sanitized, lower-cased op diagnostic and, when it is
// a rate limit, records one shared cooldown. Caller holds c.mu.
func (c *Cache) rateLimit(now time.Time, lower string) (time.Time, string, bool) {
	if !strings.Contains(lower, "rate limit") && !strings.Contains(lower, "too many requests") && !strings.Contains(lower, "rate-limited") {
		return time.Time{}, "", false
	}
	c.adoptCooldown(now)
	limit := "rate limit"
	if strings.Contains(lower, "hourly limit") || strings.Contains(lower, "hourly rate limit") {
		limit = "hourly rate limit"
	}
	if strings.Contains(lower, "daily limit") || strings.Contains(lower, "daily rate limit") {
		limit = "daily rate limit"
	}
	delay, bound := time.Minute, 24*time.Hour
	switch limit {
	case "hourly rate limit":
		delay, bound = time.Hour, time.Hour
	case "daily rate limit":
		delay = 24 * time.Hour
	default:
		// An unclassified limit may be the daily account quota. Back off
		// exponentially across consecutive answers, capped at one hour.
		delay = min(time.Minute<<min(c.unclassified, 6), time.Hour)
		c.unclassified++
	}
	jitter := time.Duration(rand.IntN(30)) * time.Second
	c.status.LastLimit = limit
	c.status.Reset = now.Add(delay + jitter)
	// Accept only explicit reset/retry times, bounded by the limit class.
	if m := resetStamp.FindStringSubmatch(lower); len(m) == 2 {
		if reset, e := time.Parse(time.RFC3339, strings.ToUpper(m[1])); e == nil && reset.After(now) && !reset.After(now.Add(bound)) {
			c.status.Reset = reset.Add(jitter)
		}
	} else if m := retryIn.FindStringSubmatch(lower); len(m) == 3 {
		n, _ := strconv.Atoi(m[1])
		unit := time.Second
		switch m[2][0] {
		case 'm':
			unit = time.Minute
		case 'h':
			unit = time.Hour
		}
		if d := time.Duration(n) * unit; d > 0 && d <= bound && d > delay {
			c.status.Reset = now.Add(d + jitter)
		}
	}
	writeCooldown(now, Cooldown{Limit: limit, Reset: c.status.Reset, Strikes: c.unclassified})
	return c.status.Reset, limit, true
}

// adoptCooldown takes the host's persisted cooldown into this cache when it
// ends later than the one in memory. Caller holds c.mu.
func (c *Cache) adoptCooldown(now time.Time) {
	cd, ok := readCooldown(now)
	if !ok {
		return
	}
	c.unclassified = max(c.unclassified, cd.Strikes)
	if now.Before(cd.Reset) && cd.Reset.After(c.status.Reset) {
		c.status.LastLimit = cd.Limit
		c.status.Reset = cd.Reset
	}
}

// LocalStatus returns the counters and the last known quota rows without
// asking 1Password anything.
func (c *Cache) LocalStatus() Status { return c.withQuota(c.counters()) }

func (c *Cache) counters() Status {
	c.mu.Lock()
	now := c.now()
	c.adoptCooldown(now)
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
	c.mu.Unlock()
	s.ReadLog = ReadLogPath()
	return s
}

// Status returns the counters and, at most once every quotaEvery and never in
// a cooldown, asks op for the account's quota rows (one real request).
func (c *Cache) Status(ctx context.Context) Status {
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
		return c.withQuota(s)
	}
	c.mu.Lock()
	c.status.Requests++
	s.Requests = c.status.Requests
	c.mu.Unlock()
	cmd := exec.CommandContext(ctx, c.op, "service-account", "ratelimit", "--format=json")
	cmd.Env = opEnv(token)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	b, err := cmd.Output()
	outcome := "error"
	c.mu.Lock()
	c.quotaAt = now
	if err != nil {
		// A quota check is a request too: a rate-limit answer starts the
		// same host-wide cooldown as a read.
		lower := strings.ToLower(refPattern.ReplaceAllString(stderr.String(), ""))
		if _, _, ok := c.rateLimit(now, lower); ok {
			c.status.RateLimited++
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
		}
	}
	c.mu.Unlock()
	LogRead(now, "hev secrets status", "ratelimit", nil, outcome)
	return c.withQuota(s)
}

func (c *Cache) withQuota(s Status) Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	s.AccountLimits = "unavailable"
	if c.quotaFlat != "" {
		s.AccountLimits = c.quotaFlat
	}
	s.Quota = append([]QuotaRow(nil), c.quota...)
	s.QuotaCheckedAt = c.quotaAt
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
