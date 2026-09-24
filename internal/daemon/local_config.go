package daemon

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"

	"github.com/BurntSushi/toml"
	"github.com/hev/kit/internal/layer"
)

// LocalWrite is what WriteLocalConfig did. Migrated is set when the file it
// replaced pointed at the lexical Postgres store the local stack used to run:
// that archive is not the one the config now names, so what the index state
// says was indexed into it is no longer true.
type LocalWrite struct {
	Path     string
	Changed  bool
	Migrated bool
}

// WriteLocalConfig points the config file at the stack `hev up` owns and
// records the `[local]` block. It is a file write and not an env export because
// launchd reads the file selected by HEV_CONFIG and inherits no shell exports.
//
// apiKey is the Turbopuffer key. The local gateway takes it as its inbound
// bearer and uses it upstream, so it is also the `[layer]` api_key; the file is
// written mode 0600 for that reason.
//
// It refuses a config that already names a Layer anywhere but this machine:
// quietly repointing a working hosted setup is the one mistake here that costs
// a user something, and `hev init` remains the way to choose a hosted Layer.
// Every key it does not own is carried over. Changed is false when the file
// already says all of this, which is what makes a second `hev up` a no-op.
func WriteLocalConfig(local LocalConfig, apiKey string) (LocalWrite, error) {
	path, doc, err := readLocalConfig()
	if err != nil {
		return LocalWrite{Path: path}, err
	}
	before := map[string]any{}
	if raw, err := tomlBytes(doc); err == nil {
		_, _ = toml.Decode(string(raw), &before)
	}

	layerTable := table(doc, "layer")
	migrated := layerTable["store"] == layer.StorePgvector
	ns, _ := layerTable["namespace"].(string)
	store, _ := layerTable["store"].(string)
	layerTable["endpoint"] = fmt.Sprintf("http://127.0.0.1:%d", local.Port)
	layerTable["api_key"] = apiKey
	layerTable["store"] = layer.StoreTurbopuffer
	layerTable["namespace"] = LocalNamespace(ns, store)
	localTable := table(doc, "local")
	localTable["image"] = local.Image
	localTable["port"] = int64(local.Port)
	localTable["project"] = local.Project
	localTable["kit_image"] = local.KitImage
	localTable["serve_port"] = int64(local.ServePort)
	if capture := table(doc, "capture"); capture["scan_interval"] == nil {
		capture["scan_interval"] = "5m"
	}

	if reflect.DeepEqual(before, doc) {
		return LocalWrite{Path: path}, nil
	}
	raw, err := tomlBytes(doc)
	if err != nil {
		return LocalWrite{Path: path}, err
	}
	if err := writeFileAtomic(path, raw, 0o600); err != nil {
		return LocalWrite{Path: path}, err
	}
	return LocalWrite{Path: path, Changed: true, Migrated: migrated}, nil
}

// LocalNamespace is the namespace the local stack archives to, given the one a
// config names and the store it names it on: the default when it names none,
// and when it still carries the Postgres-era local name.
func LocalNamespace(namespace, store string) string {
	if namespace == "" || (store == layer.StorePgvector && namespace == LegacyLocalNamespace) {
		return "hev-traces"
	}
	return namespace
}

// LocalAPIKey is the Turbopuffer key a config `hev up` wrote already holds,
// so that a second `up` needs nothing exported. It is "" for any other config,
// and for the placeholder the Postgres-era local stack wrote.
func LocalAPIKey() string {
	_, doc, err := readLocalConfig()
	if err != nil {
		return ""
	}
	if _, managed := doc["local"]; !managed {
		return ""
	}
	layerTable, _ := doc["layer"].(map[string]any)
	key, _ := layerTable["api_key"].(string)
	if key == "local" {
		return ""
	}
	return key
}

// CheckLocalConfig is the refusal on its own, reading and changing nothing
// else. `hev up` runs it before it starts anything: a hosted host is turned
// away before an image is pulled, not after the containers are up.
func CheckLocalConfig() error {
	_, _, err := readLocalConfig()
	return err
}

func readLocalConfig() (path string, doc map[string]any, err error) {
	path = DefaultConfigPath()
	doc = map[string]any{}
	if _, err := os.Stat(path); err == nil {
		if _, err := toml.DecodeFile(path, &doc); err != nil {
			return path, nil, fmt.Errorf("read config %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return path, nil, err
	}
	if layerTable, ok := doc["layer"].(map[string]any); ok {
		if endpoint, _ := layerTable["endpoint"].(string); endpoint != "" && !isLoopback(endpoint) {
			return path, nil, fmt.Errorf("%s already points at %s; `hev up` will not repoint a hosted config — move it aside, or set HEV_CONFIG to a new file, and run `hev up` again", path, endpoint)
		}
	}
	return path, doc, nil
}

// writeFileAtomic replaces path by rename from a temp file beside it, so a
// crash mid-write cannot leave the daemon a truncated config.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.toml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func table(doc map[string]any, key string) map[string]any {
	t, ok := doc[key].(map[string]any)
	if !ok {
		t = map[string]any{}
		doc[key] = t
	}
	return t
}

func tomlBytes(doc map[string]any) ([]byte, error) {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(doc); err != nil {
		return nil, fmt.Errorf("encode config: %w", err)
	}
	return buf.Bytes(), nil
}

func isLoopback(endpoint string) bool {
	u, err := url.Parse(endpoint)
	if err != nil {
		return false
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}
