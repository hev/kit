package daemon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/hev/kit/internal/platformops"
	"github.com/hev/kit/pkg/kitapi"
)

// KitConfig is this machine's sign-in to a hosted kit service. Key is an
// opaque kit key; the service decides what it may reach. Author fields label
// rows captured here; the service, not this machine, is their authority.
type KitConfig struct {
	storedKey  string
	Service    string `toml:"service"`
	API        string `toml:"api,omitempty"`
	Key        string `toml:"key,omitempty" json:"-"`
	KeyID      string `toml:"key_id,omitempty"`
	TeamID     string `toml:"team_id,omitempty"`
	TeamSlug   string `toml:"team_slug,omitempty"`
	AuthorID   string `toml:"author_id,omitempty"`
	AuthorName string `toml:"author_name,omitempty"`
}

// SignedIn reports whether a kit key is configured, as opposed to a [kit]
// table that only names the service to sign in to.
func (k *KitConfig) SignedIn() bool { return k != nil && k.Key != "" }

// KitOutboxDir is where a signed-in machine queues rows for the kit API.
func KitOutboxDir() string {
	if v := os.Getenv("HEV_KIT_OUTBOX"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hev", "kit", "outbox")
}

// KitClient is the kit API client for a signed-in config.
func KitClient(k *KitConfig) *kitapi.Client {
	c := kitapi.New(k.API, k.Key, k.storedKey)
	if k.Service != "" {
		c.BillingURL = strings.TrimRight(k.Service, "/") + "/billing"
	}
	return c
}

// WriteKitConfig records a sign-in and moves the current [layer] target aside
// so ClearKitConfig can restore it. A [kit] table that only named the service
// does not count as a sign-in, so the first real one still saves the target.
// The legacy document argument is ignored: re-read under the transaction lock.
func WriteKitConfig(_ map[string]any, kit KitConfig) error {
	return SaveKitConfig(kit)
}

func writeKitConfig(doc map[string]any, kit KitConfig) error {
	if _, saved := doc["kit_prior"]; !saved {
		if previous, ok := doc["layer"]; ok {
			doc["kit_prior"] = previous
		}
	}
	delete(doc, "layer")
	doc["kit"] = kit
	table(doc, "capture")["redact"] = true
	return writeInviteDocument(doc)
}

// ClearKitConfig removes the sign-in and restores the prior target. The
// service URL is kept so the next `hev login` needs no flag.
// The legacy document argument is ignored: re-read under the transaction lock.
func ClearKitConfig(_ map[string]any) error {
	return updateKitConfig(clearKitConfig)
}

func clearKitConfig(doc map[string]any) error {
	service := ""
	switch k := doc["kit"].(type) {
	case map[string]any:
		service, _ = k["service"].(string)
	case KitConfig:
		service = k.Service
	}
	delete(doc, "kit")
	if service != "" {
		doc["kit"] = map[string]any{"service": service}
	}
	delete(doc, "layer")
	if previous, ok := doc["kit_prior"]; ok {
		doc["layer"] = previous
	}
	delete(doc, "kit_prior")
	return writeInviteDocument(doc)
}

// ReadKitConfig resolves the hosted config, adopting an exported API key.
// Without an env key it only reads the file and never contacts the service.
func ReadKitConfig() (*KitConfig, error) {
	path := DefaultConfigPath()
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return resolveAPIKey(nil)
	}
	var fc struct {
		Kit *KitConfig `toml:"kit"`
	}
	if _, err := toml.DecodeFile(path, &fc); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	if fc.Kit.SignedIn() {
		if err := secureKitConfig(path); err != nil {
			return nil, err
		}
	}
	return resolveAPIKey(fc.Kit)
}

// ResolveAPIKey is the single hosted credential precedence rule.
func ResolveAPIKey(stored string) string {
	if key := strings.TrimSpace(os.Getenv("HEV_API_KEY")); key != "" {
		return key
	}
	return stored
}

const HostedSetup = "brew install hev/tap/hev\nexport HEV_API_KEY=…"

// resolveAPIKey saves an exported key in the private config so service managers
// need no shell environment. Identity is fetched only when adopting a new key.
// If verification is unavailable, preserve local capture using the existing
// identity and let the authenticated operation report the failure.
func resolveAPIKey(k *KitConfig) (*KitConfig, error) {
	env := strings.TrimSpace(os.Getenv("HEV_API_KEY"))
	if env == "" {
		return k, nil
	}
	if k == nil {
		k = &KitConfig{}
	}
	old := k.Key
	k.storedKey = old
	k.Key = ResolveAPIKey(old)
	if service := os.Getenv("HEV_KIT_SERVICE"); service != "" {
		k.Service = service
		k.API = service
	}
	if k.Service == "" {
		k.Service = "https://kit.hevmind.com"
	}
	if k.API == "" {
		k.API = k.Service
	}
	k.API = strings.TrimRight(k.API, "/")
	if old == env {
		return k, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c := KitClient(k)
	c.Retries = 0
	who, err := c.Whoami(ctx)
	if err != nil {
		return k, nil
	}
	k.KeyID, k.TeamID, k.TeamSlug = who.KeyID, who.Team.ID, who.Team.Slug
	k.AuthorID, k.AuthorName = who.Author.ID, who.Author.Name
	if err := SaveKitConfig(*k); err != nil {
		return nil, err
	}
	return k, nil
}

// secureKitConfig narrows existing credential files before accepting them.
func secureKitConfig(path string) error {
	infoBefore, err := os.Lstat(path)
	if err != nil || !infoBefore.Mode().IsRegular() {
		return fmt.Errorf("secure config %s: use a regular file with permissions 0600", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("secure config %s: set permissions to 0600: %w", path, err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err == nil && !os.SameFile(infoBefore, info) {
		return fmt.Errorf("secure config %s: file changed; retry with permissions 0600", path)
	}
	if err == nil && info.Mode().Perm() != 0600 {
		err = f.Chmod(0600)
	}
	if err != nil {
		return fmt.Errorf("secure config %s: set permissions to 0600: %w", path, err)
	}
	return nil
}

// SaveKitConfig re-reads under a cross-process lock after network verification,
// preserving unrelated edits made while the service was answering.
func SaveKitConfig(k KitConfig) error {
	return updateKitConfig(func(doc map[string]any) error { return writeKitConfig(doc, k) })
}

func updateKitConfig(update func(map[string]any) error) error {
	path := DefaultConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	unlock, err := platformops.LockPath(path+".lock", false)
	if err != nil {
		return err
	}
	defer unlock()
	doc, err := ReadInviteConfig()
	if err != nil {
		return err
	}
	return update(doc)
}
