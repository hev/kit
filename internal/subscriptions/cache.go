package subscriptions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const (
	claudeCacheSchema  = "hev.subscriptions.claude-cache/v1"
	agyCacheSchema     = "hev.subscriptions.agy-cache/v1"
	agyMaxCachedModels = 64
)

// claudeCache is the last Claude usage reading and the last request attempt.
// It holds meters, the plan tier and timestamps only: never a token, account
// identifier or response body.
type claudeCache struct {
	Schema        string    `json:"schema"`
	ObservedAt    time.Time `json:"observed_at,omitempty"`
	Plan          string    `json:"plan,omitempty"`
	Meters        []Meter   `json:"meters,omitempty"`
	LastAttemptAt time.Time `json:"last_attempt_at"`
	LastError     string    `json:"last_error,omitempty"`
	LastStatus    int       `json:"last_status,omitempty"`
}

func (c *claudeCache) LastAttempt() time.Time {
	if c.LastAttemptAt.IsZero() {
		return c.ObservedAt
	}
	return c.LastAttemptAt
}

// agyCache is the last Antigravity model/login probe and attempt timestamp.
// It holds sanitized model IDs, plan tier, login state and timestamps only:
// never a token, API key, email address, local path or raw command output.
type agyCache struct {
	Schema        string    `json:"schema"`
	ObservedAt    time.Time `json:"observed_at,omitempty"`
	Plan          string    `json:"plan,omitempty"`
	LoginState    string    `json:"login_state,omitempty"`
	UsedLocalFile bool      `json:"used_local_file,omitempty"`
	Models        []string  `json:"models,omitempty"`
	LastAttemptAt time.Time `json:"last_attempt_at"`
	LastError     string    `json:"last_error,omitempty"`
}

func (c *agyCache) LastAttempt() time.Time {
	if c.LastAttemptAt.IsZero() {
		return c.ObservedAt
	}
	return c.LastAttemptAt
}

// ClaudeCachePath is where the Claude reading is cached.
func ClaudeCachePath(env Env) string {
	return filepath.Join(env.StateDir, "subscriptions", "claude.json")
}

// AgyCachePath is where the Antigravity model/tier probe is cached.
func AgyCachePath(env Env) string {
	return filepath.Join(env.StateDir, "subscriptions", "agy.json")
}

func loadClaudeCache(env Env) (*claudeCache, error) {
	if env.StateDir == "" {
		return nil, os.ErrNotExist
	}
	b, err := os.ReadFile(ClaudeCachePath(env))
	if err != nil {
		return nil, err
	}
	var c claudeCache
	if err = json.Unmarshal(b, &c); err != nil || c.Schema != claudeCacheSchema {
		return nil, os.ErrNotExist
	}
	return &c, nil
}

func saveClaudeCache(env Env, c *claudeCache) error {
	if env.StateDir == "" {
		return nil
	}
	return writeCacheJSON(ClaudeCachePath(env), ".claude-*.json", c)
}

func loadAgyCache(env Env) (*agyCache, error) {
	if env.StateDir == "" {
		return nil, os.ErrNotExist
	}
	b, err := os.ReadFile(AgyCachePath(env))
	if err != nil {
		return nil, err
	}
	var c agyCache
	if err = json.Unmarshal(b, &c); err != nil || c.Schema != agyCacheSchema {
		return nil, os.ErrNotExist
	}
	c.Models = sanitizeAgyModels(c.Models)
	c.LastError = redactAgyText(c.LastError)
	return &c, nil
}

func saveAgyCache(env Env, c *agyCache) error {
	if env.StateDir == "" || c == nil {
		return nil
	}
	clean := *c
	clean.Schema = agyCacheSchema
	clean.Plan = normalizeAgyPlan(clean.Plan)
	clean.Models = sanitizeAgyModels(clean.Models)
	clean.LastError = redactAgyText(clean.LastError)
	return writeCacheJSON(AgyCachePath(env), ".agy-*.json", &clean)
}

func writeCacheJSON(path, pattern string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), pattern)
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0o600); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(append(b, '\n')); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
