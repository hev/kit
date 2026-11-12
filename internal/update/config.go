package update

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Config is the host's [update] table in its own config file. Nothing here
// comes from a sender or the network.
type Config struct {
	// Channel is release (the default) or edge.
	Channel string `toml:"channel"`
	// Auto lets the daemon update the host on its schedule. Off by default,
	// and only honoured with Channel set explicitly.
	Auto bool `toml:"auto"`
	// Interval is how often the daemon checks, a Go duration. Default 1h.
	Interval string `toml:"interval"`
	// PostUpdate are commands run, in order, after a successful update or
	// rollback: an operator's own services that wrap hev. Each is run by the
	// shell with HEV_UPDATE_FROM, HEV_UPDATE_TO and HEV_UPDATE_CHANNEL set.
	PostUpdate []string `toml:"post_update"`
}

const DefaultInterval = time.Hour

// LoadConfig reads [update] from the config file at path. A missing file or
// table is the defaults.
func LoadConfig(path string) (Config, error) {
	var file struct {
		Update Config `toml:"update"`
	}
	c := Config{}
	if path != "" {
		if _, err := os.Stat(path); err == nil {
			if _, err := toml.DecodeFile(path, &file); err != nil {
				return c, fmt.Errorf("read %s: %w", path, err)
			}
			c = file.Update
		} else if !os.IsNotExist(err) {
			return c, err
		}
	}
	if c.Channel != "" && !ValidChannel(c.Channel) {
		return c, fmt.Errorf("%s: [update] channel %q is not release or edge", path, c.Channel)
	}
	if _, err := c.interval(); err != nil {
		return c, fmt.Errorf("%s: [update] interval: %w", path, err)
	}
	return c, nil
}

// EffectiveChannel is the configured channel, or release.
func (c Config) EffectiveChannel() string {
	if c.Channel == "" {
		return ChannelRelease
	}
	return c.Channel
}

// AutoEnabled is true only with auto on and a channel chosen.
func (c Config) AutoEnabled() bool { return c.Auto && c.Channel != "" }

func (c Config) interval() (time.Duration, error) {
	if c.Interval == "" {
		return DefaultInterval, nil
	}
	d, err := time.ParseDuration(c.Interval)
	if err == nil && d < time.Minute {
		err = fmt.Errorf("%s is shorter than 1m", c.Interval)
	}
	return d, err
}

// CheckInterval is how often the daemon checks.
func (c Config) CheckInterval() time.Duration {
	d, err := c.interval()
	if err != nil {
		return DefaultInterval
	}
	return d
}

var (
	tableHeader  = regexp.MustCompile(`^\s*\[`)
	updateHeader = regexp.MustCompile(`^\s*\[\s*update\s*\]\s*(#.*)?$`)
	channelKey   = regexp.MustCompile(`^\s*channel\s*=`)
)

// SetChannel records the host's channel in the config file at path, editing
// only the channel line of [update] (or adding the table) so comments and
// every other setting stay as they are.
func SetChannel(path, channel string) error {
	if !ValidChannel(channel) {
		return fmt.Errorf("unknown channel %q (want release or edge)", channel)
	}
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	line := "channel = " + strconv.Quote(channel)
	lines := strings.Split(string(raw), "\n")
	in, header, done := false, -1, false
	for i, l := range lines {
		if tableHeader.MatchString(l) {
			if in && !done {
				break
			}
			in = updateHeader.MatchString(l)
			if in {
				header = i
			}
			continue
		}
		if in && channelKey.MatchString(l) {
			lines[i] = line
			done = true
		}
	}
	switch {
	case done:
	case header >= 0:
		lines = append(lines[:header+1], append([]string{line}, lines[header+1:]...)...)
	default:
		text := strings.TrimRight(string(raw), "\n")
		if text != "" {
			text += "\n\n"
		}
		lines = strings.Split(text+"[update]\n"+line+"\n", "\n")
		lines = lines[:len(lines)-1]
	}
	out := strings.Join(lines, "\n")
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	// Check the edit parses before it replaces the file.
	var probe map[string]any
	if _, err := toml.Decode(out, &probe); err != nil {
		return fmt.Errorf("record channel in %s: %w", path, err)
	}
	return writeAtomic(path, []byte(out), 0o600)
}
