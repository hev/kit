// Package cluster provides opt-in encrypted membership and bounded usage facts.
package cluster

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"unicode"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Bind          string   `toml:"bind"`
	Advertise     string   `toml:"advertise"`
	Seeds         []string `toml:"seeds"`
	KeyRef        string   `toml:"key_ref"`
	Name          string   `toml:"name"`
	AllowWildcard bool     `toml:"allow_wildcard"`
}

func LoadConfig(path string) (*Config, error) {
	if path == "" {
		path = os.Getenv("HEV_CONFIG")
	}
	if path == "" {
		h, _ := os.UserHomeDir()
		path = filepath.Join(h, ".hev", "config.toml")
	}
	var file struct {
		Cluster *Config `toml:"cluster"`
	}
	_, err := toml.DecodeFile(path, &file)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		var p toml.ParseError
		if errors.As(err, &p) {
			return nil, fmt.Errorf("invalid cluster configuration at line %d", p.Position.Line)
		}
		var line int
		if n, _ := fmt.Sscanf(err.Error(), "toml: line %d", &line); n == 1 {
			return nil, fmt.Errorf("invalid cluster configuration at line %d", line)
		}
		return nil, errors.New("invalid cluster configuration")
	}
	if file.Cluster != nil {
		if err = file.Cluster.Validate(); err != nil {
			return nil, err
		}
	}
	return file.Cluster, nil
}
func address(s string, wildcard bool) (string, int, error) {
	h, p, e := net.SplitHostPort(s)
	ip := net.ParseIP(h)
	n, ne := strconv.Atoi(p)
	if e != nil || ne != nil || ip == nil || n < 0 || n > 65535 || (!wildcard && ip.IsUnspecified()) {
		return "", 0, errors.New("cluster requires an explicit IP:port; wildcard requires allow_wildcard")
	}
	return h, n, nil
}
func (c Config) Validate() error {
	if _, _, e := address(c.Bind, c.AllowWildcard); e != nil {
		return e
	}
	if c.Advertise != "" {
		if _, p, e := address(c.Advertise, false); e != nil || p == 0 {
			return errors.New("invalid cluster advertise address")
		}
	}
	if len(c.Seeds) > 64 {
		return errors.New("too many cluster seeds")
	}
	for _, s := range c.Seeds {
		if _, p, e := address(s, false); e != nil || p == 0 {
			return errors.New("invalid cluster seed address")
		}
	}
	if c.KeyRef == "" {
		return errors.New("cluster key_ref is required")
	}
	if c.Name != "" && !identifier(c.Name) {
		return errors.New("invalid cluster name (maximum 64 bytes)")
	}
	return nil
}
func identifier(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	for _, r := range s {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}
