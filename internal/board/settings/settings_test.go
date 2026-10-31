package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigAndLegacyOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(path, []byte("[board]\nurl='https://board.example'\naddr='localhost:0'\nnamespace='board-test'\nidentity='agent-test'\nlayer_endpoint='https://layer.example'\nlayer_api_key='test-key'\nmodel='test-model'\n"), 0600)
	t.Setenv("HEV_CONFIG", path)
	for _, name := range []string{"FACTORY_BOARD_URL", "FACTORY_BOARD_ADDR", "FACTORY_BOARD_NAMESPACE", "FACTORY_BOARD_AS", "FACTORY_BOARD_LAYER_ENDPOINT", "FACTORY_BOARD_LAYER_API_KEY", "LAYER_EMBED_MODEL"} {
		t.Setenv(name, "")
	}
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	if Current.URL != "https://board.example" || Current.Identity != "agent-test" || Current.Namespace != "board-test" || Current.APIKey != "test-key" {
		t.Fatalf("%+v", Current)
	}
	t.Setenv("FACTORY_BOARD_URL", "https://override.example")
	t.Setenv("FACTORY_BOARD_NAMESPACE", "override")
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	if Current.URL != "https://override.example" || Current.Namespace != "override" {
		t.Fatalf("%+v", Current)
	}
}
