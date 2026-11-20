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
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/platformops"
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

	// URLSet is true when board.url, $FACTORY_BOARD_URL or the legacy
	// feedback config names a board server. Without one, a host in
	// filesystem mode works on its own files and needs no server.
	URLSet bool `toml:"-"`
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
	Current.URLSet = Current.URL != ""
	if Current.URL == "" {
		Current.URL = DefaultURL
	}
	return nil
}

// DefaultURL is where `hev board serve` listens unless board.addr says
// otherwise, and where clients look for it when nothing names a server.
const DefaultURL = "http://127.0.0.1:7700"

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

// EnsureFilesDefault writes the board's filesystem-mode default into kit's
// config at configPath ("" means the default path): board.dir =
// ~/.hev/board, unless [board] already names a dir. It adds a [board] table
// when there is none and never changes any other key. `hev up` calls it when
// it sets a machine up without a Layer key; changed reports whether the file
// was written.
func EnsureFilesDefault(configPath string) (changed bool, err error) {
	if configPath == "" {
		configPath = daemon.DefaultConfigPath()
	}
	raw, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	var file struct {
		Board struct {
			Dir string `toml:"dir"`
		} `toml:"board"`
	}
	if _, err := toml.Decode(string(raw), &file); err != nil {
		return false, fmt.Errorf("%s: %w", configPath, err)
	}
	if file.Board.Dir != "" {
		return false, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false, err
	}
	line := "dir = " + strconv.Quote(filepath.Join(home, ".hev", "board")) + "\n"
	text := string(raw)
	if loc := boardHeader.FindStringIndex(text); loc != nil {
		head := text[:loc[1]]
		if !strings.HasSuffix(head, "\n") {
			head += "\n"
		}
		text = head + line + text[loc[1]:]
	} else {
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		if text != "" {
			text += "\n"
		}
		text += "[board]\n" + line
	}
	if _, err := toml.Decode(text, &file); err != nil || file.Board.Dir == "" {
		return false, fmt.Errorf("%s: could not add board.dir (%v)", configPath, err)
	}
	if err := platformops.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		return false, err
	}
	if err := platformops.WriteFileAtomic(configPath, []byte(text), 0o600); err != nil {
		return false, err
	}
	return true, nil
}

// boardHeader is the [board] table's header line, through its newline.
var boardHeader = regexp.MustCompile(`(?m)^[ \t]*\[[ \t]*board[ \t]*\][ \t]*(#.*)?(\r?\n|$)`)
