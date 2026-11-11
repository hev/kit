package secrets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Azure Key Vault references:
//
//	azkv://VAULT/SECRET            latest version, https://VAULT.vault.azure.net
//	azkv://VAULT/SECRET/VERSION    a pinned version (32 hex digits)
//	azkv://VAULT.vault.usgovcloudapi.net/SECRET[/VERSION]
//
// A bare VAULT is a vault in Azure's public cloud. A sovereign cloud's vault
// is written with its full host name, VAULT.SUFFIX, where SUFFIX is a known
// Key Vault DNS suffix or one listed in HEV_AZKV_DNS_SUFFIXES (Azure Stack
// Hub, for example). A private endpoint keeps the vault's ordinary name (the
// private DNS zone resolves it), so it is written like any other vault. The
// token audience follows the suffix (https://SUFFIX/.default), and so does
// the default sign-in host unless AZURE_AUTHORITY_HOST is set.
//
// Names follow Key Vault's own rules: a vault name is 3-24 letters, digits
// and single hyphens, starting with a letter and ending with a letter or
// digit; a secret name is 1-127 letters, digits and hyphens. Anything else
// (userinfo, a port, a query, a fragment, percent-encoding, an empty or extra
// path segment, a trailing slash) is refused.

const azkvScheme = "azkv"

// kvDefaultSuffix is Azure's public-cloud Key Vault DNS suffix.
const kvDefaultSuffix = "vault.azure.net"

// kvSuffixes maps every known Key Vault DNS suffix to its cloud's sign-in host.
var kvSuffixes = map[string]string{
	"vault.azure.net":         "https://login.microsoftonline.com",
	"vault.azure.cn":          "https://login.chinacloudapi.cn",
	"vault.usgovcloudapi.net": "https://login.microsoftonline.us",
	"vault.microsoftazure.de": "https://login.microsoftonline.de",
}

var (
	kvVaultName  = regexp.MustCompile(`^[A-Za-z](?:-?[A-Za-z0-9])+$`)
	kvSecretName = regexp.MustCompile(`^[A-Za-z0-9-]{1,127}$`)
	kvVersion    = regexp.MustCompile(`^[0-9A-Fa-f]{32}$`)
	kvSuffixForm = regexp.MustCompile(`^vault(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`)
)

type kvRef struct {
	host    string // VAULT.SUFFIX, lower case
	suffix  string
	name    string
	version string
}

// extraSuffixes reads HEV_AZKV_DNS_SUFFIXES (comma separated). Each must look
// like a Key Vault suffix ("vault." followed by DNS labels).
func extraSuffixes() []string {
	var out []string
	for _, s := range strings.Split(os.Getenv("HEV_AZKV_DNS_SUFFIXES"), ",") {
		s = strings.ToLower(strings.TrimSpace(s))
		if s != "" && len(s) <= 200 && kvSuffixForm.MatchString(s) {
			out = append(out, s)
		}
	}
	return out
}

func knownSuffix(s string) bool {
	if _, ok := kvSuffixes[s]; ok {
		return true
	}
	for _, x := range extraSuffixes() {
		if x == s {
			return true
		}
	}
	return false
}

func parseKV(uri string) (kvRef, bool) {
	rest, ok := strings.CutPrefix(uri, azkvScheme+"://")
	if !ok || len(uri) > 512 {
		return kvRef{}, false
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 2 && len(parts) != 3 {
		return kvRef{}, false
	}
	vault, suffix, dotted := strings.Cut(parts[0], ".")
	if !dotted {
		suffix = kvDefaultSuffix
	}
	suffix = strings.ToLower(suffix)
	if len(vault) < 3 || len(vault) > 24 || !kvVaultName.MatchString(vault) || !knownSuffix(suffix) {
		return kvRef{}, false
	}
	r := kvRef{host: strings.ToLower(vault) + "." + suffix, suffix: suffix, name: parts[1]}
	if !kvSecretName.MatchString(r.name) {
		return kvRef{}, false
	}
	if len(parts) == 3 {
		if !kvVersion.MatchString(parts[2]) {
			return kvRef{}, false
		}
		r.version = strings.ToLower(parts[2])
	}
	return r, true
}

// resource is the token audience for this vault's cloud.
func (r kvRef) resource() string { return "https://" + r.suffix }

func init() {
	register(providerSpec{
		order:    10,
		scheme:   azkvScheme,
		validate: func(uri string) bool { _, ok := parseKV(uri); return ok },
		build:    func(c *Cache) Provider { return newAzureKV(c) },
	})
}

// azureKV reads Key Vault secrets over the REST API with net/http: one GET per
// unique reference, one attempt each, stopping at the first failure. Access
// tokens are kept in this process's memory only, per audience, until two
// minutes before they expire.
type azureKV struct {
	c      *Cache
	client *http.Client

	// fetch serializes token requests; mu guards the fields below it and is
	// never held across a request, so status never waits on sign-in.
	fetch     sync.Mutex
	mu        sync.Mutex
	tokens    map[string]accessToken
	tokenReqs int
}

type accessToken struct {
	value   string
	expires time.Time
}

func newAzureKV(c *Cache) *azureKV {
	return &azureKV{c: c, tokens: map[string]accessToken{}, client: &http.Client{
		Timeout: 30 * time.Second,
		// Never follow a redirect: it would carry the bearer token elsewhere.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

func (*azureKV) Scheme() string { return azkvScheme }
func (*azureKV) Name() string   { return "Azure Key Vault" }
func (*azureKV) Kind() string   { return "get" }
func (*azureKV) quotaNote() string {
	return "none: Key Vault has no quota endpoint; throttling arrives per request as HTTP 429"
}

func (p *azureKV) tokenRequests() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.tokenReqs
}

func (p *azureKV) Credential() (string, error) {
	cr, err := azureCredential()
	if err != nil {
		return "", err
	}
	return cr.label(), nil
}

func (p *azureKV) Fingerprint() string {
	cr, err := azureCredential()
	if err != nil {
		return ""
	}
	return cr.fingerprint()
}

// loopbackURL accepts an http(s) URL whose host is a loopback address. Test
// and local identity endpoints must be loopback: kit never sends a credential
// or bearer token in clear text off the machine.
func loopbackURL(raw string) (*url.URL, bool) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Host == "" {
		return nil, false
	}
	h := u.Hostname()
	if h == "localhost" {
		return u, true
	}
	ip := net.ParseIP(h)
	return u, ip != nil && ip.IsLoopback()
}

// vaultBase is https://VAULT.SUFFIX. HEV_AZKV_TEST_VAULT_URL, honoured only
// for a loopback URL, sends every vault to a local fake at URL/VAULT.SUFFIX.
func vaultBase(r kvRef) string {
	if v := os.Getenv("HEV_AZKV_TEST_VAULT_URL"); v != "" {
		if u, ok := loopbackURL(v); ok {
			return strings.TrimRight(u.String(), "/") + "/" + r.host
		}
	}
	return "https://" + r.host
}

// kvAPIVersion is the Key Vault data-plane API version kit speaks.
const kvAPIVersion = "7.4"

func (p *azureKV) Read(ctx context.Context, uris []string) ([]string, int, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cr, err := azureCredential()
	if err != nil {
		return nil, 0, err
	}
	out := make([]string, len(uris))
	n := 0
	for i, u := range uris {
		r, ok := parseKV(u)
		if !ok {
			return nil, n, errors.New("invalid secret reference")
		}
		token, err := p.token(ctx, cr, r)
		if err != nil {
			return nil, n, err
		}
		path := "/secrets/" + r.name
		if r.version != "" {
			path += "/" + r.version
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, vaultBase(r)+path+"?api-version="+kvAPIVersion, nil)
		if err != nil {
			return nil, n, errors.New("Azure Key Vault request could not be built")
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/json")
		n++
		resp, err := p.client.Do(req)
		if err != nil {
			return nil, n, errors.New("Azure Key Vault unreachable (network error)")
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusOK:
			var v struct {
				Value *string `json:"value"`
			}
			if json.Unmarshal(body, &v) != nil || v.Value == nil {
				return nil, n, errors.New("Azure Key Vault returned an invalid secret response")
			}
			out[i] = *v.Value
		case resp.StatusCode == http.StatusTooManyRequests:
			return nil, n, &RateLimit{Class: "rate limit", After: retryAfter(resp.Header.Get("Retry-After"), time.Now())}
		case resp.StatusCode == http.StatusUnauthorized:
			p.forget(r)
			return nil, n, fmt.Errorf("Azure Key Vault refused the %s credential (HTTP 401)", cr.kind)
		case resp.StatusCode == http.StatusForbidden:
			return nil, n, fmt.Errorf("Azure Key Vault denied access (HTTP 403): the %s principal needs get on secrets for this vault", cr.kind)
		case resp.StatusCode == http.StatusNotFound:
			return nil, n, errors.New("Azure Key Vault has no such secret or version (HTTP 404)")
		default:
			// Never return the body: an error page may echo the request.
			return nil, n, fmt.Errorf("Azure Key Vault request failed (HTTP %d)", resp.StatusCode)
		}
	}
	return out, n, nil
}

// retryAfter reads an HTTP Retry-After: delay seconds or an HTTP date. Zero
// means absent or unreadable; the cache then backs off on its own.
func retryAfter(v string, now time.Time) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if n, err := strconv.Atoi(v); err == nil {
		if n <= 0 {
			return 0
		}
		return time.Duration(min(n, 24*3600)) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil && t.After(now) {
		return t.Sub(now)
	}
	return 0
}

func (p *azureKV) forget(r kvRef) {
	p.mu.Lock()
	delete(p.tokens, r.resource())
	p.mu.Unlock()
}

// token returns a cached access token for the vault's audience, or asks the
// credential for a new one, one request at a time.
func (p *azureKV) token(ctx context.Context, cr azureCred, r kvRef) (string, error) {
	key := r.resource()
	cached := func() (string, bool) {
		p.mu.Lock()
		defer p.mu.Unlock()
		t, ok := p.tokens[key]
		return t.value, ok && time.Until(t.expires) > 2*time.Minute
	}
	if v, ok := cached(); ok {
		return v, nil
	}
	p.fetch.Lock()
	defer p.fetch.Unlock()
	if v, ok := cached(); ok {
		return v, nil
	}
	p.mu.Lock()
	p.tokenReqs++
	p.mu.Unlock()
	t, err := cr.get(ctx, p, r)
	if err != nil {
		return "", err
	}
	if t.value == "" {
		return "", errors.New("Azure sign-in returned no access token")
	}
	p.mu.Lock()
	p.tokens[key] = t
	p.mu.Unlock()
	return t.value, nil
}
