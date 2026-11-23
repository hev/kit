// Package hygiene implements bounded host disk-pressure and cache hygiene for hevd.
package hygiene

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/BurntSushi/toml"
)

const (
	DefaultEvery         = 6 * time.Hour
	MinEvery             = 15 * time.Minute
	DefaultMinFreeGB     = 40
	DefaultCompactAfter  = 24 * time.Hour
	DefaultPruneTmpAfter = 24 * time.Hour
	BytesPerGiB          = uint64(1 << 30)
)

// Config holds the [hygiene] section of ~/.hev/config.toml.
type Config struct {
	Enabled           bool   `toml:"enabled" json:"enabled"`
	Every             string `toml:"every" json:"every"`
	MinFreeGB         *int   `toml:"min_free_gb,omitempty" json:"min_free_gb"`
	CompactAfter      string `toml:"compact_after" json:"compact_after"`
	PruneTmpAfter     string `toml:"prune_tmp_after" json:"prune_tmp_after"`
	ThinAPFSSnapshots *bool  `toml:"thin_apfs_snapshots,omitempty" json:"thin_apfs_snapshots"`

	every         time.Duration
	compactAfter  time.Duration
	pruneTmpAfter time.Duration
	minFreeGB     int
}

// DefaultConfig returns the default opt-in hygiene configuration.
func DefaultConfig() Config {
	minFree := DefaultMinFreeGB
	thin := runtime.GOOS == "darwin"
	return Config{
		Enabled:           false,
		Every:             "6h",
		MinFreeGB:         &minFree,
		CompactAfter:      "24h",
		PruneTmpAfter:     "24h",
		ThinAPFSSnapshots: &thin,
		every:             DefaultEvery,
		compactAfter:      DefaultCompactAfter,
		pruneTmpAfter:     DefaultPruneTmpAfter,
		minFreeGB:         DefaultMinFreeGB,
	}
}

// Load reads and validates the [hygiene] table from path. A missing file returns DefaultConfig.
func Load(path string) (Config, error) {
	var file struct {
		Hygiene *Config `toml:"hygiene"`
	}
	md, err := toml.DecodeFile(path, &file)
	if os.IsNotExist(err) {
		return DefaultConfig(), nil
	}
	if err != nil {
		var parse toml.ParseError
		if errors.As(err, &parse) {
			return Config{}, fmt.Errorf("invalid hygiene configuration at line %d", parse.Position.Line)
		}
		return Config{}, errors.New("could not read hygiene configuration")
	}
	for _, key := range md.Undecoded() {
		if len(key) > 1 && key[0] == "hygiene" {
			return Config{}, errors.New("unknown field in [hygiene]")
		}
	}
	if file.Hygiene == nil {
		return DefaultConfig(), nil
	}
	cfg := *file.Hygiene
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate fills defaults for unset fields and validates all bounds.
func (c *Config) Validate() error {
	if c.Every == "" {
		c.Every = "6h"
		c.every = DefaultEvery
	} else {
		d, err := time.ParseDuration(c.Every)
		if err != nil || d < MinEvery {
			return errors.New("hygiene.every must be a Go duration of at least 15m")
		}
		c.every = d
	}

	if c.MinFreeGB == nil {
		v := DefaultMinFreeGB
		c.MinFreeGB = &v
		c.minFreeGB = v
	} else {
		if *c.MinFreeGB < 0 {
			return errors.New("hygiene.min_free_gb must be nonnegative")
		}
		c.minFreeGB = *c.MinFreeGB
	}

	if c.CompactAfter == "" {
		c.CompactAfter = "24h"
		c.compactAfter = DefaultCompactAfter
	} else {
		d, err := time.ParseDuration(c.CompactAfter)
		if err != nil || d <= 0 {
			return errors.New("hygiene.compact_after must be a positive Go duration")
		}
		c.compactAfter = d
	}

	if c.PruneTmpAfter == "" {
		c.PruneTmpAfter = "24h"
		c.pruneTmpAfter = DefaultPruneTmpAfter
	} else {
		d, err := time.ParseDuration(c.PruneTmpAfter)
		if err != nil || d <= 0 {
			return errors.New("hygiene.prune_tmp_after must be a positive Go duration")
		}
		c.pruneTmpAfter = d
	}

	if c.ThinAPFSSnapshots == nil {
		thin := runtime.GOOS == "darwin"
		c.ThinAPFSSnapshots = &thin
	}
	return nil
}

func (c Config) EveryDuration() time.Duration {
	if c.every > 0 {
		return c.every
	}
	if d, err := time.ParseDuration(c.Every); err == nil && d >= MinEvery {
		return d
	}
	return DefaultEvery
}

func (c Config) CompactAfterDuration() time.Duration {
	if c.compactAfter > 0 {
		return c.compactAfter
	}
	if d, err := time.ParseDuration(c.CompactAfter); err == nil && d > 0 {
		return d
	}
	return DefaultCompactAfter
}

func (c Config) PruneTmpAfterDuration() time.Duration {
	if c.pruneTmpAfter > 0 {
		return c.pruneTmpAfter
	}
	if d, err := time.ParseDuration(c.PruneTmpAfter); err == nil && d > 0 {
		return d
	}
	return DefaultPruneTmpAfter
}

func (c Config) MinFreeGBValue() int {
	if c.MinFreeGB != nil {
		return *c.MinFreeGB
	}
	return c.minFreeGB
}

func (c Config) MinFreeBytes() uint64 {
	gb := c.MinFreeGBValue()
	if gb <= 0 {
		return 0
	}
	return uint64(gb) * BytesPerGiB
}

func (c Config) ShouldThinAPFS(osName string) bool {
	if osName != "darwin" {
		return false
	}
	if c.ThinAPFSSnapshots == nil {
		return true
	}
	return *c.ThinAPFSSnapshots
}
