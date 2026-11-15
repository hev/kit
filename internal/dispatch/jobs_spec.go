package dispatch

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type JobSpec struct {
	Kind string `yaml:"kind" json:"kind"`
	Name string `yaml:"name" json:"name"`
	Host string `yaml:"host" json:"host"`
	Exec struct {
		Argv []string `yaml:"argv" json:"argv"`
		Cwd  string   `yaml:"cwd" json:"cwd"`
	} `yaml:"exec" json:"exec"`
	Schedule struct {
		Every   string `yaml:"every" json:"every"`
		CatchUp string `yaml:"catch_up" json:"catch_up"`
	} `yaml:"schedule" json:"schedule"`
	Secrets map[string]string `yaml:"secrets" json:"secrets"`
	Timeout string            `yaml:"timeout" json:"timeout"`
	Overlap string            `yaml:"overlap" json:"overlap"`
	Retry   struct {
		Attempts int `yaml:"attempts" json:"attempts"`
	} `yaml:"retry" json:"retry"`
	Reply string `yaml:"reply" json:"reply"`
}

var jobNameRE = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
var jobEnvRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var jobSecretRE = regexp.MustCompile(`^secret://[a-z0-9-]{1,127}$`)
var jobDurationRE = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)([smhd])`)

func JobDuration(s string) (time.Duration, error) {
	// Convert days without accepting unlimited/overflowing durations.
	matches := jobDurationRE.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return 0, fmt.Errorf("invalid duration")
	}
	rest := s
	var total time.Duration
	for _, m := range matches {
		if !strings.HasPrefix(rest, m[0]) {
			return 0, fmt.Errorf("invalid duration")
		}
		rest = strings.TrimPrefix(rest, m[0])
		unit := m[2]
		number := m[1]
		if unit == "d" {
			unit = "h"
		}
		d, e := time.ParseDuration(number + unit)
		if e != nil {
			return 0, e
		}
		if m[2] == "d" {
			if d > time.Duration(1<<63-1)/24 {
				return 0, fmt.Errorf("duration overflow")
			}
			d *= 24
		}
		if d > time.Duration(1<<63-1)-total {
			return 0, fmt.Errorf("duration overflow")
		}
		total += d
	}
	if rest != "" || total <= 0 {
		return 0, fmt.Errorf("duration must be positive")
	}
	return total, nil
}

func ParseJob(text []byte) (JobSpec, error) {
	var s JobSpec
	text = bytes.ReplaceAll(text, []byte("\r\n"), []byte("\n"))
	if !bytes.HasPrefix(text, []byte("---\n")) {
		return s, fmt.Errorf("job requires YAML frontmatter")
	}
	end := bytes.Index(text[4:], []byte("\n---"))
	if end < 0 {
		return s, fmt.Errorf("unterminated job frontmatter")
	}
	front := text[4 : 4+end]
	d := yaml.NewDecoder(bytes.NewReader(front))
	d.KnownFields(true)
	if e := d.Decode(&s); e != nil {
		return s, fmt.Errorf("invalid job fields: %w", e)
	}
	if e := d.Decode(new(any)); e != io.EOF {
		return s, fmt.Errorf("multiple YAML documents")
	}
	// yaml's string decoder coerces booleans/numbers. Require actual strings
	// throughout the reference and argv maps instead of accepting coercion.
	var node yaml.Node
	if e := yaml.Unmarshal(front, &node); e != nil {
		return s, e
	}
	var scalarCheck func(*yaml.Node, string) error
	scalarCheck = func(n *yaml.Node, path string) error {
		if n.Kind == yaml.AliasNode {
			return fmt.Errorf("YAML aliases are not supported")
		}
		if n.Kind == yaml.ScalarNode {
			want := "!!str"
			if path == "retry.attempts" {
				want = "!!int"
			}
			if n.Tag != want {
				return fmt.Errorf("invalid type for %s", path)
			}
		}
		if n.Kind == yaml.MappingNode {
			for i := 0; i < len(n.Content); i += 2 {
				if n.Content[i].Tag != "!!str" {
					return fmt.Errorf("mapping keys must be strings")
				}
				key := n.Content[i].Value
				if path != "" {
					key = path + "." + key
				}
				if e := scalarCheck(n.Content[i+1], key); e != nil {
					return e
				}
			}
		} else {
			for _, c := range n.Content {
				if e := scalarCheck(c, path); e != nil {
					return e
				}
			}
		}
		return nil
	}
	if e := scalarCheck(&node, ""); e != nil {
		return s, e
	}
	if s.Kind != "job" || !jobNameRE.MatchString(s.Name) {
		return s, fmt.Errorf("kind must be job and name must be 1–63 lowercase letters, digits or hyphens starting with a letter")
	}
	if s.Host == "" || strings.ContainsAny(s.Host, "/\\\x00\n\r ") {
		return s, fmt.Errorf("host is required")
	}
	if len(s.Exec.Argv) == 0 || s.Exec.Argv[0] == "" {
		return s, fmt.Errorf("exec.argv must be nonempty")
	}
	for _, a := range s.Exec.Argv {
		if strings.ContainsRune(a, 0) {
			return s, fmt.Errorf("argv contains NUL")
		}
	}
	if !filepath.IsAbs(s.Exec.Cwd) {
		return s, fmt.Errorf("exec.cwd must be absolute")
	}
	info, e := os.Stat(s.Exec.Cwd)
	if e != nil || !info.IsDir() {
		return s, fmt.Errorf("exec.cwd must be an existing directory")
	}
	every, e := JobDuration(s.Schedule.Every)
	if e != nil || every < time.Minute {
		return s, fmt.Errorf("schedule.every must be at least 1m")
	}
	timeout, e := JobDuration(s.Timeout)
	if e != nil || timeout > 24*time.Hour {
		return s, fmt.Errorf("timeout must be positive and at most 24h")
	}
	if s.Schedule.CatchUp == "" {
		s.Schedule.CatchUp = "none"
	}
	if s.Schedule.CatchUp != "none" && s.Schedule.CatchUp != "one" {
		return s, fmt.Errorf("catch_up must be none or one")
	}
	if s.Overlap == "" {
		s.Overlap = "skip"
	}
	if s.Overlap != "skip" {
		return s, fmt.Errorf("only overlap: skip is supported")
	}
	if s.Retry.Attempts != 0 {
		return s, fmt.Errorf("retry.attempts must be zero")
	}
	if strings.TrimSpace(s.Reply) == "" || strings.ContainsAny(s.Reply, "\x00\n\r") {
		return s, fmt.Errorf("reply inbox is required")
	}
	for k, v := range s.Secrets {
		if !jobEnvRE.MatchString(k) || reservedJobEnv(k) {
			return s, fmt.Errorf("invalid or reserved secret environment name")
		}
		if !jobSecretRE.MatchString(v) {
			return s, fmt.Errorf("secrets must contain secret://NAME references only")
		}
	}
	return s, nil
}
func reservedJobEnv(k string) bool {
	for _, p := range []string{"HEV_", "HEVLAYER_", "LAYER_", "OP_", "AZURE_", "AWS_", "GOOGLE_"} {
		if strings.HasPrefix(k, p) {
			return true
		}
	}
	for _, n := range []string{"PATH", "HOME", "USERPROFILE", "TMPDIR", "TMP", "TEMP", "SystemRoot", "WINDIR", "COMSPEC", "PATHEXT", "LD_PRELOAD", "LD_LIBRARY_PATH", "DYLD_INSERT_LIBRARIES", "BASH_ENV", "ENV"} {
		if strings.EqualFold(k, n) {
			return true
		}
	}
	return false
}
func jobDigest(s JobSpec) string { b, _ := json.Marshal(s); return fmt.Sprintf("%x", sha256.Sum256(b)) }
