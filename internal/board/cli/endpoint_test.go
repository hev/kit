package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hev/kit/internal/board/settings"
)

func TestBoardEndpointUsesInstalledPrivateConfiguration(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FACTORY_BOARD_URL", "")
	config := filepath.Join(home, ".local/state/factory-feedback/config.json")
	if err := os.MkdirAll(filepath.Dir(config), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte(`{"board_url":"https://board.example:7700/"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := settings.Load(); err != nil {
		t.Fatal(err)
	}
	if got := baseURL(); got != "https://board.example:7700" {
		t.Fatal(got)
	}
	t.Setenv("FACTORY_BOARD_URL", "http://explicit.example:9000/")
	if err := settings.Load(); err != nil {
		t.Fatal(err)
	}
	if got := baseURL(); got != "http://explicit.example:9000" {
		t.Fatal(got)
	}
	t.Setenv("FACTORY_BOARD_URL", "")
	for _, data := range []string{`broken`, `{}`, `{"board_url":"file:///tmp/board"}`, `{"board_url":"https://secret@example.com"}`} {
		if err := os.WriteFile(config, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if err := settings.Load(); err != nil {
			t.Fatal(err)
		}
		if got := baseURL(); got != "http://127.0.0.1:7700" {
			t.Fatal(got)
		}
	}
}
