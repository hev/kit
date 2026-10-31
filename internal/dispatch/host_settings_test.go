package dispatch

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestResolveRepo(t *testing.T) {
	for _, repo := range []string{"/srv/project", `C:\repos\project`, "C:/repos/project", `\\buildbox\repos\project`} {
		if got := resolveRepo(repo, "/ignored"); got != repo {
			t.Errorf("absolute %q became %q", repo, got)
		}
	}
	for _, workspace := range []string{"/srv/repos", `C:\repos`, "C:/repos"} {
		if got, want := resolveRepo("project", workspace), filepath.Join(workspace, "project"); got != want {
			t.Errorf("%q != %q", got, want)
		}
	}
	if got := resolveRepo("project", ""); got != filepath.Join(Expand("~/workspace"), "project") {
		t.Fatal(got)
	}
	if got := resolveRepo("~/project", "/ignored"); got != Expand("~/project") {
		t.Fatal(got)
	}
}

func TestHarnessCommandArgv(t *testing.T) {
	wrapper := []string{"credential launcher", "--scope", `a "quoted" scope`}
	args := []string{"exec", "--model", "a model", `prompt with "quotes" and trailing\`}
	c := harnessCommand(wrapper, "codex", args)
	want := []string{"credential launcher", "--scope", `a "quoted" scope`, "--", "codex", "exec", "--model", "a model", `prompt with "quotes" and trailing\`}
	if !reflect.DeepEqual(c.Args, want) {
		t.Fatalf("%q != %q", c.Args, want)
	}
	if got := harnessCommand(nil, "codex", args).Args; !reflect.DeepEqual(got, append([]string{"codex"}, args...)) {
		t.Fatal(got)
	}
}

func TestAdmissionWorkspaceFailureAndClean(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	cfg := filepath.Join(t.TempDir(), "dispatch.toml")
	target := filepath.Join(t.TempDir(), "target-workspace")
	if err := os.WriteFile(cfg, []byte("workspace = '"+target+"'\n[hosts.buildbox]\nlocal = true\ncap = 2\nharness_wrapper = ['launcher', '--scope']\n"), 0600); err != nil {
		t.Fatal(err)
	}
	config, err := Load(cfg)
	if err != nil || !reflect.DeepEqual(config.Hosts["buildbox"].HarnessWrapper, []string{"launcher", "--scope"}) {
		t.Fatalf("%+v %v", config, err)
	}
	for _, override := range []string{"", filepath.Join(t.TempDir(), "sender-workspace")} {
		o, err := Parse(good)
		if err != nil {
			t.Fatal(err)
		}
		q := Request{Action: "start", Name: o.Name, Order: good, Meta: Meta{Order: o, ConfigPath: cfg}, Workspace: override, Cap: 100000}
		_, err = Handle(q)
		workspace := override
		if workspace == "" {
			workspace = target
		}
		if err == nil || !strings.Contains(err.Error(), filepath.Join(workspace, "kit")) {
			t.Fatalf("expected failed git in %s: %v", workspace, err)
		}
		list, err := Handle(Request{Action: "list"})
		if err != nil {
			t.Fatal(err)
		}
		if len(list.Rows) != 1 || list.Rows[0].State != "failed" {
			t.Fatalf("%+v", list)
		}
		show, err := Handle(Request{Action: "show", Name: o.Name})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(show.Files["result.json"], `"admission_failed": true`) || !strings.Contains(show.Files["result.json"], `"error":`) {
			t.Fatal(show.Files)
		}
		if _, err = Handle(Request{Action: "clean", Name: o.Name}); err != nil {
			t.Fatal(err)
		}
		if _, err = os.Stat(filepath.Join(Root(), o.Name)); !os.IsNotExist(err) {
			t.Fatalf("record retained: %v", err)
		}
	}
}

func TestTargetHarnessWrapperAndSenderOverride(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "dispatch.toml")
	if err := os.WriteFile(cfg, []byte("[hosts.buildbox]\nlocal=true\ncap=2\nharness_wrapper=['target-launcher', '--scope']\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, parent := range []string{"", "implementation"} {
		m := Meta{ConfigPath: cfg, Order: Order{Host: "buildbox"}, Parent: parent}
		c, err := loadTarget(cfg, m.Host)
		if err != nil {
			t.Fatal(err)
		}
		resolveLaunch(&m, c)
		if got := harnessWrapper(m); !reflect.DeepEqual(got, []string{"target-launcher", "--scope"}) {
			t.Fatal(got)
		}
		m.HarnessWrapper = []string{"sender-launcher"}
		if got := harnessWrapper(m); !reflect.DeepEqual(got, m.HarnessWrapper) {
			t.Fatal(got)
		}
		m.HarnessWrapper = []string{}
		resolveLaunch(&m, c)
		path := filepath.Join(t.TempDir(), "meta.json")
		if err := WriteJSON(path, m); err != nil {
			t.Fatal(err)
		}
		var restored Meta
		if err := readJSON(path, &restored); err != nil {
			t.Fatal(err)
		}
		if got := harnessWrapper(restored); !reflect.DeepEqual(got, []string{"target-launcher", "--scope"}) {
			t.Fatalf("target wrapper must win over an empty sender list: %v", got)
		}
	}
}

func TestCleanAdmittedRecordRetainsReceipt(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	dir := filepath.Join(Root(), "example")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := WriteJSON(filepath.Join(dir, "meta.json"), Meta{Order: Order{Name: "example"}}); err != nil {
		t.Fatal(err)
	}
	if err := WriteJSON(filepath.Join(dir, "result.json"), Result{ExitCode: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := Handle(Request{Action: "clean", Name: "example"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "result.json")); err != nil {
		t.Fatal(err)
	}
}

func TestCleanFailedAdmissionRemovesCreatedBranch(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	repo := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		if _, err := command(repo, append([]string{"git"}, args...)...); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "--initial-branch=main")
	git("-c", "user.name=Example", "-c", "user.email=example@example.com", "commit", "--allow-empty", "-m", "Initial")
	dir := filepath.Join(Root(), "example")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	git("worktree", "add", "-b", "d/example", filepath.Join(dir, "wt"))
	if err := WriteJSON(filepath.Join(dir, "meta.json"), Meta{Order: Order{Name: "example", Repo: repo}, Branch: "d/example", ResolvedRepo: repo, BranchCreated: true, WorktreeCreated: true}); err != nil {
		t.Fatal(err)
	}
	if err := WriteJSON(filepath.Join(dir, "result.json"), Result{AdmissionFailed: true, ExitCode: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := Handle(Request{Action: "clean", Name: "example"}); err != nil {
		t.Fatal(err)
	}
	if _, err := command(repo, "git", "show-ref", "--verify", "refs/heads/d/example"); err == nil {
		t.Fatal("failed admission branch retained, blocking name reuse")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("record retained: %v", err)
	}
}
