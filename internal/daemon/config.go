package daemon

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/hev/kit/internal/version"
)

type Config struct {
	ConfigPath string

	// Layer archive target.
	LayerEndpoint  string
	LayerAPIKey    string
	LayerNamespace string
	// LayerStore names the store kind behind the endpoint for
	// layer.ResolveCapabilities. Empty is the hosted lane.
	LayerStore string

	// Local is the stack `hev up` owns.
	Local LocalConfig

	// S3
	ActiveBucket   string
	Buckets        []BucketConfig
	S3Endpoint     string
	S3Bucket       string
	S3Key          string
	S3Secret       string
	S3SessionToken string
	S3Region       string
	S3Profile      string

	// Scan
	ScanInterval time.Duration
	ScanRoots    []string

	// Capture policy
	ProjectAllow         []string
	ProjectDeny          []string
	UnknownProjectPolicy string
	CaptureRawAPIBodies  bool
	CaptureToolContent   bool

	// Identity
	Host   string
	GHUser string

	// Ledger
	LedgerDir string

	// Legacy envelope spool, read by migration.
	SpoolPath string

	// Claude Code raw API body files written by OTEL_LOG_RAW_API_BODIES=file:<dir>.
	ClaudeRawBodiesDir string

	// Codex CLI rollout JSONL files written under ~/.codex/sessions.
	CodexSessionsDir string
}

type BucketConfig struct {
	Name            string `toml:"name"`
	Endpoint        string `toml:"endpoint"`
	Bucket          string `toml:"bucket"`
	Region          string `toml:"region"`
	Key             string `toml:"key"`
	Secret          string `toml:"secret"`
	SessionToken    string `toml:"session_token"`
	AccessKeyEnv    string `toml:"access_key_env"`
	SecretKeyEnv    string `toml:"secret_key_env"`
	SessionTokenEnv string `toml:"session_token_env"`
	Profile         string `toml:"profile"`
}

// LocalConfig is the `[local]` block: the things about the stack `hev up`
// runs that must not need a recompile — the gateway image and port, the
// dashboard image and port, and the Compose project name. Managed is set once
// `hev up` has written the block, and is what gates the daemon's first scan on
// gateway health.
type LocalConfig struct {
	Managed   bool
	Image     string
	Port      int
	Project   string
	KitImage  string
	ServePort int
}

const (
	DefaultLocalImage     = "hevlayer/layer-gateway:0.6.0"
	DefaultLocalPort      = 8080
	DefaultLocalProject   = "hev-kit"
	DefaultLocalServePort = 8099
	// LegacyLocalNamespace is what `hev up` named the archive when the local
	// stack was a lexical Postgres store. A config still carrying it is moved
	// to the default namespace the first time `up` points it at Turbopuffer.
	LegacyLocalNamespace = "hev-traces-local"
)

// priorLocalImages are gateway images an earlier kit wrote into [local] as
// its default. A config naming one follows this binary's default instead, so
// an upgrade moves the gateway with it. Add the old default here whenever
// DefaultLocalImage changes.
var priorLocalImages = map[string]bool{"hevlayer/layer-gateway:edge": true}

// kitImageRepo is where every kit release pushes its dashboard. A config
// naming an image there was written by `hev up` for some release, and the
// dashboard follows this binary instead, so it never pairs with a daemon from
// another release. HEV_LOCAL_KIT_IMAGE still wins.
const kitImageRepo = "hevlayer/kit:"

type localFileConfig struct {
	Image     string `toml:"image"`
	Port      int    `toml:"port"`
	Project   string `toml:"project"`
	KitImage  string `toml:"kit_image"`
	ServePort int    `toml:"serve_port"`
}

type fileConfig struct {
	Layer        layerConfig      `toml:"layer"`
	Local        *localFileConfig `toml:"local,omitempty"`
	ActiveBucket string           `toml:"active_bucket"`
	Capture      captureConfig    `toml:"capture"`
	Projects     projectsConfig   `toml:"projects"`
	Buckets      []BucketConfig   `toml:"buckets"`
}

type layerConfig struct {
	Endpoint  string `toml:"endpoint"`
	APIKey    string `toml:"api_key"`
	Namespace string `toml:"namespace"`
	Store     string `toml:"store,omitempty"`
}

type captureConfig struct {
	ScanInterval       string   `toml:"scan_interval"`
	ScanRoots          []string `toml:"scan_roots"`
	RawAPIBodies       *bool    `toml:"raw_api_bodies"`
	ToolContent        *bool    `toml:"tool_content"`
	UnknownProject     string   `toml:"unknown_project"`
	ClaudeRawBodiesDir string   `toml:"claude_raw_bodies_dir"`
	CodexSessionsDir   string   `toml:"codex_sessions_dir"`
}

type projectsConfig struct {
	Allow []string `toml:"allow"`
	Deny  []string `toml:"deny"`
}

func LoadConfig() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("home dir: %w", err)
	}

	hostname, _ := os.Hostname()

	hevDir := home + "/.hev"

	c := &Config{
		ConfigPath:     DefaultConfigPath(),
		LayerNamespace: "hev-traces",
		Local: LocalConfig{
			Image:     DefaultLocalImage,
			Port:      DefaultLocalPort,
			Project:   DefaultLocalProject,
			KitImage:  version.KitImage(),
			ServePort: DefaultLocalServePort,
		},

		ActiveBucket: "local",
		Buckets: []BucketConfig{{
			Name:     "local",
			Endpoint: "http://localhost:9000",
			Bucket:   "hev-loop-raw",
			Key:      "loop",
			Secret:   "hev-loop-homelab",
		}},
		S3Endpoint: "http://localhost:9000",
		S3Bucket:   "hev-loop-raw",
		S3Key:      "loop",
		S3Secret:   "hev-loop-homelab",
		S3Region:   "us-east-1",

		ScanRoots:            []string{"~/workspace"},
		ProjectAllow:         []string{"~/workspace/**"},
		ProjectDeny:          []string{},
		UnknownProjectPolicy: "allow",
		CaptureRawAPIBodies:  false,
		CaptureToolContent:   false,

		Host:      envOr("HEVD_HOST", hostname),
		GHUser:    envOr("HEVD_GH_USER", resolveGHUser()),
		LedgerDir: envOr("HEVD_DATA_DIR", hevDir),

		SpoolPath:          envOr("HEV_SPOOL_PATH", hevDir+"/spool/envelopes.jsonl"),
		ClaudeRawBodiesDir: envOr("HEV_CLAUDE_RAW_BODIES_DIR", hevDir+"/claude-code/raw-api-bodies"),
		CodexSessionsDir:   envOr("HEV_CODEX_SESSIONS_DIR", home+"/.codex/sessions"),
	}

	if err := applyConfigFile(c); err != nil {
		return nil, err
	}
	if err := applyEnvOverrides(c); err != nil {
		return nil, err
	}
	if err := c.applyActiveBucket(); err != nil {
		return nil, err
	}
	c.expandPaths()

	if c.ScanInterval == 0 {
		c.ScanInterval = 5 * time.Minute
	}
	return c, nil
}

func DefaultConfigPath() string {
	if p := os.Getenv("HEV_CONFIG"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "config.toml"
	}
	return filepath.Join(home, ".hev", "config.toml")
}

func DefaultConfigText() string {
	return `# hev kit daemon configuration.
# Env vars still override these values when present.

[layer]
endpoint = "https://gcp-us-central1.turbopuffer.com"
api_key = ""
namespace = "hev-traces"

[capture]
scan_interval = "5m"
`
}

func WriteDefaultConfig(force bool) (string, error) {
	path := DefaultConfigPath()
	if !force {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		} else if !os.IsNotExist(err) {
			return "", err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	return path, os.WriteFile(path, []byte(DefaultConfigText()), 0o600)
}

func SetActiveBucket(name string) (string, error) {
	path := DefaultConfigPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if _, err := WriteDefaultConfig(false); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}

	var fc fileConfig
	if _, err := toml.DecodeFile(path, &fc); err != nil {
		return "", fmt.Errorf("read config %s: %w", path, err)
	}
	found := false
	for _, bucket := range fc.Buckets {
		if bucket.Name == name {
			found = true
			break
		}
	}
	if !found {
		return "", fmt.Errorf("bucket %q not found in %s", name, path)
	}
	fc.ActiveBucket = name

	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(fc); err != nil {
		return "", fmt.Errorf("encode config: %w", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func applyConfigFile(c *Config) error {
	if c.ConfigPath == "" {
		return nil
	}
	if _, err := os.Stat(c.ConfigPath); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("stat config %s: %w", c.ConfigPath, err)
	}

	var fc fileConfig
	if _, err := toml.DecodeFile(c.ConfigPath, &fc); err != nil {
		return fmt.Errorf("read config %s: %w", c.ConfigPath, err)
	}
	if fc.Layer.Endpoint != "" {
		c.LayerEndpoint = fc.Layer.Endpoint
	}
	if fc.Layer.APIKey != "" {
		c.LayerAPIKey = fc.Layer.APIKey
	}
	if fc.Layer.Namespace != "" {
		c.LayerNamespace = fc.Layer.Namespace
	}
	if fc.Layer.Store != "" {
		c.LayerStore = fc.Layer.Store
	}
	if l := fc.Local; l != nil {
		c.Local.Managed = true
		if l.Image != "" && !priorLocalImages[l.Image] {
			c.Local.Image = l.Image
		}
		if l.Port != 0 {
			c.Local.Port = l.Port
		}
		if l.Project != "" {
			c.Local.Project = l.Project
		}
		if l.KitImage != "" && !strings.HasPrefix(l.KitImage, kitImageRepo) {
			c.Local.KitImage = l.KitImage
		}
		if l.ServePort != 0 {
			c.Local.ServePort = l.ServePort
		}
	}
	if fc.ActiveBucket != "" {
		c.ActiveBucket = fc.ActiveBucket
	}
	if fc.Capture.ScanInterval != "" {
		d, err := time.ParseDuration(fc.Capture.ScanInterval)
		if err != nil {
			return fmt.Errorf("parse capture.scan_interval=%q: %w", fc.Capture.ScanInterval, err)
		}
		c.ScanInterval = d
	}
	if len(fc.Capture.ScanRoots) > 0 {
		c.ScanRoots = fc.Capture.ScanRoots
	}
	if fc.Capture.RawAPIBodies != nil {
		c.CaptureRawAPIBodies = *fc.Capture.RawAPIBodies
	}
	if fc.Capture.ToolContent != nil {
		c.CaptureToolContent = *fc.Capture.ToolContent
	}
	if fc.Capture.UnknownProject != "" {
		c.UnknownProjectPolicy = fc.Capture.UnknownProject
	}
	if fc.Capture.ClaudeRawBodiesDir != "" {
		c.ClaudeRawBodiesDir = fc.Capture.ClaudeRawBodiesDir
	}
	if fc.Capture.CodexSessionsDir != "" {
		c.CodexSessionsDir = fc.Capture.CodexSessionsDir
	}
	if len(fc.Projects.Allow) > 0 {
		c.ProjectAllow = fc.Projects.Allow
	}
	if fc.Projects.Deny != nil {
		c.ProjectDeny = fc.Projects.Deny
	}
	if len(fc.Buckets) > 0 {
		c.Buckets = fc.Buckets
	}
	return nil
}

func applyEnvOverrides(c *Config) error {
	if v := os.Getenv("LAYER_ENDPOINT"); v != "" {
		c.LayerEndpoint = v
	}
	if v := os.Getenv("LAYER_API_KEY"); v != "" {
		c.LayerAPIKey = v
	}
	if v := os.Getenv("LAYER_NAMESPACE"); v != "" {
		c.LayerNamespace = v
	}
	if v := os.Getenv("HEV_LOCAL_IMAGE"); v != "" {
		c.Local.Image = v
	}
	if v := os.Getenv("HEV_LOCAL_PROJECT"); v != "" {
		c.Local.Project = v
	}
	if v := os.Getenv("HEV_LOCAL_KIT_IMAGE"); v != "" {
		c.Local.KitImage = v
	}
	for env, dst := range map[string]*int{"HEV_LOCAL_PORT": &c.Local.Port, "HEV_LOCAL_SERVE_PORT": &c.Local.ServePort} {
		if v := os.Getenv(env); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n <= 0 || n > 65535 {
				return fmt.Errorf("parse %s=%q: not a TCP port", env, v)
			}
			*dst = n
		}
	}
	if v := os.Getenv("HEVD_ACTIVE_BUCKET"); v != "" {
		c.ActiveBucket = v
	}
	if v := os.Getenv("HEVD_SCAN_ROOTS"); v != "" {
		c.ScanRoots = splitList(v)
	}
	if v := os.Getenv("HEVD_PROJECT_ALLOW"); v != "" {
		c.ProjectAllow = splitList(v)
	}
	if v := os.Getenv("HEVD_PROJECT_DENY"); v != "" {
		c.ProjectDeny = splitList(v)
	}
	if v := os.Getenv("HEVD_UNKNOWN_PROJECT"); v != "" {
		c.UnknownProjectPolicy = v
	}
	if v := os.Getenv("HEVD_CAPTURE_RAW_API_BODIES"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("parse HEVD_CAPTURE_RAW_API_BODIES=%q: %w", v, err)
		}
		c.CaptureRawAPIBodies = b
	}
	if v := os.Getenv("HEVD_CAPTURE_TOOL_CONTENT"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("parse HEVD_CAPTURE_TOOL_CONTENT=%q: %w", v, err)
		}
		c.CaptureToolContent = b
	}
	if v := os.Getenv("HEVD_S3_ENDPOINT"); v != "" {
		c.S3Endpoint = v
	}
	if v := os.Getenv("HEVD_S3_BUCKET"); v != "" {
		c.S3Bucket = v
	}
	if v := os.Getenv("HEVD_S3_KEY"); v != "" {
		c.S3Key = v
	}
	if v := os.Getenv("HEVD_S3_SECRET"); v != "" {
		c.S3Secret = v
	}
	if v := os.Getenv("HEVD_S3_SESSION_TOKEN"); v != "" {
		c.S3SessionToken = v
	}
	if v := os.Getenv("HEVD_S3_REGION"); v != "" {
		c.S3Region = v
	}
	if v := os.Getenv("AWS_PROFILE"); v != "" {
		c.S3Profile = v
	}

	intervalStr := os.Getenv("HEVD_SCAN_INTERVAL")
	if intervalStr != "" {
		d, err := time.ParseDuration(intervalStr)
		if err != nil {
			return fmt.Errorf("parse HEVD_SCAN_INTERVAL=%q: %w", intervalStr, err)
		}
		c.ScanInterval = d
	}
	return nil
}

func (c *Config) applyActiveBucket() error {
	if len(c.Buckets) == 0 {
		return nil
	}

	var selected *BucketConfig
	if c.ActiveBucket != "" {
		for i := range c.Buckets {
			if c.Buckets[i].Name == c.ActiveBucket {
				selected = &c.Buckets[i]
				break
			}
		}
		if selected == nil {
			return fmt.Errorf("active bucket %q not found in config", c.ActiveBucket)
		}
	} else {
		selected = &c.Buckets[0]
		c.ActiveBucket = selected.Name
	}

	if selected.Endpoint != "" && os.Getenv("HEVD_S3_ENDPOINT") == "" {
		c.S3Endpoint = selected.Endpoint
	}
	if selected.Bucket != "" && os.Getenv("HEVD_S3_BUCKET") == "" {
		c.S3Bucket = selected.Bucket
	}
	if selected.Region != "" && os.Getenv("HEVD_S3_REGION") == "" {
		c.S3Region = selected.Region
	}
	if os.Getenv("HEVD_S3_KEY") == "" {
		c.S3Key = valueOrEnv(selected.Key, selected.AccessKeyEnv, "")
	}
	if os.Getenv("HEVD_S3_SECRET") == "" {
		c.S3Secret = valueOrEnv(selected.Secret, selected.SecretKeyEnv, "")
	}
	if os.Getenv("HEVD_S3_SESSION_TOKEN") == "" {
		c.S3SessionToken = valueOrEnv(selected.SessionToken, selected.SessionTokenEnv, "")
	}
	if selected.Profile != "" && os.Getenv("AWS_PROFILE") == "" {
		c.S3Profile = selected.Profile
	}
	// When the active bucket defines its own credentials (static keys or env
	// references) and does not specify a profile, drop any inherited
	// AWS_PROFILE — otherwise an ambient profile from the user's shell would
	// hijack credential resolution and bypass the bucket's explicit keys.
	bucketHasStaticCreds := selected.Key != "" || selected.Secret != "" ||
		selected.SessionToken != "" || selected.AccessKeyEnv != "" ||
		selected.SecretKeyEnv != "" || selected.SessionTokenEnv != ""
	if bucketHasStaticCreds && selected.Profile == "" {
		c.S3Profile = ""
	}
	return nil
}

func (c *Config) expandPaths() {
	c.ScanRoots = expandPathList(c.ScanRoots)
	c.ProjectAllow = expandPathList(c.ProjectAllow)
	c.ProjectDeny = expandPathList(c.ProjectDeny)
	c.ClaudeRawBodiesDir = expandPath(c.ClaudeRawBodiesDir)
	c.CodexSessionsDir = expandPath(c.CodexSessionsDir)
	c.LedgerDir = expandPath(c.LedgerDir)
	c.SpoolPath = expandPath(c.SpoolPath)
}

func valueOrEnv(value, envName, fallback string) string {
	if envName != "" {
		if v := os.Getenv(envName); v != "" {
			return v
		}
	}
	if value != "" {
		return value
	}
	return fallback
}

// resolveGHUser shells out to `gh config get -h github.com user`, which reads
// the locally cached login from ~/.config/gh/hosts.yml without making a network
// call. Returns "" silently when gh is missing, unauthenticated, or slow.
func resolveGHUser() string {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "gh", "config", "get", "-h", "github.com", "user").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func splitList(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func expandPathList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, expandPath(value))
	}
	return out
}

func expandPath(p string) string {
	if p == "" || p == "~" {
		if p == "" {
			return ""
		}
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
		return p
	}
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}
