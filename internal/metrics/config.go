// Package metrics exports strictly allowlisted hevd gauges over loopback or
// Tailscale DogStatsD UDP and an optional Prometheus /metrics endpoint.
package metrics

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

const (
	DefaultInterval     = 60 * time.Second
	MinInterval         = 10 * time.Second
	MaxInterval         = time.Hour
	MaxPlacementMembers = 64
	MaxMemberNameBytes  = 64
)

var tailscaleCGNAT = netip.MustParsePrefix("100.64.0.0/10")

// Config is the opt-in [metrics] block in ~/.hev/config.toml.
type Config struct {
	Enabled        bool     `toml:"enabled"`
	Interval       string   `toml:"interval"`
	DogStatsD      string   `toml:"dogstatsd"`
	PrometheusAddr string   `toml:"prometheus_addr"`
	Placement      []string `toml:"placement"`

	interval time.Duration
}

// Load reads and validates the optional [metrics] table from path (or HEV_CONFIG
// / ~/.hev/config.toml when path is empty). Returns (nil, nil) when the file or
// [metrics] table is absent. Error messages never include local paths or values.
func Load(path string) (*Config, error) {
	if path == "" {
		path = os.Getenv("HEV_CONFIG")
	}
	if path == "" {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, ".hev", "config.toml")
	}
	var file struct {
		Metrics *Config `toml:"metrics"`
	}
	md, err := toml.DecodeFile(path, &file)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		var p toml.ParseError
		if errors.As(err, &p) {
			return nil, fmt.Errorf("invalid metrics configuration at line %d", p.Position.Line)
		}
		var line int
		if n, _ := fmt.Sscanf(err.Error(), "toml: line %d", &line); n == 1 {
			return nil, fmt.Errorf("invalid metrics configuration at line %d", line)
		}
		return nil, errors.New("could not read metrics configuration")
	}
	for _, key := range md.Undecoded() {
		if len(key) > 1 && key[0] == "metrics" {
			return nil, errors.New("unknown field in [metrics]")
		}
	}
	if file.Metrics == nil {
		return nil, nil
	}
	if err := file.Metrics.Validate(); err != nil {
		return nil, err
	}
	return file.Metrics, nil
}

// Validate checks interval bounds, private-only network targets, and placement names.
func (c *Config) Validate() error {
	if c.Interval == "" {
		c.interval = DefaultInterval
	} else {
		d, err := time.ParseDuration(c.Interval)
		if err != nil {
			return errors.New("metrics interval must be a valid Go duration between 10s and 1h")
		}
		if d < MinInterval || d > MaxInterval {
			return errors.New("metrics interval must be between 10s and 1h")
		}
		c.interval = d
	}
	if c.DogStatsD != "" {
		if err := ValidatePrivateAddr(c.DogStatsD, false); err != nil {
			return fmt.Errorf("metrics dogstatsd: %w", err)
		}
	}
	if c.PrometheusAddr != "" {
		if err := ValidatePrivateAddr(c.PrometheusAddr, true); err != nil {
			return fmt.Errorf("metrics prometheus_addr: %w", err)
		}
	}
	if len(c.Placement) > MaxPlacementMembers {
		return errors.New("metrics placement supports at most 64 entries")
	}
	seen := make(map[string]bool, len(c.Placement))
	for _, name := range c.Placement {
		if !ValidMemberName(name) || seen[name] {
			return errors.New("metrics placement must contain distinct member names of up to 64 safe characters")
		}
		seen[name] = true
	}
	return nil
}

// EmitInterval returns the validated emission interval, defaulting to 60s.
func (c Config) EmitInterval() time.Duration {
	if c.interval >= MinInterval && c.interval <= MaxInterval {
		return c.interval
	}
	if c.Interval != "" {
		if d, err := time.ParseDuration(c.Interval); err == nil && d >= MinInterval && d <= MaxInterval {
			return d
		}
	}
	return DefaultInterval
}

// ValidatePrivateAddr enforces the loopback (127.0.0.0/8, ::1) or Tailscale
// CGNAT (100.64.0.0/10) explicit IP:port guard. Wildcards, public IPs, RFC 1918
// LAN addresses, link-local addresses, zone IDs, and hostnames are refused.
func ValidatePrivateAddr(addr string, allowZeroPort bool) error {
	if strings.TrimSpace(addr) != addr || addr == "" {
		return errors.New("address must be an explicit loopback (127.0.0.1, ::1) or Tailscale CGNAT (100.64.0.0/10) IP:port")
	}
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return errors.New("address must be an explicit loopback (127.0.0.1, ::1) or Tailscale CGNAT (100.64.0.0/10) IP:port")
	}
	port, err := strconv.Atoi(portStr)
	minPort := 1
	if allowZeroPort {
		minPort = 0
	}
	if err != nil || port < minPort || port > 65535 {
		if allowZeroPort {
			return errors.New("port must be between 0 and 65535")
		}
		return errors.New("port must be between 1 and 65535")
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || ip.Zone() != "" {
		return errors.New("host must be an explicit loopback (127.0.0.1, ::1) or Tailscale CGNAT (100.64.0.0/10) IP literal")
	}
	ip = ip.Unmap()
	if ip.IsUnspecified() {
		return errors.New("wildcard bind address (0.0.0.0 / ::) is refused; use loopback or Tailscale CGNAT (100.64.0.0/10)")
	}
	if ip.IsLoopback() || tailscaleCGNAT.Contains(ip) {
		return nil
	}
	return errors.New("non-private or non-Tailscale address refused; only loopback (127.0.0.0/8, ::1) and Tailscale CGNAT (100.64.0.0/10) are allowed")
}

// ValidMemberName reports whether name is a safe SWIM member identifier
// (1-64 ASCII letters, digits, dots, dashes, or underscores, starting with
// a letter or digit). It refuses path separators, tag delimiters, whitespace,
// and control characters.
func ValidMemberName(name string) bool {
	if len(name) == 0 || len(name) > MaxMemberNameBytes {
		return false
	}
	for i := 0; i < len(name); i++ {
		b := name[i]
		alnum := (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
		if i == 0 {
			if !alnum {
				return false
			}
			continue
		}
		if !(alnum || b == '-' || b == '_' || b == '.') {
			return false
		}
	}
	return true
}

// NormalizeSelf returns name if it is a valid member identifier, else "unknown".
func NormalizeSelf(name string) string {
	if ValidMemberName(name) {
		return name
	}
	return "unknown"
}
