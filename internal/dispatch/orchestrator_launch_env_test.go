package dispatch

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// A scheduled launch carries the non-secret context that chooses the spec and
// refuses every other difference from the profile, without printing values.
func TestScheduledLaunchEnvironment(t *testing.T) {
	profile := []string{"Path=C:\\Windows", "USERPROFILE=C:\\Users\\u", "HEV_CONFIG=C:\\Users\\u\\hev.toml", "TEMP=C:\\t", "=C:=C:\\"}
	caller := []string{"PATH=C:\\tools;C:\\Windows", "USERPROFILE=C:\\custom", "HOME=C:\\custom", "TEMP=C:\\t", "SSH_CONNECTION=a b c d", "=D:=D:\\"}
	carried, refused := scheduledLaunchEnvironment(caller, profile, nil)
	want := []string{"HEV_CONFIG", "HOME=C:\\custom", "PATH=C:\\tools;C:\\Windows", "USERPROFILE=C:\\custom"}
	if !reflect.DeepEqual(carried, want) || len(refused) != 0 {
		t.Fatalf("carried %q refused %q", carried, refused)
	}
	_, refused = scheduledLaunchEnvironment(append(caller, "GH_TOKEN=synthetic-token", "EXTRA=1"), append(profile, "PROFILE_ONLY=1"), nil)
	if strings.Join(refused, ",") != "EXTRA,GH_TOKEN,PROFILE_ONLY" {
		t.Fatalf("refused %q", refused)
	}
}

// A scheduled task starts in the orchestrator directory, so a relative path in
// the caller's context would name something else there. Each is refused by
// name, without its value, even when the profile has the same value.
func TestScheduledLaunchRefusesRelativePaths(t *testing.T) {
	profile := []string{"Path=C:\\Windows", "USERPROFILE=C:\\Users\\u", "HEV_DISPATCH_CONFIG=dispatch.toml"}
	for _, tc := range []struct {
		name, env, refused string
	}{
		{"relative config", "HEV_DISPATCH_CONFIG=conf\\dispatch.toml", "HEV_DISPATCH_CONFIG (relative path)"},
		{"relative config shared with the profile", "HEV_DISPATCH_CONFIG=dispatch.toml", "HEV_DISPATCH_CONFIG (relative path)"},
		{"home-relative config", "HEV_CONFIG=~/hev.toml", "HEV_CONFIG (relative path)"},
		{"relative profile", "USERPROFILE=custom", "USERPROFILE (relative path)"},
		{"drive-relative profile", "USERPROFILE=C:custom", "USERPROFILE (relative path)"},
		{"rooted profile without a drive", "USERPROFILE=\\custom", "USERPROFILE (relative path)"},
		{"relative PATH entry", "PATH=C:\\tools;bin;C:\\Windows", "PATH (relative path)"},
		{"dot PATH entry", "PATH=.;C:\\Windows", "PATH (relative path)"},
		{"absolute PATH with empty entries", "PATH=C:\\tools;;C:\\Windows;", ""},
		{"UNC profile", "USERPROFILE=\\\\server\\share\\u", ""},
		{"empty config", "HEV_DISPATCH_CONFIG=", ""},
		// Values are Windows paths on every OS: /custom is rooted on the
		// current drive, so it is refused even where it is a POSIX absolute path.
		{"POSIX-rooted profile", "USERPROFILE=/custom", "USERPROFILE (relative path)"},
		{"POSIX-rooted PATH entry", "PATH=C:\\Windows;/usr/bin", "PATH (relative path)"},
		{"forward-slash drive profile", "USERPROFILE=C:/Users/u", ""},
		{"forward-slash UNC profile", "USERPROFILE=//server/share/u", ""},
		{"device path profile", "USERPROFILE=\\\\?\\C:\\Users\\u", ""},
		{"relative shell", "SHELL=tools\\shell.exe", "SHELL (relative path)"},
		{"dot shell", "SHELL=.\\shell.exe", "SHELL (relative path)"},
		{"bare shell", "SHELL=shell.exe", "SHELL (relative path)"},
		{"POSIX-rooted shell", "SHELL=/usr/bin/bash", "SHELL (relative path)"},
		{"absolute shell", "SHELL=C:\\tools\\shell.exe", ""},
		{"relative peers backend", "HEV_PEERS_BACKEND=tools\\peers.exe", "HEV_PEERS_BACKEND (relative path)"},
		{"forward-slash relative peers backend", "HEV_PEERS_BACKEND=tools/peers.exe", "HEV_PEERS_BACKEND (relative path)"},
		{"drive-relative peers backend", "HEV_PEERS_BACKEND=C:peers.exe", "HEV_PEERS_BACKEND (relative path)"},
		{"dot peers backend", "HEV_PEERS_BACKEND=.", "HEV_PEERS_BACKEND (relative path)"},
		{"bare peers backend", "HEV_PEERS_BACKEND=peers.exe", ""},
		{"absolute peers backend", "HEV_PEERS_BACKEND=C:\\tools\\peers.exe", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := []string{"Path=C:\\Windows", "USERPROFILE=C:\\Users\\u"}
			if tc.env != "" {
				caller = withLaunchEnv(caller, tc.env)
			}
			carried, refused := scheduledLaunchEnvironment(caller, profile, nil)
			if strings.Join(refused, ",") != tc.refused {
				t.Fatalf("refused %q, want %q", refused, tc.refused)
			}
			_, value, _ := strings.Cut(tc.env, "=")
			if tc.refused != "" && value != "" && strings.Contains(strings.Join(refused, ","), value) {
				t.Fatal("refusal printed a value")
			}
			if tc.refused != "" && strings.Contains(strings.Join(carried, ","), tc.env) {
				t.Fatalf("carried a refused value: %q", carried)
			}
		})
	}
}

// launchNonPaths are the carried variables whose values are names, not paths:
// a peer identity, locales and a Layer namespace. No working directory changes
// what they mean.
var launchNonPaths = []string{"PEERS_SELF", "LANG", "LC_ALL", "LAYER_NAMESPACE"}

// Every carried variable is classified exactly once, so a new one cannot be
// carried without deciding whether its value depends on the directory.
func TestScheduledLaunchClassifiesEveryCarriedVariable(t *testing.T) {
	for _, k := range launchCarried {
		n := 0
		for _, class := range [][]string{{"PATH"}, launchPaths, launchCommands, launchNonPaths} {
			if launchListed(class, k) {
				n++
			}
		}
		if n != 1 {
			t.Errorf("carried %s is in %d path classes, want 1", k, n)
		}
	}
	for _, class := range [][]string{launchPaths, launchCommands, launchNonPaths} {
		for _, k := range class {
			if !launchListed(launchCarried, k) {
				t.Errorf("%s is classified but not carried", k)
			}
		}
	}
}

// hev peers runs HEV_PEERS_BACKEND through os/exec. A relative backend would
// run the task directory's program, so it is refused. A bare name resolves on
// PATH from either directory, and os/exec refuses a working-directory match.
func TestScheduledLaunchPeersBackendKeepsCallersProgram(t *testing.T) {
	caller, task, bin := t.TempDir(), t.TempDir(), t.TempDir()
	for _, dir := range []string{filepath.Join(caller, "tools"), filepath.Join(task, "tools"), bin, task} {
		if e := os.MkdirAll(dir, 0755); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(filepath.Join(dir, "peers.exe"), nil, 0755); e != nil {
			t.Fatal(e)
		}
	}
	t.Setenv("PATH", bin)
	for _, backend := range []string{filepath.Join("tools", "peers.exe"), "peers.exe"} {
		t.Chdir(caller)
		want, e := exec.LookPath(backend)
		if e == nil {
			want, e = filepath.Abs(want)
		}
		if e != nil {
			t.Fatal(e)
		}
		carried, refused := scheduledLaunchEnvironment([]string{"HEV_PEERS_BACKEND=" + backend}, nil, nil)
		if len(refused) > 0 {
			if !strings.ContainsAny(backend, `\/`) || !reflect.DeepEqual(refused, []string{"HEV_PEERS_BACKEND (relative path)"}) {
				t.Fatalf("backend %q refused %q", backend, refused)
			}
			continue
		}
		t.Chdir(task)
		t.Setenv("HEV_PEERS_BACKEND", "")
		if e = ApplyLaunchEnvironment(carried); e != nil {
			t.Fatal(e)
		}
		got, e := exec.LookPath(os.Getenv("HEV_PEERS_BACKEND"))
		if errors.Is(e, exec.ErrDot) {
			continue
		}
		if e == nil {
			got, e = filepath.Abs(got)
		}
		if e != nil || got != want {
			t.Fatalf("carried backend %q runs %s (%v) in the task's working directory, the caller runs %s", backend, got, e, want)
		}
	}
}

// withLaunchEnv replaces kv's name in env, matching names case-insensitively.
func withLaunchEnv(env []string, kv string) []string {
	name, _, _ := strings.Cut(kv, "=")
	out := []string{}
	for _, e := range env {
		if n, _, _ := strings.Cut(e, "="); !strings.EqualFold(n, name) {
			out = append(out, e)
		}
	}
	return append(out, kv)
}

// The fallback task's working directory is the orchestrator directory. With
// two configs sharing a relative name, carrying the name would make the
// supervisor load the wrong one; the launcher must refuse it, and an absolute
// config must load the caller's.
func TestScheduledLaunchRelativeConfigKeepsCallersSpec(t *testing.T) {
	caller, task := t.TempDir(), t.TempDir()
	for dir, body := range map[string]string{caller: "cap = 3\n", task: "cap = 9\n"} {
		if e := os.WriteFile(filepath.Join(dir, "dispatch.toml"), []byte(body), 0600); e != nil {
			t.Fatal(e)
		}
	}
	for _, config := range []string{"dispatch.toml", filepath.Join(caller, "dispatch.toml")} {
		t.Chdir(caller)
		t.Setenv("HEV_DISPATCH_CONFIG", config)
		want, e := Load(ConfigPath(""))
		if e != nil {
			t.Fatal(e)
		}
		carried, refused := scheduledLaunchEnvironment([]string{"HEV_DISPATCH_CONFIG=" + config}, nil, nil)
		if len(refused) > 0 {
			// Off Windows the temporary config is a POSIX path, which is
			// rooted on the current drive and so refused as well.
			if windowsAbsPath(config) || !reflect.DeepEqual(refused, []string{"HEV_DISPATCH_CONFIG (relative path)"}) {
				t.Fatalf("config %q refused %q", config, refused)
			}
			continue
		}
		// Mirror the task's WorkingDirectory and the supervise PreRunE.
		t.Chdir(task)
		t.Setenv("HEV_DISPATCH_CONFIG", "")
		if e = ApplyLaunchEnvironment(carried); e != nil {
			t.Fatal(e)
		}
		got, e := Load(ConfigPath(""))
		if e != nil {
			t.Fatal(e)
		}
		if got.Cap != want.Cap {
			t.Fatalf("carried config %q changed from cap %d to cap %d in the task's working directory", config, want.Cap, got.Cap)
		}
	}
}

func TestApplyLaunchEnvironment(t *testing.T) {
	t.Setenv("HEV_CONFIG", "from-profile")
	t.Setenv("HEV_DISPATCH_CONFIG", "")
	if e := ApplyLaunchEnvironment([]string{"HEV_CONFIG", "HEV_DISPATCH_CONFIG=custom.toml"}); e != nil {
		t.Fatal(e)
	}
	if _, set := os.LookupEnv("HEV_CONFIG"); set || os.Getenv("HEV_DISPATCH_CONFIG") != "custom.toml" {
		t.Fatalf("HEV_CONFIG set %v, HEV_DISPATCH_CONFIG %q", set, os.Getenv("HEV_DISPATCH_CONFIG"))
	}
	if e := ApplyLaunchEnvironment([]string{"GH_TOKEN=x"}); e == nil {
		t.Fatal("launch environment carried a secret")
	}
}

// launchOverlapNames are carried, session-only, path or command names that a
// source.env may also declare plugin-only.
var launchOverlapNames = []string{"HEV_CONFIG", "CODEX_HOME", "SSH_AUTH_SOCK", "TERM", "SHELL", "HEV_PEERS_BACKEND"}

// launchCaseVariants pairs a declared plugin-only name with the caller's
// spelling of it; Windows names match case-insensitively.
func launchCaseVariants(name string) [][2]string {
	return [][2]string{{name, name}, {name, strings.ToLower(name)}, {strings.ToLower(name), name}}
}

// A differing plugin-only name is refused by name before the carry, session,
// path and command rules, whatever its value, so its value never reaches the
// task's command line and the supervisor never starts without it.
func TestScheduledLaunchRefusesDifferingPluginOnlyNames(t *testing.T) {
	base := []string{"Path=C:\\Windows", "USERPROFILE=C:\\Users\\u"}
	for _, name := range launchOverlapNames {
		for _, value := range []string{`C:\synthetic\value`, `synthetic\value`, "synthetic-value"} {
			for _, v := range launchCaseVariants(name) {
				declared, key := v[0], v[1]
				for _, tc := range []struct {
					what            string
					caller, profile []string
				}{
					{"caller only", withLaunchEnv(base, key+"="+value), base},
					{"profile only", base, withLaunchEnv(base, key+"="+value)},
					{"differing", withLaunchEnv(base, key+"="+value), withLaunchEnv(base, name+"=other-synthetic")},
				} {
					carried, refused := scheduledLaunchEnvironment(tc.caller, tc.profile, []string{declared})
					if !reflect.DeepEqual(refused, []string{key + " (plugin-only)"}) {
						t.Errorf("%s %s=%s declared %s: refused %q", tc.what, key, value, declared, refused)
					}
					if len(carried) != 0 || strings.Contains(strings.Join(refused, ","), value) {
						t.Errorf("%s %s=%s: carried %q, refused %q", tc.what, key, value, carried, refused)
					}
				}
				// The profile already has the caller's value: nothing to carry or refuse.
				carried, refused := scheduledLaunchEnvironment(withLaunchEnv(base, key+"="+value), withLaunchEnv(base, name+"="+value), []string{declared})
				if len(carried) != 0 || strings.Contains(strings.Join(refused, ","), "plugin-only") {
					t.Errorf("profile-equivalent %s=%s: carried %q, refused %q", key, value, carried, refused)
				}
			}
		}
	}
}

// The supervisor restores carried context before it holds plugin-only values,
// so it refuses to set a plugin-only name under the profile's specs and again
// under the specs the restored context selects.
func TestScheduledLaunchApplyRefusesPluginOnlyNames(t *testing.T) {
	old := pluginOnlyEnvSources
	t.Cleanup(func() { pluginOnlyEnvSources = old })
	for _, name := range []string{"HEV_CONFIG", "CODEX_HOME", "SHELL", "HEV_PEERS_BACKEND"} {
		for _, v := range launchCaseVariants(name) {
			pluginOnlyEnvSources = []func() []string{func() []string { return []string{v[0]} }}
			t.Setenv(name, "before")
			e := ApplyLaunchEnvironment([]string{v[1] + `=C:\synthetic`})
			if e == nil || !strings.Contains(e.Error(), "plugin-only "+v[1]) || strings.Contains(e.Error(), "synthetic") {
				t.Errorf("carrying %s declared %s: %v", v[1], v[0], e)
			}
			if os.Getenv(name) != "before" {
				t.Errorf("refused %s was set", name)
			}
		}
	}
	// Only the restored configuration declares CODEX_HOME plugin-only.
	t.Setenv("HEV_CONFIG", "")
	t.Setenv("CODEX_HOME", "")
	pluginOnlyEnvSources = []func() []string{func() []string {
		if os.Getenv("HEV_CONFIG") == `C:\restored\hev.toml` {
			return []string{"CODEX_HOME"}
		}
		return nil
	}}
	e := ApplyLaunchEnvironment([]string{`HEV_CONFIG=C:\restored\hev.toml`, `CODEX_HOME=C:\synthetic`})
	if e == nil || !strings.Contains(e.Error(), "plugin-only CODEX_HOME") {
		t.Fatalf("restored context's plugin-only name: %v", e)
	}
}
