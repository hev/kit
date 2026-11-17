package attention

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/hev/kit/internal/secrets"
)

// Slack is [notify.slack] in kit's config.toml. It names a channel and a
// secret reference, never a token value.
type Slack struct {
	// Channel is a user ID (U...), which opens the bot's own DM, or a
	// channel (#name or C...).
	Channel string `toml:"channel"`
	// Token is a secret reference resolved through hev secrets:
	// secret://NAME, op://... or azkv://...
	Token string `toml:"token"`
	// Kinds are the notice kinds posted; default ["needs-you"].
	Kinds []string `toml:"kinds"`
	// Tag, when set, is prepended to every message (for example "[test]").
	Tag string `toml:"tag"`
}

// Wants reports whether kind is posted.
func (s *Slack) Wants(kind string) bool {
	if s == nil {
		return false
	}
	for _, k := range s.Kinds {
		if k == kind {
			return true
		}
	}
	return false
}

// Config is the [notify] table.
type Config struct {
	// Client marks this host as a client island: its notices carry only the
	// run or orchestrator name and state wherever they are delivered.
	Client bool
	// Slack is nil when [notify.slack] is absent.
	Slack *Slack
	// Path is the file read.
	Path string
}

// ConfigPath is kit's config file: HEV_CONFIG or ~/.hev/config.toml.
func ConfigPath() string { return secrets.ConfigPath() }

// LoadConfig reads [notify] from path. A missing file is an empty config.
func LoadConfig(path string) (Config, error) {
	c := Config{Path: path}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return c, err
	}
	if !info.Mode().IsRegular() {
		return c, fmt.Errorf("config %s is not a regular file", path)
	}
	var f struct {
		Notify struct {
			Client bool   `toml:"client"`
			Slack  *Slack `toml:"slack"`
		} `toml:"notify"`
	}
	if _, err := toml.DecodeFile(path, &f); err != nil {
		return c, fmt.Errorf("config %s: %w", path, err)
	}
	c.Client = f.Notify.Client
	if s := f.Notify.Slack; s != nil {
		if err := s.validate(); err != nil {
			return c, fmt.Errorf("config %s [notify.slack]: %w", path, err)
		}
		c.Slack = s
	}
	return c, nil
}

func (s *Slack) validate() error {
	s.Channel = strings.TrimSpace(s.Channel)
	s.Token = strings.TrimSpace(s.Token)
	if s.Channel == "" {
		return errors.New("channel is required (a user ID such as U0123ABCD, or #channel)")
	}
	if s.Token == "" {
		return errors.New("token is required (secret://NAME, op://... or azkv://...)")
	}
	if !isSecretRef(s.Token) {
		// Never echo the value: it may be a pasted token.
		return errors.New("token must be a secret reference (secret://NAME, op://... or azkv://...), not a token value")
	}
	if len(s.Kinds) == 0 {
		s.Kinds = []string{KindNeedsYou}
	}
	for _, k := range s.Kinds {
		if !ValidKind(k) {
			return fmt.Errorf("unknown kind %q (needs-you, ready, done)", k)
		}
	}
	return nil
}

func isSecretRef(s string) bool {
	for _, p := range []string{"secret://", "op://", "azkv://"} {
		if strings.HasPrefix(s, p) && len(s) > len(p) {
			return true
		}
	}
	return false
}
