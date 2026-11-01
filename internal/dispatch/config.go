package dispatch

import (
	"fmt"
	"github.com/BurntSushi/toml"
	"github.com/hev/kit/internal/trace"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Host struct {
	Type            string            `toml:"type"`
	From            string            `toml:"from"`
	Size            string            `toml:"size"`
	Region          string            `toml:"region"`
	AutoStopMinutes int               `toml:"auto_stop_minutes"`
	APIKeyEnv       string            `toml:"api_key_env"`
	Environment     string            `toml:"environment"`
	Name            string            `toml:"-"`
	DispatchConfig  string            `toml:"-"`
	Notify          []string          `toml:"-"`
	Workspace       string            `toml:"workspace"`
	HarnessWrapper  []string          `toml:"harness_wrapper"`
	ConfigPath      string            `toml:"config_path"`
	Env             map[string]string `toml:"env"`
	SSH             string            `toml:"ssh"`
	Cap             int               `toml:"cap"`
	Local           bool              `toml:"local"`
	OS              string            `toml:"os"`
	PathPrefix      []string          `toml:"path_prefix"`
	Namespace       string            `toml:"namespace"`
}
type ValidationConfig struct {
	AllowSameHost bool   `toml:"allow_same_host"`
	Host          string `toml:"host"`
	Harness       string `toml:"harness"`
	Model         string `toml:"model"`
}
type Config struct {
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
	c := Config{Workspace: "~/workspace"}
	path = ConfigPath(path)
	if !trace.WindowsPath(path) {
		var err error
		path, err = filepath.Abs(path)
		if err != nil {
			return c, err
		}
	}
	var md toml.MetaData
	var e error
	if target {
		var raw struct {
			Workspace   string                    `toml:"workspace"`
			Hosts       map[string]toml.Primitive `toml:"hosts"`
			Notify      toml.Primitive            `toml:"notify"`
			Validation  toml.Primitive            `toml:"validation"`
			DefaultHost toml.Primitive            `toml:"default_host"`
		}
		md, e = toml.DecodeFile(path, &raw)
		if e == nil {
			if raw.Workspace != "" {
				c.Workspace = raw.Workspace
			}
			c.Hosts = map[string]Host{}
			for n, primitive := range raw.Hosts {
				var h Host
				err := md.PrimitiveDecode(primitive, &h)
				if err != nil {
					if n == host {
						return c, err
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
		return c, e
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
		if !ValidName(n) || h.Cap < 1 || (h.Type != "boat" && !h.Local && h.SSH == "") {
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

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func validateHostType(h Host, n string) (Host, error) {
	if h.Type != "" && h.Type != "boat" {
		return h, fmt.Errorf("unknown host type %q", h.Type)
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
		if !envName.MatchString(h.APIKeyEnv) {
			return h, fmt.Errorf("invalid api_key_env")
		}
	}
	return h, nil
}
