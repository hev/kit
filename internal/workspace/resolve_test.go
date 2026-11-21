package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolutionPrecedenceAndBoundaries(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HEV_WORKSPACE", "")
	parent := filepath.Join(home, "workspace")
	client := filepath.Join(parent, "clients", "bristlecone")
	if err := os.MkdirAll(client, 0700); err != nil {
		t.Fatal(err)
	}
	cfg := Config{ActiveWorkspace: "bcc", Workspaces: map[string]Workspace{
		"hev": {Project: Project{Dir: "~/workspace"}, Hosts: []string{"tower", "oven", "mini"}},
		"bcc": {Project: Project{Dir: "~/workspace/clients/bristlecone"}, Hosts: []string{"win-oven"}},
		"old": {Archived: true, Project: Project{Dir: client}},
	}}
	for _, tc := range []struct{ name, flag, env, cwd, want, source string }{
		{"nested", "", "", filepath.Join(client, "foo"), "bcc", "pwd"},
		{"component boundary", "", "", client + "-other", "hev", "pwd"},
		{"env wins", "", "hev", client, "hev", "env"},
		{"flag wins", "bcc", "hev", parent, "bcc", "flag"},
		{"explicit unknown", "missing", "hev", client, "", ""},
		{"env unknown", "", "missing", client, "", ""},
		{"archive excluded", "old", "", client, "", ""},
		{"active fallback", "", "", home, "bcc", "active"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HEV_WORKSPACE", tc.env)
			got, err := cfg.Resolve(tc.flag, tc.cwd)
			if tc.want == "" {
				if err == nil || got.Name != "" {
					t.Fatalf("invalid selection resolved %+v", got)
				}
			} else if err != nil || got.Name != tc.want || got.Source != tc.source {
				t.Fatalf("got %+v, %v", got, err)
			}
		})
	}
	if link := filepath.Join(home, "link"); os.Symlink(client, link) == nil {
		w, err := cfg.Resolve("", link)
		if err != nil || w.Name != "bcc" || w.Source != "pwd" {
			t.Fatalf("symlink resolved %+v, %v", w, err)
		}
	}
	for _, name := range []string{"bcc", "hev"} {
		w, err := cfg.Select(name, "flag")
		if err != nil || w.AllowsHost("win-oven") != (name == "bcc") || w.AllowsHost("oven") != (name == "hev") {
			t.Fatalf("workspace %s: %+v, %v", name, w, err)
		}
	}
}

func TestWorkspaceUsePreservesOtherSettingsAndRefusesUnknown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("unrelated = 'keep'\n[workspaces.bcc]\ndir = '~/client'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Use(path, "bcc"); err != nil {
		t.Fatal(err)
	}
	if err := Use(path, "missing"); err == nil {
		t.Fatal("accepted unknown workspace")
	}
	raw, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(raw), `unrelated = "keep"`) || !strings.Contains(string(raw), `active_workspace = "bcc"`) {
		t.Fatalf("config did not preserve settings: %s, %v", raw, err)
	}
}

func TestWorkspaceStorageTargetsCannotOverlap(t *testing.T) {
	cfg := Config{Workspaces: map[string]Workspace{"bcc": {Layer: Layer{TracesNamespace: "client-traces"}}}}
	if err := cfg.CheckNamespace("hev", "client-traces-sessions"); err == nil {
		t.Fatal("accepted another workspace's derived namespace")
	}
	if err := cfg.CheckNamespace("hev", "hev-traces"); err != nil {
		t.Fatal(err)
	}
	cfg.Workspaces["bcc"] = Workspace{Layer: Layer{BoardNamespace: "factory-board"}}
	if err := validateStorage(cfg.Workspaces); err == nil {
		t.Fatal("accepted the legacy board namespace in a client workspace")
	}
}
