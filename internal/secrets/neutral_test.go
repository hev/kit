package secrets

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/secrets/fakeazure"
)

// setManager writes kit's config file with body and points HEV_CONFIG at it,
// clearing the environment override. It returns the file's path.
func setManager(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	if body != "" {
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HEV_CONFIG", p)
	t.Setenv(ManagerEnv, "")
	t.Setenv(VaultEnv, "")
	t.Setenv(FieldEnv, "")
	return p
}

const (
	opHost = "[layer]\nnamespace = \"x\"\n\n[secrets]\nmanager = \"1password\"\nvault = \"Example Vault\"\n"
	kvHost = "[secrets]\nmanager = \"azure-key-vault\"\nvault = \"" + fakeazure.Vault + "\"\n"
)

func TestNeutralReferenceMapping(t *testing.T) {
	cases := []struct{ config, in, want string }{
		{opHost, "secret://stripe-api-key", "op://Example Vault/stripe-api-key/password"},
		{opHost + "field = \"credential\"\n", "secret://stripe-api-key", "op://Example Vault/stripe-api-key/credential"},
		{kvHost, "secret://stripe-api-key", "azkv://example-vault/stripe-api-key"},
		{"[secrets]\nmanager = \"azure-key-vault\"\nvault = \"gov-vault.vault.usgovcloudapi.net\"\n", "secret://db-password", "azkv://gov-vault.vault.usgovcloudapi.net/db-password"},
		// Explicit references never read the setting and pass unchanged.
		{"", "op://example/item/field", "op://example/item/field"},
		{"", "azkv://example-vault/db-password", "azkv://example-vault/db-password"},
	}
	for _, c := range cases {
		setManager(t, c.config)
		got, err := ExpandRef(c.in)
		if err != nil || got != c.want {
			t.Errorf("%q under %q: got %q, %v; want %q", c.in, c.config, got, err, c.want)
		}
		if err == nil && !ValidRef(got) {
			t.Errorf("expansion %q is not a valid explicit reference", got)
		}
	}
	// Azure Stack Hub: a suffix listed in HEV_AZKV_DNS_SUFFIXES.
	t.Setenv("HEV_AZKV_DNS_SUFFIXES", "vault.local.azurestack.external")
	setManager(t, "[secrets]\nmanager = \"azure-key-vault\"\nvault = \"hub-vault.vault.local.azurestack.external\"\n")
	if got, err := ExpandRef("secret://db"); err != nil || got != "azkv://hub-vault.vault.local.azurestack.external/db" {
		t.Errorf("Stack Hub: %q %v", got, err)
	}
}

func TestNeutralEnvironmentOverridesFile(t *testing.T) {
	setManager(t, opHost)
	t.Setenv(ManagerEnv, "azure-key-vault")
	t.Setenv(VaultEnv, "other-vault")
	m, err := HostManager()
	if err != nil || m.Kind != ManagerKeyVault || m.Vault != "other-vault" || m.Source != "environment (HEV_SECRETS_MANAGER)" {
		t.Fatalf("environment override: %+v %v", m, err)
	}
	t.Setenv(ManagerEnv, "")
	m, err = HostManager()
	if err != nil || m.Kind != ManagerOnePassword || !strings.HasSuffix(m.Source, "config.toml [secrets]") {
		t.Fatalf("file setting: %+v %v", m, err)
	}
}

func TestNeutralNamesArePortable(t *testing.T) {
	setManager(t, kvHost)
	for _, bad := range []string{"secret://", "secret://Stripe", "secret://stripe_key", "secret://a/b", "secret://a b", "secret://a.b",
		"secret://" + strings.Repeat("a", 128), "secret://a\n", "secret://a?x=1"} {
		_, err := ExpandRef(bad)
		if err == nil || !strings.Contains(err.Error(), "lowercase letters, digits and hyphens") {
			t.Errorf("%q: %v", bad, err)
		}
		if _, err := ExpandRefs([]Ref{{"X", bad, time.Minute}}); err == nil {
			t.Errorf("ExpandRefs accepted %q", bad)
		}
	}
	if _, err := ExpandRef("secret://" + strings.Repeat("a", 127)); err != nil {
		t.Errorf("127 characters: %v", err)
	}
}

// With no manager configured a secret:// read fails, naming the setting, and
// never guesses a manager from credentials that happen to be present.
func TestNeutralNoManagerNamesTheSetting(t *testing.T) {
	f, _ := kvEnv(t) // Azure credentials present
	t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "fake-service-account-token")
	path := setManager(t, "")
	_, err := New().Resolve(context.Background(), []Ref{{"DB", "secret://db-password", time.Minute}})
	if err == nil {
		t.Fatal("resolved with no manager")
	}
	msg := err.Error()
	for _, want := range []string{"secret:// needs this host's secret manager", path, `[secrets] manager = "1password" or "azure-key-vault"`, `vault = "VAULT"`, "HEV_SECRETS_MANAGER", "HEV_SECRETS_VAULT"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error lacks %q: %s", want, msg)
		}
	}
	if f.Requests() != 0 {
		t.Fatal("a request was made with no manager")
	}
	// The client path says the same before any IPC.
	if _, err := ResolveResult(context.Background(), []Ref{{"DB", "secret://db-password", time.Minute}}, Options{CacheOnly: true}); err == nil || !strings.Contains(err.Error(), "HEV_SECRETS_MANAGER") {
		t.Fatalf("client: %v", err)
	}
	for _, bad := range []string{
		"[secrets]\nmanager = \"vault\"\nvault = \"x\"\n",
		"[secrets]\nvault = \"example-vault\"\n",
		"[secrets]\nmanager = \"azure-key-vault\"\n",
		"[secrets]\nmanager = \"azure-key-vault\"\nvault = \"bad_vault\"\n",
		"[secrets]\nmanager = \"azure-key-vault\"\nvault = \"example-vault\"\nfield = \"password\"\n",
		"[secrets]\nmanager = \"1password\"\n",
		"[secrets]\nmanager = \"1password\"\nvault = \"a/b\"\n",
		"[secrets\n",
	} {
		setManager(t, bad)
		if _, err := ExpandRef("secret://db"); err == nil || !strings.Contains(err.Error(), "secret manager") {
			t.Errorf("%q: %v", bad, err)
		}
	}
}

// secret://NAME and its explicit form are one cache entry and one request,
// whether they arrive in one batch or one after the other, on either manager.
func TestNeutralSharesTheExplicitCacheEntry(t *testing.T) {
	t.Run("azure-key-vault", func(t *testing.T) {
		f, dir := kvEnv(t)
		setManager(t, kvHost)
		c := New()
		v, _, err := c.ResolveSources(context.Background(), "test", []Ref{{"A", "secret://" + fakeazure.Secret, time.Minute}, {"B", fakeazure.Ref(), time.Minute}})
		if err != nil || v["A"] != fakeazure.Value || v["B"] != fakeazure.Value || f.VaultGets() != 1 {
			t.Fatalf("one batch: %v, %d gets", err, f.VaultGets())
		}
		_, src, err := c.ResolveSources(context.Background(), "test", []Ref{{"A", fakeazure.Ref(), time.Minute}, {"B", "secret://" + fakeazure.Secret, time.Minute}})
		if err != nil || src["A"] != SourceCache || src["B"] != SourceCache || f.VaultGets() != 1 {
			t.Fatalf("second call: %v %v, %d gets", err, src, f.VaultGets())
		}
		log, _ := os.ReadFile(filepath.Join(dir, "reads.log"))
		if strings.Count(string(log), "kind=get ") != 1 || strings.Count(string(log), "kind=cache ") != 2 || !strings.Contains(string(log), "provider=azkv") || strings.Contains(string(log), "secret://") {
			t.Fatalf("read log: %q", log)
		}
		if Label("A", "secret://"+fakeazure.Secret) == Label("A", fakeazure.Ref()) {
			t.Fatal("label test is vacuous")
		}
		if !strings.Contains(string(log), Label("A", fakeazure.Ref())) {
			t.Fatalf("read log label is not the explicit form's: %q", log)
		}
	})
	t.Run("1password", func(t *testing.T) {
		c := fake(t)
		calls := filepath.Join(t.TempDir(), "op-calls")
		t.Setenv("HEV_FAKE_CALLS", calls)
		setManager(t, "[secrets]\nmanager = \"1password\"\nvault = \"example\"\nfield = \"token\"\n")
		if _, err := c.Resolve(context.Background(), []Ref{{"A", "secret://item", time.Minute}, {"B", "op://example/item/token", time.Minute}}); err != nil {
			t.Fatal(err)
		}
		_, src, err := c.ResolveSources(context.Background(), "test", []Ref{{"A", "op://example/item/token", time.Minute}, {"B", "secret://item", time.Minute}})
		if err != nil || src["A"] != SourceCache || src["B"] != SourceCache {
			t.Fatalf("second call: %v %v", err, src)
		}
		b, _ := os.ReadFile(calls)
		if strings.Count(string(b), "inject") != 1 {
			t.Fatalf("want one op inject: %q", b)
		}
	})
}

// AZURE_CLIENT_SECRET_REF may be secret://: on a 1Password host it reads the
// principal's secret from 1Password; on a Key Vault host it is a cycle.
func TestNeutralCredentialReference(t *testing.T) {
	t.Run("cycle-on-key-vault-host", func(t *testing.T) {
		f, _ := kvEnv(t)
		setManager(t, kvHost)
		t.Setenv("AZURE_CLIENT_SECRET", "")
		t.Setenv("AZURE_CLIENT_SECRET_REF", "secret://principal-secret")
		_, err := New().Resolve(context.Background(), kvRefs())
		if err == nil || !strings.Contains(err.Error(), "cycle") || f.Requests() != 0 {
			t.Fatalf("want a cycle refusal: %v", err)
		}
	})
	t.Run("no-manager", func(t *testing.T) {
		kvEnv(t)
		setManager(t, "")
		t.Setenv("AZURE_CLIENT_SECRET", "")
		t.Setenv("AZURE_CLIENT_SECRET_REF", "secret://principal-secret")
		_, err := New().Resolve(context.Background(), kvRefs())
		if err == nil || !strings.Contains(err.Error(), "AZURE_CLIENT_SECRET_REF") || !strings.Contains(err.Error(), "HEV_SECRETS_MANAGER") {
			t.Fatalf("want the setting named: %v", err)
		}
	})
	t.Run("from-1password-host", func(t *testing.T) {
		c := fake(t)
		f := fakeazure.Start(t)
		f.Env(t)
		setManager(t, opHost)
		t.Setenv("AZURE_CLIENT_SECRET", "")
		t.Setenv("AZURE_CLIENT_SECRET_REF", "secret://azure-principal")
		t.Setenv("HEV_FAKE_VALUE", fakeazure.ClientSecret)
		calls := filepath.Join(t.TempDir(), "op-calls")
		t.Setenv("HEV_FAKE_CALLS", calls)
		v, err := c.Resolve(context.Background(), kvRefs())
		if err != nil || v["DB_PASSWORD"] != fakeazure.Value {
			t.Fatalf("read: %v", err)
		}
		// The fingerprint covers the expanded reference, as for the explicit one.
		cr, err := azureCredential()
		if err != nil || cr.secretRef != "op://Example Vault/azure-principal/password" {
			t.Fatalf("credential reference: %q %v", cr.secretRef, err)
		}
		b, _ := os.ReadFile(calls)
		if strings.Count(string(b), "inject") != 1 {
			t.Fatalf("want one op inject: %q", b)
		}
	})
}

func TestNeutralManagerStatus(t *testing.T) {
	path := setManager(t, "")
	s := HostManagerStatus()
	if s.Configured || !strings.Contains(s.Setup, path) || s.Source != path {
		t.Fatalf("unset: %+v", s)
	}
	setManager(t, kvHost)
	s = HostManagerStatus()
	if !s.Configured || s.Kind != ManagerKeyVault || s.ManagerName != "Azure Key Vault" || s.Vault != fakeazure.Vault || s.Setup != "" {
		t.Fatalf("set: %+v", s)
	}
}

// Expansion must preserve all guard behavior, including the paid provider
// identity in cross-host summaries rather than inventing a "secret" provider.
func TestNeutralKeyVaultGuardsAndAttribution(t *testing.T) {
	f, _ := kvEnv(t)
	setManager(t, kvHost)
	setPolicy(t, Policy{Daily: 2, Hourly: 2, CallerMinute: 1})
	c := New()
	ctx := context.Background()
	r := []Ref{{"A", "secret://first", time.Hour}}
	if _, err := c.ResolveAs(ctx, "poller", r); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ResolveAs(ctx, "poller", []Ref{{"A", "secret://second", time.Hour}}); !IsBudgetError(err) {
		t.Fatalf("caller brake: %v", err)
	}
	if _, err := c.ResolveAs(ctx, "other", []Ref{{"A", "secret://second", time.Hour}}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ResolveAs(ctx, "third", []Ref{{"A", "secret://third", time.Hour}}); !IsBudgetError(err) {
		t.Fatalf("host budget: %v", err)
	}
	if _, err := c.ResolveAs(ctx, "poller", r); err != nil {
		t.Fatal(err)
	}
	if f.VaultGets() != 2 {
		t.Fatalf("vault gets: %d", f.VaultGets())
	}
	rows, err := Summary(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, row := range rows {
		if row.Caller == "poller" {
			found = true
			if row.Provider != "azkv" || row.Requests != 1 || row.CacheHits != 1 || row.Throttles != 1 {
				t.Fatalf("attribution: %+v", row)
			}
		}
	}
	if !found {
		t.Fatal("missing caller")
	}
}

func TestNeutralKeyVaultStaleAndRefresh(t *testing.T) {
	f, _ := kvEnv(t)
	setManager(t, kvHost)
	c := New()
	now := time.Now()
	c.now = func() time.Time { return now }
	ctx := context.Background()
	r := []Ref{{"A", "secret://first", time.Minute}}
	if _, err := c.ResolveAs(ctx, "worker", r); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	f.Throttle("60")
	if _, src, err := c.ResolveSources(ctx, "worker", r); err != nil || src["A"] != SourceStale {
		t.Fatalf("stale: %v %v", src, err)
	}
	if _, _, err := c.ResolveSources(ctx, "worker", r); err != nil || f.VaultGets() != 2 {
		t.Fatalf("cooldown: %v gets=%d", err, f.VaultGets())
	}
	f.Unthrottle()
	now = now.Add(2 * time.Minute)
	if _, src, err := c.ResolveSources(ctx, "worker", r); err != nil || src["A"] != SourceRead {
		t.Fatalf("refresh: %v %v", src, err)
	}
	now = now.Add(2 * time.Minute)
	f.Forbid(true)
	if _, _, err := c.ResolveSources(ctx, "worker", r); err == nil {
		t.Fatal("served revoked value")
	}
	if c.LocalStatus().Stale != 2 {
		t.Fatal("stale count")
	}
}
