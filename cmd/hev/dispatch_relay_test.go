package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hev/kit/pkg/hostrelay"
)

type relayRoundTripFunc func(*http.Request) (*http.Response, error)

func (f relayRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDispatchRelayKeyValidationBothSides(t *testing.T) {
	hevKey := "hev_" + strings.Repeat("a1B2", 8)
	hvlKey := "hvl_" + strings.Repeat("Z9y8", 8)
	hkKey := "hk_" + strings.Repeat("Ab9_-1", 7) + "A"
	skOrKey := "sk-or-v1-" + strings.Repeat("deadbeef", 8)

	setupEnv := func(t *testing.T, configTOML string) {
		t.Helper()
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
		cfgPath := filepath.Join(home, ".hev", "config.toml")
		t.Setenv("HEV_CONFIG", cfgPath)
		t.Setenv("HEV_WORKSPACE", "")
		t.Setenv("HEV_API_KEY", "")
		if configTOML != "" {
			if err := os.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(cfgPath, []byte(configTOML), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}

	// Permitted side: valid hev_, hvl_, and bounded-migration hk_ keys pass validation
	// and send Authorization: Bearer <key> to the HTTPS relay endpoint.
	for _, tc := range []struct {
		name       string
		envKey     string
		configTOML string
		args       []string
		wantKey    string
	}{
		{
			name:    "env-hev-key",
			envKey:  hevKey,
			args:    []string{"--origin", "https://relay.example.test", "--host", "mini"},
			wantKey: hevKey,
		},
		{
			name:    "env-hvl-key",
			envKey:  hvlKey,
			args:    []string{"--origin", "https://relay.example.test", "--host", "mini"},
			wantKey: hvlKey,
		},
		{
			name:    "env-legacy-hk-key",
			envKey:  hkKey,
			args:    []string{"--origin", "https://relay.example.test", "--host", "mini"},
			wantKey: hkKey,
		},
		{
			name:       "config-kit-hev-key",
			configTOML: fmt.Sprintf("[kit]\napi = %q\nkey = %q\nteam_id = \"team_1\"\n", "https://relay.example.test", hevKey),
			args:       []string{"--host", "mini"},
			wantKey:    hevKey,
		},
	} {
		t.Run("permit/"+tc.name, func(t *testing.T) {
			setupEnv(t, tc.configTOML)
			if tc.envKey != "" {
				t.Setenv("HEV_API_KEY", tc.envKey)
			}
			var hits atomic.Int32
			var gotAuth, gotURL string
			oldTransport := relayTransport
			t.Cleanup(func() { relayTransport = oldTransport })
			relayTransport = relayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				hits.Add(1)
				gotAuth = r.Header.Get("Authorization")
				gotURL = r.URL.String()
				return &http.Response{
					StatusCode: http.StatusNoContent,
					Body:       io.NopCloser(strings.NewReader("")),
					Header:     make(http.Header),
				}, nil
			})

			cmd := dispatchRelayCommand()
			cmd.SilenceErrors = true
			cmd.SilenceUsage = true
			cmd.SetArgs(tc.args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("dispatch relay %v failed: %v", tc.args, err)
			}
			if hits.Load() != 1 {
				t.Fatalf("relay requests = %d, want 1", hits.Load())
			}
			if gotAuth != "Bearer "+tc.wantKey {
				t.Fatalf("Authorization ok=%v", gotAuth == "Bearer "+tc.wantKey)
			}
			if gotURL != "https://relay.example.test"+hostrelay.Path {
				t.Fatalf("URL = %q, want https://relay.example.test%s", gotURL, hostrelay.Path)
			}
		})
	}

	// Refused side: empty, bare-prefix, malformed, raw provider credentials, and non-HTTPS
	// origins must be refused before any network I/O.
	for _, tc := range []struct {
		name   string
		origin string
		key    string
	}{
		{"empty-key", "https://relay.example.test", ""},
		{"bare-hev", "https://relay.example.test", "hev_"},
		{"short-hev", "https://relay.example.test", "hev_short"},
		{"illegal-char-hev", "https://relay.example.test", "hev_" + strings.Repeat("a1B2", 6) + "_bad"},
		{"bare-hvl", "https://relay.example.test", "hvl_"},
		{"bare-hk", "https://relay.example.test", "hk_"},
		{"short-hk", "https://relay.example.test", "hk_short"},
		{"openrouter-sk", "https://relay.example.test", skOrKey},
		{"op-ref", "https://relay.example.test", "op://vault/kit/key"},
		{"http-loopback-origin", "http://127.0.0.1:8787", hevKey},
		{"http-remote-origin", "http://relay.example.test", hevKey},
		{"path-in-origin", "https://relay.example.test/api", hevKey},
	} {
		t.Run("refuse/"+tc.name, func(t *testing.T) {
			setupEnv(t, "")
			t.Setenv("HEV_API_KEY", tc.key)
			var hits atomic.Int32
			oldTransport := relayTransport
			t.Cleanup(func() { relayTransport = oldTransport })
			relayTransport = relayRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				hits.Add(1)
				return nil, fmt.Errorf("unexpected network call to %s", r.URL)
			})

			cmd := dispatchRelayCommand()
			cmd.SilenceErrors = true
			cmd.SilenceUsage = true
			cmd.SetArgs([]string{"--origin", tc.origin, "--host", "mini"})
			err := cmd.Execute()
			if err == nil {
				t.Fatalf("expected refusal for origin=%q key=%q", tc.origin, tc.key)
			}
			if hits.Load() != 0 {
				t.Fatalf("made %d network requests on refused input", hits.Load())
			}
			if len(tc.key) > 8 && strings.Contains(err.Error(), tc.key) {
				t.Fatalf("error leaked key: %v", err)
			}
		})
	}
}
