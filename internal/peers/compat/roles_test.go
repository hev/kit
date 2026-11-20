package compat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Roles are a rule: boh is factory work only (d-*, o-*, managed, or an explicit
// tag), foh is only ever an explicit tag, and everything else untagged takes
// the configured untagged role.
func TestResolveRole(t *testing.T) {
	t.Setenv("HEV_CONFIG", filepath.Join(t.TempDir(), "absent.toml"))
	t.Setenv(untaggedRoleEnv, "")
	os.Unsetenv(untaggedRoleEnv)
	p := &P{home: t.TempDir()}
	for _, tt := range []struct{ session, tag, want string }{
		// boh: factory sessions and the explicit tag.
		{"d-kit-fix", "", "boh"},
		{"o-train", "", "boh"},
		{"o-train", "orchestrator", "boh"}, // hev's orchestrator launcher tag
		{"shell", "boh", "boh"},
		// foh: only the explicit tag.
		{"foh-kit-5", "foh", "foh"},
		{"d-run", "foh", "foh"},
		// Untagged interactive sessions are local, never boh or foh.
		{"shell", "", "local"},
		{"shell-2", "", "local"},
		{"foh-kit-5", "", "local"},
		{"boh-1", "", "local"},
		{"d-", "", "local"},
		{"o-", "", "local"},
		{"dev", "", "local"},
		// Explicit tags win, including over a factory name.
		{"shell", "local", "local"},
		{"shell", "offline", "offline"},
		{"d-run", "offline", "offline"},
		{"o-train", "local", "local"},
		// An unknown tag never participates and never becomes boh.
		{"shell", "orchestrator", "orchestrator"},
		{"shell", "BOH", "BOH"},
	} {
		got := p.resolveRole(tt.session, tt.tag)
		if got != tt.want {
			t.Errorf("resolveRole(%q, %q) = %q, want %q", tt.session, tt.tag, got, tt.want)
		}
	}
	for role, want := range map[string]bool{"foh": true, "local": true, "boh": true, "": true, "offline": false, "orchestrator": false, "BOH": false} {
		if participates(role) != want {
			t.Errorf("participates(%q) = %v", role, !want)
		}
	}
}

func TestUntaggedRoleConfig(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "config.toml")
	write := func(s string) {
		if err := os.WriteFile(config, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	load := func(env *string) (string, error) {
		t.Helper()
		t.Setenv("HEV_CONFIG", config)
		if env == nil {
			t.Setenv(untaggedRoleEnv, "")
			os.Unsetenv(untaggedRoleEnv)
		} else {
			t.Setenv(untaggedRoleEnv, *env)
		}
		return loadUntaggedRole(dir)
	}
	str := func(s string) *string { return &s }
	expect := func(env *string, want string) {
		t.Helper()
		if got, err := load(env); err != nil || got != want {
			t.Fatalf("got %q, %v; want %q", got, err, want)
		}
	}
	refused := func(env *string, want string) {
		t.Helper()
		got, err := load(env)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("got %q, %v; want refusal %q", got, err, want)
		}
		// Rejection text names the setting, never the value or the path.
		if strings.Contains(err.Error(), "s3cr3t") || strings.Contains(err.Error(), dir) {
			t.Fatalf("leaked: %v", err)
		}
	}
	// Accepted: unset, local and offline, from the file or the environment.
	expect(nil, "local") // no file
	write("[peers]\nlisten = \"127.0.0.1:1\"\n")
	expect(nil, "local")
	write("[peers]\nuntagged_role = \"local\"\n")
	expect(nil, "local")
	write("[peers]\nuntagged_role = \"offline\"\n")
	expect(nil, "offline")
	expect(str("local"), "local") // the environment overrides the file
	expect(str(" offline "), "offline")
	// Refused: anything else, including roles that untagged sessions can't take.
	for _, bad := range []string{"boh", "foh", "Local", "", "s3cr3t"} {
		write("[peers]\nuntagged_role = \"" + bad + "\"\n")
		refused(nil, `[peers] untagged_role must be "local" or "offline"`)
		refused(str(bad), untaggedRoleEnv+` must be "local" or "offline"`)
	}
	write("[peers]\nuntagged_role = 1\n")
	refused(nil, "untagged_role must be a string")
	write("[peers]\nuntagged_role = \"s3cr3t\n")
	refused(nil, "invalid peers configuration at line 2")
}
