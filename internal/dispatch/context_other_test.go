//go:build !windows

package dispatch

import "testing"

// allowHarnessProbes is needed only on Windows (see context_windows_test.go).
func allowHarnessProbes(*testing.T) {}
