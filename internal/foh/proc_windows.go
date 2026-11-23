//go:build windows

package foh

import "os/exec"

// setProcessGroup keeps exec's default cancellation (kill the started
// process) on Windows.
func setProcessGroup(*exec.Cmd) {}
