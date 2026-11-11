package settings

import (
	"encoding/json"
	"fmt"
	"github.com/hev/kit/internal/layerenv"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/BurntSushi/toml"
	"github.com/hev/kit/internal/daemon"
)

type Role struct {
	Pattern  string `toml:"pattern"`
	Role     string `toml:"role"`
	Instance string `toml:"instance"`
}

type Config struct {
	ReadOnly     bool   `toml:"read_only"`
	Roles        []Role `toml:"roles"`
	Headless     bool   `toml:"headless"`
	CanvasDir    string `toml:"canvas_dir"`
	Dir          string `toml:"dir"` // filesystem mode: where threads live (default ~/.hev/board)
	PublicURL    string `toml:"public_url"`
	CORS         string `toml:"cors"`
	TrustedProxy bool   `toml:"trusted_proxy"`
	URL          string `toml:"url"`
	Addr         string `toml:"addr"`
	Namespace    string `toml:"namespace"`
	Identity     string `toml:"identity"`
	Endpoint     string `toml:"layer_endpoint"`
	Model        string `toml:"model"`
	// MaxImageBytes caps one uploaded image (default 10 MB).
	MaxImageBytes int64 `toml:"max_image_bytes"`
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
		"FACTORY_CANVAS_DIR":           &Current.CanvasDir, "HEV_BOARD_DIR": &Current.Dir, "FACTORY_BOARD_PUBLIC_URL": &Current.PublicURL, "FACTORY_BOARD_CORS": &Current.CORS,
		"HEVLAYER_EMBED_MODEL": &Current.Model,
	} {
		if value := layerenv.Get(name); value != "" {
			*target = value
		}
	}
	if v := os.Getenv("HEV_BOARD_MAX_IMAGE_BYTES"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			return fmt.Errorf("HEV_BOARD_MAX_IMAGE_BYTES: want a positive byte count, got %q", v)
		}
		Current.MaxImageBytes = n
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

// Dir is where filesystem mode keeps the board: board.dir, $HEV_BOARD_DIR, or
// ~/.hev/board beside ~/.hev/canvas.
func Dir() string {
	if Current.Dir != "" {
		return Current.Dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".hev", "board")
	}
	return filepath.Join(home, ".hev", "board")
}
