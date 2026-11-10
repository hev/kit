package dispatch

import (
	"fmt"
	"github.com/BurntSushi/toml"
	"github.com/hev/kit/internal/trace"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Host struct {
	AWS             AWSConfig      `toml:"aws"`
	SSHPublicKeyEnv string         `toml:"ssh_public_key_env"`
	Commands        CommandScripts `toml:"commands"`
	Classes         map[string]int `toml:"-"`
	Type            string         `toml:"type"`
	From            string         `toml:"from"`
	Size            string         `toml:"size"`
	Region          string         `toml:"region"`
	AutoStopMinutes int            `toml:"auto_stop_minutes"`
	APIKeyEnv       string         `toml:"api_key_env"`
	Environment     string         `toml:"environment"`
	Name            string         `toml:"-"`
	DispatchConfig  string         `toml:"-"`
	Notify          []string       `toml:"-"`
	Workspace       string         `toml:"workspace"`
	HarnessWrapper  []string       `toml:"harness_wrapper"`
	ConfigPath      string         `toml:"config_path"`
	Env             []string       `toml:"env"`
	SSH             string         `toml:"ssh"`
	Cap             int            `toml:"cap"`
	Local           bool           `toml:"local"`
	OS              string         `toml:"os"`
	PathPrefix      []string       `toml:"path_prefix"`
	Namespace       string         `toml:"namespace"`
}
type ValidationConfig struct {
	AllowSameHost bool   `toml:"allow_same_host"`
	Host          string `toml:"host"`
	Harness       string `toml:"harness"`
	Model         string `toml:"model"`
}
type Config struct {
	Cap int `toml:"cap"`
	// Classes caps concurrent runs per order class on this target.
	Classes     map[string]int   `toml:"classes"`
	Admission   Admission        `toml:"admission"`
	Validation  ValidationConfig `toml:"validation"`
	Notify      []string         `toml:"notify"`
	Workspace   string           `toml:"workspace"`
	Hosts       map[string]Host  `toml:"hosts"`
	DefaultHost string           `toml:"default_host"`
}

func Expand(s string) string {
	h, _ := os.UserHomeDir()
	if s == "~" {
		return h
	}
	if strings.HasPrefix(s, "~/") {
		return filepath.Join(h, s[2:])
	}
	return s
}
func Load(path string) (Config, error) { return loadConfig(path, "", false) }

// loadTarget validates only the selected host; unrelated routing errors are warnings.
func loadTarget(path, host string) (Config, error) { return loadConfig(path, host, true) }

func loadConfig(path, host string, target bool) (Config, error) {
	c := Config{Workspace: "~/workspace", Cap: 2}
	path = ConfigPath(path)
	if !trace.WindowsPath(path) {
		var err error
		path, err = filepath.Abs(path)
		if err != nil {
			return c, err
		}
	}
	if host == "" && target {
		host, _ = os.Hostname()
	}
	var md toml.MetaData
	var e error
	if target {
		var raw struct {
			Cap         *int                      `toml:"cap"`
			Classes     map[string]int            `toml:"classes"`
			Admission   Admission                 `toml:"admission"`
			Workspace   string                    `toml:"workspace"`
			Hosts       map[string]toml.Primitive `toml:"hosts"`
			Notify      toml.Primitive            `toml:"notify"`
			Validation  toml.Primitive            `toml:"validation"`
			DefaultHost toml.Primitive            `toml:"default_host"`
		}
		md, e = toml.DecodeFile(path, &raw)
		if e == nil {
			c.Admission = raw.Admission
			c.Classes = raw.Classes
			if raw.Cap != nil {
				c.Cap = *raw.Cap
			}
			if raw.Workspace != "" {
				c.Workspace = raw.Workspace
			}
			c.Hosts = map[string]Host{}
			for n, primitive := range raw.Hosts {
				var h Host
				err := md.PrimitiveDecode(primitive, &h)
				if err != nil {
					if n == host {
						return c, configDecodeError(err)
					}
					fmt.Fprintf(os.Stderr, "warning: invalid host %s: %v\n", n, err)
					continue
				}
				c.Hosts[n] = h
			}
		}
	} else {
		md, e = toml.DecodeFile(path, &c)
	}
	if e != nil {
		return c, configDecodeError(e)
	}
	if target {
		host = legacyTargetHost(c.Hosts, host)
		if !md.IsDefined("cap") {
			if own := c.Hosts[host]; own.Cap > 0 {
				c.Cap = own.Cap
				fmt.Fprintf(os.Stderr, "note: move hosts.%s.cap to top-level cap in the target default config\n", host)
			} else {
				fmt.Fprintln(os.Stderr, "warning: no legacy target cap found; using default cap; set top-level cap in the target default config")
			}
		}
	}
	if c.Cap < 1 {
		return c, fmt.Errorf("cap must be at least 1")
	}
	for class, n := range c.Classes {
		if !ValidLabel(class) || n < 1 {
			return c, fmt.Errorf("classes.%s must name a valid class with a cap of at least 1", class)
		}
	}
	for _, key := range md.Keys() {
		if len(key) >= 3 && key[0] == "hosts" && key[2] == "admission" {
			return c, fmt.Errorf("%s is unsupported; move policy to top-level [admission] in the target default config", key)
		}
	}
	for _, key := range md.Undecoded() {
		if target && (key[0] == "notify" || key[0] == "validation" || key[0] == "default_host") {
			continue
		}
		if target && len(key) > 1 && key[0] == "hosts" && key[1] != host {
			fmt.Fprintf(os.Stderr, "warning: unknown config key %s\n", key)
			continue
		}
		return c, fmt.Errorf("unknown config key: %s", key)
	}
	for n, h := range c.Hosts {
		if (!target && !ValidLabel(n)) || h.Cap < 0 || (!IsCloudHost(h) && !h.Local && h.SSH == "") {
			if target && n != host {
				fmt.Fprintf(os.Stderr, "warning: invalid host %s\n", n)
				continue
			}
			return c, fmt.Errorf("invalid host %s", n)
		}
		if strings.HasPrefix(h.SSH, "-") {
			if target && n != host {
				fmt.Fprintf(os.Stderr, "warning: invalid ssh target for %s\n", n)
				continue
			}
			return c, fmt.Errorf("invalid ssh target")
		}
	}
	for n, h := range c.Hosts {
		var err error
		h, err = validateHostType(h, n)
		if err != nil {
			if target && n != host {
				fmt.Fprintf(os.Stderr, "warning: %v\n", err)
				continue
			}
			return c, err
		}
		h.Name, h.Notify, h.DispatchConfig = n, c.Notify, path
		h.Classes = c.Classes
		if h.ConfigPath != "" && !filepath.IsAbs(Expand(h.ConfigPath)) && !trace.WindowsPath(h.ConfigPath) {
			h.ConfigPath = filepath.Join(filepath.Dir(path), h.ConfigPath)
		}
		c.Hosts[n] = h
	}
	if !target && c.DefaultHost != "" {
		if _, ok := c.Hosts[c.DefaultHost]; !ok {
			return c, fmt.Errorf("unknown default_host %s", c.DefaultHost)
		}
	}
	return c, nil
}
func Root() string { return Expand("~/dispatch") }

// ConfigPath resolves the flag, environment and default in one place.
func ConfigPath(path string) string {
	if path == "" {
		path = os.Getenv("HEV_DISPATCH_CONFIG")
		if path == "" {
			path = Expand("~/.config/hev/dispatch.toml")
		}
	}

	return Expand(path)
}

// Keep filesystem errors intact: callers use os.IsNotExist for optional configs.
func configDecodeError(err error) error {
	if !strings.Contains(err.Error(), ".env") {
		return err
	}
	return fmt.Errorf("hosts.NAME.env must be a list of environment names, e.g. env = [\"CUSTOM_KEY\"], not a table: %w", err)
}

// legacyTargetHost resolves machine identity independently of order-name syntax.
func legacyTargetHost(hosts map[string]Host, hostname string) string {
	var names []string
	var local string
	locals := 0
	for name, h := range hosts {
		names = append(names, name)
		if h.Local {
			local = name
			locals++
		}
	}
	if locals == 1 {
		return local
	}
	sort.Strings(names)
	for _, candidate := range []string{hostname, strings.SplitN(hostname, ".", 2)[0]} {
		for _, name := range names {
			if strings.EqualFold(name, candidate) {
				return name
			}
		}
	}
	return ""
}

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func validateHostType(h Host, n string) (Host, error) {
	if h.Type != "" && !IsCloudHost(h) {
		return h, fmt.Errorf("unknown host type %q", h.Type)
	}
	if IsCloudHost(h) && h.Cap < 1 {
		return h, fmt.Errorf("cloud host cap must be at least 1")
	}
	if h.Type == "aws" {
		if h.Local || h.SSH != "" || h.OS == "windows" {
			return h, fmt.Errorf("AWS requires a Linux cloud host")
		}
		if _, err := NewAWSProvider(h); err != nil {
			return h, err
		}
	}
	if h.Type == "command" {
		if h.Local || h.SSH != "" || h.Commands.Create == "" || h.Commands.List == "" || h.Commands.Delete == "" {
			return h, fmt.Errorf("command host requires create/list/delete scripts and cannot set local or ssh")
		}
		for _, name := range h.Env {
			if !envName.MatchString(name) {
				return h, fmt.Errorf("command env must contain environment variable names only")
			}
		}
	}
	if h.Type == "boat" {
		if h.From == "" || h.Local || h.SSH != "" || h.Workspace != "" || h.OS == "windows" || (h.AutoStopMinutes < 0 || h.AutoStopMinutes > 43200) {
			return h, fmt.Errorf("invalid Boat host %s: from required; workspace, local, ssh and Windows are unsupported", n)
		}
		if h.Size == "" {
			h.Size = "small"
		}
		switch h.Size {
		case "small", "default", "large", "xlarge":
		default:
			return h, fmt.Errorf("invalid Boat size %q", h.Size)
		}
		if h.Region != "" {
			return h, fmt.Errorf("Boat v1 does not expose region selection (EU only)")
		}
		if h.APIKeyEnv == "" {
			h.APIKeyEnv = "BOAT_API_KEY"
		}
		if h.SSHPublicKeyEnv == "" {
			h.SSHPublicKeyEnv = "BOAT_SSH_PUBLIC_KEY"
		}
		if !envName.MatchString(h.SSHPublicKeyEnv) {
			return h, fmt.Errorf("invalid ssh_public_key_env")
		}
		if !envName.MatchString(h.APIKeyEnv) {
			return h, fmt.Errorf("invalid api_key_env")
		}
	}
	return h, nil
}
