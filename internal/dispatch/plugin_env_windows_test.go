package dispatch

import (
	"testing"
)

// allowChildProbes lets harness records probe the real children these tests
// start; every such child is a copy of this test binary that exits at once.
func allowChildProbes(t *testing.T) {
	previous := created
	created = processCreated
	t.Cleanup(func() { created = previous })
}

// Tests never start a detached supervisor or scheduled task on Windows, so
// the supervisor check covers the environment launchSupervisor passes.
func TestWindowsPluginOnlyEnv(t *testing.T) {
	testPluginOnlyEnv(t)
	env := childEnvironment("")
	if hasEnv(env, pluginSecret) || hasEnv(env, otherSecret) {
		t.Fatal("worker supervisor environment carries a plugin-only name")
	}
	if !hasEnv(env, "PATH") {
		t.Fatal("worker supervisor lost its ordinary environment")
	}
}

// The Windows CI selector runs Windows-prefixed tests only.
func TestWindowsPluginOnlyEnvHelperCommands(t *testing.T) {
	t.Run("helpers", TestPluginOnlyEnvHelperCommands)
	t.Run("hold", TestHoldPluginOnlyEnv)
}
