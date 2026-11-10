package storagemode

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hev/kit/internal/layerenv"
)

const boardKey = "FACTORY_BOARD_LAYER_API_KEY"

// isolate clears every key source and points the config at an empty dir.
func isolate(t *testing.T) string {
	t.Helper()
	for _, name := range append(layerenv.Names(), boardKey) {
		t.Setenv(name, "")
		os.Unsetenv(name)
	}
	config := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("HEV_CONFIG", config)
	return config
}

func TestNoKeyIsFilesystemNotAnError(t *testing.T) {
	isolate(t)
	r, err := Resolve(Options{EnvKeys: []string{boardKey}})
	if err != nil || r.Mode != Filesystem || r.Key != "" || r.Source != "" {
		t.Fatalf("got %+v, %v", r, err)
	}
}

func TestEmptyConfigKeyIsFilesystem(t *testing.T) {
	config := isolate(t)
	os.WriteFile(config, []byte("[layer]\nendpoint = \"\"\napi_key = \"\"\n"), 0o600)
	if r, err := Resolve(Options{ConfigKey: true}); err != nil || r.Mode != Filesystem {
		t.Fatalf("got %+v, %v", r, err)
	}
}

func TestConfigKeyCountsOnlyWhenAsked(t *testing.T) {
	config := isolate(t)
	os.WriteFile(config, []byte("[layer]\napi_key = \"config-secret\"\n"), 0o600)
	if r, err := Resolve(Options{}); err != nil || r.Mode != Filesystem {
		t.Fatalf("without ConfigKey: %+v, %v", r.Mode, err)
	}
	if r, err := Resolve(Options{ConfigKey: true}); err != nil || r.Mode != Layer || r.Key != "config-secret" {
		t.Fatalf("with ConfigKey: %+v, %v", r.Mode, err)
	}
	if r, err := Resolve(Options{ConfigKey: true, ConfigPath: filepath.Join(t.TempDir(), "absent.toml")}); err != nil || r.Mode != Filesystem {
		t.Fatalf("absent config: %+v, %v", r.Mode, err)
	}
}

func TestKeySourcesSelectLayer(t *testing.T) {
	for _, c := range []struct {
		name, env, value, config, source string
	}{
		{"board key", boardKey, "board-secret", "", boardKey},
		{"kit key", "HEVLAYER_API_KEY", "kit-secret", "", "HEVLAYER_API_KEY"},
		{"legacy alias", "LAYER_API_KEY", "alias-secret", "", "HEVLAYER_API_KEY"},
		{"config key", "", "", "[layer]\napi_key = \"config-secret\"\n", "config [layer].api_key"},
	} {
		t.Run(c.name, func(t *testing.T) {
			config := isolate(t)
			if c.env != "" {
				t.Setenv(c.env, c.value)
			}
			want := c.value
			if c.config != "" {
				os.WriteFile(config, []byte(c.config), 0o600)
				want = "config-secret"
			}
			r, err := Resolve(Options{EnvKeys: []string{boardKey}, ConfigKey: true})
			if err != nil || r.Mode != Layer || r.Key != want || r.Source != c.source {
				t.Fatalf("got mode=%s source=%s keyset=%v err=%v", r.Mode, r.Source, r.Key != "", err)
			}
			p, err := Resolve(Options{EnvKeys: []string{boardKey}, ConfigKey: true, Peek: true})
			if err != nil || p.Mode != Layer || p.Key != "" {
				t.Fatalf("peek: mode=%s keyset=%v err=%v", p.Mode, p.Key != "", err)
			}
		})
	}
}

func TestCallerKeyWinsOverKitKey(t *testing.T) {
	isolate(t)
	t.Setenv("HEVLAYER_API_KEY", "kit-secret")
	t.Setenv(boardKey, "board-secret")
	if r, _ := Resolve(Options{EnvKeys: []string{boardKey}}); r.Key != "board-secret" {
		t.Fatal("the caller's own key did not win")
	}
	if r, _ := Resolve(Options{}); r.Key != "kit-secret" {
		t.Fatal("a caller without its own variable must not read another's")
	}
}

func TestUnreadableKeyIsAnErrorNotFiles(t *testing.T) {
	t.Run("op reference without op", func(t *testing.T) {
		isolate(t)
		t.Setenv("PATH", t.TempDir())
		t.Setenv("HEVLAYER_API_KEY_OP_REF", "op://vault/item/field")
		r, err := Resolve(Options{})
		if err == nil || r.Mode == Filesystem {
			t.Fatalf("an unreadable reference fell back to %q", r.Mode)
		}
		if p, err := Resolve(Options{Peek: true}); err != nil || p.Mode != Layer {
			t.Fatalf("peek must report Layer without reading: %+v %v", p, err)
		}
	})
	t.Run("op reference that fails", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("shell stub")
		}
		isolate(t)
		bin := t.TempDir()
		os.WriteFile(filepath.Join(bin, "op"), []byte("#!/bin/sh\necho denied >&2\nexit 1\n"), 0o755)
		t.Setenv("PATH", bin)
		t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "x")
		t.Setenv("HEVLAYER_API_KEY_OP_REF", "op://vault/item/field")
		if r, err := Resolve(Options{}); err == nil || !strings.Contains(err.Error(), "could not be read") {
			t.Fatalf("got %+v, %v", r, err)
		}
	})
	t.Run("blank caller key", func(t *testing.T) {
		isolate(t)
		t.Setenv(boardKey, "   ")
		if _, err := Resolve(Options{EnvKeys: []string{boardKey}}); err == nil {
			t.Fatal("a blank key fell back to files")
		}
	})
	t.Run("unparseable config", func(t *testing.T) {
		config := isolate(t)
		os.WriteFile(config, []byte("[layer\napi_key = "), 0o600)
		if _, err := Resolve(Options{ConfigKey: true}); err == nil {
			t.Fatal("an unreadable config fell back to files")
		}
	})
}

func TestErrorsNeverCarryTheKey(t *testing.T) {
	isolate(t)
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HEVLAYER_API_KEY_OP_REF", "op://vault/item/field")
	_, err := Resolve(Options{})
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("%v", err)
	}
}

// The placeholder the retired local Docker lane wrote is not a key,
// whether it sits in the config file or the environment.
func TestPlaceholderLocalIsNoKey(t *testing.T) {
	config := isolate(t)
	os.WriteFile(config, []byte("[layer]\napi_key = \"local\"\n"), 0o600)
	if r, err := Resolve(Options{ConfigKey: true}); err != nil || r.Mode != Filesystem {
		t.Fatalf("config placeholder: %+v, %v", r.Mode, err)
	}
	t.Setenv("HEVLAYER_API_KEY", "local")
	if r, err := Resolve(Options{ConfigKey: true}); err != nil || r.Mode != Filesystem {
		t.Fatalf("env placeholder: %+v, %v", r.Mode, err)
	}
	t.Setenv(boardKey, "local")
	if r, err := Resolve(Options{EnvKeys: []string{boardKey}}); err != nil || r.Mode != Filesystem {
		t.Fatalf("caller placeholder: %+v, %v", r.Mode, err)
	}
}
