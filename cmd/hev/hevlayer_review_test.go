package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hev/kit/internal/daemon"
)

func TestHEVLayerOpenRejectsUnrelatedCredentials(t *testing.T) {
	for _, target := range []struct{ endpoint, store string }{
		{"http://localhost:8787", "turbopuffer"},
		{"http://localhost:8787", "pgvector"},
		{"https://gateway.example.net", ""},
		{"https://gateway.example.net", "hosted"},
	} {
		t.Run(target.endpoint+target.store, func(t *testing.T) {
			cleanLayerEnv(t)
			t.Setenv("HEV_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
			if err := writeInitConfig(daemon.DefaultConfigPath(), initFileConfig{Layer: initLayerConfig{Endpoint: target.endpoint, Store: target.store, APIKey: "unrelated-credential"}}); err != nil {
				t.Fatal(err)
			}
			previous := inviteHTTP
			calls := 0
			inviteHTTP = &http.Client{Transport: hostedTestTransport(func(r *http.Request) (*http.Response, error) { calls++; return nil, nil })}
			t.Cleanup(func() { inviteHTTP = previous })
			if err := runOpen(context.Background(), io.Discard); err == nil {
				t.Fatal("unrelated target accepted")
			}
			if calls != 0 {
				t.Fatal("credential sent to session broker")
			}
		})
	}
}

func TestHEVLayerSavedStoreWinsOverExportedKeys(t *testing.T) {
	for _, store := range []string{"turbopuffer", "pgvector"} {
		for _, name := range []string{"HEVLAYER_API_KEY", "LAYER_API_KEY", "TURBOPUFFER_API_KEY", "TPUF_API_KEY"} {
			t.Run(store+name, func(t *testing.T) {
				cleanLayerEnv(t)
				t.Setenv("HEV_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
				if err := writeInitConfig(daemon.DefaultConfigPath(), initFileConfig{Layer: initLayerConfig{Endpoint: "http://localhost:8787", Store: store, APIKey: "saved-key"}}); err != nil {
					t.Fatal(err)
				}
				t.Setenv(name, "exported-key")
				got, key, err := upLane("")
				if err != nil || got != store {
					t.Fatalf("saved store changed: %s %v", got, err)
				}
				if store == "turbopuffer" && key != "saved-key" {
					t.Fatal("provider key replaced by gateway key")
				}
			})
		}
	}
}

func TestHEVLayerSavedRemoteEndpointKeepsCredentialDestination(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX service fixture")
	}
	for _, name := range []string{"HEVLAYER_API_KEY", "LAYER_API_KEY"} {
		t.Run(name, func(t *testing.T) {
			cleanLayerEnv(t)
			onOS(t, "darwin")
			home, _ := sandbox(t, "1")
			writeConfig(t, home, "[layer]\nendpoint = \"https://gateway.example.net\"\napi_key = \"saved-key\"\nnamespace = \"archive\"\n")
			t.Setenv(name, "gateway-key")
			withStore(t, "")
			previous := inviteHTTP
			calls := 0
			inviteHTTP = &http.Client{Transport: hostedTestTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Host != "gateway.example.net" || r.Header.Get("Authorization") != "Bearer saved-key" {
					t.Errorf("wrong credential destination: %s", r.URL.Host)
				}
				return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader("{}")), Header: http.Header{}}, nil
			})}
			t.Cleanup(func() { inviteHTTP = previous })
			if err := runUp(context.Background(), io.Discard, true); err == nil {
				t.Fatal("rejected key accepted")
			}
			if calls != 1 {
				t.Fatalf("checks = %d", calls)
			}
			raw, err := os.ReadFile(daemon.DefaultConfigPath())
			if err != nil || !bytes.Contains(raw, []byte("gateway.example.net")) || bytes.Contains(raw, []byte("hosted")) {
				t.Fatal("saved target changed")
			}
		})
	}
}

func TestHEVLayerUnrelatedCommandsIgnoreArchiveConflicts(t *testing.T) {
	if os.Getenv("HEV_TEST_ARCHIVE_CONFLICT_CHILD") == "1" {
		os.Args = append([]string{"hev"}, strings.Split(os.Getenv("HEV_TEST_CLI_ARGS"), "|")...)
		main()
		return
	}
	for _, args := range [][]string{{"peers", "--help"}, {"dispatch", "--help"}, {"orchestrate", "--help"}, {"d", "--help"}} {
		t.Run(args[0], func(t *testing.T) {
			cleanLayerEnv(t)
			t.Setenv("HEVLAYER_API_KEY", "gateway-new")
			t.Setenv("LAYER_API_KEY", "gateway-old")
			t.Setenv("TURBOPUFFER_API_KEY", "provider")
			t.Setenv("HEV_TEST_ARCHIVE_CONFLICT_CHILD", "1")
			cmd := exec.Command(os.Args[0], "-test.run=^TestHEVLayerUnrelatedCommandsIgnoreArchiveConflicts$")
			// main reads os.Args directly; a child test supplies the intended CLI args.
			cmd.Env = append(os.Environ(), "HEV_TEST_CLI_ARGS="+strings.Join(args, "|"))
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("unrelated command failed: %v: %s", err, out)
			}
		})
	}
}
