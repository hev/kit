package daemon

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/secrets"
	"github.com/hev/kit/internal/secrets/fakeazure"
	"github.com/hev/kit/internal/secrets/fakeop"
)

// HEVLAYER_API_KEY_REF takes either scheme; HEVLAYER_API_KEY_OP_REF, the old
// name, still works; the new name wins when both are set.
func TestLayerKeyReferenceNamesAndSchemes(t *testing.T) {
	f, home := layerKeyEnv(t) // fake op, a token, HEVLAYER_API_KEY_OP_REF=op://...
	kv := fakeazure.Start(t)
	kv.Env(t)
	t.Setenv("HEVLAYER_API_KEY_REF", "")
	t.Setenv("HEV_SECRETS_ENDPOINT", filepath.Join(home, "agent.sock"))

	// Old name, 1Password: unchanged.
	if key, err := LayerKey(); err != nil || key != fakeop.Value {
		t.Fatalf("old name: %v", err)
	}
	// New name, 1Password.
	t.Setenv("HEVLAYER_API_KEY_OP_REF", "")
	t.Setenv("HEVLAYER_API_KEY_REF", "op://example/layer/credential")
	if key, err := LayerKey(); err != nil || key != fakeop.Value {
		t.Fatalf("new name, op: %v", err)
	}
	// New name, Key Vault, no daemon: a direct read with no op involved.
	opCalls := f.Count(t)
	t.Setenv("HEVLAYER_API_KEY_REF", fakeazure.Ref())
	if key, err := LayerKey(); err != nil || key != fakeazure.Value {
		t.Fatalf("new name, azkv, direct: %v", err)
	}
	// Old name, Key Vault: the old name is only a name.
	t.Setenv("HEVLAYER_API_KEY_REF", "")
	t.Setenv("HEVLAYER_API_KEY_OP_REF", fakeazure.Ref())
	if key, err := LayerKey(); err != nil || key != fakeazure.Value {
		t.Fatalf("old name, azkv: %v", err)
	}
	// Both set: the new name wins.
	t.Setenv("HEVLAYER_API_KEY_REF", "op://example/layer/credential")
	if key, err := LayerKey(); err != nil || key != fakeop.Value {
		t.Fatalf("new name should win: %v", err)
	}
	if f.Count(t) != opCalls+1 || kv.VaultGets() != 2 {
		t.Fatalf("op %d (want %d), vault %d (want 2)", f.Count(t), opCalls+1, kv.VaultGets())
	}

	// New name, Key Vault, through a daemon: two reads, one vault request.
	t.Setenv("HEVLAYER_API_KEY_OP_REF", "")
	t.Setenv("HEVLAYER_API_KEY_REF", "azkv://example-vault/layer-key")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop, err := secrets.Serve(ctx, secrets.New())
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	for i := 0; i < 2; i++ {
		if key, err := LayerKey(); err != nil || key != fakeazure.Value {
			t.Fatalf("daemon read %d: %v", i, err)
		}
	}
	if kv.VaultGets() != 3 {
		t.Fatalf("two daemon reads cost %d vault requests", kv.VaultGets()-2)
	}
	log, _ := os.ReadFile(filepath.Join(home, "op-reads.log"))
	if strings.Contains(string(log), fakeazure.Value) || strings.Contains(string(log), "azkv://") || !strings.Contains(string(log), "provider=azkv") {
		t.Fatalf("read log: %q", log)
	}

	// Neither name: the error names the new one and the old one.
	t.Setenv("HEVLAYER_API_KEY_REF", "")
	if _, err := LayerKey(); err == nil || !strings.Contains(err.Error(), "HEVLAYER_API_KEY_REF") || !strings.Contains(err.Error(), "HEVLAYER_API_KEY_OP_REF") {
		t.Fatalf("missing reference error: %v", err)
	}
}
