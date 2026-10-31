package dispatch

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestTargetSettingsIgnoreUnrelatedInvalidRoute(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(p, []byte("workspace='/src'\n[hosts.here]\nlocal=true\ncap=1\nharness_wrapper=['launcher']\nconfig_path='nested/config.toml'\n[hosts.unrelated]\ncap='invalid'\n"), 0600)
	c, err := loadTarget(p, "here")
	if err != nil {
		t.Fatal(err)
	}
	if c.Hosts["here"].ConfigPath != filepath.Join(filepath.Dir(p), "nested/config.toml") {
		t.Fatal(c)
	}
	m := Meta{Order: Order{Host: "here", Repo: "project"}}
	resolveLaunch(&m, c)
	os.WriteFile(p, []byte("invalid = ["), 0600)
	if !reflect.DeepEqual(harnessWrapper(m), []string{"launcher"}) {
		t.Fatal("wrapper changed after admission")
	}
	if _, err = loadTarget(p, "here"); err == nil {
		t.Fatal("malformed admission config accepted")
	}
}

func TestAdmissionPreservesOriginalRepoAndExistingBranch(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	cfg := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(cfg, []byte("workspace='/missing-workspace'"), 0600)
	o, err := Parse(good)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Handle(Request{Action: "start", Name: o.Name, Order: good, Meta: Meta{Order: o, ConfigPath: cfg}, Cap: 100000})
	if err == nil {
		t.Fatal("expected failure")
	}
	var m Meta
	if err = readJSON(filepath.Join(Root(), o.Name, "meta.json"), &m); err != nil {
		t.Fatal(err)
	}
	if m.Repo != o.Repo || m.ResolvedRepo != resolveRepo(o.Repo, "/missing-workspace") {
		t.Fatal(m)
	}
	// An existing branch belongs to someone else, even if its name matches.
	repo := t.TempDir()
	for _, args := range [][]string{{"init", "--initial-branch=main"}, {"-c", "user.name=Example", "-c", "user.email=example@example.com", "commit", "--allow-empty", "-m", "Initial"}, {"branch", m.Branch}} {
		if _, err = command(repo, append([]string{"git"}, args...)...); err != nil {
			t.Fatal(err)
		}
	}
	m.ResolvedRepo = repo
	WriteJSON(filepath.Join(Root(), o.Name, "meta.json"), m)
	if _, err = Handle(Request{Action: "clean", Name: o.Name}); err != nil {
		t.Fatal(err)
	}
	if _, err = command(repo, "git", "show-ref", "--verify", "refs/heads/"+m.Branch); err != nil {
		t.Fatal("deleted unowned branch", err)
	}
}

func TestLaunchUnconfirmedGraceAndInspection(t *testing.T) {
	dir := t.TempDir()
	r := Result{LaunchUnconfirmed: true, ExitCode: 1, Start: time.Now()}
	reconcileLaunch(dir, &r)
	if !r.LaunchUnconfirmed || State(&r, false) != "launch-unconfirmed" {
		t.Fatal(r)
	}
	r.Start = time.Now().Add(-2 * launchGrace)
	reconcileLaunch(dir, &r)
	if r.LaunchUnconfirmed || r.AdmissionFailed || State(&r, false) != "failed" {
		t.Fatal(r)
	}
}

func TestMissingValidatorParentRouteIsCleanable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	p := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(p, []byte("[hosts.here]\nlocal=true\ncap=1\n"), 0600)
	o, err := Parse(good)
	if err != nil {
		t.Fatal(err)
	}
	o.Name = "example-v1"
	o.Host = "here"
	_, err = Handle(Request{Action: "start", Name: o.Name, Order: good, Cap: 100000, Meta: Meta{Order: o, Parent: "example", ParentHostName: "missing", ConfigPath: p}})
	if err == nil {
		t.Fatal("missing parent accepted")
	}
	var r Result
	if err = readJSON(filepath.Join(Root(), o.Name, "result.json"), &r); err != nil || !r.AdmissionFailed {
		t.Fatalf("%+v %v", r, err)
	}
	if _, err = Handle(Request{Action: "clean", Name: o.Name}); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(Root(), o.Name)); !os.IsNotExist(err) {
		t.Fatal("record retained", err)
	}
}

func TestUnresolvedLaunchSettingsFailClosed(t *testing.T) {
	dir := t.TempDir()
	WriteJSON(filepath.Join(dir, "meta.json"), Meta{ConfigPath: "missing"})
	if _, err := launchEnvironment(dir); err == nil {
		t.Fatal("unresolved launch allowed")
	}
}

func TestRelativeConfigPathIsAbsolute(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	os.WriteFile(p, []byte("[hosts.here]\nlocal=true\ncap=1\nconfig_path='child.toml'\n"), 0600)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(cwd, p)
	if err != nil {
		t.Fatal(err)
	}
	c, err := Load(relative)
	if err != nil {
		t.Fatal(err)
	}
	if c.Hosts["here"].ConfigPath != filepath.Join(dir, "child.toml") {
		t.Fatal(c.Hosts["here"].ConfigPath)
	}
}

func TestMissingLogWithConfigPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	dir := filepath.Join(Root(), "example")
	os.MkdirAll(dir, 0700)
	WriteJSON(filepath.Join(dir, "meta.json"), Meta{ConfigPath: "config.toml", Order: Order{Name: "example"}})
	r, err := Handle(Request{Action: "logs", Name: "example"})
	if err != nil || r.Log != "" || r.Active {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestLaunchEnvironmentStoresNamesOnly(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-secret-value")
	m := Meta{Order: Order{Host: "here"}}
	resolveLaunch(&m, Config{Hosts: map[string]Host{"here": {Env: map[string]string{"CUSTOM_KEY": "never-store-this"}}}})
	dir := t.TempDir()
	WriteJSON(filepath.Join(dir, "meta.json"), m)
	b, _ := os.ReadFile(filepath.Join(dir, "meta.json"))
	if strings.Contains(string(b), "test-secret-value") || strings.Contains(string(b), "never-store-this") {
		t.Fatal("persisted values")
	}
	env, err := launchEnvironment(dir)
	if err != nil || !slices.Contains(env, "OPENAI_API_KEY=test-secret-value") {
		t.Fatalf("environment missing: %v", err)
	}
}

func TestCleanReconcilesUnconfirmedLaunch(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	dir := filepath.Join(Root(), "example")
	os.MkdirAll(dir, 0700)
	m := Meta{Order: Order{Name: "example"}, Start: time.Now()}
	WriteJSON(filepath.Join(dir, "meta.json"), m)
	recordUnconfirmed(dir, m, errLaunchUnconfirmed)
	if _, err := Handle(Request{Action: "clean", Name: "example"}); err == nil {
		t.Fatal("clean allowed during grace")
	}
	m.Start = time.Now().Add(-2 * launchGrace)
	recordUnconfirmed(dir, m, errLaunchUnconfirmed)
	if _, err := Handle(Request{Action: "clean", Name: "example"}); err != nil {
		t.Fatal(err)
	}
	var r Result
	readJSON(filepath.Join(dir, "result.json"), &r)
	if State(&r, false) != "failed" || r.AdmissionFailed {
		t.Fatal(r)
	}
}
