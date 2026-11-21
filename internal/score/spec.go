// Package score implements host-pinned metric commit scorers (RFC 0021, RFC 0023).
//
// A metric scorer is a Markdown file with strict YAML frontmatter whose
// command, spec and declared data files are pinned on the host by SHA-256.
// Running a scorer against a commit verifies the pin set before and after the
// command runs, disqualifies commits whose diff from a parent touches a
// protected repository path without invoking the command, and executes the host
// copy of the command in a detached temporary checkout under a bounded timeout
// and minimal environment.
package score

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	KindMetric    = "metric"
	SubjectCommit = "commit"

	DirectionMax = "max"
	DirectionMin = "min"

	VisibilityVisible = "visible"
	VisibilityHidden  = "hidden"

	// MaxSpecBytes caps scorer definition files read from disk.
	MaxSpecBytes = 1 << 20
	// MaxTimeout bounds any single scorer subprocess invocation.
	MaxTimeout = 24 * time.Hour
	// MaxScale bounds the integer mark multiplier to prevent float/int overflow.
	MaxScale = 1_000_000_000
)

var (
	scorerNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
)

// Spec is a parsed and validated metric scorer definition.
type Spec struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Kind        string   `json:"kind"`
	Subject     string   `json:"subject"`
	Command     string   `json:"command"`
	Direction   string   `json:"direction"`
	Unit        string   `json:"unit,omitempty"`
	Scale       int      `json:"scale"`
	Timeout     string   `json:"timeout"`
	Data        []string `json:"data,omitempty"`
	Hidden      bool     `json:"hidden"`
	Visibility  string   `json:"visibility,omitempty"`
	Protect     []string `json:"protect,omitempty"`
	Body        string   `json:"-"`

	timeout time.Duration
}

type rawSpec struct {
	Name        string   `yaml:"name,omitempty"`
	Description string   `yaml:"description,omitempty"`
	Kind        string   `yaml:"kind"`
	Subject     string   `yaml:"subject,omitempty"`
	Command     string   `yaml:"command"`
	Direction   string   `yaml:"direction"`
	Unit        string   `yaml:"unit,omitempty"`
	Scale       *int     `yaml:"scale,omitempty"`
	Timeout     string   `yaml:"timeout"`
	Data        []string `yaml:"data,omitempty"`
	Hidden      *bool    `yaml:"hidden,omitempty"`
	Visibility  string   `yaml:"visibility,omitempty"`
	Protect     []string `yaml:"protect,omitempty"`
}

// TimeoutDuration returns the validated positive bounded timeout duration.
func (s Spec) TimeoutDuration() time.Duration {
	return s.timeout
}

// MarkName returns the RFC 0005 mark column name (dashes replaced by underscores).
func (s Spec) MarkName() string {
	return strings.ReplaceAll(s.Name, "-", "_")
}

// ParseSpec decodes and validates a scorer Markdown file with YAML frontmatter.
func ParseSpec(text string) (Spec, error) {
	return parseSpecWithDefaultName(text, "")
}

// LoadSpec reads, parses, and validates a scorer spec file from path.
func LoadSpec(path string) (Spec, error) {
	if strings.TrimSpace(path) == "" {
		return Spec{}, fmt.Errorf("scorer file path is required")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return Spec{}, err
	}
	if !info.Mode().IsRegular() {
		return Spec{}, fmt.Errorf("scorer spec %s must be a regular file", filepath.Base(path))
	}
	if info.Size() > MaxSpecBytes {
		return Spec{}, fmt.Errorf("scorer spec %s exceeds %d bytes", filepath.Base(path), MaxSpecBytes)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return Spec{}, err
	}
	return parseSpecWithDefaultName(string(b), defaultNameFromPath(path))
}

func defaultNameFromPath(path string) string {
	clean := filepath.Clean(path)
	base := filepath.Base(clean)
	if base == "scorer.md" {
		parent := filepath.Base(filepath.Dir(clean))
		if scorerNamePattern.MatchString(parent) {
			return parent
		}
		return "scorer"
	}
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	if scorerNamePattern.MatchString(stem) {
		return stem
	}
	return "scorer"
}

func parseSpecWithDefaultName(text, fallbackName string) (Spec, error) {
	var out Spec
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return out, fmt.Errorf("scorer spec must start with ---")
	}
	front, body, ok := strings.Cut(text[4:], "\n---")
	if !ok || (body != "" && !strings.HasPrefix(body, "\n")) {
		return out, fmt.Errorf("unclosed frontmatter")
	}

	var raw rawSpec
	dec := yaml.NewDecoder(bytes.NewBufferString(front))
	dec.KnownFields(true)
	if err := dec.Decode(&raw); err != nil {
		return out, fmt.Errorf("scorer frontmatter: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return out, fmt.Errorf("one frontmatter document required")
	}

	name := strings.TrimSpace(raw.Name)
	if name == "" {
		name = fallbackName
		if name == "" {
			name = "scorer"
		}
	}
	subject := strings.TrimSpace(raw.Subject)
	if subject == "" {
		subject = SubjectCommit
	}
	scale := 1
	if raw.Scale != nil {
		scale = *raw.Scale
	}
	hidden := false
	if raw.Hidden != nil {
		hidden = *raw.Hidden
	}
	vis := strings.TrimSpace(raw.Visibility)
	if vis == VisibilityHidden {
		if raw.Hidden != nil && !*raw.Hidden {
			return out, fmt.Errorf("visibility: hidden conflicts with hidden: false")
		}
		hidden = true
	} else if vis == VisibilityVisible && raw.Hidden != nil && *raw.Hidden {
		return out, fmt.Errorf("visibility: visible conflicts with hidden: true")
	}

	out = Spec{
		Name:        name,
		Description: strings.TrimSpace(raw.Description),
		Kind:        strings.TrimSpace(raw.Kind),
		Subject:     subject,
		Command:     strings.TrimSpace(raw.Command),
		Direction:   strings.TrimSpace(raw.Direction),
		Unit:        strings.TrimSpace(raw.Unit),
		Scale:       scale,
		Timeout:     strings.TrimSpace(raw.Timeout),
		Data:        append([]string(nil), raw.Data...),
		Hidden:      hidden,
		Visibility:  vis,
		Protect:     append([]string(nil), raw.Protect...),
		Body:        strings.TrimSpace(body),
	}
	if err := out.Validate(); err != nil {
		return out, err
	}
	d, _ := time.ParseDuration(out.Timeout)
	out.timeout = d
	return out, nil
}

// Validate checks every rule on a metric scorer spec.
func (s Spec) Validate() error {
	if !scorerNamePattern.MatchString(s.Name) {
		return fmt.Errorf("invalid scorer name %q (must match ^[a-z][a-z0-9-]*$)", s.Name)
	}
	if s.Kind != KindMetric {
		return fmt.Errorf("kind must be %q, got %q", KindMetric, s.Kind)
	}
	if s.Subject != SubjectCommit {
		return fmt.Errorf("subject must be %q, got %q", SubjectCommit, s.Subject)
	}
	if s.Command == "" || strings.ContainsRune(s.Command, 0) {
		return fmt.Errorf("command is required")
	}
	if s.Direction != DirectionMax && s.Direction != DirectionMin {
		return fmt.Errorf("direction must be %q or %q, got %q", DirectionMax, DirectionMin, s.Direction)
	}
	if s.Scale <= 0 || s.Scale > MaxScale {
		return fmt.Errorf("scale must be a positive integer in 1..%d, got %d", MaxScale, s.Scale)
	}
	if s.Timeout == "" {
		return fmt.Errorf("timeout is required")
	}
	d, err := time.ParseDuration(s.Timeout)
	if err != nil || d <= 0 || d > MaxTimeout {
		return fmt.Errorf("timeout %q must be a valid positive duration up to %s", s.Timeout, MaxTimeout)
	}
	if s.Visibility != "" && s.Visibility != VisibilityVisible && s.Visibility != VisibilityHidden {
		return fmt.Errorf("visibility must be %q or %q, got %q", VisibilityVisible, VisibilityHidden, s.Visibility)
	}
	if strings.ContainsRune(s.Unit, 0) {
		return fmt.Errorf("invalid unit")
	}
	seenData := make(map[string]bool, len(s.Data))
	for i, p := range s.Data {
		t := strings.TrimSpace(p)
		if t == "" || strings.ContainsRune(t, 0) {
			return fmt.Errorf("data[%d]: path must be non-empty", i)
		}
		if seenData[t] {
			return fmt.Errorf("data[%d]: duplicate path %q", i, t)
		}
		seenData[t] = true
	}
	for i, pat := range s.Protect {
		if err := ValidateProtectPattern(pat); err != nil {
			return fmt.Errorf("protect[%d] (%q): %w", i, pat, err)
		}
	}
	return nil
}

// ResolvedPaths holds canonical host filesystem paths for a scorer spec, its
// command executable/script, and its declared data files.
type ResolvedPaths struct {
	Spec    string
	Command string
	Data    []string
}

// ResolvePaths resolves a scorer spec file path along with its command and data
// paths relative to the scorer spec's directory. Parent directories are
// canonicalized via EvalSymlinks without dereferencing leaf symlinks, so
// replacing a pinned file with a symlink is caught by Lstat regular-file checks.
func ResolvePaths(specPath string, s Spec) (ResolvedPaths, error) {
	canonSpec, err := canonicalLeafPath(specPath)
	if err != nil {
		return ResolvedPaths{}, err
	}
	baseDir := filepath.Dir(canonSpec)
	cmdPath, err := resolveMemberPath(baseDir, s.Command)
	if err != nil {
		return ResolvedPaths{}, fmt.Errorf("command path: %w", err)
	}
	dataPaths := make([]string, len(s.Data))
	seen := make(map[string]bool, len(s.Data))
	for i, raw := range s.Data {
		p, err := resolveMemberPath(baseDir, raw)
		if err != nil {
			return ResolvedPaths{}, fmt.Errorf("data[%d]: %w", i, err)
		}
		if seen[p] {
			return ResolvedPaths{}, fmt.Errorf("data[%d]: duplicate resolved path", i)
		}
		seen[p] = true
		dataPaths[i] = p
	}
	return ResolvedPaths{
		Spec:    canonSpec,
		Command: cmdPath,
		Data:    dataPaths,
	}, nil
}

func resolveMemberPath(baseDir, p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", fmt.Errorf("empty path")
	}
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if p == "~" {
			p = home
		} else {
			p = filepath.Join(home, p[2:])
		}
	} else if !filepath.IsAbs(p) {
		p = filepath.Join(baseDir, p)
	}
	return canonicalLeafPath(p)
}

func canonicalLeafPath(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	clean := filepath.Clean(abs)
	dir := filepath.Dir(clean)
	if evalDir, err := filepath.EvalSymlinks(dir); err == nil {
		return filepath.Join(evalDir, filepath.Base(clean)), nil
	}
	return clean, nil
}
