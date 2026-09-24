package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

// This test intentionally uses only the pre-existing interactive entry point,
// so it can run unchanged on the regression baseline.
func TestInteractiveInitPreservesConfigBytes(t *testing.T) {
	before, err := os.ReadFile("testdata/init-preserve.toml")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEV_CONFIG", path)
	var out bytes.Buffer
	if err := runInteractiveInit(strings.NewReader("y\nhttps://hosted.test\nhosted-fixture-key\nhosted-traces\n"), &out); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.NewReplacer("'http://localhost:8080'", `"https://hosted.test"`, `"local-fixture-key"`, `"hosted-fixture-key"`, `"""local-traces"""`, `"hosted-traces"`).Replace(string(before))
	if string(after) != want {
		t.Fatalf("config bytes changed outside prompted values\nwant:\n%s\ngot:\n%s", want, after)
	}
	for _, phrase := range []string{"Warning:", "[local]", "preserves local and archive settings", "does not stop the local stack", "layer.store", "HEV_CONFIG"} {
		if !strings.Contains(out.String(), phrase) {
			t.Fatalf("missing local warning detail %q: %s", phrase, &out)
		}
	}
	for _, secret := range []string{"local-fixture-key", "hosted-fixture-key", "fixture-secret"} {
		if strings.Contains(out.String(), secret) {
			t.Fatal("output exposed a fixture secret")
		}
	}
}

func TestInteractiveInitSyntaxAndMissingFields(t *testing.T) {
	cases := []struct{ name, before, want string }{
		{"quoted dotted", "'layer'.\"end\\u0070oint\" = 'old' # keep\nlayer.'api_key' = 'key'\nlayer.namespace = 'ns'\n", "'layer'.\"end\\u0070oint\" = \"new\" # keep\nlayer.'api_key' = \"key2\"\nlayer.namespace = \"ns2\"\n"},
		{"quoted header CRLF", "[ 'layer' ] # header\r\nendpoint = 'old'\r\n", "[ 'layer' ] # header\r\napi_key = \"key2\"\r\nnamespace = \"ns2\"\r\nendpoint = \"new\"\r\n"},
		{"inline", "layer = { endpoint = 'old', future = { x = 1 } } # keep\n", "layer = {api_key = \"key2\", namespace = \"ns2\",  endpoint = \"new\", future = { x = 1 } } # keep\n"},
		{"empty inline", "layer = {}", "layer = {endpoint = \"new\", api_key = \"key2\", namespace = \"ns2\"}"},
		{"empty header", "[layer] # no newline", "[layer] # no newline\nendpoint = \"new\"\napi_key = \"key2\"\nnamespace = \"ns2\"\n"},
		{"missing table", "# keep\n[capture]\nscan_interval = ''\n", "layer.endpoint = \"new\"\nlayer.api_key = \"key2\"\nlayer.namespace = \"ns2\"\n# keep\n[capture]\nscan_interval = ''\n"},
		{"implicit table", "[layer.future]\nx = 1\n", "layer.endpoint = \"new\"\nlayer.api_key = \"key2\"\nlayer.namespace = \"ns2\"\n[layer.future]\nx = 1\n"},
		{"dotted missing", "layer.endpoint = 'old'\n[other]\nx = 1", "layer.api_key = \"key2\"\nlayer.namespace = \"ns2\"\nlayer.endpoint = \"new\"\n[other]\nx = 1"},
		{"multiline value", "[layer]\nendpoint = \"\"\"\nold\\\n  endpoint\"\"\" # keep\napi_key = '''key'''\nnamespace = 'ns'", "[layer]\nendpoint = \"new\" # keep\napi_key = \"key2\"\nnamespace = \"ns2\""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(path, []byte(tc.before), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Setenv("HEV_CONFIG", path)
			if err := runInteractiveInit(strings.NewReader("y\nnew\nkey2\nns2\n"), &bytes.Buffer{}); err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(raw) != tc.want {
				t.Fatalf("want %q, got %q", tc.want, raw)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0o600 {
				t.Fatalf("mode = %o", info.Mode().Perm())
			}
		})
	}
}

func TestInteractiveInitLeavesInvalidConfigUntouched(t *testing.T) {
	// TempDir derives its path from the test name, and init prints that path.
	// Keep fixture contents out of names so leak checks test config disclosure.
	for _, tc := range []struct{ name, input string }{
		{"malformed header", "[layer\napi_key = 'fixture-secret'"},
		{"duplicate key", "[layer]\nendpoint='a'\nendpoint='b'"},
		{"scalar layer", "layer = 'wrong type'"},
		{"numeric endpoint", "[layer]\nendpoint = 123"},
		{"array layer", "[[layer]]\nendpoint = 'wrong table'"},
		{"table api key", "[layer]\napi_key = { nested = 'fixture-secret' }"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := tc.input
			path := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("HEV_CONFIG", path)
			var out bytes.Buffer
			err := runInteractiveInit(strings.NewReader("y\nnew\nkey\nns\n"), &out)
			if err == nil {
				t.Fatal("expected invalid config error")
			}
			if strings.Contains(err.Error()+out.String(), "fixture-secret") {
				t.Fatal("error leaked secret")
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(raw) != input {
				t.Fatal("invalid config was rewritten")
			}
		})
	}
}

func TestInteractiveInitDeclineAndKeepDefaults(t *testing.T) {
	before := "[layer]\nendpoint = 'old'\napi_key = '''fixture-key'''\nnamespace = 'ns'\n[local]\n"
	for _, answer := range []string{"n\n", "y\n\n\n\n"} {
		t.Run(answer, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(path, []byte(before), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("HEV_CONFIG", path)
			var out bytes.Buffer
			if err := runInteractiveInit(strings.NewReader(answer), &out); err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(raw) != before {
				t.Fatal("unchanged values were reformatted")
			}
			if strings.Contains(out.String(), "fixture-key") {
				t.Fatal("prompt leaked key")
			}
		})
	}
}

func TestInteractiveInitEscapesInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("HEV_CONFIG", path)
	key := "quotes\"backslash\\tab\tcontrol\x01☃"
	if err := runInteractiveInit(strings.NewReader("new\n"+key+"\nns\n"), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	// Exercise the existing-file editor with a different escaped value too.
	key += "updated"
	if err := runInteractiveInit(strings.NewReader("y\nnew\n"+key+"\nns\n"), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var cfg initFileConfig
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Layer.APIKey != key || cfg.Capture.ScanInterval != "5m" {
		t.Fatal("new config did not round trip")
	}
}

func TestInitWriteFailurePreservesDestination(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(path, "keep")
	if err := os.WriteFile(sentinel, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := initFileConfig{Layer: initLayerConfig{Endpoint: "new", APIKey: "key", Namespace: "ns"}}
	if err := writeInitConfig(path, cfg); err == nil {
		t.Fatal("expected rename failure")
	}
	raw, err := os.ReadFile(sentinel)
	if err != nil || string(raw) != "original" {
		t.Fatalf("destination changed: %s, %v", raw, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatal("temporary file leaked")
	}
	if err := writeInitConfig(filepath.Join(sentinel, "config.toml"), cfg); err == nil {
		t.Fatal("expected create failure")
	}
}

func TestInteractiveInitRefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	target, path := filepath.Join(dir, "target.toml"), filepath.Join(dir, "config.toml")
	before := "[layer]\napi_key = 'fixture-key'\n"
	if err := os.WriteFile(target, []byte(before), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEV_CONFIG", path)
	if err := runInteractiveInit(strings.NewReader("y\nnew\nkey\nns\n"), &bytes.Buffer{}); err == nil {
		t.Fatal("expected symlink refusal")
	}
	raw, err := os.ReadFile(target)
	if err != nil || string(raw) != before {
		t.Fatal("symlink target changed")
	}
	if _, err := os.Readlink(path); err != nil {
		t.Fatal("symlink replaced")
	}
}
