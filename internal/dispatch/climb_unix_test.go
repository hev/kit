//go:build !windows

package dispatch

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// The worker harness a supervisor starts gets exactly the minimal
// environment for env: minimal and class: climb, and today's environment for
// a standard run. The fake harness records the environment it received.
func TestClimbWorkerEnvironmentEndToEnd(t *testing.T) {
	for _, tc := range []struct {
		name  string
		order Order
	}{
		{"standard", Order{}},
		{"minimal", Order{Env: EnvMinimal}},
		{"climb", Order{Class: ClimbClass, Env: EnvMinimal}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			orchHome(t)
			allowChildProbes(t)
			setClimbHostEnv(t)
			real := realHomeFixture(t)
			stubGitIdentity(t, "Example", "example@example.com")
			fakeHarnessSecrets(t, "env-ref-gh-token", nil)
			bin, dump := t.TempDir(), t.TempDir()
			installEnvDump(t, bin, "claude")
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("DISPATCH_ENV_DUMP", dump)
			// env_refs cannot put a refused name back into a minimal worker.
			cfgPath := writeDispatchConfig(t, "[climb]\nworker_env = [\"DISPATCH_ENV_DUMP\"]\n[hosts.here]\nlocal = true\nenv = [\"DISPATCH_ENV_DUMP\", \"CUSTOM_HOST_KEY\"]\n[harnesses.claude.env_refs]\nGH_TOKEN = \"op://example/gh/token\"\n")
			t.Setenv("HEV_DISPATCH_CONFIG", cfgPath)
			cfg, err := loadTarget(cfgPath, "here")
			if err != nil {
				t.Fatal(err)
			}
			name := "climb-env-" + tc.name
			o := tc.order
			o.Name, o.Repo, o.Harness, o.Host, o.Timeout = name, "app", "claude", "here", 1
			m := Meta{Start: time.Now().UTC(), ConfigPath: cfgPath, Order: o}
			if err := resolveLaunch(&m, cfg); err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(Root(), name)
			if err := os.MkdirAll(filepath.Join(dir, "wt"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := WriteJSON(filepath.Join(dir, "meta.json"), m); err != nil {
				t.Fatal(err)
			}
			_ = superviseRound(name)
			env := readEnvDump(t, dump, "claude")

			if tc.name == "standard" {
				for _, n := range append([]string{"CUSTOM_HOST_KEY", "GH_TOKEN"}, climbRefusedFixture[:len(climbRefusedFixture)-1]...) {
					if !hasEnv(env, n) {
						t.Errorf("standard worker lost %s", n)
					}
				}
				if envValue(env, "HOME") != real || hasEnv(env, RunClassEnv) || hasEnv(env, RunEnvEnv) {
					t.Errorf("standard worker changed: HOME=%s class=%v", envValue(env, "HOME"), hasEnv(env, RunClassEnv))
				}
				return
			}
			for _, n := range append([]string{"CUSTOM_HOST_KEY", "USERPROFILE", "XDG_CONFIG_HOME"}, climbRefusedFixture...) {
				if hasEnv(env, n) {
					t.Errorf("minimal worker has refused %s", n)
				}
			}
			for _, n := range climbKeptFixture {
				if envValue(env, n) == "" || envValue(env, n) != os.Getenv(n) {
					t.Errorf("minimal worker lost kept %s", n)
				}
			}
			home := filepath.Join(dir, scratchHomeDir)
			if envValue(env, "HOME") != home {
				t.Fatalf("HOME=%q, want scratch %q", envValue(env, "HOME"), home)
			}
			if envValue(env, RunEnvEnv) != EnvMinimal || envValue(env, "GIT_CONFIG_NOSYSTEM") != "1" {
				t.Errorf("minimal markers missing: %v", env)
			}
			if want := map[bool]string{true: ClimbClass, false: ""}[o.Class == ClimbClass]; envValue(env, RunClassEnv) != want {
				t.Errorf("%s=%q, want %q", RunClassEnv, envValue(env, RunClassEnv), want)
			}
			for _, p := range []string{".config", ".ssh", ".git-credentials"} {
				if _, err := os.Lstat(filepath.Join(home, p)); !os.IsNotExist(err) {
					t.Errorf("scratch HOME has %s", p)
				}
			}
			if target, err := os.Readlink(filepath.Join(home, ".claude")); err != nil || target != filepath.Join(real, ".claude") {
				t.Errorf("claude login not reachable: %q %v", target, err)
			}
			var stored Meta
			if err := readJSON(filepath.Join(dir, "meta.json"), &stored); err != nil || !stored.MinimalEnv() || (o.Class == ClimbClass) != (stored.Class == ClimbClass) {
				t.Errorf("meta.json does not mark the run: %+v %v", stored.Order, err)
			}
		})
	}
}

// climbAdmissionFixture is a target with one live run, one ended run and an
// origin repository an ordinary start can fetch.
type climbAdmissionFixture struct{ workspace, repo, cfg string }

func newClimbAdmissionFixture(t *testing.T) climbAdmissionFixture {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	unsetEnv(t, RunClassEnv)
	bin := t.TempDir()
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := os.WriteFile(filepath.Join(bin, "tmux"), []byte("#!/bin/sh\nif [ \"$3\" = list-sessions ]; then echo d-live; fi\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"live", "ended"} {
		dir := filepath.Join(Root(), name)
		if err := os.MkdirAll(filepath.Join(dir, "wt"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := WriteJSON(filepath.Join(dir, "meta.json"), Meta{Order: Order{Name: name}}); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "order.md"), []byte("original"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	origin := t.TempDir()
	for _, args := range [][]string{{"init", "--initial-branch=main"}, {"-c", "user.name=Example", "-c", "user.email=example@example.com", "commit", "--allow-empty", "-m", "Initial"}} {
		if _, err := command(origin, append([]string{"git"}, args...)...); err != nil {
			t.Fatal(err)
		}
	}
	f := climbAdmissionFixture{workspace: t.TempDir()}
	f.repo = filepath.Join(f.workspace, "kit")
	if _, err := command("", "git", "clone", origin, f.repo); err != nil {
		t.Fatal(err)
	}
	f.cfg = filepath.Join(t.TempDir(), "dispatch.toml")
	t.Setenv("HEV_DISPATCH_CONFIG", f.cfg)
	if err := os.WriteFile(f.cfg, []byte("[hosts.buildbox]\nlocal=true\ncap=2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return f
}

// do sends one action as the given caller and reports what it changed.
func (f climbAdmissionFixture) do(t *testing.T, action string, q Request) (error, bool) {
	t.Helper()
	switch action {
	case "start":
		// A missing base stops setup after fetch, before any supervisor.
		text := strings.Replace(good, "repo: kit", "repo: kit\nbase: missing-admission-base", 1)
		o, err := Parse(text)
		if err != nil {
			t.Fatal(err)
		}
		q.Action, q.Name, q.Order, q.Meta, q.Workspace, q.Cap = "start", o.Name, text, Meta{Order: o, ConfigPath: f.cfg}, f.workspace, 100000
		_, err = Handle(q)
		_, fetched := os.Stat(filepath.Join(f.repo, ".git", "FETCH_HEAD"))
		return err, fetched == nil
	case "say":
		q.Action, q.Name, q.Note, q.Cap = "say", "live", "climb test note", 1
		_, err := Handle(q)
		_, wrote := os.Stat(filepath.Join(Root(), "live", "inbox.md"))
		return err, wrote == nil
	case "rework":
		q.Action, q.Name, q.Note, q.Cap = "rework", "ended", "climb test rework", 100000
		_, err := Handle(q)
		b, _ := os.ReadFile(filepath.Join(Root(), "ended", "order.md"))
		return err, strings.Contains(string(b), "climb test rework")
	case "stop":
		q.Action, q.Name = "stop", "live"
		_, err := Handle(q)
		_, stopped := os.Stat(filepath.Join(Root(), "live", "stop"))
		return err, stopped == nil
	}
	t.Fatalf("unknown action %s", action)
	return nil, false
}

// Admission refuses start, say, rework and stop from a climb unit, recognized
// by its environment marker, by the request a remote caller carried, or by
// process lineage; an ordinary caller's same request goes through.
func TestClimbUnitDispatchRefusedInAdmission(t *testing.T) {
	callers := []struct {
		name  string
		setup func(t *testing.T) Request
	}{
		{"marker", func(t *testing.T) Request { t.Setenv(RunClassEnv, ClimbClass); return Request{} }},
		{"carried", func(t *testing.T) Request { return Request{CallerClass: ClimbClass} }},
	}
	if runtime.GOOS == "linux" {
		callers = append(callers, struct {
			name  string
			setup func(t *testing.T) Request
		}{"lineage", func(t *testing.T) Request {
			// This test process stands in for a climb unit's harness.
			dir := filepath.Join(Root(), "climber")
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			if err := WriteJSON(filepath.Join(dir, "meta.json"), Meta{Order: Order{Name: "climber", Class: ClimbClass, Env: EnvMinimal}}); err != nil {
				t.Fatal(err)
			}
			if err := writeProcessRecord(filepath.Join(dir, "harness.json"), os.Getpid()); err != nil {
				t.Fatal(err)
			}
			return Request{}
		}})
	}
	for _, action := range []string{"start", "say", "rework", "stop"} {
		t.Run(action+"/ordinary", func(t *testing.T) {
			f := newClimbAdmissionFixture(t)
			err, changed := f.do(t, action, Request{})
			if err != nil && strings.Contains(err.Error(), "refused:") {
				t.Fatalf("ordinary %s refused: %v", action, err)
			}
			if !changed {
				t.Fatalf("ordinary %s did not reach the run: %v", action, err)
			}
		})
		for _, c := range callers {
			t.Run(action+"/"+c.name, func(t *testing.T) {
				f := newClimbAdmissionFixture(t)
				q := c.setup(t)
				err, changed := f.do(t, action, q)
				if err == nil || !strings.Contains(err.Error(), "refused: "+action+" from inside") {
					t.Fatalf("climb %s not refused: %v", action, err)
				}
				if c.name == "lineage" && !strings.Contains(err.Error(), "climb unit climber") {
					t.Errorf("refusal does not name the unit: %v", err)
				}
				if changed {
					t.Fatalf("refused %s still changed the run", action)
				}
			})
		}
	}
	// A non-climb run's recorded harness in the lineage is no refusal.
	t.Run("lineage/standard-run", func(t *testing.T) {
		f := newClimbAdmissionFixture(t)
		dir := filepath.Join(Root(), "ordinary")
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := WriteJSON(filepath.Join(dir, "meta.json"), Meta{Order: Order{Name: "ordinary", Env: EnvMinimal}}); err != nil {
			t.Fatal(err)
		}
		if err := writeProcessRecord(filepath.Join(dir, "harness.json"), os.Getpid()); err != nil {
			t.Fatal(err)
		}
		if err, changed := f.do(t, "stop", Request{}); err != nil || !changed {
			t.Fatalf("ordinary stop under a non-climb run: %v %v", err, changed)
		}
	})
}
