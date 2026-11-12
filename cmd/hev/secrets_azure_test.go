//go:build !windows

package main

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/secrets/fakeazure"
	"github.com/hev/kit/internal/secrets/fakeop"
)

// withAzure points the host at a fake Key Vault and sign-in endpoint with a
// client-secret service principal, for the in-process daemon and every hev
// the scripts run.
func (h *shellHost) withAzure(t *testing.T) *fakeazure.Fake {
	t.Helper()
	f := fakeazure.Start(t)
	h.env = append(h.env, f.Env(t)...)
	return f
}

// noKeyVaultSecretAnywhere fails if a Key Vault value, the access token or
// the principal's secret is in any file under the host root (the read log,
// the cooldown file, the argv log included).
func (h *shellHost) noKeyVaultSecretAnywhere(t *testing.T) {
	t.Helper()
	for _, s := range []string{fakeazure.Value, fakeazure.AccessToken, fakeazure.ClientSecret} {
		filepath.WalkDir(h.root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || d.Type()&fs.ModeSymlink != 0 || d.Type()&fs.ModeSocket != 0 {
				return nil
			}
			if b, _ := os.ReadFile(p); bytes.Contains(b, []byte(s)) {
				t.Fatalf("Key Vault secret material on disk: %s", p)
			}
			return nil
		})
	}
}

// One 1Password and one Key Vault reference in a single `hev secrets run`:
// both arrive in the child's environment, neither in an argument, file or
// log, and a second run inside the TTL makes no request to either store.
func TestSecretsRunMixesOnePasswordAndKeyVault(t *testing.T) {
	h := newShellHost(t, true)
	f := h.withAzure(t)
	h.serve(t)
	h.env = append(h.env, "WANT_A="+fakeop.Value, "WANT_B="+fakeazure.Value)
	script := `hev secrets run --ref A=op://example/db/password --ref B=` + fakeazure.Ref() + ` --ttl B=1h -- sh -c 'test "$A" = "$WANT_A" && test "$B" = "$WANT_B" && echo both-delivered'`
	for i := 0; i < 2; i++ {
		out, errs, err := h.sh(t, "sh", script)
		if err != nil || strings.TrimSpace(out) != "both-delivered" {
			t.Fatalf("run %d: %v %q %q", i+1, err, out, errs)
		}
		if strings.Contains(errs, fakeop.Value) || strings.Contains(errs, fakeazure.Value) {
			t.Fatal("value on stderr")
		}
	}
	if h.op.Count(t) != 1 || f.VaultGets() != 1 || f.TokenPosts() != 1 {
		t.Fatalf("want one request per store: op %d, vault %d, token %d", h.op.Count(t), f.VaultGets(), f.TokenPosts())
	}
	argv, _ := os.ReadFile(h.argv)
	if !strings.Contains(string(argv), "B="+fakeazure.Ref()) || strings.Contains(string(argv), fakeazure.Value) {
		t.Fatalf("hev arguments: %q", argv)
	}
	h.noValueAnywhere(t)
	h.noKeyVaultSecretAnywhere(t)
	log, _ := os.ReadFile(h.log)
	if !strings.Contains(string(log), "labels=A#") || !strings.Contains(string(log), "provider=op") ||
		!strings.Contains(string(log), "labels=B#") || !strings.Contains(string(log), "provider=azkv") || strings.Contains(string(log), "azkv://") {
		t.Fatalf("read log: %q", log)
	}
}

// A Key Vault-only host: no op anywhere on PATH, no 1Password token or
// token file. read, env and run work through the daemon and directly.
func TestSecretsKeyVaultOnlyHostCLI(t *testing.T) {
	h := newShellHost(t, true)
	f := h.withAzure(t)
	hevBin := filepath.Join(h.root, "hevbin")
	clean := []string{}
	for _, kv := range h.env {
		k, _, _ := strings.Cut(kv, "=")
		if k != "PATH" && k != "OP_SERVICE_ACCOUNT_TOKEN" {
			clean = append(clean, kv)
		}
	}
	h.env = append(clean, "PATH="+hevBin, "OP_SERVICE_ACCOUNT_TOKEN=")
	t.Setenv("PATH", hevBin)
	t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "")
	if p, err := exec.LookPath("op"); err == nil {
		t.Fatalf("op on PATH: %s", p)
	}
	sh := "/bin/sh"
	h.env = append(h.env, "WANT="+fakeazure.Value)
	out, errs, err := h.sh(t, sh, `x=$(hev secrets read --label DB --direct-status `+fakeazure.Ref()+`); s=$?; [ "$x" = "$WANT" ] && echo "direct:$s"`)
	if err != nil || strings.TrimSpace(out) != "direct:78" {
		t.Fatalf("direct read with no op: %v %q %q", err, out, errs)
	}
	h.serve(t)
	for i := 0; i < 2; i++ {
		out, errs, err = h.sh(t, sh, `hev secrets run --ref DB=`+fakeazure.Ref()+` -- /bin/sh -c '[ "$DB" = "$WANT" ] && echo ok'`)
		if err != nil || strings.TrimSpace(out) != "ok" || strings.Contains(errs, "fallback") {
			t.Fatalf("daemon run %d: %v %q %q", i, err, out, errs)
		}
	}
	if f.VaultGets() != 2 { // one direct read, then one daemon read for two runs
		t.Fatalf("vault reads: %d", f.VaultGets())
	}
	if h.op.Count(t) != 0 {
		t.Fatal("op ran on a Key Vault-only host")
	}
	h.noKeyVaultSecretAnywhere(t)
}

// Status for a two-provider host, from fakes: each provider's credential type,
// reachability, counters and cooldown, and Key Vault's missing quota said
// plainly. Run with -v to see the output.
func TestSecretsStatusTwoProviders(t *testing.T) {
	h := newShellHost(t, true)
	f := h.withAzure(t)
	h.serve(t)
	if _, errs, err := h.sh(t, "sh", `hev secrets env --ref A=op://example/db/password --ref B=`+fakeazure.Ref()+` >/dev/null && hev secrets env --ref A=op://example/db/password --ref B=`+fakeazure.Ref()+` >/dev/null`); err != nil {
		t.Fatalf("env: %v %q", err, errs)
	}
	f.Throttle("600")
	if _, _, err := h.sh(t, "sh", `hev secrets read --label OTHER azkv://example-vault/other-secret`); err == nil {
		t.Fatal("throttled read succeeded")
	}
	out, errs, err := h.sh(t, "sh", `hev secrets status`)
	if err != nil {
		t.Fatalf("status: %v %q", err, errs)
	}
	t.Logf("hev secrets status (two providers, fakes):\n%s", out)
	for _, want := range []string{"providers:", "1Password", "op://", "service-account token (environment)", "Azure Key Vault", "azkv://", "client-secret",
		"rate limit until", "Azure Key Vault quota: none: Key Vault has no quota endpoint", "account quota:"} {
		if !strings.Contains(out, want) {
			t.Errorf("status lacks %q", want)
		}
	}
	for _, s := range []string{fakeazure.Value, fakeazure.AccessToken, fakeazure.ClientSecret, fakeop.Value, fakeazure.Tenant, fakeazure.Client} {
		if strings.Contains(out, s) {
			t.Fatalf("status leaks %q", s)
		}
	}
	h.noKeyVaultSecretAnywhere(t)
}
