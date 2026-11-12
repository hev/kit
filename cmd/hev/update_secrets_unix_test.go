//go:build !windows

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/update"
)

// scriptedUserService answers systemctl --user as a host whose secrets-only
// unit is installed and in the given state.
type scriptedUserService struct {
	state       string
	unavailable bool
	calls       []string
}

func (f *scriptedUserService) Available() bool { return !f.unavailable }
func (f *scriptedUserService) Run(args ...string) ([]byte, error) {
	f.calls = append(f.calls, strings.Join(args, " "))
	switch args[0] {
	case "is-active":
		return []byte(f.state + "\n"), nil
	case "show":
		return []byte("MainPID=4242\nActiveState=" + f.state + "\n"), nil
	}
	return nil, nil
}

func (f *scriptedUserService) restarted(unit string) bool {
	for _, c := range f.calls {
		if c == "restart "+unit {
			return true
		}
	}
	return false
}

// secretsOnlyHost is updateSandbox on a Linux host that runs no hevd, only
// the staged hev-secrets.service, with a cooldown on disk from a rate limit.
// The updater keeps this host's real service list (with a fake systemctl).
func secretsOnlyHost(t *testing.T, state string, unavailable bool) (link, cooldown string, fake *scriptedUserService) {
	t.Helper()
	link, _ = updateSandbox(t)
	home := os.Getenv("HOME")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	savedOS, savedSvc, savedAdjust := hostOS, linuxService, adjustUpdater
	t.Cleanup(func() { hostOS, linuxService, adjustUpdater = savedOS, savedSvc, savedAdjust })
	hostOS = "linux"
	fake = &scriptedUserService{state: state, unavailable: unavailable}
	linuxService = fake
	adjustUpdater = func(u *update.Updater) {
		savedAdjust(u)
		u.Services = hostServices(home)
	}
	unit := filepath.Join(home, ".config", "systemd", "user", secretsOnlyUnit)
	os.MkdirAll(filepath.Dir(unit), 0o700)
	os.WriteFile(unit, []byte("[Service]\nExecStart=%h/.local/bin/hev daemon --secrets-only\n"), 0o644)
	cooldown = filepath.Join(home, ".hev", "secrets", "op-cooldown.json")
	os.MkdirAll(filepath.Dir(cooldown), 0o700)
	os.WriteFile(cooldown, []byte(`{"limit":"daily rate limit","reset":"2099-01-01T00:00:00Z","strikes":1}`), 0o600)
	return link, cooldown, fake
}

func runUpdate(t *testing.T, args ...string) (string, error) {
	t.Helper()
	c := newUpdateCommand()
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&out)
	c.SetArgs(args)
	err := c.Execute()
	return out.String(), err
}

func cooldownUnchanged(t *testing.T, path string, want []byte, mode os.FileMode) {
	t.Helper()
	got, err := os.ReadFile(path)
	info, serr := os.Stat(path)
	if err != nil || serr != nil || !bytes.Equal(got, want) || info.Mode().Perm() != mode {
		t.Fatalf("%s changed by the update: %v %q", path, err, got)
	}
}

// hev update moves a running secrets-only cache onto the new build, and
// leaves the host's rate-limit cooldown exactly as it was, so the restarted
// cache still holds off until the reset.
func TestUpdateRestartsRunningSecretsOnlyCache(t *testing.T) {
	link, cooldown, fake := secretsOnlyHost(t, "active", false)
	before, _ := os.ReadFile(cooldown)
	out, err := runUpdate(t, "--channel", "edge")
	if err != nil || runsAs(t, link) != cmdEdgeB {
		t.Fatalf("update: %v %s", err, out)
	}
	if !fake.restarted(secretsOnlyUnit) || !strings.Contains(out, "restarted secrets cache (systemd --user hev-secrets.service)") {
		t.Fatalf("secrets-only cache not restarted onto the new build: calls %q, out %s", fake.calls, out)
	}
	if fake.restarted(daemonUnit) || strings.Contains(out, "outside a service manager") {
		t.Fatalf("a host with no hevd unit was treated as running hevd: calls %q, out %s", fake.calls, out)
	}
	cooldownUnchanged(t, cooldown, before, 0o600)
}

// A secrets-only unit the user installed but stopped stays stopped.
func TestUpdateLeavesStoppedSecretsOnlyCache(t *testing.T) {
	link, cooldown, fake := secretsOnlyHost(t, "inactive", false)
	before, _ := os.ReadFile(cooldown)
	out, err := runUpdate(t, "--channel", "edge")
	if err != nil || runsAs(t, link) != cmdEdgeB {
		t.Fatalf("update: %v %s", err, out)
	}
	if fake.restarted(secretsOnlyUnit) || strings.Contains(out, "restarted secrets cache") {
		t.Fatalf("a stopped secrets-only unit was started: calls %q, out %s", fake.calls, out)
	}
	cooldownUnchanged(t, cooldown, before, 0o600)
}

// When the cache's unit is installed but the user manager is out of reach
// (an SSH session without lingering), the update is not reported as done
// with the cache left on the old build: it fails and switches back.
func TestUpdateWithUnreachableSecretsOnlyUnitSwitchesBack(t *testing.T) {
	link, cooldown, _ := secretsOnlyHost(t, "active", true)
	before, _ := os.ReadFile(cooldown)
	out, err := runUpdate(t, "--channel", "edge")
	if err == nil || !strings.Contains(out+err.Error(), "secrets cache") || runsAs(t, link) != cmdEdgeA {
		t.Fatalf("want a failed restart and the old build back: %v %s", err, out)
	}
	cooldownUnchanged(t, cooldown, before, 0o600)
}
