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

// LocalWrite is what WriteLocalConfig or WriteFilesConfig did. Migrated is
// set when the archive the file named is not the one it names now, so what
// the index state says was indexed into it is no longer true.
type LocalWrite struct {
	Path     string
	Changed  bool
	Migrated bool
}

// WriteLocalConfig points the config file at the stack `hev up` owns on the
// Turbopuffer lane and records the `[local]` block. It is a file write and not
// an env export because launchd reads the file selected by HEV_CONFIG and
// inherits no shell exports.
//
// apiKey is the Turbopuffer key. The local gateway takes it as its inbound
// bearer and uses it upstream, so it is also the `[layer]` api_key, and the
// file is written mode 0600 for that reason.
//
// Unless explicitMove is true, it refuses a config that names a remote Layer:
// quietly repointing a working hosted setup is the one mistake here that costs
// a user something, and `hev init` remains the way to choose a hosted Layer.
// Every key it does not own is carried over. Changed is false when the file
// already says all of this, which is what makes a second `hev up` a no-op.
func WriteLocalConfig(local LocalConfig, store, apiKey string, explicitMove ...bool) (LocalWrite, error) {
	path, doc, err := readLocalConfig(explicitMove...)
	if err != nil {
		return LocalWrite{Path: path}, err
	}
	before := snapshot(doc)

	layerTable := table(doc, "layer")
	ns, _ := layerTable["namespace"].(string)
	was, _ := layerTable["store"].(string)
	namespace := ns
	if namespace == "" {
		namespace = "hev-traces"
	}
	oldEndpoint, _ := layerTable["endpoint"].(string)
	migrated := (oldEndpoint != "" && !isLoopback(oldEndpoint)) || (was != "" && was != store)
	layerTable["endpoint"] = fmt.Sprintf("http://127.0.0.1:%d", local.Port)
	layerTable["api_key"] = apiKey
	layerTable["store"] = store
	layerTable["namespace"] = namespace
	localTable := table(doc, "local")
	localTable["image"] = local.Image
	delete(localTable, "embed_image")
	localTable["port"] = int64(local.Port)
	localTable["project"] = local.Project
	localTable["kit_image"] = local.KitImage
	localTable["serve_port"] = int64(local.ServePort)
	if capture := table(doc, "capture"); capture["scan_interval"] == nil {
		capture["scan_interval"] = "5m"
	}
	return writeLocal(path, before, doc, migrated)
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
	retired := strings.EqualFold(store, RetiredStore) || strings.TrimSpace(key) == layerenv.PlaceholderKey || (endpoint != "" && isLoopback(endpoint))
	if store != "" {
		delete(layerTable, "store")
	}
	if strings.TrimSpace(key) == layerenv.PlaceholderKey {
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

// LocalStore is the store a config `hev up` wrote archives to, and "" for
// any other config: `up` keeps a machine on the lane it chose, and moves it
// only when told to.
func LocalStore() string {
	_, doc, err := readLocalConfig()
	if err != nil {
		return ""
	}
	if _, managed := doc["local"]; !managed {
		return ""
	}
	layerTable, _ := doc["layer"].(map[string]any)
	store, _ := layerTable["store"].(string)
	return store
}

// LocalAPIKey is the Turbopuffer key a config `hev up` wrote already holds,
// so that a second `up` needs nothing exported. It is "" for any other config,
// and for the placeholder an older local config holds.
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
	if key == layerenv.PlaceholderKey {
		return ""
	}
	return key
}

// CheckLocalConfig is the refusal on its own, reading and changing nothing
// else. `hev up` runs it before it starts anything: a hosted host is turned
// away before an image is pulled, unless an explicit store move was requested.
func CheckLocalConfig(explicitMove ...bool) error {
	_, _, err := readLocalConfig(explicitMove...)
	return err
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
