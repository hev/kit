package dispatch

import (
	"os"
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
	carried, refused := scheduledLaunchEnvironment(caller, profile)
	want := []string{"HEV_CONFIG", "HOME=C:\\custom", "PATH=C:\\tools;C:\\Windows", "USERPROFILE=C:\\custom"}
	if !reflect.DeepEqual(carried, want) || len(refused) != 0 {
		t.Fatalf("carried %q refused %q", carried, refused)
	}
	_, refused = scheduledLaunchEnvironment(append(caller, "GH_TOKEN=synthetic-token", "EXTRA=1"), append(profile, "PROFILE_ONLY=1"))
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := []string{"Path=C:\\Windows", "USERPROFILE=C:\\Users\\u"}
			if tc.env != "" {
				caller = withLaunchEnv(caller, tc.env)
			}
			carried, refused := scheduledLaunchEnvironment(caller, profile)
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
		carried, refused := scheduledLaunchEnvironment([]string{"HEV_DISPATCH_CONFIG=" + config}, nil)
		if len(refused) > 0 {
			if filepath.IsAbs(config) || !reflect.DeepEqual(refused, []string{"HEV_DISPATCH_CONFIG (relative path)"}) {
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
