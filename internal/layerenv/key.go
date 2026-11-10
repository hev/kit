package layerenv

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ConfigPath is kit's config file: HEV_CONFIG, or ~/.hev/config.toml.
func ConfigPath() string {
	if p := os.Getenv("HEV_CONFIG"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "config.toml"
	}
	return filepath.Join(home, ".hev", "config.toml")
}

// PlaceholderKey is what the retired local Docker lane wrote as its API
// key. It is not a key, and counts as none.
const PlaceholderKey = "local"

// ReadKey resolves the Layer API key: the environment first for a shell that
// already has it, then 1Password for a machine provisioned from the vault.
// The keychain is deliberately not consulted — `security` refuses
// non-interactively over ssh, which is exactly how the headless host runs,
// and a lookup that can only succeed on a laptop is worse than one that fails
// the same way everywhere.
func ReadKey() (string, error) {
	WarnDeprecated(os.Stderr)
	if key := Get("HEVLAYER_API_KEY"); key != "" {
		return key, nil
	}
	ref := Get("HEVLAYER_API_KEY_OP_REF")
	if ref == "" {
		return "", fmt.Errorf("no HEVLAYER_API_KEY configured; set HEVLAYER_API_KEY or HEVLAYER_API_KEY_OP_REF explicitly")
	}
	if _, err := exec.LookPath("op"); err != nil {
		return "", fmt.Errorf("no HEVLAYER_API_KEY in the environment and no `op` to read %s", ref)
	}
	cmd := exec.Command("op", "read", ref)
	// The service-account token is what authorizes 1Password, so it cannot come
	// from 1Password. A non-login shell has no profile to export it.
	if os.Getenv("OP_SERVICE_ACCOUNT_TOKEN") == "" {
		if home, err := os.UserHomeDir(); err == nil {
			if b, err := os.ReadFile(home + "/.config/op/token"); err == nil {
				cmd.Env = append(os.Environ(), "OP_SERVICE_ACCOUNT_TOKEN="+strings.TrimSpace(string(b)))
			}
		}
	}
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("op read %s: %w", ref, err)
	}
	key := strings.TrimSpace(string(out))
	if key == "" {
		return "", fmt.Errorf("op read %s returned nothing", ref)
	}
	return key, nil
}
