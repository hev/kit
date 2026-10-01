package daemon

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// InviteConfig records the redeem response, never the invite code. Machine is
// the number of machines used, as returned at redemption (not a live census).
type InviteConfig struct {
	KeyID        string `toml:"key_id" json:"key_id"`
	Machine      int    `toml:"machine" json:"machine"`
	MachinesLeft int    `toml:"machines_left" json:"machines_left"`
	Until        string `toml:"until" json:"until"`
	For          string `toml:"for" json:"for"`
}

// ReadInviteConfig reads only the file: shell overrides must never select the
// credential used to revoke an invite or sign into its dashboard.
func ReadInviteConfig() (map[string]any, error) {
	path := DefaultConfigPath()
	doc := map[string]any{}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return doc, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("config %s must be a regular file", path)
	}
	if _, err := toml.DecodeFile(path, &doc); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	return doc, nil
}

// WriteInviteConfig preserves unrelated settings and the local stack's target
// so leaving can restore it without starting it or touching its Docker volume.
func WriteInviteConfig(doc map[string]any, endpoint, namespace, key string, invite InviteConfig) error {
	if _, exists := doc["invite"]; !exists && doc["local"] != nil {
		if previous, ok := doc["layer"].(map[string]any); ok {
			doc["invite_local"] = previous
		}
	}
	doc["layer"] = map[string]any{"endpoint": endpoint, "namespace": namespace, "api_key": key}
	doc["invite"] = invite
	capture := table(doc, "capture")
	capture["redact"] = true
	if capture["scan_interval"] == nil {
		capture["scan_interval"] = "5m"
	}
	return writeInviteDocument(doc)
}

func ClearInviteConfig(doc map[string]any) error {
	delete(doc, "layer")
	if previous, ok := doc["invite_local"]; ok {
		doc["layer"] = previous
	}
	delete(doc, "invite_local")
	delete(doc, "invite")
	return writeInviteDocument(doc)
}

func writeInviteDocument(doc map[string]any) error {
	raw, err := tomlBytes(doc)
	if err != nil {
		return err
	}
	return writeFileAtomic(DefaultConfigPath(), raw, 0600)
}
