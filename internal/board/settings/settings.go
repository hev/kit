package settings

import (
	"os"

	"github.com/BurntSushi/toml"
	"github.com/hev/kit/internal/daemon"
)

type Config struct {
	URL       string `toml:"url"`
	Addr      string `toml:"addr"`
	Namespace string `toml:"namespace"`
	Identity  string `toml:"identity"`
	Endpoint  string `toml:"layer_endpoint"`
	APIKey    string `toml:"layer_api_key"`
	Model     string `toml:"model"`
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
		"FACTORY_BOARD_NAMESPACE": &Current.Namespace, "FACTORY_BOARD_AS": &Current.Identity,
		"FACTORY_BOARD_LAYER_ENDPOINT": &Current.Endpoint, "FACTORY_BOARD_LAYER_API_KEY": &Current.APIKey,
		"LAYER_EMBED_MODEL": &Current.Model,
	} {
		if value := os.Getenv(name); value != "" {
			*target = value
		}
	}
	return nil
}
