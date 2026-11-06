package main

import (
	"os"
	"path/filepath"
	"strings"
)

func daemonEnvironmentInScope(env []string) bool {
	values := map[string]string{}
	for _, s := range env {
		k, v, ok := strings.Cut(s, "=")
		if ok {
			values[k] = v
		}
	}
	home, err := os.UserHomeDir()
	if err != nil || values["HOME"] != home {
		return false
	}
	config := func(h, c string) string {
		if c == "" {
			c = filepath.Join(h, ".hev", "config.toml")
		}
		p, e := filepath.Abs(c)
		if e != nil {
			return ""
		}
		return p
	}
	// Relative explicit paths cannot safely be compared across process working directories.
	if values["HEV_CONFIG"] != "" && !filepath.IsAbs(values["HEV_CONFIG"]) {
		return false
	}
	return config(home, values["HEV_CONFIG"]) == config(home, os.Getenv("HEV_CONFIG"))
}
