package secrets

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Azure credentials, non-interactive only. Selection, in order, unless
// HEV_AZKV_CREDENTIAL names one:
//
//	client-secret       AZURE_TENANT_ID, AZURE_CLIENT_ID and AZURE_CLIENT_SECRET,
//	                    or AZURE_CLIENT_SECRET_REF (a reference to another
//	                    provider, e.g. op://vault/item/field, read through the
//	                    cache; a reference back to azkv is a cycle and refused)
//	client-certificate  AZURE_TENANT_ID, AZURE_CLIENT_ID and
//	                    AZURE_CLIENT_CERTIFICATE_PATH (PEM: certificate and an
//	                    unencrypted RSA private key)
//	workload-identity   AZURE_TENANT_ID, AZURE_CLIENT_ID and AZURE_FEDERATED_TOKEN_FILE
//	managed-identity    automatic when IDENTITY_ENDPOINT is set (App Service,
//	                    Azure Arc); HEV_AZKV_CREDENTIAL=managed-identity for a
//	                    VM's instance metadata endpoint. AZURE_CLIENT_ID picks a
//	                    user-assigned identity.
//	azure-cli           only with HEV_AZKV_CREDENTIAL=azure-cli: an existing
//	                    `az login` on the host, a labelled convenience
//
// No browser, device-code or other prompt exists in kit. With nothing
// configured the error names what to set.

const (
	credClientSecret = "client-secret"
	credCertificate  = "client-certificate"
	credWorkload     = "workload-identity"
	credManaged      = "managed-identity"
	credAzureCLI     = "azure-cli"
)

// errNoAzureCredential names every way to configure Key Vault.
var errNoAzureCredential = errors.New("Azure Key Vault has no credential configured; set AZURE_TENANT_ID and AZURE_CLIENT_ID with one of AZURE_CLIENT_SECRET, AZURE_CLIENT_SECRET_REF, AZURE_CLIENT_CERTIFICATE_PATH or AZURE_FEDERATED_TOKEN_FILE, or set HEV_AZKV_CREDENTIAL=managed-identity or HEV_AZKV_CREDENTIAL=azure-cli (interactive sign-in is never attempted)")

var (
	azTenantForm = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{0,99}$`)
	azClientForm = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)
)

type azureCred struct {
	kind      string
	flavor    string // managed identity: imds, app-service, arc
	tenant    string
	client    string
	secretRef string
	certPath  string
	tokenFile string
	endpoint  string // managed identity endpoint
}

func (cr azureCred) label() string {
	switch {
	case cr.kind == credClientSecret && cr.secretRef != "":
		return credClientSecret + " (from a " + schemeOf(cr.secretRef) + " reference)"
	case cr.kind == credManaged:
		return credManaged + " (" + cr.flavor + ")"
	case cr.kind == credAzureCLI:
		return credAzureCLI + " (convenience: an existing az login)"
	}
	return cr.kind
}

// fingerprint identifies the credential configuration for the endpoint's
// account check. A client secret contributes only its SHA-256.
func (cr azureCred) fingerprint() string {
	secret := ""
	if cr.kind == credClientSecret && cr.secretRef == "" {
		secret = fingerprint(os.Getenv("AZURE_CLIENT_SECRET"))
	}
	cfg := ""
	if cr.kind == credAzureCLI {
		cfg = os.Getenv("AZURE_CONFIG_DIR")
	}
	return fingerprint(azkvScheme, cr.kind, cr.flavor, cr.tenant, cr.client, secret, cr.secretRef, cr.certPath, cr.tokenFile, cr.endpoint, cfg, os.Getenv("AZURE_AUTHORITY_HOST"))
}

func azureCredential() (azureCred, error) {
	cr := azureCred{tenant: os.Getenv("AZURE_TENANT_ID"), client: os.Getenv("AZURE_CLIENT_ID")}
	want := strings.TrimSpace(os.Getenv("HEV_AZKV_CREDENTIAL"))
	sp := cr.tenant != "" && cr.client != ""
	if want == "" || want == "auto" {
		switch {
		case sp && (os.Getenv("AZURE_CLIENT_SECRET") != "" || os.Getenv("AZURE_CLIENT_SECRET_REF") != ""):
			want = credClientSecret
		case sp && os.Getenv("AZURE_CLIENT_CERTIFICATE_PATH") != "":
			want = credCertificate
		case sp && os.Getenv("AZURE_FEDERATED_TOKEN_FILE") != "":
			want = credWorkload
		case os.Getenv("IDENTITY_ENDPOINT") != "":
			want = credManaged
		default:
			return cr, errNoAzureCredential
		}
	}
	cr.kind = want
	needSP := func(extra string) error {
		if !sp {
			return fmt.Errorf("Azure Key Vault %s credential needs AZURE_TENANT_ID, AZURE_CLIENT_ID and %s", want, extra)
		}
		if !azTenantForm.MatchString(cr.tenant) || !azClientForm.MatchString(cr.client) {
			return errors.New("Azure Key Vault: AZURE_TENANT_ID or AZURE_CLIENT_ID is malformed")
		}
		return nil
	}
	switch want {
	case credClientSecret:
		if err := needSP("AZURE_CLIENT_SECRET or AZURE_CLIENT_SECRET_REF"); err != nil {
			return cr, err
		}
		if os.Getenv("AZURE_CLIENT_SECRET") == "" {
			cr.secretRef = os.Getenv("AZURE_CLIENT_SECRET_REF")
			if cr.secretRef == "" {
				return cr, needSP("AZURE_CLIENT_SECRET or AZURE_CLIENT_SECRET_REF")
			}
			if schemeOf(cr.secretRef) == azkvScheme {
				return cr, errors.New("Azure Key Vault: AZURE_CLIENT_SECRET_REF cannot be an azkv:// reference (credential reference cycle); use another provider")
			}
			if !ValidRef(cr.secretRef) {
				return cr, errors.New("Azure Key Vault: AZURE_CLIENT_SECRET_REF is not a valid secret reference")
			}
		}
	case credCertificate:
		if err := needSP("AZURE_CLIENT_CERTIFICATE_PATH"); err != nil {
			return cr, err
		}
		cr.certPath = os.Getenv("AZURE_CLIENT_CERTIFICATE_PATH")
		if cr.certPath == "" {
			return cr, needSP("AZURE_CLIENT_CERTIFICATE_PATH")
		}
	case credWorkload:
		if err := needSP("AZURE_FEDERATED_TOKEN_FILE"); err != nil {
			return cr, err
		}
		cr.tokenFile = os.Getenv("AZURE_FEDERATED_TOKEN_FILE")
		if cr.tokenFile == "" {
			return cr, needSP("AZURE_FEDERATED_TOKEN_FILE")
		}
	case credManaged:
		if cr.client != "" && !azClientForm.MatchString(cr.client) {
			return cr, errors.New("Azure Key Vault: AZURE_CLIENT_ID is malformed")
		}
		ep := os.Getenv("IDENTITY_ENDPOINT")
		switch {
		case ep != "" && os.Getenv("IDENTITY_HEADER") != "":
			cr.flavor = "app-service"
		case ep != "" && os.Getenv("IMDS_ENDPOINT") != "":
			cr.flavor = "arc"
		case ep != "":
			return cr, errors.New("Azure Key Vault managed identity: IDENTITY_ENDPOINT needs IDENTITY_HEADER (App Service) or IMDS_ENDPOINT (Azure Arc)")
		default:
			cr.flavor = "imds"
			ep = imdsURL()
		}
		if _, ok := loopbackURL(ep); !ok && cr.flavor != "imds" {
			return cr, errors.New("Azure Key Vault managed identity: IDENTITY_ENDPOINT must be a local (loopback) address")
		}
		cr.endpoint = ep
	case credAzureCLI:
	default:
		return cr, fmt.Errorf("HEV_AZKV_CREDENTIAL must be one of auto, %s, %s, %s, %s or %s", credClientSecret, credCertificate, credWorkload, credManaged, credAzureCLI)
	}
	return cr, nil
}

// imdsURL is the instance metadata token endpoint. HEV_AZKV_TEST_IMDS_URL,
// honoured only for a loopback URL, points it at a local fake.
func imdsURL() string {
	if v := os.Getenv("HEV_AZKV_TEST_IMDS_URL"); v != "" {
		if u, ok := loopbackURL(v); ok {
			return u.String()
		}
	}
	return "http://169.254.169.254/metadata/identity/oauth2/token"
}

// authority is the sign-in host for the vault's cloud: AZURE_AUTHORITY_HOST
// when set (https, or http on loopback only), else the suffix's own.
func authority(r kvRef) (string, error) {
	if v := os.Getenv("AZURE_AUTHORITY_HOST"); v != "" {
		u, err := url.Parse(v)
		if _, loop := loopbackURL(v); err != nil || u.Host == "" || u.User != nil || !(u.Scheme == "https" || loop) {
			return "", errors.New("AZURE_AUTHORITY_HOST must be an https URL")
		}
		return strings.TrimRight(u.Scheme+"://"+u.Host+u.Path, "/"), nil
	}
	if a, ok := kvSuffixes[r.suffix]; ok {
		return a, nil
	}
	return "", errors.New("set AZURE_AUTHORITY_HOST for a vault outside Azure's known clouds")
}

func (cr azureCred) get(ctx context.Context, p *azureKV, r kvRef) (accessToken, error) {
	switch cr.kind {
	case credClientSecret:
		secret := os.Getenv("AZURE_CLIENT_SECRET")
		if cr.secretRef != "" {
			v, err := p.c.credentialValue(ctx, azkvScheme, "AZURE_CLIENT_SECRET", cr.secretRef, time.Hour)
			if err != nil {
				return accessToken{}, err
			}
			secret = strings.TrimSpace(v)
		}
		return cr.clientCredentials(ctx, p, r, url.Values{"client_secret": {secret}})
	case credCertificate:
		auth, err := authority(r)
		if err != nil {
			return accessToken{}, err
		}
		assertion, err := certAssertion(cr.certPath, cr.client, auth+"/"+cr.tenant+"/oauth2/v2.0/token")
		if err != nil {
			return accessToken{}, err
		}
		return cr.clientCredentials(ctx, p, r, url.Values{
			"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
			"client_assertion":      {assertion}})
	case credWorkload:
		b, err := readSmall(cr.tokenFile, 64<<10)
		if err != nil {
			return accessToken{}, errors.New("Azure workload identity: AZURE_FEDERATED_TOKEN_FILE is unreadable")
		}
		return cr.clientCredentials(ctx, p, r, url.Values{
			"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
			"client_assertion":      {strings.TrimSpace(string(b))}})
	case credManaged:
		return cr.managed(ctx, p, r)
	case credAzureCLI:
		return cr.azureCLI(ctx, r)
	}
	return accessToken{}, errNoAzureCredential
}

func readSmall(path string, max int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, max))
}

var oauthErrorCode = regexp.MustCompile(`^[a-z_]{1,40}$`)

// tokenAnswer turns a token endpoint's response into a token or a sanitized
// error: the status, and the OAuth error code when it is a plain code. The
// body is otherwise discarded.
func tokenAnswer(resp *http.Response, kind string) (accessToken, error) {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return accessToken{}, &RateLimit{Class: "rate limit", After: retryAfter(resp.Header.Get("Retry-After"), time.Now())}
	}
	var v struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   any    `json:"expires_in"`
		ExpiresOn   any    `json:"expires_on"`
		Error       string `json:"error"`
	}
	_ = json.Unmarshal(body, &v)
	if resp.StatusCode != http.StatusOK || v.AccessToken == "" {
		code := ""
		if oauthErrorCode.MatchString(v.Error) {
			code = ": " + v.Error
		}
		return accessToken{}, fmt.Errorf("Azure sign-in failed for the %s credential (HTTP %d%s)", kind, resp.StatusCode, code)
	}
	return accessToken{value: v.AccessToken, expires: expiry(v.ExpiresIn, v.ExpiresOn, time.Now())}, nil
}

// expiry reads expires_in (seconds) or expires_on (Unix seconds), as numbers
// or strings; with neither it assumes five minutes.
func expiry(in, on any, now time.Time) time.Time {
	num := func(v any) (int64, bool) {
		switch x := v.(type) {
		case float64:
			return int64(x), true
		case string:
			n, err := strconv.ParseInt(x, 10, 64)
			return n, err == nil
		}
		return 0, false
	}
	if n, ok := num(in); ok && n > 0 {
		return now.Add(time.Duration(n) * time.Second)
	}
	if n, ok := num(on); ok && n > now.Unix() {
		return time.Unix(n, 0)
	}
	return now.Add(5 * time.Minute)
}

func (cr azureCred) clientCredentials(ctx context.Context, p *azureKV, r kvRef, form url.Values) (accessToken, error) {
	auth, err := authority(r)
	if err != nil {
		return accessToken{}, err
	}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", cr.client)
	form.Set("scope", r.resource()+"/.default")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, auth+"/"+url.PathEscape(cr.tenant)+"/oauth2/v2.0/token", strings.NewReader(form.Encode()))
	if err != nil {
		return accessToken{}, errors.New("Azure sign-in request could not be built")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := p.client.Do(req)
	if err != nil {
		return accessToken{}, fmt.Errorf("Azure sign-in unreachable for the %s credential (network error)", cr.kind)
	}
	return tokenAnswer(resp, cr.kind)
}

// certAssertion signs a client assertion (RS256, x5t thumbprint) with the
// certificate's private key, read from a PEM file at use time.
func certAssertion(path, client, audience string) (string, error) {
	b, err := readSmall(path, 1<<20)
	if err != nil {
		return "", errors.New("Azure client certificate: AZURE_CLIENT_CERTIFICATE_PATH is unreadable")
	}
	var cert *x509.Certificate
	var key *rsa.PrivateKey
	for {
		var blk *pem.Block
		blk, b = pem.Decode(b)
		if blk == nil {
			break
		}
		switch blk.Type {
		case "CERTIFICATE":
			if cert == nil {
				cert, _ = x509.ParseCertificate(blk.Bytes)
			}
		case "PRIVATE KEY":
			if k, e := x509.ParsePKCS8PrivateKey(blk.Bytes); e == nil {
				key, _ = k.(*rsa.PrivateKey)
			}
		case "RSA PRIVATE KEY":
			key, _ = x509.ParsePKCS1PrivateKey(blk.Bytes)
		}
	}
	if cert == nil || key == nil {
		return "", errors.New("Azure client certificate: the PEM file needs a certificate and an unencrypted RSA private key")
	}
	thumb := sha1.Sum(cert.Raw)
	enc := base64.RawURLEncoding
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT", "x5t": enc.EncodeToString(thumb[:])})
	jti := make([]byte, 16)
	rand.Read(jti)
	now := time.Now()
	claims, _ := json.Marshal(map[string]any{"aud": audience, "iss": client, "sub": client, "jti": hex.EncodeToString(jti),
		"nbf": now.Unix(), "iat": now.Unix(), "exp": now.Add(10 * time.Minute).Unix()})
	signing := enc.EncodeToString(header) + "." + enc.EncodeToString(claims)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", errors.New("Azure client certificate: signing failed")
	}
	return signing + "." + enc.EncodeToString(sig), nil
}

// arcTokenDir is where the Azure Arc agent writes its challenge key files.
var arcTokenDir = func() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("ProgramData"), "AzureConnectedMachineAgent", "Tokens")
	}
	return "/var/opt/azcmagent/tokens"
}

func (cr azureCred) managed(ctx context.Context, p *azureKV, r kvRef) (accessToken, error) {
	q := url.Values{"resource": {r.resource()}}
	switch cr.flavor {
	case "app-service":
		q.Set("api-version", "2019-08-01")
	case "arc":
		q.Set("api-version", "2020-06-01")
	default:
		q.Set("api-version", "2018-02-01")
	}
	if cr.client != "" && cr.flavor != "arc" {
		q.Set("client_id", cr.client)
	}
	do := func(basic string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, cr.endpoint+"?"+q.Encode(), nil)
		if err != nil {
			return nil, err
		}
		if cr.flavor == "app-service" {
			req.Header.Set("X-IDENTITY-HEADER", os.Getenv("IDENTITY_HEADER"))
		} else {
			req.Header.Set("Metadata", "true")
		}
		if basic != "" {
			req.Header.Set("Authorization", "Basic "+basic)
		}
		return p.client.Do(req)
	}
	resp, err := do("")
	if err != nil {
		return accessToken{}, errors.New("Azure managed identity endpoint unreachable (" + cr.flavor + ")")
	}
	if cr.flavor == "arc" && resp.StatusCode == http.StatusUnauthorized {
		// Arc proves local admission with a key file named in the challenge.
		realm := resp.Header.Get("WWW-Authenticate")
		io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		resp.Body.Close()
		key, err := arcKey(realm)
		if err != nil {
			return accessToken{}, err
		}
		if resp, err = do(key); err != nil {
			return accessToken{}, errors.New("Azure managed identity endpoint unreachable (arc)")
		}
	}
	return tokenAnswer(resp, credManaged)
}

// arcKey reads the key file an Arc challenge names, only from the agent's
// token directory and only a small .key file.
func arcKey(challenge string) (string, error) {
	_, path, ok := strings.Cut(challenge, "realm=")
	path = strings.Trim(strings.TrimSpace(path), `"`)
	dir := filepath.Clean(arcTokenDir())
	clean := filepath.Clean(path)
	if !ok || filepath.Ext(clean) != ".key" || filepath.Dir(clean) != dir {
		return "", errors.New("Azure Arc identity challenge named an unexpected key file")
	}
	b, err := readSmall(clean, 4096)
	if err != nil {
		return "", errors.New("Azure Arc identity key file is unreadable (the user must be in the agent's token group)")
	}
	return strings.TrimSpace(string(b)), nil
}

// azureCLI asks an existing `az login` for a token. The token arrives on a
// pipe; az's stderr (which can name accounts) is discarded. kit never runs
// `az login`.
func (cr azureCred) azureCLI(ctx context.Context, r kvRef) (accessToken, error) {
	az, err := exec.LookPath("az")
	if err != nil {
		return accessToken{}, errors.New("Azure CLI credential selected but az is not on PATH")
	}
	args := []string{"account", "get-access-token", "--scope", r.resource() + "/.default", "--output", "json"}
	if cr.tenant != "" && azTenantForm.MatchString(cr.tenant) {
		args = append(args, "--tenant", cr.tenant)
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, az, args...)
	cmd.Stdin = nil
	var out bytes.Buffer
	cmd.Stdout = &out
	if cmd.Run() != nil {
		return accessToken{}, errors.New("az account get-access-token failed; run `az login` on this host yourself (kit never signs in interactively)")
	}
	var v struct {
		AccessToken string `json:"accessToken"`
		ExpiresOn   any    `json:"expires_on"`
	}
	if json.Unmarshal(out.Bytes(), &v) != nil || v.AccessToken == "" {
		return accessToken{}, errors.New("az account get-access-token returned no token")
	}
	return accessToken{value: v.AccessToken, expires: expiry(nil, v.ExpiresOn, time.Now())}, nil
}
