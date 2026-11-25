package dispatch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScopeApplyUpdatesWithLiveOwnerAndEnforcesTargetAdmission(t *testing.T) {
	orchHome(t)
	t.Setenv("HEV_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	ownerRoot := t.TempDir()
	t.Setenv("HEV_ORDERS_DIR", ownerRoot)
	t.Setenv("PEERS_SELF", "oven")
	ownerHome := os.Getenv("HOME")
	s := applyOrch(t)
	if err := writeProcessRecord(filepath.Join(OrchestratorDir(s.Name), "supervisor.json"), os.Getpid()); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyOrchestrator(orchestratorExample, false); err == nil || !strings.Contains(err.Error(), "stop orchestrator") {
		t.Fatalf("owner must refuse live supervisor reapply: %v", err)
	}

	targetHome := t.TempDir()
	t.Setenv("HOME", targetHome)
	t.Setenv("USERPROFILE", targetHome)
	t.Setenv("HEV_ORDERS_DIR", filepath.Join(targetHome, "orders"))
	t.Setenv("PEERS_SELF", "mini")
	config := filepath.Join(targetHome, "dispatch.toml")
	t.Setenv("HEV_DISPATCH_CONFIG", config)
	if err := os.WriteFile(config, []byte("cap=2\n[hosts.oven]\nssh='oven-from-mini'\nenv=['TOKEN=must-not-persist']\nconfig_path='private-machine-config'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	spec, err := OrchestratorSpecForHost(orchestratorExample, "mini")
	if err != nil {
		t.Fatal(err)
	}
	register := Request{Action: "orchestrator-apply", AdmissionOnly: true, ScopeOwnerLabel: "oven", Name: s.Name, Order: spec}
	if r, err := Handle(register); err != nil || !r.ScopeRegistered {
		t.Fatalf("initial scope: %+v, %v", r, err)
	}
	register.Order = strings.Replace(spec, "repos: [app]", "repos: [shipping]", 1)
	if register.Order == spec {
		t.Fatal("test failed to narrow target scope")
	}
	if r, err := Handle(register); err != nil || !r.ScopeRegistered {
		t.Fatalf("update with live owner: %+v, %v", r, err)
	}
	if err := LaunchOrchestrator(s.Name); err == nil {
		t.Fatal("scope-only target launched rounds")
	}
	dir := OrchestratorDir(s.Name)
	data, err := os.ReadFile(filepath.Join(dir, "scope-owner.json"))
	if err != nil || !strings.Contains(string(data), "oven-from-mini") ||
		strings.Contains(string(data), "must-not-persist") || strings.Contains(string(data), "private-machine-config") ||
		strings.Contains(string(data), targetHome) {
		t.Fatal("scope entry should contain only target-local route fields")
	}

	oldRPC := routeRPC
	t.Cleanup(func() { routeRPC = oldRPC })
	ownerChecks := 0
	ownerStopped := false
	routeRPC = func(ctx context.Context, route Host, q Request) (Response, error) {
		ownerChecks++
		if _, ok := ctx.Deadline(); !ok || route.SSH != "oven-from-mini" || !q.AdmissionOnly {
			t.Fatal("missing bounded owner check")
		}
		t.Setenv("HOME", ownerHome)
		t.Setenv("USERPROFILE", ownerHome)
		t.Setenv("HEV_ORDERS_DIR", ownerRoot)
		t.Setenv("PEERS_SELF", "oven")
		defer func() {
			t.Setenv("HOME", targetHome)
			t.Setenv("USERPROFILE", targetHome)
			t.Setenv("HEV_ORDERS_DIR", filepath.Join(targetHome, "orders"))
			t.Setenv("PEERS_SELF", "mini")
		}()
		if ownerStopped {
			if err := os.WriteFile(filepath.Join(OrchestratorDir(s.Name), "stop"), nil, 0600); err != nil {
				t.Fatal(err)
			}
		}
		return Handle(q)
	}
	for _, tc := range []struct{ action, repo, wantErr string }{
		{"say", "shipping", ""},
		{"say", "app", "outside manages scope"},
		{"stop", "shipping", "lacks authority stop"},
	} {
		run := filepath.Join(Root(), "work")
		if err := os.MkdirAll(run, 0700); err != nil {
			t.Fatal(err)
		}
		if err := WriteJSON(filepath.Join(run, "meta.json"), Meta{Order: Order{Name: "work", Repo: tc.repo, Issue: "REL-1"}}); err != nil {
			t.Fatal(err)
		}
		q := Request{Action: tc.action, Name: "work", Orchestrator: s.Name, Order: spec, ScopeOwnerLabel: "spoofed"}
		err := admitOrchestrator(q)
		if tc.wantErr == "" && err != nil || tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
			t.Fatalf("%+v: %v", tc, err)
		}
	}
	ownerStopped = true
	q := Request{Action: "say", Name: "work", Orchestrator: s.Name}
	if err := admitOrchestrator(q); err == nil || !strings.Contains(err.Error(), "owner admission: orchestrator is stopped") {
		t.Fatalf("stopped owner: %v", err)
	}
	if ownerChecks != 4 {
		t.Fatalf("expected live owner check for each target admission, got %d", ownerChecks)
	}
}

func TestScopeApplyRPCRefusesForeignTargetsAndOwners(t *testing.T) {
	orchHome(t)
	t.Setenv("PEERS_SELF", "mini")
	config := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("HEV_CONFIG", config)
	if err := os.WriteFile(config, []byte("[workspaces.scoped]\ndir='~/workspace'\nhosts=['mini']\n"), 0600); err != nil {
		t.Fatal(err)
	}
	base := strings.Replace(orchestratorExample, "kind:", "workspace: scoped\nkind:", 1)
	for _, tc := range []struct{ owner, target, want string }{
		{"mini.local", "mini", "must be different hosts"},
		{"oven", "oven", "does not match this host"},
		{"oven", "mini", "outside workspace"},
	} {
		spec, err := OrchestratorSpecForHost(base, tc.target)
		if err != nil {
			t.Fatal(err)
		}
		req := Request{Action: "orchestrator-apply", AdmissionOnly: true, WorkspaceName: "scoped", ScopeOwnerLabel: tc.owner, Order: spec}
		if r, err := Handle(req); err == nil || !strings.Contains(err.Error(), tc.want) || r.ScopeRegistered {
			t.Fatalf("%+v: %+v, %v", tc, r, err)
		}
	}
	if exists(OrchestratorConfigDir()) {
		t.Fatal("refused registrations wrote state")
	}
}

func TestOrchestratorScopeHostRewritePreservesAdmissionFields(t *testing.T) {
	original := strings.Replace(orchestratorExample, "reply: owner", "host: &owner oven\nreply: *owner", 1)
	original = strings.Replace(original, "repos: [app]", "repos: [FALSE]", 1)
	original = strings.Replace(original, "[say, rework, retry, validate, dispatch]", "[]", 1)
	copy, err := OrchestratorSpecForHost(original, "mini")
	if err != nil {
		t.Fatal(err)
	}
	s, err := ParseOrchestrator(copy)
	if err != nil || s.Host != "mini" || s.Reply != "oven" || len(s.Authority) != 0 ||
		len(s.Manages.Repos) != 1 || s.Manages.Repos[0] != "FALSE" || s.Brief != "Keep moving." {
		t.Fatalf("host rewrite changed admission fields: %+v, %v", s, err)
	}
}

func TestScopeApplyRPCRequiresChildAdmission(t *testing.T) {
	orchHome(t)
	t.Setenv("PEERS_SELF", "mini")
	t.Setenv("HEV_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	cfg := filepath.Join(t.TempDir(), "dispatch.toml")
	t.Setenv("HEV_DISPATCH_CONFIG", cfg)
	if err := os.WriteFile(cfg, []byte("cap=2\n[hosts.oven]\nssh='oven'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	parent := strings.Replace(orchestratorExample, "name: release-train", "name: parent", 1)
	parent = strings.Replace(parent, "manages:", "allow_nested: true\nmanages:", 1)
	parent = strings.Replace(parent, `selector: "issue=REL-*"`, "names: [release-train, allowed-work]", 1)
	if _, err := ApplyOrchestrator(parent, false); err != nil {
		t.Fatal(err)
	}
	child := strings.Replace(orchestratorExample, `selector: "issue=REL-*"`, "names: [work]", 1)
	child, err := OrchestratorSpecForHost(child, "mini")
	if err != nil {
		t.Fatal(err)
	}
	req := Request{Action: "orchestrator-apply", AdmissionOnly: true, ScopeOwnerLabel: "oven", Orchestrator: "parent", Name: "release-train", Order: child}
	if r, err := Handle(req); err == nil || !strings.Contains(err.Error(), "exceeds parent") || r.ScopeRegistered {
		t.Fatalf("child exceeded parent scope: %+v, %v", r, err)
	}
	if exists(filepath.Join(OrchestratorDir(req.Name), "scope-owner.json")) {
		t.Fatal("refused child wrote a registration")
	}
	req.Order = strings.Replace(child, "[work]", "[allowed-work]", 1)
	if r, err := Handle(req); err != nil || !r.ScopeRegistered {
		t.Fatalf("child within parent scope: %+v, %v", r, err)
	}

}
