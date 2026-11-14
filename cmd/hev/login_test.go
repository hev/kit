package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/hev/kit/internal/layerenv"
)

const fakeKitKey = "hk_secret_do_not_print_7f3a"

// fakeKit is a kit sign-in service and API. The browser half is played by
// browse, which follows the login URL the way a signed-in user would.
type fakeKit struct {
	t         *testing.T
	srv       *httptest.Server
	mu        sync.Mutex
	challenge string
	exchanges int
	verifier  string
	logouts   []string
	whoamiOK  bool
	logoutErr int
	invite    string
}

func newFakeKit(t *testing.T) *fakeKit {
	f := &fakeKit{t: t, whoamiOK: true}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /cli/exchange", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Code, Verifier, Hostname string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		defer f.mu.Unlock()
		f.exchanges++
		f.verifier = body.Verifier
		sum := sha256.Sum256([]byte(body.Verifier))
		if body.Code != "code-1" || base64.RawURLEncoding.EncodeToString(sum[:]) != f.challenge || body.Hostname == "" {
			http.Error(w, "bad exchange", 400)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"api": f.srv.URL, "key": fakeKitKey, "key_id": "key_42",
			"team":   map[string]string{"id": "team_1", "slug": "acme"},
			"author": map[string]string{"id": "user_9", "name": "Ada"},
		})
	})
	mux.HandleFunc("GET /v1/whoami", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		ok := f.whoamiOK
		f.mu.Unlock()
		if !ok || r.Header.Get("Authorization") != "Bearer "+fakeKitKey {
			http.Error(w, "unauthorized", 401)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"key_id": "key_42", "hostname": "box",
			"team": map[string]string{"id": "team_1", "slug": "acme"}, "author": map[string]string{"id": "user_9", "name": "Ada"}})
	})
	mux.HandleFunc("POST /cli/logout", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.logouts = append(f.logouts, r.Header.Get("Authorization"))
		code := f.logoutErr
		f.mu.Unlock()
		if code != 0 {
			http.Error(w, "nope", code)
		}
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeKit) set(change func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	change()
}

// browse returns a fake browser that answers the loopback callback with state.
func (f *fakeKit) browse(state func(sent string) string) func(string) {
	return f.browseWith(state, "code=code-1")
}

// browseWith answers the callback with the given result parameters.
func (f *fakeKit) browseWith(state func(sent string) string, result string) func(string) {
	return func(raw string) {
		u, err := url.Parse(raw)
		if err != nil || !strings.HasPrefix(raw, f.srv.URL+"/cli/login?") {
			f.t.Errorf("login URL %q", raw)
			return
		}
		q := u.Query()
		f.mu.Lock()
		f.challenge = q.Get("challenge")
		f.invite = q.Get("code")
		f.mu.Unlock()
		cb := "http://127.0.0.1:" + q.Get("port") + "/callback?" + result + "&state=" + url.QueryEscape(state(q.Get("state")))
		resp, err := http.Get(cb)
		if err == nil {
			resp.Body.Close()
		}
	}
}

func kitTestHome(t *testing.T) string {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	path := filepath.Join(home, ".hev", "config.toml")
	t.Setenv("HEV_CONFIG", path)
	t.Setenv(kitServiceEnv, "")
	for _, name := range layerenv.Names() {
		t.Setenv(name, "")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	prior := "[layer]\nendpoint = 'http://127.0.0.1:8080'\napi_key = 'local-key'\nnamespace = 'mine'\n"
	if err := os.WriteFile(path, []byte(prior), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func readTOML(t *testing.T, path string) map[string]any {
	doc := map[string]any{}
	if _, err := toml.DecodeFile(path, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestKitLoginRejectsStateMismatch(t *testing.T) {
	path := kitTestHome(t)
	f := newFakeKit(t)
	old, oldTimeout := openBrowser, loginTimeout
	t.Cleanup(func() { openBrowser, loginTimeout = old, oldTimeout })
	loginTimeout = 2 * time.Second
	openBrowser = f.browse(func(string) string { return "forged-state" })
	err := runLogin(context.Background(), &bytes.Buffer{}, f.srv.URL, "")
	if err == nil || !strings.Contains(err.Error(), "timed out") || !strings.Contains(err.Error(), "hev login --code CODE") {
		t.Fatalf("forged state accepted, or no invite hint on timeout: %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.exchanges != 0 {
		t.Fatal("code exchanged after a state mismatch")
	}
	if doc := readTOML(t, path); doc["kit"] != nil || doc["layer"] == nil {
		t.Fatalf("config changed: %v", doc)
	}
}

func TestKitLoginStatusLogout(t *testing.T) {
	path := kitTestHome(t)
	f := newFakeKit(t)
	old := openBrowser
	t.Cleanup(func() { openBrowser = old })
	openBrowser = f.browse(func(sent string) string { return sent })
	t.Setenv(kitServiceEnv, f.srv.URL) // the service comes from the environment

	var out bytes.Buffer
	if err := runLogin(context.Background(), &out, "", ""); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	exchanges, verifier := f.exchanges, f.verifier
	f.mu.Unlock()
	if exchanges != 1 || verifier == "" {
		t.Fatal("PKCE verifier not sent")
	}
	if strings.Contains(out.String(), fakeKitKey) || strings.Contains(out.String(), "hk_") {
		t.Fatalf("login printed the key: %s", out.String())
	}
	if !strings.Contains(out.String(), "not wired yet") {
		t.Fatalf("login did not say hosted capture is unwired: %s", out.String())
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("config mode %v %v", info.Mode(), err)
	}
	doc := readTOML(t, path)
	kit, _ := doc["kit"].(map[string]any)
	want := map[string]string{"service": f.srv.URL, "api": f.srv.URL, "key": fakeKitKey, "key_id": "key_42", "team_id": "team_1", "team_slug": "acme", "author_id": "user_9", "author_name": "Ada"}
	for k, v := range want {
		if kit[k] != v {
			t.Fatalf("[kit] %s = %v, want %v", k, kit[k], v)
		}
	}
	if doc["layer"] != nil {
		t.Fatal("[layer] still selected after sign-in")
	}
	if prior, _ := doc["kit_prior"].(map[string]any); prior["api_key"] != "local-key" {
		t.Fatalf("prior target not kept: %v", doc["kit_prior"])
	}
	if err := runLogin(context.Background(), &bytes.Buffer{}, "", ""); err == nil {
		t.Fatal("second login over an existing one")
	}

	out.Reset()
	if err := runLoginStatus(context.Background(), &out); err != nil || !strings.Contains(out.String(), "acme as Ada") || strings.Contains(out.String(), fakeKitKey) {
		t.Fatalf("status: %v %q", err, out.String())
	}
	f.set(func() { f.whoamiOK = false })
	err = runLoginStatus(context.Background(), &out)
	if err == nil || !strings.Contains(err.Error(), "rejected") || strings.Contains(err.Error(), fakeKitKey) {
		t.Fatalf("whoami 401: %v", err)
	}

	f.set(func() { f.logoutErr = 503 })
	if err := runLogout(context.Background(), &out, false); err == nil {
		t.Fatal("logout cleared the key though revocation failed")
	}
	if readTOML(t, path)["kit_prior"] == nil {
		t.Fatal("failed logout restored the target")
	}
	f.set(func() { f.logoutErr = 0 })
	out.Reset()
	if err := runLogout(context.Background(), &out, false); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	got := f.logouts[len(f.logouts)-1]
	f.mu.Unlock()
	if got != "Bearer "+fakeKitKey {
		t.Fatalf("logout bearer %q", got)
	}
	doc = readTOML(t, path)
	layer, _ := doc["layer"].(map[string]any)
	if layer["api_key"] != "local-key" || layer["namespace"] != "mine" || doc["kit_prior"] != nil {
		t.Fatalf("prior target not restored: %v", doc)
	}
	if kit, _ := doc["kit"].(map[string]any); kit["key"] != nil || kit["service"] != f.srv.URL {
		t.Fatalf("[kit] after logout: %v", kit)
	}
	if strings.Contains(out.String(), fakeKitKey) {
		t.Fatal("logout printed the key")
	}
	if err := runLoginStatus(context.Background(), &out); err == nil || !strings.Contains(err.Error(), "not signed in") {
		t.Fatalf("status after logout: %v", err)
	}
}

func TestKitLoginNeedsService(t *testing.T) {
	kitTestHome(t)
	if err := runLogin(context.Background(), &bytes.Buffer{}, "", ""); err == nil || !strings.Contains(err.Error(), "no kit service") {
		t.Fatalf("login without a service: %v", err)
	}
}

func TestLoginCallbackRejectsWrongState(t *testing.T) {
	codes := make(chan loginResult, 1)
	h := loginCallback("expected", codes)
	for _, path := range []string{"/callback?state=wrong&code=secret", "/callback?state=wrong&error=invite_required", "/callback?state=expected", "/callback?state=expected&error=Not+A+Code", "/else?state=expected&code=secret"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 400 {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
	select {
	case <-codes:
		t.Fatal("invalid callback delivered code")
	default:
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/callback?state=expected&code=valid", nil))
	if w.Code != 200 || (<-codes).code != "valid" {
		t.Fatal("valid callback rejected")
	}
}

func TestKitServiceURLBoundary(t *testing.T) {
	for _, raw := range []string{"http://example.com", "https://u:p@example.com", "https://example.com?key=x", "file:///tmp/x"} {
		if validKitURL(raw) {
			t.Fatalf("accepted %s", raw)
		}
	}
	for _, raw := range []string{"https://example.com", "http://127.0.0.1:8787"} {
		if !validKitURL(raw) {
			t.Fatalf("rejected %s", raw)
		}
	}
}

func TestKitLoginPassesInviteCode(t *testing.T) {
	kitTestHome(t)
	f := newFakeKit(t)
	old := openBrowser
	t.Cleanup(func() { openBrowser = old })
	openBrowser = f.browse(func(sent string) string { return sent })
	if err := runLogin(context.Background(), &bytes.Buffer{}, f.srv.URL, "INV-123"); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.invite != "INV-123" {
		t.Fatalf("invite code on /cli/login = %q", f.invite)
	}
}

func TestKitLoginHintsWhenInviteRequired(t *testing.T) {
	path := kitTestHome(t)
	f := newFakeKit(t)
	old := openBrowser
	t.Cleanup(func() { openBrowser = old })
	openBrowser = f.browseWith(func(sent string) string { return sent }, "error="+inviteRequired)
	err := runLogin(context.Background(), &bytes.Buffer{}, f.srv.URL, "")
	if err == nil || strings.Contains(err.Error(), "\n") || !strings.Contains(err.Error(), "hev login --code CODE") {
		t.Fatalf("no one-line invite hint: %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.exchanges != 0 || f.invite != "" {
		t.Fatalf("exchanges=%d invite=%q", f.exchanges, f.invite)
	}
	if readTOML(t, path)["kit"] != nil {
		t.Fatal("refused sign-in wrote [kit]")
	}
}
