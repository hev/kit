//go:build !windows

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDispatchIdentityWorkspaceFlag(t *testing.T) {
	const workspace = "42d2ad7b-ad91-46c5-89eb-6e726cbbfb24"
	requests := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case requests <- r.Method + " " + r.URL.Path:
		default:
			t.Error("unexpected additional identity request")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"items":[],"next_after":null,"has_more":false}`)
	}))
	defer server.Close()

	home := t.TempDir()
	env := []string{
		"HEV_WORK_IDENTITY_ENDPOINT=" + server.URL,
		"HEV_WORK_IDENTITY_KEY=fixture-bearer",
	}
	_, stderr, code := runHevChild(t, home, env, "dispatch", "identity", "--workspace", workspace, "list")
	if code != 0 {
		t.Fatalf("identity list exited %d: %s", code, stderr)
	}
	select {
	case got := <-requests:
		want := "POST /v2/namespaces/work-" + workspace + "/work-identity/query"
		if got != want {
			t.Fatalf("request = %q, want %q", got, want)
		}
	default:
		t.Fatal("identity list made no request")
	}

	_, stderr, code = runHevChild(t, home, env, "dispatch", "identity", "--workspace=", "list")
	if code == 0 || !strings.Contains(stderr, "--workspace:") {
		t.Fatalf("empty identity workspace: exit %d, stderr %q", code, stderr)
	}
	select {
	case got := <-requests:
		t.Fatalf("empty identity workspace made a request: %s", got)
	default:
	}

	_, stderr, code = runHevChild(t, home, nil, "--workspace=", "workspace", "current")
	if code == 0 || !strings.Contains(stderr, "workspace name must not be empty") {
		t.Fatalf("empty root workspace: exit %d, stderr %q", code, stderr)
	}
}

func TestDispatchIdentityConfigDefaultsAndOverrides(t *testing.T) {
	const workspace = "42d2ad7b-ad91-46c5-89eb-6e726cbbfb24"
	const override = "11d2ad7b-ad91-46c5-89eb-6e726cbbfb24"
	for _, tc := range []struct {
		name, section, whoami, wantKey, wantWorkspace, wantErr string
		explicitWorkspace, explicitKey, explicitEndpoint       bool
	}{
		{name: "kit defaults", section: "kit", whoami: workspace, wantKey: "config-key", wantWorkspace: workspace},
		{name: "Layer defaults", section: "layer", whoami: workspace, wantKey: "config-key", wantWorkspace: workspace},
		{name: "explicit workspace", section: "kit", wantKey: "config-key", wantWorkspace: override, explicitWorkspace: true},
		{name: "explicit key", section: "kit", whoami: workspace, wantKey: "override-key", wantWorkspace: workspace, explicitKey: true},
		{name: "unsafe partial service override", section: "kit", explicitEndpoint: true, wantErr: "set HEV_WORK_IDENTITY_KEY"},
		{name: "complete service override", section: "kit", explicitEndpoint: true, explicitKey: true, whoami: workspace, wantWorkspace: workspace, wantKey: "override-key"},
		{name: "missing discovery", section: "kit", wantKey: "config-key", wantErr: "specify --workspace"},
		{name: "malformed discovery", section: "kit", whoami: "not-a-uuid", wantKey: "config-key", wantErr: "specify --workspace"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var discoveries, queries atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer "+tc.wantKey {
					t.Error("unexpected credential")
				}
				switch r.Method + " " + r.URL.Path {
				case "GET /v1/whoami":
					discoveries.Add(1)
					fmt.Fprintf(w, `{"work_workspace_id":%q}`, tc.whoami)
				case "POST /v2/namespaces/work-" + tc.wantWorkspace + "/work-identity/query":
					queries.Add(1)
					fmt.Fprint(w, `{"items":[],"next_after":null,"has_more":false}`)
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			home := t.TempDir()
			config := filepath.Join(home, "config.toml")
			url := server.URL
			if tc.explicitEndpoint {
				url = "https://other-origin.invalid"
			}
			text := "[kit]\napi = '" + url + "'\nkey = 'config-key'\n"
			if tc.section == "layer" {
				text = "[layer]\nendpoint = '" + url + "'\napi_key = 'config-key'\n"
			}
			if err := os.WriteFile(config, []byte(text), 0600); err != nil {
				t.Fatal(err)
			}
			env := []string{"HEV_CONFIG=" + config}
			args := []string{"dispatch", "identity", "list"}
			if tc.explicitEndpoint {
				env = append(env, "HEV_WORK_IDENTITY_ENDPOINT="+server.URL)
			}
			if tc.explicitKey {
				env = append(env, "HEV_WORK_IDENTITY_KEY=override-key")
			}
			if tc.explicitWorkspace {
				args = append(args, "--workspace", override)
			}
			_, stderr, code := runHevChild(t, home, env, args...)
			if tc.wantErr == "" {
				if code != 0 || queries.Load() != 1 {
					t.Fatalf("identity failed: %d %s", code, stderr)
				}
			} else if code == 0 || !strings.Contains(stderr, tc.wantErr) || queries.Load() != 0 ||
				strings.Contains(stderr, "config-key") || strings.Contains(stderr, "override-key") {
				t.Fatalf("identity refusal: %d %s", code, stderr)
			}
			var wantDiscoveries int32 = 1
			if tc.explicitWorkspace || (tc.explicitEndpoint && !tc.explicitKey) {
				wantDiscoveries = 0
			}
			if discoveries.Load() != wantDiscoveries {
				t.Fatalf("discovery requests: %d, want %d", discoveries.Load(), wantDiscoveries)
			}
		})
	}
}

func TestDispatchIdentityEvidenceReferenceOnLinkAndAccept(t *testing.T) {
	const rawUUID = "42d2ad7b-ad91-46c5-89eb-6e726cbbfb24"
	const evidenceID = "ev_01arz3ndektsv4rrffq69g5fav"
	const mappedUUID = "c3a8958f-c6a7-52d1-b651-2c4303c824e3"
	requests := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var data map[string]any
		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			t.Error(err)
		}
		select {
		case requests <- data:
		default:
			t.Error("multiple writes for one command")
		}
		w.WriteHeader(http.StatusConflict)
	}))
	defer server.Close()
	home := t.TempDir()
	env := []string{"HEV_WORK_IDENTITY_ENDPOINT=" + server.URL, "HEV_WORK_IDENTITY_KEY=fixture-key"}
	for _, op := range []string{"link", "accept"} {
		for _, tc := range []struct{ input, want string }{
			{evidenceID, mappedUUID}, {rawUUID, rawUUID}, {"ev_short", ""}, {"ev_" + strings.Repeat("/", 26), ""}, {"private-not-a-uuid", ""},
		} {
			t.Run(op+"/"+tc.input, func(t *testing.T) {
				arg := "artifact"
				field := "reference"
				if op == "accept" {
					arg, field = "1", "evidence_ref"
				}
				_, stderr, code := runHevChild(t, home, env, "dispatch", "identity", "--workspace", rawUUID, op, rawUUID, "1", arg, tc.input)
				if tc.want == "" {
					if code == 0 || !strings.Contains(stderr, "expected a canonical UUID or evidence package ID") || strings.Contains(stderr, tc.input) || len(requests) != 0 {
						t.Fatalf("malformed reference: %d %s", code, stderr)
					}
					return
				}
				select {
				case got := <-requests:
					if got[field] != tc.want {
						t.Fatalf("reference body: %v", got)
					}
				default:
					t.Fatalf("no write request: %d %s", code, stderr)
				}
			})
		}
	}
}
