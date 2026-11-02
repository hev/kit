package dispatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepoIdentity(t *testing.T) {
	for _, pair := range [][2]string{
		{"git@example.com:team/project.git", "https://example.com/team/project"},
		{"ssh://git@EXAMPLE.com/team/project.git/", "https://example.com/team/project.git"},
		{`C:\SRC\Project.git`, "c:/src/project"},
		{`\\SERVER\Share\Project.git`, "//server/share/project"},
	} {
		if repoIdentity(pair[0]) != repoIdentity(pair[1]) {
			t.Errorf("identities differ: %q %q", pair[0], pair[1])
		}
	}
	if repoIdentity("https://example.com/Team/project") == repoIdentity("https://example.com/team/project") {
		t.Fatal("repository path case lost")
	}
}

func TestRepoAdmission(t *testing.T) {
	repo := t.TempDir()
	for _, args := range [][]string{{"init"}, {"remote", "add", "origin", "git@example.com:team/project.git"}} {
		if _, err := command(repo, append([]string{"git"}, args...)...); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name    string
		repos   []string
		allowed bool
	}{
		{"allowed", []string{"https://example.com/team/project"}, true},
		{"refused", []string{"https://example.com/other/project"}, false},
		{"no-list", nil, true},
		{"empty-list", []string{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := admitRepo(Admission{Repos: tc.repos}, repo, "project", "here")
			if (err == nil) != tc.allowed {
				t.Fatalf("allowed=%v: %v", tc.allowed, err)
			}
		})
	}
}

func TestTargetRepoRefusalBeforeGitMutation(t *testing.T) {
	for _, setting := range []string{"repos = []", "repos = ['https://example.com/other/project']"} {
		t.Run(setting, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			repo := filepath.Join(t.TempDir(), "kit")
			if err := os.Mkdir(repo, 0700); err != nil {
				t.Fatal(err)
			}
			for _, args := range [][]string{{"init"}, {"remote", "add", "origin", "https://example.com/team/project.git"}} {
				if _, err := command(repo, append([]string{"git"}, args...)...); err != nil {
					t.Fatal(err)
				}
			}
			cfg := filepath.Join(t.TempDir(), "config.toml")
			t.Setenv("HEV_DISPATCH_CONFIG", cfg)
			if err := os.WriteFile(cfg, []byte("[hosts.buildbox]\nlocal=true\ncap=2\n[admission]\n"+setting+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			o, err := Parse(good)
			if err != nil {
				t.Fatal(err)
			}
			// The repo label and workspace are sender controlled; origin is checked locally.
			_, err = Handle(Request{Action: "start", Name: o.Name, Order: good, Meta: Meta{Order: o, ConfigPath: cfg}, Workspace: filepath.Dir(repo), Cap: 100000})
			if err == nil || !strings.Contains(err.Error(), "not admitted on host") {
				t.Fatalf("expected refusal: %v", err)
			}
			dir := filepath.Join(Root(), o.Name)
			if _, err := os.Stat(filepath.Join(dir, "wt")); !os.IsNotExist(err) {
				t.Fatalf("worktree exists: %v", err)
			}
			for _, file := range []string{"FETCH_HEAD", "refs/heads/d/" + o.Name} {
				if _, err := os.Stat(filepath.Join(repo, ".git", file)); !os.IsNotExist(err) {
					t.Fatalf("Git mutation %s: %v", file, err)
				}
			}
			list, err := Handle(Request{Action: "list"})
			if err != nil {
				t.Fatal(err)
			}
			if len(list.Rows) != 1 || list.Rows[0].Error == "" || list.Rows[0].State != "failed" {
				t.Fatalf("missing refusal: %+v", list)
			}
			show, err := Handle(Request{Action: "show", Name: o.Name})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(show.Files["result.json"], "not admitted on host") {
				t.Fatal(show.Files)
			}
		})
	}
}

func TestTargetRepoAdmissionAllowsFetch(t *testing.T) {
	for _, restricted := range []bool{true, false} {
		t.Run(map[bool]string{true: "allowed", false: "no-list"}[restricted], func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			origin := t.TempDir()
			for _, args := range [][]string{{"init", "--initial-branch=main"}, {"-c", "user.name=Example", "-c", "user.email=example@example.com", "commit", "--allow-empty", "-m", "Initial"}} {
				if _, err := command(origin, append([]string{"git"}, args...)...); err != nil {
					t.Fatal(err)
				}
			}
			workspace := t.TempDir()
			repo := filepath.Join(workspace, "kit")
			if _, err := command("", "git", "clone", origin, repo); err != nil {
				t.Fatal(err)
			}
			cfg := filepath.Join(t.TempDir(), "dispatch.toml")
			t.Setenv("HEV_DISPATCH_CONFIG", cfg)
			policy := ""
			if restricted {
				policy = "[admission]\nrepos=['" + filepath.ToSlash(origin) + "']\n"
			}
			if err := os.WriteFile(cfg, []byte("[hosts.buildbox]\nlocal=true\ncap=2\n"+policy), 0600); err != nil {
				t.Fatal(err)
			}
			// A missing base stops setup after fetch, before any supervisor can launch.
			text := strings.Replace(good, "repo: kit", "repo: kit\nbase: missing-admission-test", 1)
			o, err := Parse(text)
			if err != nil {
				t.Fatal(err)
			}
			_, err = Handle(Request{Action: "start", Name: o.Name, Order: text, Meta: Meta{Order: o, ConfigPath: cfg}, Workspace: workspace, Cap: 100000})
			if err == nil || strings.Contains(err.Error(), "not admitted") {
				t.Fatalf("expected missing base after admission: %v", err)
			}
			if _, err = os.Stat(filepath.Join(repo, ".git", "FETCH_HEAD")); err != nil {
				t.Fatalf("fetch not reached: %v", err)
			}
		})
	}
}

func TestRestrictedRepoUnreadableOrigin(t *testing.T) {
	repo := t.TempDir()
	if _, err := command(repo, "git", "init"); err != nil {
		t.Fatal(err)
	}
	if err := admitRepo(Admission{Repos: []string{"https://example.com/team/project"}}, repo, "project", "bb"); err == nil {
		t.Fatal("missing origin admitted")
	}
}

func TestTargetPolicyIgnoresSenderAliasAndConfig(t *testing.T) {
	for _, kind := range []string{"worker", "validator", "null-config"} {
		t.Run(kind, func(t *testing.T) {
			validator := kind == "validator"
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			target := filepath.Join(t.TempDir(), "target.toml")
			t.Setenv("HEV_DISPATCH_CONFIG", target)
			if err := os.WriteFile(target, []byte("[admission]\nrepos=['https://example.com/allowed/project']\n[hosts.buildbox]\nlocal=true\ncap=2\n"), 0600); err != nil {
				t.Fatal(err)
			}
			sender := filepath.Join(t.TempDir(), "sender.toml")
			// Sender chooses an unrestricted config with enough routing for a validator.
			if err := os.WriteFile(sender, []byte("[hosts.source]\nlocal=true\ncap=2\n"), 0600); err != nil {
				t.Fatal(err)
			}
			repo := t.TempDir()
			for _, args := range [][]string{{"init"}, {"remote", "add", "origin", "https://example.com/refused/project"}} {
				if _, err := command(repo, append([]string{"git"}, args...)...); err != nil {
					t.Fatal(err)
				}
			}
			orderText := strings.Replace(good, "repo: kit", "repo: "+filepath.ToSlash(repo), 1)
			o, err := Parse(orderText)
			if err != nil {
				t.Fatal(err)
			}
			o.Host, o.Repo = "bb", repo
			m := Meta{Order: o, ConfigPath: sender}
			if kind == "null-config" {
				m.ConfigPath = os.DevNull
			}
			if validator {
				m.Parent, m.ParentHostName, m.Name = o.Name, "source", o.Name+"-v1"
			}
			_, err = Handle(Request{Action: "start", Name: m.Name, Order: orderText, Meta: m, Workspace: filepath.Dir(repo), Cap: 100000})
			if err == nil || !strings.Contains(err.Error(), "not admitted on host \"bb\"") {
				t.Fatalf("sender bypass: %v", err)
			}
			if _, err := os.Stat(filepath.Join(Root(), m.Name, "wt")); !os.IsNotExist(err) {
				t.Fatalf("worktree created: %v", err)
			}
			if _, err := os.Stat(filepath.Join(repo, ".git", "FETCH_HEAD")); !os.IsNotExist(err) {
				t.Fatalf("fetch reached: %v", err)
			}
		})
	}
}

func TestRelaunchChecksCurrentTargetPolicy(t *testing.T) {
	for _, action := range []string{"rework", "say"} {
		t.Run(action, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			cfg := filepath.Join(t.TempDir(), "target.toml")
			t.Setenv("HEV_DISPATCH_CONFIG", cfg)
			if err := os.WriteFile(cfg, []byte("[admission]\nrepos=[]\n"), 0600); err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(Root(), "example")
			if err := os.MkdirAll(filepath.Join(dir, "wt"), 0700); err != nil {
				t.Fatal(err)
			}
			m := Meta{Order: Order{Name: "example", Repo: "project", Host: "bb"}, ConfigPath: os.DevNull}
			if err := WriteJSON(filepath.Join(dir, "meta.json"), m); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "order.md"), []byte(good), 0600); err != nil {
				t.Fatal(err)
			}
			if err := WriteJSON(filepath.Join(dir, "result.json"), Result{ExitCode: 0, PR: "https://example.com/team/project/pull/1"}); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(filepath.Join(dir, "result.json"))
			if err != nil {
				t.Fatal(err)
			}
			_, err = Handle(Request{Action: action, Name: "example", Note: "try again", Cap: 100000})
			if err == nil || !strings.Contains(err.Error(), "not admitted") {
				t.Fatalf("relaunch bypass: %v", err)
			}
			content, err := os.ReadFile(filepath.Join(dir, "order.md"))
			if err != nil || string(content) != good {
				t.Fatal("refusal mutated order", err)
			}
			after, err := os.ReadFile(filepath.Join(dir, "result.json"))
			if err != nil || string(after) != string(before) {
				t.Fatalf("refusal mutated completed result: %s %v", after, err)
			}

		})
	}
}
