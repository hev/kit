//go:build !windows

package layerenv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/secrets/fakeazure"
	"github.com/hev/kit/internal/secrets/fakeop"
)

// HEVLAYER_API_KEY_REF=secret://NAME reads through the host's secret manager:
// 1Password, then Key Vault after the host's config alone changes. No daemon
// runs, so each read is the direct path for the explicit form.
func TestLayerKeyNeutralReference(t *testing.T) {
	op := fakeop.Install(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("HEV_SECRETS_CACHE", "off")
	t.Setenv("HEV_SECRETS_READ_LOG", filepath.Join(home, "reads.log"))
	t.Setenv("HEVLAYER_API_KEY", "")
	for _, k := range []string{"LAYER_API_KEY", GatewayKey, "TPUF_API_KEY", "HEVLAYER_API_KEY_OP_REF", "LAYER_API_KEY_OP_REF", "HEV_SECRETS_MANAGER", "HEV_SECRETS_VAULT", "HEV_SECRETS_FIELD"} {
		t.Setenv(k, "")
	}
	t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "fake-service-account-token")
	f := fakeazure.Start(t)
	f.Env(t)
	cfg := filepath.Join(home, "config.toml")
	t.Setenv("HEV_CONFIG", cfg)
	t.Setenv("HEVLAYER_API_KEY_REF", "secret://layer-api-key")

	if _, err := ReadKey(); err == nil || !strings.Contains(err.Error(), "HEVLAYER_API_KEY_REF") || !strings.Contains(err.Error(), "HEV_SECRETS_MANAGER") {
		t.Fatalf("no manager: %v", err)
	}
	os.WriteFile(cfg, []byte("[secrets]\nmanager = \"1password\"\nvault = \"example\"\n"), 0o600)
	if key, err := ReadKey(); err != nil || key != fakeop.Value {
		t.Fatalf("1Password host: %v", err)
	}
	if lines := op.CallLines(t); len(lines) != 1 || lines[0] != "read op://example/layer-api-key/password" {
		t.Fatalf("op calls: %q", lines)
	}
	os.WriteFile(cfg, []byte("[secrets]\nmanager = \"azure-key-vault\"\nvault = \""+fakeazure.Vault+"\"\n"), 0o600)
	if key, err := ReadKey(); err != nil || key != fakeazure.Value {
		t.Fatalf("Key Vault host: %v", err)
	}
	if f.VaultGets() != 1 || len(op.CallLines(t)) != 1 {
		t.Fatalf("Key Vault host: vault %d, op %d", f.VaultGets(), len(op.CallLines(t)))
	}
}
