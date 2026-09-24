package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Resolving the key: the environment first for a shell that already has it,
// then 1Password for a machine provisioned from the vault. The keychain is
// deliberately not consulted — `security` refuses non-interactively over ssh,
// which is exactly how the headless host runs, and a lookup that can only
// succeed on a laptop is worse than one that fails the same way everywhere.
func LayerKey() (string, error) {
	for _, env := range []string{"LAYER_API_KEY", "TURBOPUFFER_API_KEY", "TPUF_API_KEY"} {
		if v := os.Getenv(env); v != "" {
			return v, nil
		}
	}
	ref := os.Getenv("LAYER_API_KEY_OP_REF")
	if ref == "" {
		ref = "op://layer-factory/layer-turbopuffer/credential"
	}
	if _, err := exec.LookPath("op"); err != nil {
		return "", fmt.Errorf("no LAYER_API_KEY in the environment and no `op` to read %s", ref)
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
