// Package preview implements private-network-only static site previews:
// configuration validation, atomic Git-backed builds, and safe static serving.
package preview

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

const (
	SchemaVersion     = "hev.private-preview/v1"
	DefaultKeepBuilds = 2
	MinKeepBuilds     = 1
	MaxKeepBuilds     = 5
	DefaultTimeout    = 10 * time.Minute
	MaxTimeout        = 30 * time.Minute
	DefaultRef        = "origin/main"
	DefaultOutDir     = "dist"
	MinPort           = 1024
	MaxPort           = 65535
	MaxPreviews       = 64
	MaxEnvVars        = 64
	MaxBuildArgs      = 128
)

var (
	previewName    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	envName        = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	tailscaleCGNAT = netip.MustParsePrefix("100.64.0.0/10")
	ipv6Loopback   = netip.MustParseAddr("::1")
)

// Config represents one [[preview]] entry from ~/.hev/config.toml.
type Config struct {
	Name       string            `toml:"name" json:"name"`
	Bind       string            `toml:"bind" json:"bind"`
	Port       int               `toml:"port" json:"port"`
	Repo       string            `toml:"repo,omitempty" json:"repo,omitempty"`
	Ref        string            `toml:"ref,omitempty" json:"ref,omitempty"`
	Subdir     string            `toml:"subdir,omitempty" json:"subdir,omitempty"`
	BuildCmd   []string          `toml:"build_cmd,omitempty" json:"build_cmd,omitempty"`
	OutDir     string            `toml:"out_dir,omitempty" json:"out_dir,omitempty"`
	Root       string            `toml:"root,omitempty" json:"root,omitempty"`
	KeepBuilds *int              `toml:"keep_builds,omitempty" json:"-"`
	Timeout    string            `toml:"timeout,omitempty" json:"-"`
	Env        map[string]string `toml:"env,omitempty" json:"-"`

	bindAddr   netip.Addr
	keepBuilds int
	timeout    time.Duration
}

// Load reads and validates all [[preview]] entries from path.
// A missing config file returns nil, nil.
func Load(path string) ([]Config, error) {
	var file struct {
		Preview []Config `toml:"preview"`
	}
	md, err := toml.DecodeFile(path, &file)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		var parse toml.ParseError
		if errors.As(err, &parse) {
			return nil, fmt.Errorf("invalid preview configuration at line %d", parse.Position.Line)
		}
		return nil, errors.New("could not read preview configuration")
	}
	for _, key := range md.Undecoded() {
		if len(key) > 1 && key[0] == "preview" {
			return nil, fmt.Errorf("unknown field %q in [[preview]]", key[len(key)-1])
		}
	}
	if len(file.Preview) > MaxPreviews {
		return nil, fmt.Errorf("at most %d previews may be configured", MaxPreviews)
	}
	seen := make(map[string]bool, len(file.Preview))
	for i := range file.Preview {
		p := &file.Preview[i]
		if err := p.Validate(); err != nil {
			return nil, fmt.Errorf("preview entry %d: %w", i+1, err)
		}
		if seen[p.Name] {
			return nil, fmt.Errorf("preview entry %d has duplicate name %q", i+1, p.Name)
		}
		seen[p.Name] = true
	}
	return file.Preview, nil
}

// ValidateBind structurally enforces private-only bind addresses:
// only IPv4 loopback (127.0.0.0/8), IPv6 loopback (::1), or Tailscale CGNAT
// (100.64.0.0/10) literal IP addresses are permitted.
func ValidateBind(bind string) (netip.Addr, error) {
	if bind == "" || strings.TrimSpace(bind) != bind {
		return netip.Addr{}, errors.New("bind must be a literal loopback or Tailscale IP address")
	}
	addr, err := netip.ParseAddr(bind)
	if err != nil || addr.Zone() != "" || addr.Is4In6() {
		return netip.Addr{}, errors.New("bind must be a literal loopback (127.0.0.1/::1) or Tailscale CGNAT (100.64.0.0/10) IP address")
	}
	if addr.Is4() && (addr.IsLoopback() || tailscaleCGNAT.Contains(addr)) {
		return addr, nil
	}
	if addr == ipv6Loopback {
		return addr, nil
	}
	return netip.Addr{}, fmt.Errorf("bind address %q is refused: must be loopback (127.0.0.1/::1) or Tailscale CGNAT (100.64.0.0/10)", bind)
}

// ValidatePort ensures port is in the unprivileged TCP range 1024..65535.
func ValidatePort(port int) error {
	if port < MinPort || port > MaxPort {
		return fmt.Errorf("port must be in %d..%d", MinPort, MaxPort)
	}
	return nil
}

// Validate validates and applies defaults to c.
func (c *Config) Validate() error {
	if !previewName.MatchString(c.Name) || len(c.Name) > 128 {
		return errors.New("name must match ^[a-z0-9][a-z0-9-]*$ and fit in 128 bytes")
	}
	addr, err := ValidateBind(c.Bind)
	if err != nil {
		return err
	}
	c.bindAddr = addr
	c.Bind = addr.String()

	if err := ValidatePort(c.Port); err != nil {
		return err
	}

	hasRepo := strings.TrimSpace(c.Repo) != ""
	hasRoot := strings.TrimSpace(c.Root) != ""
	if hasRepo == hasRoot {
		return errors.New("set exactly one of repo or root")
	}
	if strings.ContainsRune(c.Repo, 0) || strings.ContainsRune(c.Root, 0) {
		return errors.New("repo and root must not contain NUL bytes")
	}

	if hasRoot {
		if c.Ref != "" || c.Subdir != "" || len(c.BuildCmd) > 0 || c.OutDir != "" {
			return errors.New("root cannot be combined with ref, subdir, build_cmd or out_dir")
		}
	} else {
		if strings.TrimSpace(c.Repo) != c.Repo || strings.HasPrefix(c.Repo, "-") {
			return errors.New("repo must not have leading/trailing whitespace or start with '-'")
		}
		if c.Ref == "" {
			c.Ref = DefaultRef
		}
		if err := validateRef(c.Ref); err != nil {
			return err
		}
		if c.Subdir != "" {
			if err := validateRelPath(c.Subdir, false); err != nil {
				return fmt.Errorf("subdir: %w", err)
			}
			c.Subdir = path.Clean(c.Subdir)
		}
		if c.OutDir == "" {
			c.OutDir = DefaultOutDir
		}
		if err := validateRelPath(c.OutDir, true); err != nil {
			return fmt.Errorf("out_dir: %w", err)
		}
		c.OutDir = path.Clean(c.OutDir)
		if len(c.BuildCmd) > MaxBuildArgs {
			return fmt.Errorf("build_cmd supports at most %d arguments", MaxBuildArgs)
		}
		if len(c.BuildCmd) > 0 {
			if strings.TrimSpace(c.BuildCmd[0]) == "" {
				return errors.New("build_cmd requires a nonempty executable")
			}
			for _, arg := range c.BuildCmd {
				if strings.ContainsRune(arg, 0) {
					return errors.New("build_cmd must not contain NUL bytes")
				}
			}
		}
	}

	if c.KeepBuilds == nil {
		c.keepBuilds = DefaultKeepBuilds
	} else {
		if *c.KeepBuilds < MinKeepBuilds || *c.KeepBuilds > MaxKeepBuilds {
			return fmt.Errorf("keep_builds must be between %d and %d", MinKeepBuilds, MaxKeepBuilds)
		}
		c.keepBuilds = *c.KeepBuilds
	}

	if c.Timeout == "" {
		c.timeout = DefaultTimeout
	} else {
		d, err := time.ParseDuration(c.Timeout)
		if err != nil || d <= 0 || d > MaxTimeout {
			return fmt.Errorf("timeout must be a positive Go duration up to %s", MaxTimeout)
		}
		c.timeout = d
	}

	if len(c.Env) > MaxEnvVars {
		return fmt.Errorf("env supports at most %d entries", MaxEnvVars)
	}
	seenEnv := make(map[string]bool, len(c.Env))
	for k, v := range c.Env {
		upper := strings.ToUpper(k)
		if !envName.MatchString(k) || seenEnv[upper] {
			return errors.New("env keys must match ^[A-Za-z_][A-Za-z0-9_]*$ and be case-insensitively distinct")
		}
		if strings.ContainsRune(v, 0) {
			return errors.New("env values must not contain NUL bytes")
		}
		seenEnv[upper] = true
	}
	return nil
}

func validateRef(ref string) error {
	if ref == "" || strings.TrimSpace(ref) != ref {
		return errors.New("ref must not be empty or contain surrounding whitespace")
	}
	if strings.HasPrefix(ref, "-") || strings.ContainsAny(ref, "\x00\r\n \t") || strings.Contains(ref, "..") {
		return errors.New("ref contains disallowed characters or prefix")
	}
	return nil
}

func validateRelPath(p string, allowDot bool) error {
	if p == "" || strings.ContainsRune(p, 0) {
		return errors.New("must be a nonempty relative path")
	}
	if strings.ContainsRune(p, '\\') || strings.HasPrefix(p, "/") {
		return errors.New("must use forward slashes and be relative")
	}
	if len(p) >= 2 && p[1] == ':' {
		return errors.New("must not contain a drive prefix")
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return errors.New("must not contain '..'")
		}
	}
	clean := path.Clean(p)
	if clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return errors.New("must remain within the working directory")
	}
	if !allowDot && clean == "." {
		return errors.New("must name a subdirectory")
	}
	return nil
}

// Mode returns "static" when root is configured, else "repo".
func (c Config) Mode() string {
	if strings.TrimSpace(c.Root) != "" {
		return "static"
	}
	return "repo"
}

// EffectiveKeepBuilds returns the validated retention limit.
func (c Config) EffectiveKeepBuilds() int {
	if c.keepBuilds > 0 {
		return c.keepBuilds
	}
	if c.KeepBuilds != nil {
		return *c.KeepBuilds
	}
	return DefaultKeepBuilds
}

// EffectiveTimeout returns the validated sync timeout.
func (c Config) EffectiveTimeout() time.Duration {
	if c.timeout > 0 {
		return c.timeout
	}
	if c.Timeout != "" {
		if d, err := time.ParseDuration(c.Timeout); err == nil && d > 0 {
			return d
		}
	}
	return DefaultTimeout
}

// EffectiveOutDir returns the validated output directory for repo previews.
func (c Config) EffectiveOutDir() string {
	if c.OutDir != "" {
		return c.OutDir
	}
	return DefaultOutDir
}

// URL returns the listen URL for this preview.
func (c Config) URL() string {
	bind := c.Bind
	if c.bindAddr.IsValid() {
		bind = c.bindAddr.String()
	}
	return "http://" + net.JoinHostPort(bind, strconv.Itoa(c.Port))
}

// ExpandedRoot expands a leading ~ against home for static-root previews.
func (c Config) ExpandedRoot(home string) string {
	p := c.Root
	if p == "" {
		return ""
	}
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		return filepath.Join(home, p[2:])
	}
	return p
}

// EnvKeys returns sorted configured environment variable names.
func (c Config) EnvKeys() []string {
	if len(c.Env) == 0 {
		return nil
	}
	keys := make([]string, 0, len(c.Env))
	for k := range c.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// EnvFingerprint computes a deterministic SHA-256 hex digest of env.
func EnvFingerprint(env map[string]string) string {
	h := sha256.New()
	if len(env) > 0 {
		keys := make([]string, 0, len(env))
		for k := range env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := env[k]
			fmt.Fprintf(h, "%d:%s\x00%d:%s\x00", len(k), k, len(v), v)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// SanitizeRepo strips URL credentials and replaces local filesystem paths with
// a basename-only label so preview.json and logs never leak secrets or host paths.
func SanitizeRepo(repo string) string {
	repo = strings.TrimSpace(repo)
	if repo == "" {
		return ""
	}
	if strings.Contains(repo, "://") {
		u, err := url.Parse(repo)
		if err == nil && u.Scheme != "" {
			if strings.EqualFold(u.Scheme, "file") {
				return localRepoLabel(u.Path)
			}
			if u.User != nil {
				if _, hasPass := u.User.Password(); hasPass || !strings.EqualFold(u.Scheme, "ssh") {
					u.User = nil
				}
			}
			return u.String()
		}
	}
	// SCP-like SSH syntax: [user@]host:path (exclude Windows drive paths like C:\...)
	if colon := strings.IndexByte(repo, ':'); colon > 1 && !strings.Contains(repo[:colon], "/") && !strings.Contains(repo[:colon], `\`) {
		hostPart := repo[:colon]
		pathPart := repo[colon+1:]
		if at := strings.IndexByte(hostPart, '@'); at >= 0 {
			userPart := hostPart[:at]
			host := hostPart[at+1:]
			if strings.Contains(userPart, ":") {
				return host + ":" + pathPart
			}
		}
		return repo
	}
	return localRepoLabel(repo)
}

func localRepoLabel(p string) string {
	clean := filepath.Clean(strings.ReplaceAll(p, `\`, "/"))
	base := path.Base(filepath.ToSlash(clean))
	if base == "" || base == "." || base == "/" {
		return "local:repo"
	}
	return "local:" + base
}

// PreviewDir returns ~/.hev/previews/<name> under home.
func PreviewDir(home, name string) string {
	return filepath.Join(home, ".hev", "previews", name)
}
