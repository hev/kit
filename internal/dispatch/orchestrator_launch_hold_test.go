package dispatch

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// holdForTest runs HoldPluginOnlyEnv as a supervisor does and forgets the held
// values when the test ends.
func holdForTest(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		heldMu.Lock()
		heldEnv = map[string]string{}
		heldMu.Unlock()
	})
	HoldPluginOnlyEnv()
}

// testNestedHoldDirectSupervisor starts a supervisor directly from a process
// that holds plugin-only values, as a supervisor launching another one does.
// The child is the one control process that keeps them.
func testNestedHoldDirectSupervisor(t *testing.T) {
	_, dump := pluginEnvFixture(t)
	holdForTest(t)
	bin := t.TempDir()
	installEnvDump(t, bin, "nested-supervisor")
	exe := filepath.Join(bin, "nested-supervisor")
	if _, e := os.Stat(exe + ".exe"); e == nil {
		exe += ".exe"
	}
	// The stand-in records its environment and exits without confirming.
	_ = startOrchestratorSupervisor(t.TempDir(), "other", []string{exe})
	env := readEnvDump(t, dump, "nested-supervisor")
	if !slices.Contains(env, pluginSecret+"=source-secret-value") || !slices.Contains(env, otherSecret+"=other-secret-value") {
		t.Fatal("a supervisor started by a holding supervisor lost the held plugin-only values")
	}
}
