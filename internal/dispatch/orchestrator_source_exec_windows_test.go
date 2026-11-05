package dispatch

import (
	"testing"
	"time"
)

// The batch plugin's PowerShell grandchild outlives cmd.exe unless the job
// object is closed on cancellation.
func TestWindowsSourcePluginJobKillsGrandchild(t *testing.T) {
	testSourceTimeoutKillsTree(t, 9*time.Second)
}
