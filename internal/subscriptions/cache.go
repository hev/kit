package subscriptions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const claudeCacheSchema = "hev.subscriptions.claude-cache/v1"

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

// ClaudeCachePath is where the Claude reading is cached.
func ClaudeCachePath(env Env) string {
	return filepath.Join(env.StateDir, "subscriptions", "claude.json")
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
	path := ClaudeCachePath(env)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".claude-*.json")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(append(b, '\n')); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
