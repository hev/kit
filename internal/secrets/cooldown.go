package secrets

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// The cooldown file carries a rate-limit answer across processes, so that no
// kit process on the host sends another 1Password request before the reset
// time: not a restarted hevd, not a fresh direct-fallback client. It holds
// only the limit class, the reset time and the count of consecutive
// unclassified limits. Never a value, a token or a reference. It is written on
// a rate-limit answer, honoured before every request, and removed by the first
// successful request after it expires.
//
// HEV_SECRETS_COOLDOWN selects the file; "off" disables it. The default is
// op-cooldown.json beside the read log (~/.hev/secrets/), mode 0600 in a
// 0700 directory.
//
// The cooldown is per provider: 1Password's is op-cooldown.json (the native
// gate reads and writes it too); every other provider's is
// SCHEME-cooldown.json in the same directory (azkv-cooldown.json for Key
// Vault), so a throttled vault never holds back 1Password reads, or the
// reverse.
const CooldownEnv = "HEV_SECRETS_COOLDOWN"

// Cooldown is the persisted, non-secret state of a rate-limit answer.
type Cooldown struct {
	Limit string    `json:"limit"`
	Reset time.Time `json:"reset"`
	// Strikes counts consecutive unclassified limits, so their backoff keeps
	// doubling across process restarts instead of starting at one minute.
	Strikes int `json:"strikes,omitempty"`
}

var cooldownMu sync.Mutex

// CooldownPath is the host's cooldown file, or "" when disabled.
func CooldownPath() string {
	if p := os.Getenv(CooldownEnv); p != "" {
		if p == "off" {
			return ""
		}
		return filepath.Clean(p)
	}
	if p := os.Getenv("HEV_SECRETS_READ_LOG"); p != "" && p != "off" {
		return filepath.Join(filepath.Dir(p), "op-cooldown.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".hev", "secrets", "op-cooldown.json")
}

// cooldownPathFor is the cooldown file of the provider with this scheme.
func cooldownPathFor(scheme string) string {
	p := CooldownPath()
	if p == "" || scheme == "op" {
		return p
	}
	return filepath.Join(filepath.Dir(p), scheme+"-cooldown.json")
}

var knownLimits = map[string]bool{"rate limit": true, "hourly rate limit": true, "daily rate limit": true}

// readCooldown returns the persisted cooldown. A file that is missing,
// malformed, of an unknown class or with a reset beyond any limit's bound
// (24 hours plus jitter) is ignored: it cannot extend a cooldown forever.
func readCooldown(now time.Time) (Cooldown, bool) { return readCooldownFor("op", now) }

func readCooldownFor(scheme string, now time.Time) (Cooldown, bool) {
	p := cooldownPathFor(scheme)
	if p == "" {
		return Cooldown{}, false
	}
	b, err := os.ReadFile(p)
	if err != nil || len(b) > 4096 {
		return Cooldown{}, false
	}
	var c Cooldown
	if json.Unmarshal(b, &c) != nil || !knownLimits[c.Limit] || c.Reset.IsZero() || c.Reset.After(now.Add(24*time.Hour+time.Minute)) || c.Strikes < 0 || c.Strikes > 64 {
		return Cooldown{}, false
	}
	return c, true
}

// writeCooldown persists c unless the file already holds a later reset.
// Failures never block resolution: the in-memory cooldown still holds.
func writeCooldown(now time.Time, c Cooldown) { writeCooldownFor("op", now, c) }

func writeCooldownFor(scheme string, now time.Time, c Cooldown) {
	p := cooldownPathFor(scheme)
	if p == "" || !knownLimits[c.Limit] {
		return
	}
	cooldownMu.Lock()
	defer cooldownMu.Unlock()
	if old, ok := readCooldownFor(scheme, now); ok && old.Reset.After(c.Reset) {
		return
	}
	if os.MkdirAll(filepath.Dir(p), 0o700) != nil {
		return
	}
	b, _ := json.Marshal(Cooldown{Limit: c.Limit, Reset: c.Reset.UTC(), Strikes: c.Strikes})
	tmp, err := os.CreateTemp(filepath.Dir(p), "."+scheme+"-cooldown-*")
	if err != nil {
		return
	}
	_, werr := tmp.Write(append(b, '\n'))
	cerr := tmp.Close()
	if werr != nil || cerr != nil || os.Chmod(tmp.Name(), 0o600) != nil || os.Rename(tmp.Name(), p) != nil {
		os.Remove(tmp.Name())
	}
}

// clearCooldown removes an expired cooldown after a successful request. A
// cooldown another process wrote since (reset still ahead) is kept.
func clearCooldown(now time.Time) { clearCooldownFor("op", now) }

func clearCooldownFor(scheme string, now time.Time) {
	p := cooldownPathFor(scheme)
	if p == "" {
		return
	}
	cooldownMu.Lock()
	defer cooldownMu.Unlock()
	if c, ok := readCooldownFor(scheme, now); ok && now.Before(c.Reset) {
		return
	}
	if _, err := os.Stat(p); err == nil {
		os.Remove(p)
	}
}

func cooldownError(limit string, reset time.Time) error {
	return providerCooldownError("1Password", limit, reset)
}

func providerCooldownError(name, limit string, reset time.Time) error {
	return fmt.Errorf("%s %s; retry after %s", name, limit, reset.UTC().Format(time.RFC3339))
}

// ActiveCooldownFor is ActiveCooldown for the provider with this scheme.
func ActiveCooldownFor(scheme string, now time.Time) (Cooldown, bool) {
	c, ok := readCooldownFor(scheme, now)
	if !ok || !now.Before(c.Reset) {
		return Cooldown{}, false
	}
	return c, true
}

// ActiveCooldown returns the host's persisted cooldown when its reset is
// still ahead of now. It never makes a request.
func ActiveCooldown(now time.Time) (Cooldown, bool) {
	c, ok := readCooldown(now)
	if !ok || !now.Before(c.Reset) {
		return Cooldown{}, false
	}
	return c, true
}

// CheckCooldown returns the rate-limit error a request would get now, or nil
// when the host may send one. A caller that runs op itself (kit's direct
// Layer-key read) calls it first.
func CheckCooldown(now time.Time) error {
	if c, ok := ActiveCooldown(now); ok {
		return cooldownError(c.Limit, c.Reset)
	}
	return nil
}

var refPattern = regexp.MustCompile(`op://[^\s"']+`)

// NoteDirectResult records the outcome of a request a caller made with its
// own op: on a rate-limit diagnostic it persists the cooldown and returns the
// rate-limit error; on success (stderr ignored, ok true) it clears an expired
// cooldown. The diagnostic is classified only; it is never stored or returned.
func NoteDirectResult(now time.Time, ok bool, stderr string) error {
	if ok {
		clearCooldown(now)
		return nil
	}
	lower := strings.ToLower(refPattern.ReplaceAllString(stderr, ""))
	c := New()
	c.now = func() time.Time { return now }
	if reset, limit, isLimit := c.rateLimit(now, lower); isLimit {
		return cooldownError(limit, reset)
	}
	return nil
}

// CooldownReset returns the reset time in a rate-limit error from this
// package, or the zero time.
func CooldownReset(err error) time.Time {
	if !IsRateLimit(err) {
		return time.Time{}
	}
	_, after, _ := strings.Cut(err.Error(), "retry after ")
	t, e := time.Parse(time.RFC3339, strings.TrimSpace(after))
	if e != nil {
		return time.Time{}
	}
	return t
}
