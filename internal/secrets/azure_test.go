package secrets

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/secrets/fakeazure"
)

// kvEnv isolates a Key Vault-only host: the fake vault and sign-in endpoint,
// a private read log and cooldown directory, no 1Password token and a PATH
// with no op on it.
func kvEnv(t *testing.T) (*fakeazure.Fake, string) {
	t.Helper()
	f := fakeazure.Start(t)
	f.Env(t)
	dir := t.TempDir()
	t.Setenv("HEV_SECRETS_READ_LOG", filepath.Join(dir, "reads.log"))
	t.Setenv("HEV_SECRETS_COOLDOWN", "")
	t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "")
	t.Setenv("HEV_OP_TOKEN_FILE", filepath.Join(dir, "absent-token"))
	t.Setenv("HEV_FAKE_OP", "")
	empty := filepath.Join(dir, "empty-bin")
	os.Mkdir(empty, 0o700)
	t.Setenv("PATH", empty)
	return f, dir
}

func kvRefs() []Ref { return []Ref{{"DB_PASSWORD", fakeazure.Ref(), time.Minute}} }

func TestKeyVaultThroughCacheSecondReadMakesNoRequest(t *testing.T) {
	f, dir := kvEnv(t)
	c := New()
	now := time.Now()
	c.now = func() time.Time { return now }
	v, src, err := c.ResolveSources(context.Background(), "test", kvRefs())
	if err != nil || v["DB_PASSWORD"] != fakeazure.Value || src["DB_PASSWORD"] != SourceRead {
		t.Fatalf("first read: %v", err)
	}
	if f.VaultGets() != 1 || f.TokenPosts() != 1 {
		t.Fatalf("want one vault read and one token request, got %d and %d", f.VaultGets(), f.TokenPosts())
	}
	now = now.Add(30 * time.Second)
	v, src, err = c.ResolveSources(context.Background(), "test", kvRefs())
	if err != nil || v["DB_PASSWORD"] != fakeazure.Value || src["DB_PASSWORD"] != SourceCache {
		t.Fatalf("second read: %v", err)
	}
	if f.Requests() != 2 {
		t.Fatalf("a read inside the TTL made an HTTP request: %d", f.Requests())
	}
	// After the TTL: a new vault read, but the access token is reused.
	now = now.Add(time.Minute)
	if _, err = c.Resolve(context.Background(), kvRefs()); err != nil || f.VaultGets() != 2 || f.TokenPosts() != 1 {
		t.Fatalf("after TTL: %v, %d gets, %d token requests", err, f.VaultGets(), f.TokenPosts())
	}
	log, _ := os.ReadFile(filepath.Join(dir, "reads.log"))
	if !strings.Contains(string(log), "kind=get refs=1 labels=DB_PASSWORD#") || !strings.Contains(string(log), "provider=azkv") || strings.Contains(string(log), fakeazure.Vault) {
		t.Fatalf("read log line: %q", log)
	}
}

// With only Azure configured: no op binary on PATH, no 1Password token, and
// reads succeed through the cache, the direct fallback and a daemon.
func TestKeyVaultOnlyHostNeedsNoOp(t *testing.T) {
	f, _ := kvEnv(t)
	if _, err := os.Stat(filepath.Join(os.Getenv("PATH"), "op")); err == nil {
		t.Fatal("op on PATH")
	}
	if _, err := New().Resolve(context.Background(), kvRefs()); err != nil {
		t.Fatal(err)
	}
	var notice bytes.Buffer
	t.Setenv("HEV_SECRETS_ENDPOINT", shortEndpoint(t))
	res, err := ResolveResult(context.Background(), kvRefs(), Options{Caller: "t", Notice: &notice})
	if err != nil || res.Via != ViaDirect || res.Values["DB_PASSWORD"] != fakeazure.Value {
		t.Fatalf("direct: %v %+v", err, res.Via)
	}
	if n := notice.String(); !strings.HasPrefix(n, FallbackPrefix+" Azure Key Vault read (") || strings.Contains(n, "1Password") {
		t.Fatalf("a Key Vault fallback must name Key Vault, not 1Password: %q", n)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop, err := Serve(ctx, New())
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	if err := Ping(ctx); err != nil {
		t.Fatalf("an Azure-only daemon must answer ping: %v", err)
	}
	before := f.VaultGets()
	for i := 0; i < 3; i++ {
		res, err = ResolveResult(ctx, kvRefs(), Options{Caller: "t", Notice: io.Discard})
		if err != nil || res.Via != ViaDaemon || res.Values["DB_PASSWORD"] != fakeazure.Value {
			t.Fatalf("daemon: %v %s", err, res.Via)
		}
	}
	if f.VaultGets() != before+1 {
		t.Fatalf("three daemon reads cost %d requests", f.VaultGets()-before)
	}
	// A client with a different principal is not served by this daemon.
	t.Setenv("AZURE_CLIENT_SECRET", "another-fake-secret")
	if _, err = ResolveWith(ctx, kvRefs(), Options{CacheOnly: true}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("a different principal was served: %v", err)
	}
}

// The mirror: 1Password with no Azure configuration at all, and the Azure
// provider never asked.
func TestOnePasswordOnlyHostNeedsNoAzure(t *testing.T) {
	c := fake(t)
	f := fakeazure.Start(t)
	f.Env(t)
	for _, k := range []string{"AZURE_TENANT_ID", "AZURE_CLIENT_ID", "AZURE_CLIENT_SECRET", "AZURE_AUTHORITY_HOST", "HEV_AZKV_TEST_VAULT_URL"} {
		t.Setenv(k, "")
	}
	if _, err := c.Resolve(context.Background(), refs()); err != nil {
		t.Fatal(err)
	}
	s := c.LocalStatus()
	var kv ProviderStatus
	for _, p := range s.Providers {
		if p.Scheme == "azkv" {
			kv = p
		}
	}
	if kv.Configured || !strings.Contains(kv.Setup, "AZURE_CLIENT_SECRET") || f.Requests() != 0 {
		t.Fatalf("Azure must be unconfigured and untouched: %+v, %d requests", kv, f.Requests())
	}
	if _, err := c.Resolve(context.Background(), kvRefs()); err == nil || !strings.Contains(err.Error(), "no credential configured") {
		t.Fatalf("an azkv read on a 1Password-only host must name what to set: %v", err)
	}
}

// A 429 with Retry-After is one request; no Key Vault request follows until
// that time, across processes (the persisted cooldown), and 1Password reads
// continue meanwhile.
func TestKeyVaultRetryAfterCooldownLeavesOnePasswordAlone(t *testing.T) {
	c := fake(t) // fake op, token set
	f := fakeazure.Start(t)
	f.Env(t)
	t.Setenv("HEV_SECRETS_COOLDOWN", "")
	now := time.Now()
	c.now = func() time.Time { return now }
	f.Throttle("120")
	_, err := c.Resolve(context.Background(), kvRefs())
	if err == nil || !IsRateLimit(err) || !strings.Contains(err.Error(), "Azure Key Vault rate limit; retry after ") || strings.Contains(err.Error(), fakeazure.Value) {
		t.Fatalf("want a Key Vault rate-limit error: %v", err)
	}
	if f.VaultGets() != 1 {
		t.Fatalf("a 429 must be one request, got %d", f.VaultGets())
	}
	reset := CooldownReset(err)
	if d := reset.Sub(now); d < 119*time.Second || d > 121*time.Second {
		t.Fatalf("Retry-After not honoured: %s", d)
	}
	f.Unthrottle()
	for _, at := range []time.Duration{0, time.Second, 60 * time.Second, 119 * time.Second} {
		now2 := now.Add(at)
		c.now = func() time.Time { return now2 }
		if _, err := c.Resolve(context.Background(), kvRefs()); !IsRateLimit(err) {
			t.Fatalf("at +%s: want the cooldown, got %v", at, err)
		}
		// 1Password reads continue during the vault's cooldown.
		fresh := []Ref{{"OP_FRESH", "op://example/item/token", 0}}
		if _, err := c.Resolve(context.Background(), fresh); err != nil {
			t.Fatalf("1Password blocked by a Key Vault cooldown: %v", err)
		}
		// A fresh process sees the same cooldown on disk.
		other := New()
		other.now = c.now
		if _, err := other.Resolve(context.Background(), kvRefs()); !IsRateLimit(err) {
			t.Fatalf("another process ignored the persisted cooldown: %v", err)
		}
	}
	if f.VaultGets() != 1 {
		t.Fatalf("Key Vault requests during the cooldown: %d", f.VaultGets()-1)
	}
	if _, ok := ActiveCooldown(now); ok {
		t.Fatal("a Key Vault cooldown must not become 1Password's")
	}
	// A mixed request in the vault's cooldown fails as a whole and asks
	// neither store.
	reads := len(c.reads)
	mixed := []Ref{{"OP_NEW", "op://example/other/new", 0}, {"KV", fakeazure.Ref(), 0}}
	if v, err := c.Resolve(context.Background(), mixed); err == nil || v != nil || len(c.reads) != reads {
		t.Fatalf("a mixed request must fail atomically: %v %v", v, err)
	}
	after := now.Add(122 * time.Second)
	c.now = func() time.Time { return after }
	if v, err := c.Resolve(context.Background(), kvRefs()); err != nil || v["DB_PASSWORD"] != fakeazure.Value || f.VaultGets() != 2 {
		t.Fatalf("after the reset: %v, %d gets", err, f.VaultGets())
	}
	if _, ok := ActiveCooldownFor("azkv", after); ok {
		t.Fatal("success after the reset must clear the cooldown")
	}
}

func TestKeyVaultRetryAfterForms(t *testing.T) {
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	for in, want := range map[string]time.Duration{"": 0, "x": 0, "-5": 0, "30": 30 * time.Second, "999999": 24 * time.Hour,
		"Wed, 11 Nov 2026 12:02:00 GMT": 2 * time.Minute, "Wed, 11 Nov 2026 11:00:00 GMT": 0} {
		if got := retryAfter(in, now); got != want {
			t.Errorf("Retry-After %q: got %s want %s", in, got, want)
		}
	}
}

// Every supported credential type obtains a token from the fake endpoint and
// reads the secret.
func TestKeyVaultCredentialTypes(t *testing.T) {
	read := func(t *testing.T, f *fakeazure.Fake, wantLabel string) {
		t.Helper()
		c := New()
		if cred, err := c.provider("azkv").Credential(); err != nil || !strings.HasPrefix(cred, wantLabel) {
			t.Fatalf("credential %q, %v; want %s", cred, err, wantLabel)
		}
		v, err := c.Resolve(context.Background(), kvRefs())
		if err != nil || v["DB_PASSWORD"] != fakeazure.Value {
			t.Fatalf("read: %v", err)
		}
	}
	t.Run("client-secret", func(t *testing.T) {
		f, _ := kvEnv(t)
		read(t, f, "client-secret")
		if f.TokenPosts() != 1 {
			t.Fatal("no token request")
		}
	})
	t.Run("client-secret-from-1password-reference", func(t *testing.T) {
		c := fake(t)
		f := fakeazure.Start(t)
		f.Env(t)
		t.Setenv("AZURE_CLIENT_SECRET", "")
		t.Setenv("AZURE_CLIENT_SECRET_REF", "op://example/azure-principal/secret")
		t.Setenv("HEV_FAKE_VALUE", fakeazure.ClientSecret)
		calls := filepath.Join(t.TempDir(), "op-calls")
		t.Setenv("HEV_FAKE_CALLS", calls)
		v, err := c.Resolve(context.Background(), kvRefs())
		if err != nil || v["DB_PASSWORD"] != fakeazure.Value {
			t.Fatalf("read: %v", err)
		}
		// Token expiry forces a second sign-in; the principal secret comes
		// from the cache, not a second 1Password request.
		c.provider("azkv").(*azureKV).tokens = map[string]accessToken{}
		if _, err = c.Resolve(context.Background(), []Ref{{"DB_PASSWORD", fakeazure.Ref(), 0}}); err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(calls)
		if strings.Count(string(b), "inject") != 1 || f.TokenPosts() != 2 {
			t.Fatalf("want one 1Password read for two sign-ins: %q, %d", b, f.TokenPosts())
		}
	})
	t.Run("client-certificate", func(t *testing.T) {
		f, dir := kvEnv(t)
		key, _ := rsa.GenerateKey(rand.Reader, 2048)
		tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "example"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
		if err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(dir, "principal.pem")
		os.WriteFile(p, fakeazure.CertificatePEM(t, der, key), 0o600)
		f.TrustCertificate(&key.PublicKey)
		t.Setenv("AZURE_CLIENT_SECRET", "")
		t.Setenv("AZURE_CLIENT_CERTIFICATE_PATH", p)
		read(t, f, "client-certificate")
	})
	t.Run("workload-identity", func(t *testing.T) {
		f, dir := kvEnv(t)
		p := filepath.Join(dir, "federated-token")
		os.WriteFile(p, []byte(fakeazure.FederatedToken+"\n"), 0o600)
		t.Setenv("AZURE_CLIENT_SECRET", "")
		t.Setenv("AZURE_FEDERATED_TOKEN_FILE", p)
		read(t, f, "workload-identity")
	})
	t.Run("managed-identity-imds", func(t *testing.T) {
		f, _ := kvEnv(t)
		t.Setenv("AZURE_CLIENT_SECRET", "")
		t.Setenv("HEV_AZKV_CREDENTIAL", "managed-identity")
		t.Setenv("HEV_AZKV_TEST_IMDS_URL", f.URL+"/imds")
		read(t, f, "managed-identity (imds)")
		if f.IdentityGets() != 1 || f.TokenPosts() != 0 {
			t.Fatal("token did not come from the identity endpoint")
		}
	})
	t.Run("managed-identity-app-service", func(t *testing.T) {
		f, _ := kvEnv(t)
		for k, v := range map[string]string{"AZURE_CLIENT_SECRET": "", "AZURE_TENANT_ID": "", "AZURE_CLIENT_ID": "",
			"IDENTITY_ENDPOINT": f.URL + "/appservice", "IDENTITY_HEADER": fakeazure.IdentityHeader} {
			t.Setenv(k, v)
		}
		read(t, f, "managed-identity (app-service)")
	})
	t.Run("managed-identity-arc", func(t *testing.T) {
		f, dir := kvEnv(t)
		old := arcTokenDir
		arcTokenDir = func() string { return dir }
		t.Cleanup(func() { arcTokenDir = old })
		fakeazure.ArcRealm = fakeazure.ArcKeyFile(t, dir)
		for k, v := range map[string]string{"AZURE_CLIENT_SECRET": "", "AZURE_TENANT_ID": "", "AZURE_CLIENT_ID": "",
			"IDENTITY_ENDPOINT": f.URL + "/arc", "IMDS_ENDPOINT": f.URL} {
			t.Setenv(k, v)
		}
		read(t, f, "managed-identity (arc)")
		if f.IdentityGets() != 2 {
			t.Fatalf("want the challenge and the answer, got %d", f.IdentityGets())
		}
		// A challenge naming a file outside the agent's directory is refused.
		fakeazure.ArcRealm = filepath.Join(filepath.Dir(dir), "elsewhere.key")
		c := New()
		if _, err := c.Resolve(context.Background(), kvRefs()); err == nil || !strings.Contains(err.Error(), "unexpected key file") {
			t.Fatalf("arc realm outside the token directory: %v", err)
		}
	})
	t.Run("azure-cli", func(t *testing.T) {
		f, _ := kvEnv(t)
		bin := os.Getenv("PATH")
		calls := fakeazure.FakeAz(t, bin)
		t.Setenv("PATH", bin+string(os.PathListSeparator)+"/usr/bin"+string(os.PathListSeparator)+"/bin")
		for k, v := range map[string]string{"AZURE_CLIENT_SECRET": "", "AZURE_CLIENT_ID": "", "HEV_AZKV_CREDENTIAL": "azure-cli"} {
			t.Setenv(k, v)
		}
		read(t, f, "azure-cli (convenience")
		b, _ := os.ReadFile(calls)
		if !strings.HasPrefix(string(b), "account get-access-token --scope https://vault.azure.net/.default --output json") || f.TokenPosts() != 0 {
			t.Fatalf("az arguments: %q", b)
		}
	})
}

func TestCredentialReferenceCycleRefused(t *testing.T) {
	f, _ := kvEnv(t)
	t.Setenv("AZURE_CLIENT_SECRET", "")
	t.Setenv("AZURE_CLIENT_SECRET_REF", "azkv://other-vault/principal-secret")
	_, err := New().Resolve(context.Background(), kvRefs())
	if err == nil || !strings.Contains(err.Error(), "cycle") || f.Requests() != 0 {
		t.Fatalf("an azkv principal secret must be refused as a cycle: %v", err)
	}
	// The run-time guard refuses any loop the static check cannot see.
	c := New()
	ctx := context.WithValue(context.Background(), credChainKey{}, []string{"op"})
	if _, err := c.credentialValue(ctx, "azkv", "X", "op://example/a/b", time.Minute); !errors.Is(err, errCredentialCycle) {
		t.Fatalf("op -> azkv -> op: %v", err)
	}
}

// With nothing configured no interactive flow is attempted: no request to
// any endpoint, no az run, and the error names what to set.
func TestKeyVaultNoCredentialNeverPrompts(t *testing.T) {
	f, _ := kvEnv(t)
	bin := os.Getenv("PATH")
	calls := filepath.Join(filepath.Dir(bin), "az-calls")
	if runtime.GOOS != "windows" {
		calls = fakeazure.FakeAz(t, bin) // an az login present must not be used unasked
	}
	for _, k := range []string{"AZURE_TENANT_ID", "AZURE_CLIENT_ID", "AZURE_CLIENT_SECRET"} {
		t.Setenv(k, "")
	}
	_, err := New().Resolve(context.Background(), kvRefs())
	if err == nil {
		t.Fatal("resolved with no credential")
	}
	for _, want := range []string{"AZURE_TENANT_ID", "AZURE_CLIENT_ID", "AZURE_CLIENT_SECRET", "AZURE_CLIENT_CERTIFICATE_PATH", "AZURE_FEDERATED_TOKEN_FILE", "HEV_AZKV_CREDENTIAL=managed-identity", "HEV_AZKV_CREDENTIAL=azure-cli", "interactive sign-in is never attempted"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not name %s: %v", want, err)
		}
	}
	if _, e := os.Stat(calls); e == nil || f.Requests() != 0 {
		t.Fatal("a request or az run was attempted with no credential")
	}
	t.Setenv("HEV_AZKV_CREDENTIAL", "device-code")
	if _, err := New().Resolve(context.Background(), kvRefs()); err == nil || !strings.Contains(err.Error(), "must be one of") {
		t.Fatalf("an unknown credential type must be refused: %v", err)
	}
}

// The access token and the principal's secret never reach disk, the read log,
// an error string, status or a process argument, even when the vault and the
// sign-in endpoint echo them back.
func TestKeyVaultTokenAndPrincipalSecretNeverLeak(t *testing.T) {
	f, dir := kvEnv(t)
	t.Setenv("HEV_SECRETS_COOLDOWN", filepath.Join(dir, "op-cooldown.json"))
	sentinels := []string{fakeazure.AccessToken, fakeazure.ClientSecret, fakeazure.Value}
	var errs []string
	c := New()
	if _, err := c.Resolve(context.Background(), kvRefs()); err != nil {
		t.Fatal(err)
	}
	f.Forbid(true) // 403 whose body echoes the bearer token and a value
	if _, err := c.Resolve(context.Background(), []Ref{{"X", fakeazure.Ref() + "/0123456789abcdef0123456789abcdef", 0}}); err == nil {
		t.Fatal("403 accepted")
	} else {
		errs = append(errs, err.Error())
	}
	f.Forbid(false)
	t.Setenv("AZURE_CLIENT_SECRET", "wrong-"+fakeazure.ClientSecret) // sign-in echoes the secret
	if _, err := New().Resolve(context.Background(), kvRefs()); err == nil {
		t.Fatal("wrong secret accepted")
	} else {
		errs = append(errs, err.Error())
	}
	t.Setenv("AZURE_CLIENT_SECRET", fakeazure.ClientSecret)
	f.Throttle("5") // 429 whose body carries a value
	if _, err := c.Resolve(context.Background(), []Ref{{"Y", "azkv://example-vault/other", 0}}); err == nil {
		t.Fatal("429 accepted")
	} else {
		errs = append(errs, err.Error())
	}
	f.Unthrottle()
	st, _ := json.Marshal(c.LocalStatus())
	errs = append(errs, string(st))
	for _, s := range sentinels {
		for _, e := range errs {
			if strings.Contains(e, s) {
				t.Fatalf("leak in error or status: %q", e)
			}
		}
		filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				if b, _ := os.ReadFile(p); bytes.Contains(b, []byte(s)) {
					t.Fatalf("leak on disk: %s", p)
				}
			}
			return nil
		})
	}
	log, _ := os.ReadFile(filepath.Join(dir, "reads.log"))
	if strings.Count(string(log), "provider=azkv") != 4 || !strings.Contains(string(log), "outcome=rate_limited") {
		t.Fatalf("read log: %q", log)
	}
	// The cooldown file exists and carries no secret (checked by the walk).
	if _, err := os.Stat(filepath.Join(dir, "azkv-cooldown.json")); err != nil {
		t.Fatal("no Key Vault cooldown file")
	}
	// Process arguments: the only subprocess Key Vault ever runs is az, with
	// the scope and tenant only.
	if runtime.GOOS != "windows" {
		bin := os.Getenv("PATH")
		calls := fakeazure.FakeAz(t, bin)
		t.Setenv("PATH", bin+string(os.PathListSeparator)+"/usr/bin"+string(os.PathListSeparator)+"/bin")
		t.Setenv("HEV_AZKV_CREDENTIAL", "azure-cli")
		t.Setenv("HEV_SECRETS_COOLDOWN", "off") // past the 429 above
		if _, err := New().Resolve(context.Background(), kvRefs()); err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(calls)
		for _, s := range sentinels {
			if strings.Contains(string(b), s) {
				t.Fatalf("leak in az arguments: %q", b)
			}
		}
	}
}

// Malformed Key Vault references are refused with the same strictness as
// malformed 1Password ones (control characters, template braces, an empty
// reference), plus Key Vault's own name rules.
func TestMalformedReferencesRefused(t *testing.T) {
	kvEnv(t)
	bad := []string{
		// refused for either scheme
		"", "op:/", "azkv:/", "op://example/{{x}}/field", "azkv://example-vault/{{x}}", "op://example/a\nb", "azkv://example-vault/a\nb",
		"op://example/a\x00", "azkv://example-vault/a\x00", "OP://example/a/b", "AZKV://example-vault/a", "gcpsm://project/secret",
		// Key Vault structure
		"azkv://", "azkv://example-vault", "azkv://example-vault/", "azkv://example-vault//", "azkv://example-vault/a/",
		"azkv://example-vault/a/b/c", "azkv:///db-password", "azkv://example-vault/a/not-a-version",
		"azkv://example-vault/a/0123456789abcdef0123456789abcde", "azkv://example-vault/a?api-version=1",
		"azkv://example-vault/a#frag", "azkv://example-vault/a%2Fb", "azkv://user@example-vault/a", "azkv://example-vault:443/a",
		"azkv://example-vault/a b", "azkv://example-vault/a_b", "azkv://example-vault/a.b", "azkv://example-vault/" + strings.Repeat("a", 128),
		// vault names
		"azkv://ab/a", "azkv://" + strings.Repeat("a", 25) + "/a", "azkv://1vault/a", "azkv://vault-/a", "azkv://va--ult/a", "azkv://va_ult/a",
		// hosts: only known Key Vault suffixes
		"azkv://example-vault.example.com/a", "azkv://example-vault.vault.azure.net.example.com/a", "azkv://example-vault.vault.azure.net./a",
		"azkv://127.0.0.1/a", "azkv://example-vault.vault.example.internal/a",
	}
	for _, uri := range bad {
		if err := Validate([]Ref{{"X", uri, time.Minute}}); err == nil {
			t.Errorf("accepted %q", uri)
		}
	}
	good := []string{"azkv://example-vault/db-password", "azkv://Example-Vault/DB-Password-2",
		"azkv://example-vault/db-password/0123456789abcdef0123456789ABCDEF", "azkv://abc/a",
		"azkv://example-vault.vault.azure.net/a", "azkv://example-vault.vault.usgovcloudapi.net/a", "azkv://example-vault.vault.azure.cn/a"}
	for _, uri := range good {
		if err := Validate([]Ref{{"X", uri, time.Minute}}); err != nil {
			t.Errorf("refused %q", uri)
		}
	}
	// An Azure Stack Hub style suffix is accepted only when listed.
	stack := "azkv://example-vault.vault.region.example.internal/a"
	if Validate([]Ref{{"X", stack, time.Minute}}) == nil {
		t.Fatal("unlisted suffix accepted")
	}
	t.Setenv("HEV_AZKV_DNS_SUFFIXES", "vault.region.example.internal")
	if err := Validate([]Ref{{"X", stack, time.Minute}}); err != nil {
		t.Fatal("listed suffix refused")
	}
}

// Sovereign clouds: the audience and sign-in host follow the vault's suffix.
func TestKeyVaultCloudFollowsSuffix(t *testing.T) {
	t.Setenv("AZURE_AUTHORITY_HOST", "")
	r, ok := parseKV("azkv://example-vault.vault.usgovcloudapi.net/a")
	if !ok || r.resource() != "https://vault.usgovcloudapi.net" || r.host != "example-vault.vault.usgovcloudapi.net" {
		t.Fatalf("%+v", r)
	}
	if a, err := authority(r); err != nil || a != "https://login.microsoftonline.us" {
		t.Fatalf("authority %q %v", a, err)
	}
	t.Setenv("AZURE_AUTHORITY_HOST", "http://login.example.com")
	if _, err := authority(r); err == nil {
		t.Fatal("clear-text authority accepted")
	}
	t.Setenv("HEV_AZKV_TEST_VAULT_URL", "http://vault.example.com")
	if vaultBase(r) != "https://example-vault.vault.usgovcloudapi.net" {
		t.Fatal("non-loopback test vault URL honoured")
	}
}

// shortEndpoint is a private socket or pipe name short enough for sockaddr_un.
func shortEndpoint(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		return `\\.\pipe\hev-secrets-test-` + strings.ReplaceAll(t.Name(), "/", "-")
	}
	dir, err := os.MkdirTemp("/tmp", "kvs")
	if err != nil {
		t.Fatal(err)
	}
	os.Chmod(dir, 0o700)
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "agent.sock")
}

// The fallback notice names the stores a direct read asks, keeps the old
// 1Password text byte for byte, and uses each provider's own name.
func TestFallbackNoticeNamesTheStores(t *testing.T) {
	op := Ref{"A", "op://vault/item/field", 0}
	kv := Ref{"B", "azkv://example-vault/secret", 0}
	for _, c := range []struct {
		refs []Ref
		want string
	}{
		{[]Ref{op}, FallbackNotice},
		{[]Ref{op}, "hev secrets: notice: fallback to direct 1Password read"},
		{[]Ref{kv}, "hev secrets: notice: fallback to direct Azure Key Vault read"},
		{[]Ref{kv, op, kv}, "hev secrets: notice: fallback to direct 1Password and Azure Key Vault read"},
	} {
		if got := fallbackNotice(c.refs); got != c.want {
			t.Errorf("fallbackNotice(%v) = %q, want %q", c.refs, got, c.want)
		}
	}
	c := New()
	for _, s := range registry {
		if p := s.build(c); p.Name() != s.name {
			t.Errorf("provider %s: spec name %q, Name() %q", s.scheme, s.name, p.Name())
		}
	}
}
