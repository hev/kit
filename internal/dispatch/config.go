package dispatch

import (
	"fmt"
	"github.com/BurntSushi/toml"
	"os"
	"path/filepath"
	"strings"
)

type Host struct {
	SSH        string   `toml:"ssh"`
	Cap        int      `toml:"cap"`
	Local      bool     `toml:"local"`
	OS         string   `toml:"os"`
	PathPrefix []string `toml:"path_prefix"`
	Namespace  string   `toml:"namespace"`
}
type Config struct {
	Notify    []string        `toml:"notify"`
	Workspace string          `toml:"workspace"`
	Hosts     map[string]Host `toml:"hosts"`
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
	if path == "" {
		path = Expand("~/.config/hev/dispatch.toml")
	}
	md, e := toml.DecodeFile(path, &c)
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
