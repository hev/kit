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
	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/layerenv"
	"github.com/hev/kit/pkg/kitapi"
)

const fakeKitKey = "hk_secret_do_not_print_7f3a"

// fakeKit is a kit sign-in service and API. The browser half is played by
// browse, which follows the login URL the way a signed-in user would.
type fakeKit struct {
	t              *testing.T
	srv            *httptest.Server
	mu             sync.Mutex
	challenge      string
	exchanges      int
	whoamis        int
	verifier       string
	logouts        []string
	whoamiOK       bool
	logoutErr      int
	invite         string
	mintKey        string
	whoamiMetadata map[string]any
}

func newFakeKit(t *testing.T) *fakeKit {
	f := &fakeKit{t: t, whoamiOK: true, mintKey: fakeKitKey}
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
			"api": f.srv.URL, "key": f.mintKey, "key_id": "key_42",
			"team":   map[string]string{"id": "team_1", "slug": "acme"},
			"author": map[string]string{"id": "user_9", "name": "Ada"},
		})
	})
	mux.HandleFunc("GET /v1/whoami", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.whoamis++
		ok := f.whoamiOK
		wantKey := f.mintKey
		metadata := f.whoamiMetadata
		f.mu.Unlock()
		if !ok || r.Header.Get("Authorization") != "Bearer "+wantKey {
			http.Error(w, "unauthorized", 401)
			return
		}
		who := map[string]any{"key_id": "key_42", "hostname": "box",
			"team": map[string]string{"id": "team_1", "slug": "acme"}, "author": map[string]string{"id": "user_9", "name": "Ada"}}
		for field, value := range metadata {
			who[field] = value
		}
		json.NewEncoder(w).Encode(who)
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
	onOS(t, "linux")
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
	if strings.Contains(out.String(), fakeKitKey) || !strings.Contains(out.String(), "Key: box (hk_••••7f3a)\n") || strings.Contains(out.String(), "secret_do_not_print") || strings.Contains(out.String(), "key_42") {
		t.Fatalf("login printed the key: %s", out.String())
	}
	if !strings.Contains(out.String(), "queue rows for the hevmind.com service API") {
		t.Fatalf("login did not say where capture goes: %s", out.String())
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
	if err := runLoginStatus(context.Background(), &out); err != nil || !strings.Contains(out.String(), "acme as Ada") || !strings.Contains(out.String(), "Key: box (hk_••••7f3a)\n") || strings.Contains(out.String(), "key_42") || strings.Contains(out.String(), "Scope:") || strings.Contains(out.String(), "Expires:") || strings.Contains(out.String(), fakeKitKey) {
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

func TestKitLoginServicePrecedence(t *testing.T) {
	kitTestHome(t)
	doc := map[string]any{}
	if got := resolveKitService("", doc); got != defaultKitService {
		t.Fatalf("default service = %s", got)
	}
	doc["kit"] = map[string]any{"service": "https://configured.example"}
	if got := resolveKitService("", doc); got != "https://configured.example" {
		t.Fatalf("configured service = %s", got)
	}
	t.Setenv(kitServiceEnv, "https://environment.example")
	if got := resolveKitService("", doc); got != "https://environment.example" {
		t.Fatalf("environment service = %s", got)
	}
	if got := resolveKitService("https://flag.example", doc); got != "https://flag.example" {
		t.Fatalf("explicit service = %s", got)
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

func TestKitConfigShowHidesKey(t *testing.T) {
	path := kitTestHome(t)
	doc := readTOML(t, path)
	if err := daemon.WriteKitConfig(doc, daemon.KitConfig{Key: fakeKitKey, TeamID: "team_1"}); err != nil {
		t.Fatal(err)
	}
	previous := configShowCmd.OutOrStdout()
	var out bytes.Buffer
	configShowCmd.SetOut(&out)
	t.Cleanup(func() { configShowCmd.SetOut(previous) })
	if err := configShowCmd.RunE(configShowCmd, nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), fakeKitKey) || !strings.Contains(out.String(), "HEVLAYER_NAMESPACE") {
		t.Fatal("config show exposed the Club Kit key or lost the effective settings")
	}
}

func TestKitLoginMintedHevKeyParityAndRefusals(t *testing.T) {
	hevPayload := strings.Repeat("a1B2", 8)
	hevKey := "hev_" + hevPayload
	skOrKey := "sk-or-v1-" + strings.Repeat("deadbeef", 8)

	// Permitted side: /cli/exchange mints a unified hev_<32base62> key.
	// login writes [kit].key, login status succeeds via /v1/whoami, logout revokes it,
	// and stdout/errors never contain hev_ secret material.
	t.Run("minted-hev-key-roundtrip", func(t *testing.T) {
		path := kitTestHome(t)
		f := newFakeKit(t)
		f.set(func() {
			f.mintKey = hevKey
			f.whoamiMetadata = map[string]any{
				"key_name": "Build machine", "key_mask": "hev_••••a1B2",
				"scopes": []string{"read", "write"}, "expires_at": "2027-01-01T00:00:00Z",
			}
		})
		const keyDetails = "Key: Build machine (hev_••••a1B2)\nScope: read, write\nExpires: 2027-01-01T00:00:00Z\n"
		old := openBrowser
		t.Cleanup(func() { openBrowser = old })
		openBrowser = f.browse(func(sent string) string { return sent })

		var out bytes.Buffer
		if err := runLogin(context.Background(), &out, f.srv.URL, ""); err != nil {
			t.Fatalf("runLogin with minted hev_ key: %v", err)
		}
		if strings.Contains(out.String(), hevKey) || strings.Contains(out.String(), hevPayload) || strings.Contains(out.String(), "key_42") || !strings.Contains(out.String(), keyDetails) {
			t.Fatalf("login output leaked hev_ material: %q", out.String())
		}
		doc := readTOML(t, path)
		kit, _ := doc["kit"].(map[string]any)
		if kit["key"] != hevKey {
			t.Fatalf("[kit].key = %v, want minted hev_ key", kit["key"])
		}

		f.set(func() {
			f.whoamiMetadata["expires_at"] = time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
		})

		out.Reset()
		if err := runLoginStatus(context.Background(), &out); err != nil {
			t.Fatalf("runLoginStatus: %v", err)
		}
		if !strings.Contains(out.String(), "acme as Ada") || strings.Contains(out.String(), hevKey) || strings.Contains(out.String(), hevPayload) || strings.Contains(out.String(), "key_42") || !strings.Contains(out.String(), keyDetails) {
			t.Fatalf("login status output = %q", out.String())
		}

		out.Reset()
		if err := runLogout(context.Background(), &out, false); err != nil {
			t.Fatalf("runLogout: %v", err)
		}
		if strings.Contains(out.String(), hevKey) || strings.Contains(out.String(), hevPayload) || strings.Contains(out.String(), "hev_") {
			t.Fatalf("logout output leaked hev_ material: %q", out.String())
		}
		f.mu.Lock()
		gotLogout := f.logouts[len(f.logouts)-1]
		f.mu.Unlock()
		if gotLogout != "Bearer "+hevKey {
			t.Fatalf("logout Authorization = %q, want Bearer <hevKey>", gotLogout)
		}
	})

	// Refused side: malformed, bare-prefix, short, or raw provider credentials from /cli/exchange
	// must fail closed without calling /v1/whoami, writing [kit], or leaking the token.
	for _, tc := range []struct {
		name string
		key  string
	}{
		{"bare-hev", "hev_"},
		{"short-hev", "hev_short"},
		{"illegal-char-hev", "hev_" + strings.Repeat("a1B2", 6) + "_bad"},
		{"bare-hk", "hk_"},
		{"short-hk", "hk_short"},
		{"openrouter-key", skOrKey},
		{"op-ref", "op://vault/kit/key"},
	} {
		t.Run("refuse/"+tc.name, func(t *testing.T) {
			path := kitTestHome(t)
			f := newFakeKit(t)
			f.set(func() { f.mintKey = tc.key })
			old := openBrowser
			t.Cleanup(func() { openBrowser = old })
			openBrowser = f.browse(func(sent string) string { return sent })

			var out bytes.Buffer
			err := runLogin(context.Background(), &out, f.srv.URL, "")
			if err == nil {
				t.Fatalf("expected refusal for exchanged key %q", tc.key)
			}
			if len(tc.key) > 8 && (strings.Contains(err.Error(), tc.key) || strings.Contains(out.String(), tc.key)) {
				t.Fatalf("refusal leaked key %q: err=%v out=%q", tc.key, err, out.String())
			}
			f.mu.Lock()
			whoamis := f.whoamis
			f.mu.Unlock()
			if whoamis != 0 {
				t.Fatalf("called /v1/whoami %d times after invalid exchange key", whoamis)
			}
			if doc := readTOML(t, path); doc["kit"] != nil {
				t.Fatalf("invalid exchange wrote [kit] config: %v", doc["kit"])
			}
		})
	}
}

func TestLoginCLIUsesServiceNameAndConciseErrors(t *testing.T) {
	home := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, fakeKitKey, http.StatusServiceUnavailable)
	}))
	defer server.Close()
	config := filepath.Join(home, "config.toml")
	if err := os.WriteFile(config, []byte("[kit]\napi = '"+server.URL+"'\nservice = '"+server.URL+"'\nkey = '"+fakeKitKey+"'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	env := []string{"HEV_CONFIG=" + config}
	for _, args := range [][]string{{"login"}, {"login", "status"}, {"logout"}} {
		help, stderr, code := runHevChild(t, home, env, append(append([]string{}, args...), "--help")...)
		if code != 0 || !strings.Contains(help, "Usage:") || !strings.Contains(help, "hevmind.com service") || strings.Contains(help+stderr, "Club Kit") {
			t.Fatalf("%v help: exit %d, %s%s", args, code, help, stderr)
		}
		errorEnv := append(append([]string{}, env...), "HEVLAYER_API_KEY_REF=op://fixture/unused/key")
		out, stderr, code := runHevChild(t, home, errorEnv, args...)
		want := "HTTP 503"
		if len(args) == 1 && args[0] == "login" {
			want = "unset HEVLAYER_API_KEY_REF"
		}
		if code == 0 || !strings.Contains(stderr, want) || !strings.Contains(stderr, "hevmind.com service") ||
			strings.Contains(out+stderr, "Usage:") || strings.Contains(out+stderr, fakeKitKey) || strings.Contains(out+stderr, "op://fixture") {
			t.Fatalf("%v error: exit %d, %s%s", args, code, out, stderr)
		}
	}
}

func TestLoginStatusTeamDisplayFallbacks(t *testing.T) {
	for _, tc := range []struct{ team, want string }{
		{`{"id":"team_1","slug":"acme","name":"Acme Inc","display_name":"Acme Studio"}`, "Acme Studio"},
		{`{"id":"team_1","slug":"acme","name":"Acme Inc","display_name":" "}`, "Acme Inc"},
		{`{"id":"team_1","slug":"acme"}`, "acme"},
		{`{"id":"team_1"}`, "team_1"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			path := kitTestHome(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(`{"team":` + tc.team + `,"author":{"id":"user_1"}}`))
			}))
			defer server.Close()
			if err := os.WriteFile(path, []byte("[kit]\napi = '"+server.URL+"'\nkey = '"+fakeKitKey+"'\nteam_id = 'team_1'\n"), 0600); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			if err := runLoginStatus(context.Background(), &out); err != nil ||
				!strings.Contains(out.String(), "team "+tc.want+" as user_1") || strings.Contains(out.String(), fakeKitKey) {
				t.Fatalf("status: %v, %s", err, out.String())
			}
		})
	}
}

func TestLoginRefreshesOnlyInstalledCaptureService(t *testing.T) {
	for _, kind := range []string{"launchd", "systemd", "absent", "operator-managed"} {
		t.Run(kind, func(t *testing.T) {
			home, log := sandbox(t, "0")
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
			t.Setenv("HEV_HOST", "fixture-host")
			f := newFakeKit(t)
			oldBrowser, oldService := openBrowser, linuxService
			t.Cleanup(func() { openBrowser, linuxService = oldBrowser, oldService })
			openBrowser = f.browse(func(state string) string { return state })
			userService := &fakeUserService{}
			linuxService = userService
			onOS(t, "linux")
			path := systemdPath(home)
			stale := "HEVLAYER_API_KEY_REF=stale\nHEVLAYER_API_KEY_OP_REF=stale\n"
			if kind == "systemd" {
				stale = managedUnitMarker + "\n" + stale
			}
			if kind == "launchd" || kind == "absent" {
				onOS(t, "darwin")
				job := daemonJob(filepath.Join(home, ".local", "bin", "hev"), home, daemon.DefaultConfigPath())
				path = job.plistPath(home)
				job.Env["HEVLAYER_API_KEY_REF"] = "stale"
				job.Env["HEVLAYER_API_KEY_OP_REF"] = "stale"
				stale = job.plist()
			}
			if kind != "absent" {
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(stale), 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				t.Setenv("FAKE_UNLOADED", "1")
			}
			if err := os.MkdirAll(filepath.Dir(daemon.DefaultConfigPath()), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(daemon.DefaultConfigPath(), []byte("[layer]\napi_key = 'old-key'\n"), 0600); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			if err := runLogin(context.Background(), &out, f.srv.URL, ""); err != nil {
				t.Fatal(err)
			}
			doc := readTOML(t, daemon.DefaultConfigPath())
			if doc["kit"] == nil || doc["layer"] != nil || doc["kit_prior"] == nil {
				t.Fatal("login did not move the Layer configuration aside")
			}
			raw, err := os.ReadFile(path)
			switch kind {
			case "absent":
				if !os.IsNotExist(err) || strings.Contains(calls(t, log), "bootstrap") || len(userService.calls) != 0 {
					t.Fatal("login installed a previously absent service")
				}
			case "operator-managed":
				if err != nil || string(raw) != stale || len(userService.calls) != 0 {
					t.Fatal("login touched an operator-managed service")
				}
			default:
				if err != nil || strings.Contains(string(raw), "HEVLAYER_API_KEY_") || strings.Contains(string(raw), fakeKitKey) ||
					!strings.Contains(string(raw), daemon.DefaultConfigPath()) {
					t.Fatal("refresh leaked credentials or lost daemon configuration")
				}
				if kind == "launchd" {
					if !strings.Contains(string(raw), "fixture-host") || !strings.Contains(calls(t, log), "launchctl bootout") ||
						!strings.Contains(calls(t, log), "launchctl bootstrap") {
						t.Fatal("login did not restart launchd with the current environment")
					}
				} else if strings.Join(userService.calls, ",") != "stop hevd.service,daemon-reload,enable --now hevd.service,restart hevd.service" {
					t.Fatalf("unexpected systemd calls: %v", userService.calls)
				}
			}
		})
	}
}

// A service may supply only part of the optional metadata. An unsafe mask
// falls back to the local mask and other fields still go through redaction.
func TestWriteLoginKeyPartialMetadata(t *testing.T) {
	var out bytes.Buffer
	who := kitapi.Whoami{KeyName: fakeKitKey, KeyMask: fakeKitKey}
	writeLoginKey(&out, who, "local-box", fakeKitKey)
	if got := out.String(); got != "Key: [redacted] (hk_••••7f3a)\n" {
		t.Fatal("expected a redacted name and local mask")
	}
	out.Reset()
	writeLoginKey(&out, kitapi.Whoami{KeyMask: "hev_••••a1B2"}, "local-box", fakeKitKey)
	if got := out.String(); got != "Key: local-box (hev_••••a1B2)\n" {
		t.Fatal("expected the local hostname and safe service mask")
	}
}
