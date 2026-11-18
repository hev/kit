package daemon

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/hev/kit/internal/layerenv"
	"github.com/hev/kit/internal/platformops"
)

// LocalWrite is what WriteFilesConfig did. Migrated is
// set when the archive the file named is not the one it names now, so what
// the index state says was indexed into it is no longer true.
type LocalWrite struct {
	Path     string
	Changed  bool
	Migrated bool
}

// WriteFilesConfig is `hev up` with no Layer API key: the config names no key
// and no store, so hevd keeps traces in the file archive. It drops what the
// retired local Docker lane wrote into [layer] (its store, its placeholder
// key and its loopback endpoint) and the sidecar image from [local], keeps
// every other key, and records the [local] block so `hev down` knows the
// jobs are its own. It never touches Docker.
func WriteFilesConfig(local LocalConfig) (LocalWrite, error) {
	// A remote endpoint with no key is left as it is: it says this machine
	// uses hev layer, and kit refuses files until a key is configured or the
	// operator removes it.
	path, doc, err := readLocalConfig(true)
	if err != nil {
		return LocalWrite{Path: path}, err
	}
	before := snapshot(doc)
	layerTable := table(doc, "layer")
	store, _ := layerTable["store"].(string)
	key, _ := layerTable["api_key"].(string)
	endpoint, _ := layerTable["endpoint"].(string)
	// The removed BYO Turbopuffer lane kept the provider key as [layer]
	// api_key, in front of a loopback gateway: it is not a hev layer key.
	byoTurbopuffer := strings.EqualFold(store, "turbopuffer") && endpoint != "" && isLoopback(endpoint)
	retired := strings.EqualFold(store, RetiredStore) || byoTurbopuffer || strings.TrimSpace(key) == layerenv.PlaceholderKey || (endpoint != "" && isLoopback(endpoint))
	if store != "" {
		delete(layerTable, "store")
	}
	if strings.TrimSpace(key) == layerenv.PlaceholderKey || byoTurbopuffer {
		delete(layerTable, "api_key")
	}
	if endpoint != "" && isLoopback(endpoint) {
		delete(layerTable, "endpoint")
	}
	// The retired lane's namespace moves to [archive]: left under [layer] it
	// would read as a machine set up for hev layer (storagemode.Intent). The
	// default name needs no line at all.
	if ns, _ := layerTable["namespace"].(string); retired && ns != "" {
		delete(layerTable, "namespace")
		if ns != "hev-traces" {
			if archive := table(doc, "archive"); archive["namespace"] == nil {
				archive["namespace"] = ns
			}
		}
	}
	if len(layerTable) == 0 {
		delete(doc, "layer")
	}
	localTable := table(doc, "local")
	delete(localTable, "embed_image")
	if localTable["project"] == nil {
		localTable["project"] = local.Project
	}
	if capture := table(doc, "capture"); capture["scan_interval"] == nil {
		capture["scan_interval"] = "5m"
	}
	return writeLocal(path, before, doc, false)
}

func snapshot(doc map[string]any) map[string]any {
	before := map[string]any{}
	if raw, err := tomlBytes(doc); err == nil {
		_, _ = toml.Decode(string(raw), &before)
	}
	return before
}

func writeLocal(path string, before, doc map[string]any, migrated bool) (LocalWrite, error) {
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

func readLocalConfig(explicitMove ...bool) (path string, doc map[string]any, err error) {
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
		if endpoint, _ := layerTable["endpoint"].(string); endpoint != "" && !isLoopback(endpoint) && !(len(explicitMove) > 0 && explicitMove[0]) {
			return path, nil, fmt.Errorf("%s already points at %s; `hev up` will not repoint a hosted config — move it aside, or set HEV_CONFIG to a new file, and run `hev up` again", path, endpoint)
		}
	}
	return path, doc, nil
}

// writeFileAtomic replaces path by rename from a temp file beside it, so a
// crash mid-write cannot leave the daemon a truncated config.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	if err := platformops.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := platformops.CreateTemp(filepath.Dir(path), ".config-*.toml")
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
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return platformops.Rename(tmp.Name(), path)
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
