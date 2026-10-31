package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigAndLegacyOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(path, []byte("[board]\nurl='https://board.example'\naddr='localhost:0'\nnamespace='board-test'\nidentity='agent-test'\nlayer_endpoint='https://layer.example'\nmodel='test-model'\n"), 0600)
	t.Setenv("HEV_CONFIG", path)
	for _, name := range []string{"FACTORY_BOARD_URL", "FACTORY_BOARD_ADDR", "FACTORY_BOARD_NAMESPACE", "FACTORY_BOARD_AS", "FACTORY_BOARD_LAYER_ENDPOINT", "FACTORY_BOARD_LAYER_API_KEY", "LAYER_EMBED_MODEL"} {
		t.Setenv(name, "")
	}
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	if Current.URL != "https://board.example" || Current.Identity != "agent-test" || Current.Namespace != "board-test" {
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

func TestDeploymentFieldsAndRoles(t *testing.T) {
	old := Current
	t.Cleanup(func() { Current = old })
	path := filepath.Join(t.TempDir(), "config.toml")
	data := `[board]
canvas_dir = "/example/canvas"
public_url = "https://board.example"
cors = "https://reader.example"
headless = true
trusted_proxy = true
[[board.roles]]
pattern = "^worker-(.+)$"
role = "worker"
instance = "$1"
`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEV_CONFIG", path)
	for _, k := range []string{"FACTORY_CANVAS_DIR", "FACTORY_BOARD_PUBLIC_URL", "FACTORY_BOARD_CORS"} {
		t.Setenv(k, "")
	}
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	if Current.CanvasDir != "/example/canvas" || Current.PublicURL != "https://board.example" || Current.CORS != "https://reader.example" || !Current.Headless || !Current.TrustedProxy || len(Current.Roles) != 1 {
		t.Fatalf("%+v", Current)
	}
	t.Setenv("FACTORY_CANVAS_DIR", "/override/canvas")
	t.Setenv("FACTORY_BOARD_PUBLIC_URL", "https://override.example")
	t.Setenv("FACTORY_BOARD_CORS", "https://override.example")
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	if Current.CanvasDir != "/override/canvas" || Current.PublicURL != "https://override.example" || Current.CORS != "https://override.example" {
		t.Fatalf("%+v", Current)
	}
	if err := os.WriteFile(path, []byte("[[board.roles]]\npattern='['\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Load(); err == nil {
		t.Fatal("invalid role pattern accepted")
	}
}
