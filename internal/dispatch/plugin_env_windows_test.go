package dispatch

import (
	"os"
	"os/exec"
	"path/filepath"
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

// A CLI stop with a source.env name exported reaches taskkill through
// stopHarness; taskkill, like every helper, never receives the name.
func TestWindowsPluginOnlyEnvTaskkill(t *testing.T) {
	_, dump := pluginEnvFixture(t)
	bin := t.TempDir()
	installEnvDump(t, bin, "taskkill")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	allowChildProbes(t)
	dir := t.TempDir()
	// The fake taskkill records its environment and kills nothing, so the
	// live harness record can be this test process.
	if e := writeProcessRecord(filepath.Join(dir, "harness.json"), os.Getpid()); e != nil {
		t.Fatal(e)
	}
	if e := stopHarness(dir); e != nil {
		t.Fatal(e)
	}
	pluginOnlyAbsent(t, dump, "taskkill", "stopHarness taskkill")
	p, e := os.FindProcess(os.Getpid())
	if e != nil {
		t.Fatal(e)
	}
	killTree(&exec.Cmd{Process: p})
	pluginOnlyAbsent(t, dump, "taskkill", "killTree taskkill")
}
