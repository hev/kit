package dispatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOrchestratorSnapshotRequiresScopedHosts(t *testing.T) {
	for _, tc := range []struct {
		name, scope, failed string
		inventory, fatal    bool
	}{
		{"unrelated discovery failure", "runs: mine", "other", true, false},
		{"unrelated fixed host", "runs: {names: [mini/work]}", "other", false, false},
		{"separate workspace", "runs: mine", "client", false, false},
		{"managed host", "runs: mine, hosts: [other]", "other", true, true},
		{"qualified run", "runs: {names: [mini/work]}", "mini", true, true},
		{"workspace host", "runs: mine", "mini", true, true},
		{"owner host", "runs: mine", "test-host", true, true},
		{"tracked host", "runs: mine", "mini", true, true},
		{"follow snapshot host", "runs: {names: [work]}", "mini", true, true},
		{"unrelated follow host", "runs: {names: [work]}", "other", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			orchHome(t)
			t.Setenv("HEV_ORDERS_DIR", t.TempDir())
			path := filepath.Join(t.TempDir(), "config.toml")
			t.Setenv("HEV_CONFIG", path)
			if err := os.WriteFile(path, []byte("[workspaces.client]\nlogin = 'own'\nhosts = ['client']\n[workspaces.restricted]\nhosts = ['test-host', 'mini']\n"), 0600); err != nil {
				t.Fatal(err)
			}
			s, err := ParseOrchestrator("---\nkind: orchestrator\nname: test\ngoal: Observe work\nreply: owner\nmanages: {" + tc.scope + ", repos: [kit]}\n---\n")
			if err != nil {
				t.Fatal(err)
			}
			work := Row{Meta: Meta{Order: Order{Name: "work", Repo: "kit", Reply: "owner"}}}
			switch tc.name {
			case "workspace host":
				s.WorkspaceName = "restricted"
			case "tracked host":
				dir := filepath.Join(OrchestratorDir(s.Name), "rounds", "001")
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
				packet := OrchestratorPacket{Evidence: []OrchestratorEvidence{{Host: "mini", Row: work}}}
				if err := WriteJSON(filepath.Join(dir, "packet.json"), packet); err != nil {
					t.Fatal(err)
				}
				if err := WriteJSON(filepath.Join(OrchestratorDir(s.Name), "state.json"), OrchestratorState{Rounds: 1}); err != nil {
					t.Fatal(err)
				}
			case "follow snapshot host", "unrelated follow host":
				s.Name = "follow-test"
				s.Brief = FollowPlanMarker + `{"runs":[{"host":"mini","name":"work"}]}` + "\n-->"
			}
			cfg := Config{Hosts: map[string]Host{
				"test-host": {Local: true},
				"mini":      {SSH: "mini"},
				"other":     {SSH: "other"},
				"client":    {SSH: "client"},
			}}
			cfg, required, err := orchestratorSnapshotHosts(s, cfg)
			if err != nil {
				t.Fatal(err)
			}
			if _, present := cfg.Hosts[tc.failed]; present != tc.inventory {
				t.Fatalf("inventory hosts = %v, scanning %s should be %t", cfg.Hosts, tc.failed, tc.inventory)
			}
			inv := map[string]Response{}
			for name := range cfg.Hosts {
				switch name {
				case tc.failed:
					inv[name] = Response{Error: "offline"}
				case "mini":
					inv[name] = Response{Rows: []Row{work, {Meta: Meta{Order: Order{Name: "different-repo", Repo: "other", Reply: "owner"}}}}}
				}
			}
			rows, err := orchestratorSnapshotRows(s, inv, required)
			if tc.fatal {
				if err == nil || !strings.Contains(err.Error(), "cannot snapshot "+tc.failed+": offline") {
					t.Fatalf("expected closed snapshot, got rows %v and error %v", rows, err)
				}
			} else if err != nil || len(rows) != 1 || rows[0].Host != "mini" || rows[0].Row.Name != "work" {
				t.Fatalf("expected managed evidence, got rows %v and error %v", rows, err)
			}
		})
	}
}

func TestOrchestratorQualifiedRunMatchesOnlyItsHost(t *testing.T) {
	spec := "---\nkind: orchestrator\nname: test\ngoal: Observe work\nmanages: {runs: {names: [%s]}}\n---\n"
	s, err := ParseOrchestrator(strings.Replace(spec, "%s", "mini/work", 1))
	if err != nil {
		t.Fatal(err)
	}
	row := Row{Meta: Meta{Order: Order{Name: "work"}}}
	rows, err := orchestratorSnapshotRows(s, map[string]Response{
		"mini": {Rows: []Row{row}}, "other": {Rows: []Row{row}},
	}, nil)
	if err != nil || len(rows) != 1 || rows[0].Host != "mini" {
		t.Fatalf("expected only mini/work, got %v, %v", rows, err)
	}
	if _, err := ParseOrchestrator(strings.Replace(spec, "%s", "mini/sub/work", 1)); err == nil {
		t.Fatal("accepted malformed host-qualified run")
	}
}
