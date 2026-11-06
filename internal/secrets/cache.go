// Package secrets resolves service-account references without persisting values.
package secrets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"regexp"
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
	Hits          int       `json:"hits"`
	Misses        int       `json:"misses"`
	ReadsHour     int       `json:"reads_hour"`
	ReadsDay      int       `json:"reads_day"`
	LastLimit     string    `json:"last_limit,omitempty"`
	Reset         time.Time `json:"reset,omitempty"`
	AccountLimits string    `json:"account_limits"`
}
type Cache struct {
	mu      sync.Mutex
	entries map[string]entry
	reads   []time.Time
	status  Status
	now     func() time.Time
	op      string
}

func New() *Cache { return &Cache{entries: map[string]entry{}, now: time.Now, op: "op"} }

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
	if err := Validate(refs); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	out := map[string]string{}
	missing := []Ref{}
	for k, e := range c.entries {
		if !now.Before(e.expires) {
			delete(c.entries, k)
		}
	}
	for _, r := range refs {
		if e, ok := c.entries[r.URI]; ok && r.TTL > 0 && now.Before(e.created.Add(r.TTL)) {
			out[r.Name] = e.value
			c.status.Hits++
		} else {
			missing = append(missing, r)
			c.status.Misses++
		}
	}
	if len(missing) == 0 {
		return out, nil
	}
	if now.Before(c.status.Reset) {
		return nil, fmt.Errorf("1Password %s; retry after %s", c.status.LastLimit, c.status.Reset.UTC().Format(time.RFC3339))
	}
	// A random boundary frames arbitrary multiline values without escaping.
	// The template and injected result travel only through anonymous pipes.
	if os.Getenv("OP_SERVICE_ACCOUNT_TOKEN") == "" {
		return nil, errors.New("1Password service-account token is required; desktop unlock disabled")
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
	c.reads = append(c.reads, now)
	opCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(opCtx, c.op, "inject")
	cmd.Env = append(os.Environ(), "OP_BIOMETRIC_UNLOCK_ENABLED=false")
	cmd.Stdin = strings.NewReader(template.String())
	var stderr strings.Builder
	cmd.Stderr = &stderr
	data, err := cmd.Output()
	if err != nil {
		// Never return raw op output: it can echo injected values or references.
		lower := strings.ToLower(regexp.MustCompile(`op://[^\s"']+`).ReplaceAllString(stderr.String(), ""))
		if strings.Contains(lower, "rate limit") || strings.Contains(lower, "too many requests") || strings.Contains(lower, "rate-limited") {
			limit := "rate limit"
			if strings.Contains(lower, "hourly limit") || strings.Contains(lower, "hourly rate limit") {
				limit = "hourly rate limit"
			}
			if strings.Contains(lower, "daily limit") || strings.Contains(lower, "daily rate limit") {
				limit = "daily rate limit"
			}
			delay := time.Minute
			if limit == "hourly rate limit" {
				delay = time.Hour
			}
			if limit == "daily rate limit" {
				delay = 24 * time.Hour
			}
			c.status.LastLimit = limit
			c.status.Reset = now.Add(delay + time.Duration(rand.IntN(30))*time.Second)
			// Accept only explicit reset/retry timestamps, bounded by the limit class.
			stamps := regexp.MustCompile(`(?:reset|retry)(?: at| after| time)?[:= ]+(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z)`).FindStringSubmatch(lower)
			if len(stamps) == 2 {
				if reset, e := time.Parse(time.RFC3339, strings.ToUpper(stamps[1])); e == nil && reset.After(now) && reset.Before(now.Add(delay)) {
					c.status.Reset = reset.Add(time.Duration(rand.IntN(30)) * time.Second)
				}
			}
			return nil, fmt.Errorf("1Password %s; retry after %s", limit, c.status.Reset.UTC().Format(time.RFC3339))
		}
		return nil, errors.New("1Password secret resolution failed (service account required; desktop unlock disabled)")
	}
	parts := strings.Split(string(data), boundary)
	if len(parts) != len(unique)+1 || strings.TrimSpace(parts[len(parts)-1]) != "" {
		return nil, errors.New("1Password returned invalid batch framing")
	}
	for _, r := range missing {
		value := parts[indices[r.URI]]
		out[r.Name] = value
		if r.TTL > 0 {
			expiry := now.Add(r.TTL)
			if old, ok := c.entries[r.URI]; ok && old.expires.Before(expiry) {
				expiry = old.expires
			}
			c.entries[r.URI] = entry{value: value, expires: expiry, created: now}
		}
	}
	return out, nil
}
func (c *Cache) Status(ctx context.Context) Status {
	c.mu.Lock()
	s := c.status
	now := c.now()
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
	// Parse and expose only numeric quota fields, never raw CLI output.
	if os.Getenv("OP_SERVICE_ACCOUNT_TOKEN") == "" || now.Before(s.Reset) {
		s.AccountLimits = "unavailable"
		return s
	}
	cmd := exec.CommandContext(ctx, c.op, "service-account", "ratelimit", "--format=json")
	cmd.Env = append(os.Environ(), "OP_BIOMETRIC_UNLOCK_ENABLED=false")
	b, err := cmd.Output()
	s.AccountLimits = "unavailable"
	if err == nil {
		var raw any
		if json.Unmarshal(b, &raw) == nil {
			safe := map[string]float64{}
			collectNumbers("", raw, safe)
			v, _ := json.Marshal(safe)
			s.AccountLimits = string(v)
		}
	}
	return s
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
