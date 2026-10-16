//go:build windows

package platformops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsTraceMutationsFailBeforeEffects(t *testing.T) {
	path := filepath.Join(t.TempDir(), "must-not-exist")
	if e := RequireTraceWrites(); e == nil || !strings.Contains(e.Error(), "UnsupportedCapability") {
		t.Fatal(e)
	}
	if release, e := LockPath(path, true); e == nil || release != nil {
		t.Fatal("Windows lock capability invented")
	}
	if _, e := os.Stat(path); !os.IsNotExist(e) {
		t.Fatalf("unsupported lock created file: %v", e)
	}
	if e := SignalPID(os.Getpid(), true); e == nil {
		t.Fatal("Windows signal capability invented")
	}
}
