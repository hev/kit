// Package storagemode decides where kit keeps its data. The rule is the same
// for every piece of kit: no Layer API key means files on the host kit runs
// on; a Layer API key means Layer.
//
// A missing key is not an error. A key that is configured but cannot be read
// (a 1Password reference that fails, a blank value, an unreadable config) is
// an error: an operator who asked for Layer never silently gets files.
package storagemode

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/layerenv"
)

// Mode is where a piece of kit stores its data.
type Mode string

const (
	Filesystem Mode = "filesystem"
	Layer      Mode = "layer"
)

// Resolution is the answer. Key is only set in Layer mode and must never be
// printed or logged; Source names where the key came from, never its value.
type Resolution struct {
	Mode   Mode
	Key    string
	Source string
}

// Options lets a caller add its own key variables in front of kit's.
type Options struct {
	// EnvKeys are checked first, in order, before kit's own lookup
	// (for example the board's FACTORY_BOARD_LAYER_API_KEY).
	EnvKeys []string
	// ConfigKey also counts [layer].api_key in kit's config file, as the
	// trace daemon does. The board does not: its documented contract is that
	// no key is read from the TOML file.
	ConfigKey bool
	// ConfigPath is the kit config to read [layer].api_key from;
	// "" means daemon.DefaultConfigPath().
	ConfigPath string
	// Peek reports the mode without reading a secret: a configured 1Password
	// reference is reported as Layer without running op. Key stays empty.
	Peek bool
}

// Resolve answers "layer or filesystem" from the key lookup kit already uses:
// the caller's own variables, then HEVLAYER_API_KEY (and its aliases), then
// HEVLAYER_API_KEY_OP_REF through daemon.LayerKey, then (with ConfigKey)
// [layer].api_key in kit's config file.
func Resolve(o Options) (Resolution, error) {
	for _, name := range o.EnvKeys {
		if value, set := os.LookupEnv(name); set && value != "" {
			if strings.TrimSpace(value) == "" {
				return Resolution{}, fmt.Errorf("%s is set but blank; unset it to store data in files", name)
			}
			return layer(o, strings.TrimSpace(value), name), nil
		}
	}
	if layerenv.Get("HEVLAYER_API_KEY") != "" || layerenv.Get("HEVLAYER_API_KEY_OP_REF") != "" {
		source := "HEVLAYER_API_KEY"
		if layerenv.Get("HEVLAYER_API_KEY") == "" {
			source = "HEVLAYER_API_KEY_OP_REF"
			if o.Peek {
				return Resolution{Mode: Layer, Source: source}, nil
			}
		}
		key, err := daemon.LayerKey()
		if err != nil {
			return Resolution{}, fmt.Errorf("a Layer API key is configured but could not be read: %w", err)
		}
		if strings.TrimSpace(key) == "" {
			return Resolution{}, fmt.Errorf("%s is set but blank; unset it to store data in files", source)
		}
		return layer(o, strings.TrimSpace(key), source), nil
	}
	if !o.ConfigKey {
		return Resolution{Mode: Filesystem}, nil
	}
	path := o.ConfigPath
	if path == "" {
		path = daemon.DefaultConfigPath()
	}
	var file struct {
		Layer struct {
			APIKey *string `toml:"api_key"`
		} `toml:"layer"`
	}
	if _, err := toml.DecodeFile(path, &file); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Resolution{Mode: Filesystem}, nil
		}
		return Resolution{}, fmt.Errorf("read %s to find the Layer API key: %w", path, err)
	}
	if file.Layer.APIKey != nil && strings.TrimSpace(*file.Layer.APIKey) != "" {
		return layer(o, strings.TrimSpace(*file.Layer.APIKey), "config [layer].api_key"), nil
	}
	return Resolution{Mode: Filesystem}, nil
}

func layer(o Options, key, source string) Resolution {
	if o.Peek {
		key = ""
	}
	return Resolution{Mode: Layer, Key: key, Source: source}
}
