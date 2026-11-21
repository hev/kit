// Package jobs runs bounded, user-configured schedules for hevd.
package jobs

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/BurntSushi/toml"
)

type Job struct {
	Name      string            `toml:"name"`
	Every     string            `toml:"every"`
	At        string            `toml:"at"`
	TZ        string            `toml:"tz"`
	Cmd       []string          `toml:"cmd"`
	Workdir   string            `toml:"workdir"`
	Timeout   string            `toml:"timeout"`
	Cooldown  string            `toml:"cooldown"`
	EnvRefs   map[string]string `toml:"env_refs"`
	Placement []string          `toml:"placement"`
	Enabled   *bool             `toml:"enabled"`

	interval, timeout, cooldown time.Duration
	clock                       time.Time
	location                    *time.Location
}

var (
	jobName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

func Load(path string) ([]Job, error) {
	var file struct {
		Jobs []Job `toml:"jobs"`
	}
	md, err := toml.DecodeFile(path, &file)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		var parse toml.ParseError
		if errors.As(err, &parse) {
			return nil, fmt.Errorf("invalid jobs configuration at line %d", parse.Position.Line)
		}
		return nil, errors.New("could not read jobs configuration")
	}
	for _, key := range md.Undecoded() {
		if len(key) > 1 && key[0] == "jobs" {
			return nil, errors.New("unknown field in [[jobs]]")
		}
	}
	if len(file.Jobs) > 128 {
		return nil, errors.New("at most 128 jobs may be configured")
	}
	seen := make(map[string]bool, len(file.Jobs))
	for i := range file.Jobs {
		j := &file.Jobs[i]
		if err := j.validate(); err != nil {
			return nil, fmt.Errorf("jobs entry %d: %w", i+1, err)
		}
		if seen[j.Name] {
			return nil, fmt.Errorf("jobs entry %d has a duplicate name", i+1)
		}
		seen[j.Name] = true
	}
	return file.Jobs, nil
}

func duration(value string, fallback, min, max time.Duration) (time.Duration, bool) {
	if value == "" {
		return fallback, true
	}
	d, err := time.ParseDuration(value)
	return d, err == nil && d >= min && d <= max
}

func (j *Job) validate() error {
	if !jobName.MatchString(j.Name) || len(j.Name) > 128 {
		return errors.New("name must match ^[a-z0-9][a-z0-9-]*$ and fit in 128 bytes")
	}
	if len(j.Cmd) == 0 || strings.TrimSpace(j.Cmd[0]) == "" {
		return errors.New("cmd requires a nonempty executable in argv")
	}
	for _, arg := range j.Cmd {
		if strings.ContainsRune(arg, 0) {
			return errors.New("cmd contains a NUL byte")
		}
	}
	if (j.Every == "") == (j.At == "") {
		return errors.New("set exactly one of every and at")
	}
	const maxDuration = time.Duration(1<<63 - 1)
	var ok bool
	if j.Every != "" {
		if j.interval, ok = duration(j.Every, 0, 10*time.Second, maxDuration); !ok {
			return errors.New("every must be a Go duration of at least 10s")
		}
		if j.TZ != "" {
			return errors.New("tz requires at")
		}
	} else {
		clock, err := time.Parse("15:04", j.At)
		if err != nil || clock.Format("15:04") != j.At {
			return errors.New("at must use 24-hour HH:MM")
		}
		if j.TZ == "" {
			j.TZ = "UTC"
		}
		j.clock = clock
		j.location, err = time.LoadLocation(j.TZ)
		if err != nil || j.TZ == "Local" {
			return errors.New("tz must be an IANA time zone")
		}
	}
	if j.timeout, ok = duration(j.Timeout, 5*time.Minute, 1, 24*time.Hour); !ok {
		return errors.New("timeout must be a positive Go duration up to 24h")
	}
	if j.cooldown, ok = duration(j.Cooldown, 0, 0, maxDuration); !ok {
		return errors.New("cooldown must be a nonnegative Go duration")
	}
	if len(j.Placement) > 64 || len(j.EnvRefs) > 64 {
		return errors.New("placement and env_refs support at most 64 entries each")
	}
	members := map[string]bool{}
	for _, name := range j.Placement {
		if name == "" || len(name) > 64 || strings.TrimSpace(name) != name || members[name] {
			return errors.New("placement must contain distinct member names of up to 64 bytes")
		}
		members[name] = true
	}
	env := map[string]bool{}
	for key, ref := range j.EnvRefs {
		upper := strings.ToUpper(key)
		if !envName.MatchString(key) || strings.TrimSpace(ref) == "" || env[upper] {
			return errors.New("env_refs requires distinct environment names and nonempty references")
		}
		env[upper] = true
	}
	return nil
}

func (j Job) Schedule() string {
	if j.interval != 0 {
		return "every " + j.Every
	}
	return j.At + " " + j.TZ
}

func (j Job) enabled() bool { return j.Enabled == nil || *j.Enabled }

// Next returns the first tick strictly after now.
func (j Job) Next(now time.Time) time.Time {
	if j.interval != 0 {
		offset := time.Duration(now.UnixNano() % int64(j.interval))
		if offset < 0 {
			offset += j.interval
		}
		return now.Add(j.interval - offset)
	}
	year, month, day := now.In(j.location).Date()
	wall := time.Date(year, month, day, j.clock.Hour(), j.clock.Minute(), 0, 0, time.UTC)
	for {
		if tick := j.wallTick(wall); tick.After(now) {
			return tick
		}
		wall = wall.AddDate(0, 0, 1)
	}
}

// Check each zone offset near the date to distinguish gaps from repeats.
func (j Job) wallTick(wall time.Time) time.Time {
	var first time.Time
	limit := wall.Add(26 * time.Hour)
	for probe := wall.Add(-26 * time.Hour).In(j.location); probe.Before(limit); {
		_, offset := probe.Zone()
		tick := wall.Add(-time.Duration(offset) * time.Second).In(j.location)
		_, actual := tick.Zone()
		if actual == offset && (first.IsZero() || tick.Before(first)) {
			first = tick
		}
		_, end := probe.ZoneBounds()
		if end.IsZero() || !end.After(probe) {
			break
		}
		probe = end.In(j.location)
	}
	return first
}
