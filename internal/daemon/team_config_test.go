package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTeamConfigRestoresLocalAndProtectsKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("HEV_CONFIG", path)
	doc := map[string]any{"layer": map[string]any{"endpoint": "http://localhost:8080", "api_key": "local", "namespace": "local"}}
	team := TeamConfig{Service: "http://localhost:8787", KeyID: "machine"}
	team.Author.ID = "member"
	team.Author.Name = "Member"
	if err := WriteTeamConfig(doc, "http://localhost:8080", "test-team-traces", "machine-token", team); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("credential file must be private")
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Team == nil || cfg.Team.Author.ID != "member" || cfg.LayerAPIKey != "machine-token" {
		t.Fatal("membership not loaded")
	}
	reread, err := ReadInviteConfig()
	if err != nil {
		t.Fatal(err)
	}
	if err := ClearTeamConfig(reread); err != nil {
		t.Fatal(err)
	}
	cfg, err = LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Team != nil || cfg.LayerAPIKey != "local" {
		t.Fatal("local target not restored")
	}
}
