//go:build windows

package hooks

import "os/exec"

// ownGroup keeps the default cancellation (kill the handler process) on
// Windows.
func ownGroup(c *exec.Cmd) {}
