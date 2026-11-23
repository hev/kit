package dispatch

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/hev/kit/internal/hostlabel"
	"github.com/hev/kit/internal/workspace"
)

// A discovery host can provide evidence while reachable. Only explicitly
// scoped hosts and hosts with known managed work must answer every round.
func orchestratorSnapshotHosts(s OrchestratorSpec, cfg Config) (Config, map[string]bool, error) {
	workspaces, err := workspace.Read(workspace.ConfigPath())
	if err != nil {
		return cfg, nil, err
	}
	workspaceName := workspaceOfRun(s.WorkspaceName)
	if s.WorkspaceName != "" || len(workspaces.Workspaces[workspaceName].Hosts) != 0 {
		selected, err := workspaces.Select(workspaceName, "run")
		if err != nil {
			return cfg, nil, err
		}
		cfg.ScopeWorkspace(selected)
	}

	self := hostlabel.Self(os.Hostname)
	required := orchestratorKnownHosts(s, cfg.Hosts)
	hasLocal := false
	for name, h := range cfg.Hosts {
		if h.Local || hostlabel.Same(name, self) {
			required[name] = true
			hasLocal = true
		}
	}
	if !hasLocal && cfg.SelectedWorkspace.AllowsHost(self) {
		if cfg.Hosts == nil {
			cfg.Hosts = map[string]Host{}
		}
		cfg.Hosts[self] = Host{Local: true}
		required[self] = true
	}
	require := func(host string) error {
		found := false
		for name := range cfg.Hosts {
			if hostlabel.Same(name, host) {
				required[name] = true
				found = true
			}
		}
		// An owner with a local route can have a different configured label
		// from its OS hostname.
		if found || (hostlabel.Same(host, self) && hasLocal) {
			return nil
		}
		return fmt.Errorf("cannot snapshot %s: host has no route in orchestrator workspace", host)
	}
	if s.Host != "" {
		if err := require(s.Host); err != nil {
			return cfg, nil, err
		}
	}
	for _, host := range cfg.SelectedWorkspace.Hosts {
		if err := require(host); err != nil {
			return cfg, nil, err
		}
	}
	for _, host := range s.Manages.Hosts {
		if err := require(host); err != nil {
			return cfg, nil, err
		}
	}
	fixed, followHosts := followSnapshotHosts(s)
	discover := s.Manages.Runs.Selector != ""
	for _, ref := range s.Manages.Runs.Names {
		host, _ := splitScopedRun(ref)
		if host == "" {
			discover = true
		} else if err := require(host); err != nil {
			return cfg, nil, err
		}
	}
	for _, host := range followHosts {
		if err := require(host); err != nil {
			return cfg, nil, err
		}
	}

	discover = discover && !fixed && len(cfg.SelectedWorkspace.Hosts) == 0 && len(s.Manages.Hosts) == 0
	for name := range cfg.Hosts {
		if required[name] {
			continue
		}
		if !discover {
			delete(cfg.Hosts, name)
			continue
		}
		for other, w := range workspaces.Workspaces {
			if other != workspaceName && !w.Archived && w.Login == "own" && len(w.Hosts) != 0 {
				if (workspace.Selection{Workspace: w}).AllowsHost(name) {
					delete(cfg.Hosts, name)
					break
				}
			}
		}
	}
	return cfg, required, nil
}

// Follow-through specs already carry host-qualified run identities in their
// plan, including specs created before host-qualified names were supported.
func followSnapshotHosts(s OrchestratorSpec) (bool, []string) {
	var plan struct {
		Runs []struct{ Host, Name string }
	}
	raw, ok := FollowPlanJSON(s.Brief)
	if !IsFollowName(s.Name) || !ok || json.Unmarshal([]byte(raw), &plan) != nil || len(plan.Runs) == 0 {
		return false, nil
	}
	hosts := make([]string, 0, len(plan.Runs))
	for _, run := range plan.Runs {
		if run.Host == "" {
			return false, nil
		}
		hosts = append(hosts, run.Host)
	}
	return true, hosts
}

func orchestratorSnapshotRows(s OrchestratorSpec, inv map[string]Response, required map[string]bool) ([]InventoryEvidence, error) {
	for host, response := range inv {
		if response.Error != "" && !required[host] {
			delete(inv, host)
		}
	}
	return selectInventoryEvidence(inv, func(host string, row Row) bool {
		return row.Kind != "orch" && s.includesOnHost(row.Order, host)
	})
}
