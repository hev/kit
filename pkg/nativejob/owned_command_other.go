//go:build !linux && !darwin && !windows

package nativejob

import "os/exec"

// Unsupported platforms require a separately accepted original-handle backend.
func platformStartCommand(*exec.Cmd) (ownedCommand, error) { return nil, ErrOwnedCommandUnknown }
