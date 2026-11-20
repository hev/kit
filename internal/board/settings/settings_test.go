package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestConfigAndLegacyOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(path, []byte("[board]\nurl='https://board.example'\naddr='localhost:0'\nnamespace='board-test'\nidentity='agent-test'\nlayer_endpoint='https://layer.example'\nmodel='test-model'\n"), 0600)
	t.Setenv("HEV_CONFIG", path)
	for _, name := range []string{"FACTORY_BOARD_URL", "FACTORY_BOARD_ADDR", "FACTORY_BOARD_NAMESPACE", "FACTORY_BOARD_AS", "FACTORY_BOARD_LAYER_ENDPOINT", "FACTORY_BOARD_LAYER_API_KEY", "HEVLAYER_EMBED_MODEL"} {
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
read_only = true
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
	if Current.CanvasDir != "/example/canvas" || Current.PublicURL != "https://board.example" || Current.CORS != "https://reader.example" || !Current.Headless || !Current.TrustedProxy || !Current.ReadOnly || len(Current.Roles) != 1 {
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

func TestURLSetOnlyWhenSomethingNamesAServer(t *testing.T) {
	old := Current
	t.Cleanup(func() { Current = old })
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("FACTORY_BOARD_URL", "")
	t.Setenv("HEV_CONFIG", filepath.Join(home, "config.toml"))
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	if Current.URLSet || Current.URL != DefaultURL {
		t.Fatalf("nothing configured: %+v", Current)
	}
	t.Setenv("FACTORY_BOARD_URL", "https://board.example")
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	if !Current.URLSet {
		t.Fatalf("FACTORY_BOARD_URL: %+v", Current)
	}
}

func TestEnsureFilesDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	want := filepath.Join(home, ".hev", "board")
	read := func(path string) (string, Config) {
		t.Helper()
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var f struct {
			Board Config `toml:"board"`
		}
		if _, err := toml.Decode(string(b), &f); err != nil {
			t.Fatalf("%v:\n%s", err, b)
		}
		return string(b), f.Board
	}
	for name, c := range map[string]struct {
		before  string
		changed bool
		keep    string
	}{
		"no file":             {"", true, ""},
		"other tables":        {"[layer]\nnamespace = \"x\"", true, "[layer]\nnamespace = \"x\"\n"},
		"board without dir":   {"[board]\nurl = \"https://b.example\"\n[layer]\nx = 1\n", true, "url = \"https://b.example\"\n[layer]\nx = 1\n"},
		"board header at EOF": {"# kit\n[board]", true, "# kit\n[board]\n"},
		"roles only":          {"[[board.roles]]\npattern = \"^w\"\nrole = \"worker\"\n", true, "[[board.roles]]\npattern = \"^w\"\nrole = \"worker\"\n"},
		"dir already set":     {"[board]\ndir = \"/srv/board\"\n", false, "dir = \"/srv/board\"\n"},
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "kit", "config.toml")
			if c.before != "" {
				os.MkdirAll(filepath.Dir(path), 0o700)
				os.WriteFile(path, []byte(c.before), 0o600)
			}
			changed, err := EnsureFilesDefault(path)
			if err != nil || changed != c.changed {
				t.Fatalf("changed=%v err=%v", changed, err)
			}
			text, b := read(path)
			if c.changed && b.Dir != want {
				t.Fatalf("dir %q, want %q:\n%s", b.Dir, want, text)
			}
			if !strings.Contains(text, c.keep) {
				t.Fatalf("lost %q:\n%s", c.keep, text)
			}
			if again, err := EnsureFilesDefault(path); err != nil || again {
				t.Fatalf("second call changed=%v err=%v", again, err)
			}
		})
	}
	bad := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(bad, []byte("[board\n"), 0o600)
	if _, err := EnsureFilesDefault(bad); err == nil {
		t.Fatal("a config that does not parse was rewritten")
	}
}
