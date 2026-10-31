package settings

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"

	"github.com/BurntSushi/toml"
	"github.com/hev/kit/internal/daemon"
)

type Role struct {
	Pattern  string `toml:"pattern"`
	Role     string `toml:"role"`
	Instance string `toml:"instance"`
}

type Config struct {
	Roles        []Role `toml:"roles"`
	Headless     bool   `toml:"headless"`
	CanvasDir    string `toml:"canvas_dir"`
	PublicURL    string `toml:"public_url"`
	CORS         string `toml:"cors"`
	TrustedProxy bool   `toml:"trusted_proxy"`
	URL          string `toml:"url"`
	Addr         string `toml:"addr"`
	Namespace    string `toml:"namespace"`
	Identity     string `toml:"identity"`
	Endpoint     string `toml:"layer_endpoint"`
	Model        string `toml:"model"`
}

var Current Config

func Load() error {
	var file struct {
		Board Config `toml:"board"`
	}
	if _, err := toml.DecodeFile(daemon.DefaultConfigPath(), &file); err != nil && !os.IsNotExist(err) {
		return err
	}
	Current = file.Board
	for name, target := range map[string]*string{
		"FACTORY_BOARD_URL": &Current.URL, "FACTORY_BOARD_ADDR": &Current.Addr,
		"FACTORY_BOARD_NAMESPACE":      &Current.Namespace,
		"FACTORY_BOARD_LAYER_ENDPOINT": &Current.Endpoint,
		"FACTORY_CANVAS_DIR":           &Current.CanvasDir, "FACTORY_BOARD_PUBLIC_URL": &Current.PublicURL, "FACTORY_BOARD_CORS": &Current.CORS,
		"LAYER_EMBED_MODEL": &Current.Model,
	} {
		if value := os.Getenv(name); value != "" {
			*target = value
		}
	}
	for _, role := range Current.Roles {
		if _, err := regexp.Compile(role.Pattern); err != nil {
			return fmt.Errorf("board role pattern: %w", err)
		}
	}
	if Current.URL == "" {
		home, _ := os.UserHomeDir()
		var legacy struct {
			URL string `json:"board_url"`
		}
		if b, err := os.ReadFile(filepath.Join(home, ".local/state/factory-feedback/config.json")); err == nil {
			_ = json.Unmarshal(b, &legacy)
		}
		if u, err := url.Parse(legacy.URL); err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil {
			Current.URL = legacy.URL
		}
	}
	if Current.URL == "" {
		Current.URL = "http://127.0.0.1:7700"
	}
	return nil
}
