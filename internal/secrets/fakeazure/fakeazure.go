// Package fakeazure runs a fake Azure Key Vault, a fake Microsoft identity
// token endpoint and fake managed-identity endpoints on one loopback HTTP
// server for tests. It never contacts Azure: every answer is made up, and
// every request is counted so a test can assert exactly how many were made.
package fakeazure

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// Example identifiers and sentinels. None is a real tenant, client, vault or
// credential; each sentinel is a string a leak check can search for.
const (
	Tenant         = "00000000-0000-0000-0000-00000000000a"
	Client         = "00000000-0000-0000-0000-00000000000c"
	ClientSecret   = "fake-sp-client-secret-never-log-this"
	AccessToken    = "fake-kv-access-token-never-log-this"
	FederatedToken = "fake-federated-token-never-log-this"
	IdentityHeader = "fake-identity-header"
	ArcKey         = "fake-arc-key-never-log-this"
	Value          = "fake-kv-secret-value-never-log-this"
	Vault          = "example-vault"
	Secret         = "db-password"
)

// Fake is a running fake. Its counters are safe for concurrent use.
type Fake struct {
	URL string
	srv *httptest.Server

	mu         sync.Mutex
	vaultGets  int
	tokenPosts int
	miGets     int
	// Throttle answers vault GETs with 429 and this Retry-After value while
	// set ("" = no header); ThrottleOn turns it on.
	throttleOn bool
	retryAfter string
	// Forbid answers vault GETs with 403 and a body echoing the request's
	// authorization header (a leak trap).
	forbid bool
	// CertKey verifies certificate assertions when set.
	certKey *rsa.PublicKey
	paths   []string
}

// Start runs a fake until the test ends.
func Start(t testing.TB) *Fake {
	t.Helper()
	f := &Fake{}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	f.URL = f.srv.URL
	return f
}

// Env points kit's Azure provider at this fake with a client-secret service
// principal, clearing every other Azure and 1Password setting. It sets the
// variables with t.Setenv and returns them as KEY=VALUE for a child process.
func (f *Fake) Env(t testing.TB) []string {
	t.Helper()
	vars := map[string]string{
		"AZURE_AUTHORITY_HOST":    f.URL,
		"HEV_AZKV_TEST_VAULT_URL": f.URL + "/vault",
		"AZURE_TENANT_ID":         Tenant,
		"AZURE_CLIENT_ID":         Client,
		"AZURE_CLIENT_SECRET":     ClientSecret,
	}
	for _, k := range []string{"AZURE_CLIENT_SECRET_REF", "AZURE_CLIENT_CERTIFICATE_PATH", "AZURE_FEDERATED_TOKEN_FILE",
		"IDENTITY_ENDPOINT", "IDENTITY_HEADER", "IMDS_ENDPOINT", "HEV_AZKV_CREDENTIAL", "HEV_AZKV_TEST_IMDS_URL",
		"HEV_AZKV_DNS_SUFFIXES", "AZURE_CONFIG_DIR"} {
		vars[k] = ""
	}
	out := []string{}
	for k, v := range vars {
		t.Setenv(k, v)
		out = append(out, k+"="+v)
	}
	return out
}

// Ref is an example Key Vault reference served by the fake.
func Ref() string { return "azkv://" + Vault + "/" + Secret }

func (f *Fake) count(p *int) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return *p
}

// VaultGets is how many secret reads reached the fake vault.
func (f *Fake) VaultGets() int { return f.count(&f.vaultGets) }

// TokenPosts is how many token requests reached the fake sign-in endpoint.
func (f *Fake) TokenPosts() int { return f.count(&f.tokenPosts) }

// IdentityGets is how many requests reached the managed-identity endpoints.
func (f *Fake) IdentityGets() int { return f.count(&f.miGets) }

// Requests is every request the fake has seen.
func (f *Fake) Requests() int { return f.VaultGets() + f.TokenPosts() + f.IdentityGets() }

// Throttle makes the vault answer 429 (with Retry-After when non-empty) until
// Unthrottle.
func (f *Fake) Throttle(retryAfter string) {
	f.mu.Lock()
	f.throttleOn, f.retryAfter = true, retryAfter
	f.mu.Unlock()
}

func (f *Fake) Unthrottle() { f.mu.Lock(); f.throttleOn = false; f.mu.Unlock() }

// Forbid makes the vault answer 403 with a body that echoes the bearer token.
func (f *Fake) Forbid(on bool) { f.mu.Lock(); f.forbid = on; f.mu.Unlock() }

// TrustCertificate makes the token endpoint accept assertions signed by key.
func (f *Fake) TrustCertificate(key *rsa.PublicKey) { f.mu.Lock(); f.certKey = key; f.mu.Unlock() }

// ArcKeyFile writes the Arc challenge key into dir and returns its path.
func ArcKeyFile(t testing.TB, dir string) string {
	t.Helper()
	p := filepath.Join(dir, "example.key")
	if err := os.WriteFile(p, []byte(ArcKey), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// ArcRealm is the key file the Arc endpoint names in its challenge.
var ArcRealm string

func token(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"access_token": AccessToken, "token_type": "Bearer", "expires_in": 3600})
}

func (f *Fake) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.paths = append(f.paths, r.URL.Path)
	f.mu.Unlock()
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/"+Tenant+"/oauth2/v2.0/token":
		f.mu.Lock()
		f.tokenPosts++
		key := f.certKey
		f.mu.Unlock()
		r.ParseForm()
		ok := r.PostForm.Get("client_id") == Client && r.PostForm.Get("grant_type") == "client_credentials" &&
			strings.HasSuffix(r.PostForm.Get("scope"), "/.default")
		switch r.PostForm.Get("client_assertion_type") {
		case "":
			ok = ok && r.PostForm.Get("client_secret") == ClientSecret
		default:
			a := r.PostForm.Get("client_assertion")
			ok = ok && (a == FederatedToken || (key != nil && verifyAssertion(a, key, f.URL+"/"+Tenant+"/oauth2/v2.0/token")))
		}
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"error":"invalid_client","error_description":"echo %s %s"}`, r.PostForm.Get("client_secret"), r.PostForm.Get("client_assertion"))
			return
		}
		token(w)
	case r.URL.Path == "/imds" || r.URL.Path == "/appservice" || r.URL.Path == "/arc":
		f.mu.Lock()
		f.miGets++
		f.mu.Unlock()
		if r.URL.Query().Get("resource") != "https://vault.azure.net" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		switch r.URL.Path {
		case "/imds":
			if r.Header.Get("Metadata") != "true" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		case "/appservice":
			if r.Header.Get("X-IDENTITY-HEADER") != IdentityHeader {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		case "/arc":
			if r.Header.Get("Authorization") != "Basic "+ArcKey {
				w.Header().Set("WWW-Authenticate", "Basic realm="+ArcRealm)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"access_token": AccessToken, "expires_on": fmt.Sprint(time.Now().Add(time.Hour).Unix()), "resource": "https://vault.azure.net"})
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/vault/"):
		f.mu.Lock()
		f.vaultGets++
		throttle, after, forbid := f.throttleOn, f.retryAfter, f.forbid
		f.mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer "+AccessToken || r.URL.Query().Get("api-version") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if throttle {
			if after != "" {
				w.Header().Set("Retry-After", after)
			}
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"error":{"code":"Throttled","message":"Too many requests. `+Value+`"}}`)
			return
		}
		if forbid {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprintf(w, `{"error":{"code":"Forbidden","message":"caller %s %s"}}`, r.Header.Get("Authorization"), Value)
			return
		}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/vault/"), "/")
		// HOST/secrets/NAME[/VERSION]
		if len(parts) < 3 || parts[1] != "secrets" || !strings.HasPrefix(parts[0], Vault+".") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"value": Value, "id": "https://" + parts[0] + "/secrets/" + parts[2] + "/0123456789abcdef0123456789abcdef"})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func verifyAssertion(a string, key *rsa.PublicKey, aud string) bool {
	parts := strings.Split(a, ".")
	if len(parts) != 3 {
		return false
	}
	enc := base64.RawURLEncoding
	sig, err := enc.DecodeString(parts[2])
	if err != nil {
		return false
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(key, crypto.SHA256, sum[:], sig) != nil {
		return false
	}
	var claims struct {
		Aud string `json:"aud"`
		Iss string `json:"iss"`
		Exp int64  `json:"exp"`
	}
	b, _ := enc.DecodeString(parts[1])
	return json.Unmarshal(b, &claims) == nil && claims.Aud == aud && claims.Iss == Client && claims.Exp > time.Now().Unix()
}

// FakeAz installs a fake `az` that prints a token for `az account
// get-access-token` and records its arguments (never stdin or a token), one
// line per run. It skips on Windows, which has no sh.
func FakeAz(t testing.TB, bin string) (calls string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake az is a shell script")
	}
	calls = filepath.Join(filepath.Dir(bin), "az-calls")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "` + calls + `"
[ "$1 $2" = "account get-access-token" ] || exit 2
printf '{"accessToken":"` + AccessToken + `","expires_on":%s,"tokenType":"Bearer"}\n' "$(( $(date +%s) + 3600 ))"
`
	if err := os.WriteFile(filepath.Join(bin, "az"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return calls
}

// CertificatePEM is a certificate and its RSA key as one PEM file's contents.
func CertificatePEM(t testing.TB, der []byte, key *rsa.PrivateKey) []byte {
	t.Helper()
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return append(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})...)
}
