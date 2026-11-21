package license

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/secrets"
	"github.com/hev/kit/internal/secrets/fakeazure"
)

func isolatedConfig(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HEV_LICENSE", "")
	t.Setenv("HEV_LICENSE_REF", "")
	t.Setenv("HEV_SECRETS_CACHE", "off")
	t.Setenv("HEV_SECRETS_READ_LOG", filepath.Join(home, "reads.log"))
	t.Setenv("HEV_SECRETS_COOLDOWN", filepath.Join(home, "cooldown.json"))
	t.Setenv("HEV_OP_TOKEN_FILE", filepath.Join(home, "absent-token"))
	cfg := filepath.Join(home, "config.toml")
	t.Setenv("HEV_CONFIG", cfg)
	return cfg
}

func TestKitConfigurationPrecedenceAndProjectionRedaction(t *testing.T) {
	cfg := isolatedConfig(t)
	const token = "v4.public.explicit-config-token.footer"
	t.Setenv("LAYER_LICENSE", token)
	t.Setenv("HEVLAYER_LICENSE", token)
	if s := Resolve(t.Context()); s.Entitled() || s.State != Floor || s.Reason != "missing" {
		t.Fatal("Layer variables influenced Kit's OSS floor")
	}
	if err := os.WriteFile(cfg, []byte("[license]\nkey = \""+token+"\"\nkey_ref = \"op://vault/item/password\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	assertSource := func(key, ref string) {
		t.Helper()
		got, err := Configuration()
		if err != nil || got.Key != key || got.KeyRef != ref {
			t.Fatal("license source precedence changed")
		}
	}
	assertSource(token, "op://vault/item/password")
	t.Setenv("HEV_LICENSE_REF", "secret://other")
	assertSource("", "secret://other")
	t.Setenv("HEV_LICENSE", "v4.public.preferred-token.footer")
	assertSource("v4.public.preferred-token.footer", "")
	s := Resolve(t.Context())
	body, err := os.ReadFile(projectionPath())
	if err != nil || s.Valid || s.Entitled() || !strings.Contains(string(body), `"state":"floor"`) || strings.Contains(string(body), "v4.public.") {
		t.Fatal("invalid license was granted or leaked")
	}
	info, err := os.Stat(projectionPath())
	if err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0600) {
		t.Fatal("license status must have private permissions")
	}
}

func TestProjectionEntitlementRechecksTimelineWithoutResolver(t *testing.T) {
	isolatedConfig(t)
	exp := time.Now().UTC()
	s := floor("", exp)
	s.Valid = true
	s.Expires = &exp
	s.Tier = "trial"
	s.Features = []string{"workspaces"}
	s.Limits = map[string]uint64{"runs": 8}
	if err := writeProjection(s); err != nil {
		t.Fatal(err)
	}
	got := ReadProjection(exp)
	limit, ok := got.Limit("runs")
	if !got.Entitled() || !got.HasFeature("workspaces") || got.HasFeature("unknown") || !ok || limit != 8 {
		t.Fatal("current verified projection lost entitlement")
	}
	got = ReadProjection(exp.Add(time.Second))
	if _, ok := got.Limit("runs"); got.Entitled() || got.HasFeature("workspaces") || ok || got.Reason != "expired" {
		t.Fatal("expired trial projection retained entitlement")
	}
	if ReadProjection(exp.Add(secretTTL + time.Second)).Entitled() {
		t.Fatal("stale projection retained entitlement")
	}
}

func TestKeyVaultAndNeutralReferencesUseProviderRouting(t *testing.T) {
	cfg := isolatedConfig(t)
	fake := fakeazure.Start(t)
	fake.Env(t)
	t.Setenv(secrets.ManagerEnv, secrets.ManagerKeyVault)
	t.Setenv(secrets.VaultEnv, fakeazure.Vault)
	t.Setenv("HEV_LICENSE_REF", fakeazure.Ref())
	for i, ref := range []string{fakeazure.Ref(), "secret://" + fakeazure.Secret} {
		if i == 1 {
			t.Setenv("HEV_LICENSE_REF", "")
			if err := os.WriteFile(cfg, []byte("[license]\nkey_ref = \""+ref+"\"\n"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		// The fake vault returns a non-license. The token diagnostic confirms
		// resolution succeeded and the value reached the production verifier.
		if s := Resolve(t.Context()); s.Reason != "invalid Kit license token" || s.Valid || fake.VaultGets() != i+1 {
			t.Fatalf("reference %d did not reach the vault and verifier", i)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := readReference(ctx, fakeazure.Ref()); err == nil {
		t.Fatal("cancelled context resolved a license")
	}
}

func TestOnePasswordReferenceHonorsCooldown(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake op is a POSIX executable")
	}
	isolatedConfig(t)
	bin := t.TempDir()
	path := filepath.Join(bin, "op")
	const script = `#!/bin/sh
IFS= read -r input || :
tail=${input##* }
printf 'v4.public.fixture.footer%s' "${tail#??}"
`
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "fixture-service-account-token")
	t.Setenv("HEV_LICENSE_REF", "op://vault/item/password")
	if s := Resolve(t.Context()); s.Reason != "invalid Kit license token" {
		t.Fatalf("1Password fixture did not reach verifier: %s", s.Reason)
	}
	if err := secrets.NoteDirectResult(time.Now(), false, "daily rate limit exceeded"); err == nil {
		t.Fatal("fixture cooldown was not recorded")
	}
	if s := Resolve(t.Context()); s.Reason != "Kit license secret provider is in cooldown" || s.Entitled() {
		t.Fatal("1Password cooldown did not block a direct read")
	}
}
