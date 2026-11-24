package daemon

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/hev/kit/internal/filearchive"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/layerenv"
	"github.com/hev/kit/internal/platformops"
	"github.com/hev/kit/internal/storagemode"
)

func usableLegacyKey(value string) bool {
	key := strings.TrimSpace(value)
	return key != "" && key != layerenv.PlaceholderKey
}

// primaryCredential acquires the primary archive identity before the daemon
// starts its secrets sidecar. References belong to clients and scoped consumers.
func primaryCredential(cfg *Config) (string, string, error) {
	key := strings.TrimSpace(cfg.LayerAPIKey)
	source := "config [layer].api_key"
	if cfg.bootstrapFile != nil {
		if strings.TrimSpace(*cfg.bootstrapFile) == "" {
			return "", "", errors.New("hevd: [layer].api_key_file is empty")
		}
		path := expandPath(*cfg.bootstrapFile)
		if !filepath.IsAbs(path) {
			path = filepath.Join(filepath.Dir(cfg.ConfigPath), path)
		}
		raw, err := platformops.ReadPrivateFile(path, 4096)
		if err != nil {
			return "", "", errors.New("hevd: [layer].api_key_file is unreadable or unprotected")
		}
		key, source = strings.TrimSpace(string(raw)), "config [layer].api_key_file"
	} else if env := layerenv.Get("HEVLAYER_API_KEY"); usableLegacyKey(env) {
		source = "HEVLAYER_API_KEY"
	} else if !usableLegacyKey(key) {
		if _, ref := layerenv.KeyRef(); ref != "" {
			return "", "", errors.New("hevd: migrate the primary Layer key reference to [layer].api_key_file")
		}
		return "", "", nil
	}
	// Accept existing opaque Layer keys while refusing references, placeholders
	// and bytes that cannot be carried in an Authorization header.
	if !usableLegacyKey(key) || len(key) > 4096 || strings.Contains(key, "://") ||
		strings.IndexFunc(key, func(r rune) bool { return r < 33 || r > 126 }) >= 0 {
		return "", "", errors.New("hevd: invalid primary Layer credential")
	}
	return key, source, nil
}

func openPrimary(cfg *Config, logger *slog.Logger) (Target, error) {
	key, source, err := primaryCredential(cfg)
	if err != nil {
		return Target{}, err
	}
	res := storagemode.Resolution{Mode: storagemode.Filesystem}
	conf := *cfg
	if key == "" {
		reasons, err := LayerIntent(cfg)
		if err != nil || len(reasons) > 0 {
			return Target{}, errors.New("hevd: primary Layer credential unavailable")
		}
	} else {
		res.Mode, res.Source = storagemode.Layer, source
		if conf.LayerEndpoint == "" {
			conf.LayerEndpoint = layer.DefaultEndpoint
		}
		endpoint, err := url.Parse(conf.LayerEndpoint)
		if err != nil || endpoint.Hostname() == "" || endpoint.User != nil ||
			(endpoint.Scheme != "https" && !(endpoint.Scheme == "http" && storagemode.Loopback(conf.LayerEndpoint))) {
			return Target{}, errors.New("hevd: invalid primary Layer endpoint")
		}
		conf.LayerAPIKey = key
		if err := validateLayerTarget(&conf); err != nil {
			return Target{}, err
		}
	}
	target, err := openResolvedTarget(&conf, "", filearchive.Open, res, key)
	if err != nil {
		return Target{}, errors.New("hevd: could not open primary archive")
	}
	if target.Retired {
		logger.Warn("local Docker lane retired", "message", RetiredMessage(cfg))
	}
	if target.Layer != nil {
		target.Layer.HTTP.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
	return target, nil
}
