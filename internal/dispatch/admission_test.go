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
		{`C:\src\project.git`, "c:/src/project"},
		{`\\server\share\project.git`, "//server/share/project"},
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
			err := admitRepo(Host{Admission: Admission{Repos: tc.repos}}, repo, "project", "here")
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
			if err := os.WriteFile(cfg, []byte("[hosts.buildbox]\nlocal=true\ncap=2\n[hosts.buildbox.admission]\n"+setting+"\n"), 0600); err != nil {
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
			policy := ""
			if restricted {
				policy = "[hosts.buildbox.admission]\nrepos=['" + filepath.ToSlash(origin) + "']\n"
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
