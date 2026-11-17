package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The mini's hevd plist sets ThrottleInterval 30; the updater waited a fixed
// 20s, so a new build whose first start failed was still held down by
// launchd when the wait ran out and the update rolled back (2026-11-17).
func TestLaunchdRestartWaitCoversThrottle(t *testing.T) {
	dir := t.TempDir()
	mini := filepath.Join(dir, "mini.plist")
	if err := os.WriteFile(mini, []byte("<plist><dict>\n<key>KeepAlive</key><true/>\n<key>ThrottleInterval</key>\n\t<integer>30</integer>\n</dict></plist>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := launchdThrottle(mini); got != 30*time.Second {
		t.Fatalf("throttle = %s, want 30s", got)
	}
	if w := launchdRestartWait(launchdThrottle(mini)); w < 30*time.Second+launchdThrottleMargin {
		t.Fatalf("wait %s does not cover a 30s throttle plus margin", w)
	}
	// kit's own plist.
	own := filepath.Join(dir, "own.plist")
	if err := os.WriteFile(own, []byte(daemonJob("/bin/hev", dir, "").plist()), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := launchdThrottle(own); got != 10*time.Second {
		t.Fatalf("kit plist throttle = %s, want 10s", got)
	}
	// None set, or no plist: launchd's default, and never under the old wait.
	if got := parseLaunchdThrottle([]byte("<plist><dict></dict></plist>")); got != launchdDefaultThrottle {
		t.Fatalf("unset throttle = %s", got)
	}
	if got := launchdThrottle(filepath.Join(dir, "missing.plist")); got != launchdDefaultThrottle {
		t.Fatalf("missing plist throttle = %s", got)
	}
	if w := launchdRestartWait(0); w != defaultRestartWait {
		t.Fatalf("wait with no throttle = %s, want %s", w, defaultRestartWait)
	}
}
