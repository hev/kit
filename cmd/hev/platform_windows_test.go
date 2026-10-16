//go:build windows

package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsTraceLifecycleRejectsBeforeEffects(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "absent", "config.toml")
	t.Setenv("HEV_CONFIG", path)
	checks := []func() error{
		func() error {
			_, err := stopInviteWriter(context.Background(), filepath.Join(home, "new-home"))
			return err
		},
		func() error { return runJoin(context.Background(), io.Discard, "must-not-redeem") },
		func() error { return runLeave(context.Background(), io.Discard) },
		func() error { cmd := newBackfillCommand(backfillRuntime{}); return cmd.RunE(cmd, nil) },
		func() error { return daemonStopCmd.RunE(daemonStopCmd, nil) },
	}
	for _, check := range checks {
		if err := check(); err == nil || !strings.Contains(err.Error(), "UnsupportedCapability") {
			t.Fatalf("missing explicit unsupported: %v", err)
		}
	}
	if _, err := os.Stat(filepath.Join(home, "absent")); !os.IsNotExist(err) {
		t.Fatalf("config directory created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "new-home")); !os.IsNotExist(err) {
		t.Fatalf("daemon directory created: %v", err)
	}
}
