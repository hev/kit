package dispatch

import (
	"fmt"
	"github.com/BurntSushi/toml"
	"os"
	"path/filepath"
	"strings"
)

type Host struct {
	ConfigPath string            `toml:"config_path"`
	Env        map[string]string `toml:"env"`
	SSH        string            `toml:"ssh"`
	Cap        int               `toml:"cap"`
	Local      bool              `toml:"local"`
	OS         string            `toml:"os"`
	PathPrefix []string          `toml:"path_prefix"`
	Namespace  string            `toml:"namespace"`
}
type ValidationConfig struct {
	Host    string `toml:"host"`
	Harness string `toml:"harness"`
	Model   string `toml:"model"`
}
type Config struct {
	Validation ValidationConfig `toml:"validation"`
	Notify     []string         `toml:"notify"`
	Workspace  string           `toml:"workspace"`
	Hosts      map[string]Host  `toml:"hosts"`
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
func Load(path string) (Config, error) {
	c := Config{Workspace: "~/workspace"}
	path = ConfigPath(path)
	md, e := toml.DecodeFile(Expand(path), &c)
	if e != nil {
		return c, e
	}
	if len(md.Undecoded()) > 0 {
		return c, fmt.Errorf("unknown config keys: %v", md.Undecoded())
	}
	for n, h := range c.Hosts {
		if !ValidName(n) || h.Cap < 1 || (!h.Local && h.SSH == "") {
			return c, fmt.Errorf("invalid host %s", n)
		}
		if strings.HasPrefix(h.SSH, "-") {
			return c, fmt.Errorf("invalid ssh target")
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
