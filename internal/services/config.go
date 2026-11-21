// Package services supervises long-lived processes for hevd.
package services

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

const (
	MaxServices        = 64
	RestartAlways      = "always"
	RestartOnFailure   = "on-failure"
	RestartNever       = "never"
	DefaultStopTimeout = 15 * time.Second
	MaxStopTimeout     = 5 * time.Minute
)

type Service struct {
	Name        string            `toml:"name"`
	Cmd         []string          `toml:"cmd"`
	Workdir     string            `toml:"workdir"`
	EnvRefs     map[string]string `toml:"env_refs"`
	Restart     string            `toml:"restart"`
	StopTimeout string            `toml:"stop_timeout"`
	Placement   []string          `toml:"placement"`
	Enabled     *bool             `toml:"enabled"`

	stopTimeout time.Duration
}

var (
	serviceName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	envName     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

func Load(path string) ([]Service, error) {
	var file struct {
		Service []Service `toml:"service"`
	}
	md, err := toml.DecodeFile(path, &file)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		var parse toml.ParseError
		if errors.As(err, &parse) {
			return nil, fmt.Errorf("invalid service configuration at line %d", parse.Position.Line)
		}
		return nil, errors.New("could not read service configuration")
	}
	for _, key := range md.Undecoded() {
		if len(key) > 1 && key[0] == "service" {
			return nil, errors.New("unknown field in [[service]]")
		}
	}
	if len(file.Service) > MaxServices {
		return nil, fmt.Errorf("at most %d services may be configured", MaxServices)
	}
	seen := make(map[string]bool, len(file.Service))
	for i := range file.Service {
		s := &file.Service[i]
		if err := s.validate(); err != nil {
			return nil, fmt.Errorf("service entry %d: %w", i+1, err)
		}
		if seen[s.Name] {
			return nil, fmt.Errorf("service entry %d has a duplicate name", i+1)
		}
		seen[s.Name] = true
	}
	return file.Service, nil
}

func (s *Service) validate() error {
	if !serviceName.MatchString(s.Name) || len(s.Name) > 128 {
		return errors.New("name must match ^[a-z0-9][a-z0-9-]*$ and fit in 128 bytes")
	}
	if len(s.Cmd) == 0 || strings.TrimSpace(s.Cmd[0]) == "" {
		return errors.New("cmd requires a nonempty executable in argv")
	}
	for _, arg := range s.Cmd {
		if strings.ContainsRune(arg, 0) {
			return errors.New("cmd contains a NUL byte")
		}
	}
	if strings.ContainsRune(s.Workdir, 0) {
		return errors.New("workdir contains a NUL byte")
	}
	if s.Restart == "" {
		s.Restart = RestartAlways
	}
	switch s.Restart {
	case RestartAlways, RestartOnFailure, RestartNever:
	default:
		return errors.New(`restart must be "always", "on-failure", or "never"`)
	}
	if s.StopTimeout == "" {
		s.StopTimeout = "15s"
		s.stopTimeout = DefaultStopTimeout
	} else {
		d, err := time.ParseDuration(s.StopTimeout)
		if err != nil || d <= 0 || d > MaxStopTimeout {
			return errors.New("stop_timeout must be a positive Go duration up to 5m")
		}
		s.stopTimeout = d
	}
	if len(s.Placement) > 64 || len(s.EnvRefs) > 64 {
		return errors.New("placement and env_refs support at most 64 entries each")
	}
	members := make(map[string]bool, len(s.Placement))
	for _, name := range s.Placement {
		if name == "" || len(name) > 64 || strings.TrimSpace(name) != name || strings.ContainsRune(name, 0) || members[name] {
			return errors.New("placement must contain distinct member names of up to 64 bytes")
		}
		members[name] = true
	}
	env := make(map[string]bool, len(s.EnvRefs))
	for key, ref := range s.EnvRefs {
		upper := strings.ToUpper(key)
		if !envName.MatchString(key) || strings.TrimSpace(ref) == "" || strings.ContainsAny(ref, "\r\n\x00") || env[upper] {
			return errors.New("env_refs requires distinct environment names and nonempty references")
		}
		if !validSecretRefScheme(ref) {
			return errors.New("env_refs values must use secret://, op://, or azkv:// references")
		}
		env[upper] = true
	}
	return nil
}

func validSecretRefScheme(ref string) bool {
	return strings.HasPrefix(ref, "secret://") || strings.HasPrefix(ref, "op://") || strings.HasPrefix(ref, "azkv://")
}

func (s Service) enabled() bool { return s.Enabled == nil || *s.Enabled }

func (s Service) RestartMode() string {
	if s.Restart != "" {
		return s.Restart
	}
	return RestartAlways
}

func (s Service) StopTimeoutDuration() time.Duration {
	if s.stopTimeout > 0 {
		return s.stopTimeout
	}
	return DefaultStopTimeout
}
