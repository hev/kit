package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/dispatch"
)

const multiHostSpec = `---
kind: orchestrator
name: scoped
goal: Keep the train moving
host: oven
manages:
  hosts: [oven, mini, adams-mac-mini]
  runs: {names: [builder/task, plain]}
authority: []
---
Keep moving.
`

func scopeApplyHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HEV_ORDERS_DIR", filepath.Join(home, "orders"))
	t.Setenv("HEV_CONFIG", filepath.Join(home, "config.toml"))
	t.Setenv("HEV_WORKSPACE", "")
	t.Setenv("HEV_ORCHESTRATOR", "")
	t.Setenv("PEERS_SELF", "desk")
	path := filepath.Join(home, "dispatch.toml")
	t.Setenv("HEV_DISPATCH_CONFIG", path)
	if err := os.WriteFile(path, []byte("cap = 2\n[hosts.oven]\nssh='oven-from-desk'\n[hosts.mini]\nssh='mini-from-desk'\n[hosts.builder]\nssh='builder-from-desk'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	old := orchestrateApplyRPC
	t.Cleanup(func() { orchestrateApplyRPC = old })
}

func runScopeApply(t *testing.T, spec string, flags ...string) (string, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "spec.md")
	if err := os.WriteFile(path, []byte(spec), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	cmd := newOrchestrateCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(append([]string{"apply", "-f", path}, flags...))
	err := cmd.Execute()
	return out.String(), err
}

func TestOrchestrateApplyRegistersEveryManagedHost(t *testing.T) {
	scopeApplyHome(t)
	var calls []string
	orchestrateApplyRPC = func(ctx context.Context, h dispatch.Host, req dispatch.Request) (dispatch.Response, error) {
		calls = append(calls, h.SSH)
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("apply RPC has no deadline")
		}
		s, err := dispatch.ParseOrchestrator(req.Order)
		if err != nil || s.Host+"-from-desk" != h.SSH || len(s.Authority) != 0 {
			t.Fatalf("misrouted or broadened spec: %+v, %v", s, err)
		}
		if s.Host == "oven" {
			if req.AdmissionOnly || req.ScopeOwnerLabel != "" {
				t.Fatal("owner received admission-only apply")
			}
			return dispatch.Response{}, nil
		}
		if !req.AdmissionOnly || req.ScopeOwnerLabel != "oven" || req.ScopeOwner != nil {
			t.Fatalf("expected target-resolved owner route, got %+v", req)
		}
		return dispatch.Response{ScopeRegistered: true}, nil
	}
	out, err := runScopeApply(t, multiHostSpec, "--dry-run")
	if err != nil || len(calls) != 0 || !strings.Contains(out, "would register scope on mini (owner oven)") ||
		!strings.Contains(out, "would register scope on builder (owner oven)") {
		t.Fatalf("dry run output=%q calls=%v err=%v", out, calls, err)
	}
	out, err = runScopeApply(t, multiHostSpec)
	if err != nil || strings.Join(calls, ",") != "oven-from-desk,builder-from-desk,mini-from-desk" ||
		!strings.Contains(out, "registered scope on mini (owner oven)") ||
		!strings.Contains(out, "registered scope on builder (owner oven)") {
		t.Fatalf("apply output=%q calls=%v err=%v", out, calls, err)
	}
}

func TestOrchestrateApplyPreflightsManagedHosts(t *testing.T) {
	scopeApplyHome(t)
	orchestrateApplyRPC = func(context.Context, dispatch.Host, dispatch.Request) (dispatch.Response, error) {
		t.Fatal("RPC before preflight")
		return dispatch.Response{}, nil
	}
	_, err := runScopeApply(t, strings.Replace(multiHostSpec, "builder/task", "missing/task", 1), "--dry-run")
	if err == nil || !strings.Contains(err.Error(), `"missing" is not configured`) {
		t.Fatalf("missing route: %v", err)
	}
	if err := os.WriteFile(os.Getenv("HEV_CONFIG"), []byte("[workspaces.scoped]\ndir='~/workspace'\nhosts=['desk','oven','mini']\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEV_WORKSPACE", "scoped")
	_, err = runScopeApply(t, multiHostSpec)
	if err == nil || !strings.Contains(err.Error(), `workspace boundary: host "builder"`) {
		t.Fatalf("outside workspace: %v", err)
	}
}

func TestOrchestrateApplyReportsAllScopeFailures(t *testing.T) {
	scopeApplyHome(t)
	orchestrateApplyRPC = func(_ context.Context, h dispatch.Host, _ dispatch.Request) (dispatch.Response, error) {
		if h.SSH == "builder-from-desk" {
			return dispatch.Response{}, errors.New("host unreachable")
		}
		return dispatch.Response{}, nil // mini does not acknowledge scope registration.
	}
	_, err := runScopeApply(t, multiHostSpec)
	if err == nil || !strings.Contains(err.Error(), "builder: host unreachable") ||
		!strings.Contains(err.Error(), "mini: host did not acknowledge") {
		t.Fatalf("partial scope registration reported as success: %v", err)
	}
}

func TestOrchestrateApplyExplicitScopeStaysOnTarget(t *testing.T) {
	scopeApplyHome(t)
	t.Setenv("PEERS_SELF", "mini")
	orchestrateApplyRPC = func(context.Context, dispatch.Host, dispatch.Request) (dispatch.Response, error) {
		t.Fatal("explicit registration forwarded")
		return dispatch.Response{}, nil
	}
	out, err := runScopeApply(t, multiHostSpec, "--scope-only-owner", "Oven.local")
	if err != nil || !strings.Contains(out, "registered scope on mini (owner Oven.local)") {
		t.Fatalf("local registration output=%q, err=%v", out, err)
	}
	s, err := dispatch.LoadOrchestrator("scoped")
	if err != nil || s.Host != "mini" {
		t.Fatalf("scope host=%q, err=%v", s.Host, err)
	}
	t.Setenv("PEERS_SELF", "oven")
	_, err = runScopeApply(t, multiHostSpec, "--scope-only-owner", "Oven.local", "--dry-run")
	if err == nil || !strings.Contains(err.Error(), "must be different hosts") {
		t.Fatalf("self-ownership: %v", err)
	}
}
