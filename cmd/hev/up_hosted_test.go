package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/layerenv"
)

func cleanLayerEnv(t *testing.T) {
	t.Helper()
	for _, name := range layerenv.Names() {
		t.Setenv(name, "")
	}
}

func TestHEVLayerUpLane(t *testing.T) {
	for _, name := range []string{"HEVLAYER_API_KEY", "LAYER_API_KEY", "TURBOPUFFER_API_KEY", "TPUF_API_KEY"} {
		t.Run(name, func(t *testing.T) {
			cleanLayerEnv(t)
			t.Setenv("HEV_CONFIG", filepath.Join(t.TempDir(), "absent.toml"))
			t.Setenv(name, "test-key")
			store, key, err := upLane("")
			if err != nil || store != "hosted" || key != "test-key" {
				t.Fatalf("lane: %s %s %v", store, key, err)
			}
			store, key, err = upLane("turbopuffer")
			if err != nil || store != "turbopuffer" || key != "test-key" {
				t.Fatalf("explicit BYO: %s %s %v", store, key, err)
			}
			store, key, err = upLane("pgvector")
			if err != nil || store != "pgvector" || key != "" {
				t.Fatalf("explicit local: %s %s %v", store, key, err)
			}
		})
	}
	t.Run("keyless", func(t *testing.T) {
		cleanLayerEnv(t)
		t.Setenv("HEV_CONFIG", filepath.Join(t.TempDir(), "absent.toml"))
		store, key, err := upLane("")
		if err != nil || store != "pgvector" || key != "" {
			t.Fatalf("keyless: %s %s %v", store, key, err)
		}
	})
}

func TestHEVLayerHostedUpSkipsDockerAndPersistsTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fake service manager; lane selection runs natively on Windows")
	}
	for _, name := range []string{"HEVLAYER_API_KEY", "LAYER_API_KEY"} {
		t.Run(name, func(t *testing.T) {
			cleanLayerEnv(t)
			onOS(t, "darwin")
			home, log := sandbox(t, "1")
			t.Setenv("FAKE_UNLOADED", "1")
			t.Setenv(name, "hosted-test-key")
			t.Setenv("HEVLAYER_NAMESPACE", "shared-traces")
			withStore(t, "")
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Header.Get("Authorization") != "Bearer hosted-test-key" || r.URL.Path != "/v2/namespaces/shared-traces/query" {
					t.Errorf("wrong hosted request: %s", r.URL.Path)
				}
				w.WriteHeader(http.StatusNotFound)
			}))
			defer server.Close()
			previousHTTP := inviteHTTP
			inviteHTTP = &http.Client{Transport: hostedTestTransport(func(r *http.Request) (*http.Response, error) {
				if "http://"+r.URL.Host != server.URL {
					return nil, fmt.Errorf("test refused unexpected destination")
				}
				return http.DefaultTransport.RoundTrip(r)
			})}
			t.Cleanup(func() { inviteHTTP = previousHTTP })
			t.Setenv("HEVLAYER_ENDPOINT", server.URL)
			var out bytes.Buffer
			if err := runUp(context.Background(), &out, false); err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatalf("hosted checks = %d", calls)
			}
			if strings.Contains(allCalls(t, log), "docker") {
				t.Fatal("hosted lane called Docker")
			}
			if !strings.Contains(out.String(), server.URL+" · shared-traces") || strings.Contains(out.String(), "<(°O°)>") {
				t.Fatalf("archive target missing: %s", &out)
			}
			if !strings.Contains(out.String(), "hev open") {
				t.Fatalf("dashboard instruction missing: %s", &out)
			}
			for _, key := range layerenv.Names() {
				t.Setenv(key, "")
			}
			cfg, err := daemon.LoadConfig()
			if err != nil {
				t.Fatal(err)
			}
			if cfg.LayerEndpoint != server.URL || cfg.LayerAPIKey != "hosted-test-key" || cfg.LayerNamespace != "shared-traces" || cfg.LayerStore != "hosted" {
				t.Fatal("hosted target was not persisted")
			}
			raw, err := os.ReadFile(filepath.Join(home, "Library", "LaunchAgents", launchdLabel()+".plist"))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(raw, []byte(cfg.ConfigPath)) {
				t.Fatal("capture does not read saved target")
			}
			indexPath := filepath.Join(home, ".hev", "index-state.json")
			if err := os.WriteFile(indexPath, []byte("existing-index"), 0600); err != nil {
				t.Fatal(err)
			}
			out.Reset()
			if err := runUp(context.Background(), &out, true); err != nil {
				t.Fatal(err)
			}
			if calls != 2 {
				t.Fatalf("stored target was not checked: %d", calls)
			}
			if got, _ := os.ReadFile(indexPath); string(got) != "existing-index" {
				t.Fatal("unchanged archive reset its index")
			}
			if strings.Contains(out.String(), "Dashboard:") {
				t.Fatal("no-dashboard was ignored")
			}
			t.Setenv("FAKE_UNLOADED", "")
			if err := runDown(context.Background(), &out); err != nil {
				t.Fatal(err)
			}
			if got := allCalls(t, log); strings.Contains(got, "docker") || !strings.Contains(got, "launchctl bootout") {
				t.Fatalf("hosted down failed to unload capture without Docker: %s", got)
			}
			if _, err := os.Stat(filepath.Join(home, "Library", "LaunchAgents", launchdLabel()+".plist")); !os.IsNotExist(err) {
				t.Fatalf("capture plist remains after down: %v", err)
			}

		})
	}
}

func TestHEVLayerHostedDefault(t *testing.T) {
	if layerenv.HostedEndpoint != "https://aws-us-east-1.hevlayer.com" {
		t.Fatal("default must match public gateway contract")
	}
}

type hostedTestTransport func(*http.Request) (*http.Response, error)

func (f hostedTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestHEVLayerOpenUsesSavedHostedKey(t *testing.T) {
	f := inviteSandbox(t)
	path := daemon.DefaultConfigPath()
	if err := writeInitConfig(path, initFileConfig{Layer: initLayerConfig{Endpoint: layerenv.HostedEndpoint, APIKey: f.key, Namespace: "archive", Store: "hosted"}}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEVLAYER_API_KEY", "unrelated-shell-key")
	previousOpen := inviteOpenURL
	opened := false
	inviteOpenURL = func(out io.Writer, target string) error {
		opened = strings.HasPrefix(target, "https://app.hevkit.com/")
		return nil
	}
	t.Cleanup(func() { inviteOpenURL = previousOpen })
	if err := runOpen(context.Background(), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !opened || f.requests() != "POST /join/session" {
		t.Fatal("hosted dashboard did not use saved target")
	}
}

func TestHEVLayerStoreEnvironmentChoice(t *testing.T) {
	cleanLayerEnv(t)
	t.Setenv("HEV_CONFIG", filepath.Join(t.TempDir(), "absent.toml"))
	t.Setenv("HEVLAYER_API_KEY", "fixture-key")
	for _, name := range []string{"HEVLAYER_STORE", "LAYER_STORE"} {
		t.Setenv(name, "turbopuffer")
		store, key, err := upLane("")
		if err != nil || store != "turbopuffer" || key != "fixture-key" {
			t.Fatalf("explicit store: %s %s %v", store, key, err)
		}
		t.Setenv(name, "")
	}
	store, _, err := upLane("hosted")
	if err != nil || store != "hosted" {
		t.Fatalf("explicit hosted: %s %v", store, err)
	}
}

func TestHEVLayerInitStoreChoice(t *testing.T) {
	for _, store := range []string{"hosted", "pgvector", "turbopuffer"} {
		t.Run(store, func(t *testing.T) {
			cleanLayerEnv(t)
			path := filepath.Join(t.TempDir(), "config.toml")
			t.Setenv("HEV_CONFIG", path)
			if err := runInteractiveInit(strings.NewReader("https://gateway.example\nfixture-key\narchive\n"+store+"\n"), &bytes.Buffer{}); err != nil {
				t.Fatal(err)
			}
			file, err := loadInitConfig(path)
			if err != nil || file.Layer.Store != store {
				t.Fatalf("init store = %s, %v", file.Layer.Store, err)
			}
		})
	}
}

func TestHEVLayerConfigShowRedactsCanonicalKey(t *testing.T) {
	cleanLayerEnv(t)
	t.Setenv("HEV_CONFIG", filepath.Join(t.TempDir(), "absent.toml"))
	t.Setenv("HEVLAYER_API_KEY", "config-show-fixture-key")
	t.Setenv("HEVLAYER_NAMESPACE", "archive")
	previous := configShowCmd.OutOrStdout()
	var out bytes.Buffer
	configShowCmd.SetOut(&out)
	t.Cleanup(func() { configShowCmd.SetOut(previous) })
	if err := configShowCmd.RunE(configShowCmd, nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "config-show-fixture-key") || !strings.Contains(out.String(), `"HEVLAYER_API_KEY": "[REDACTED]"`) || !strings.Contains(out.String(), `"HEVLAYER_NAMESPACE": "archive"`) {
		t.Fatal("config show missed canonical names or exposed key")
	}
}
