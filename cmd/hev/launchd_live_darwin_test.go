//go:build darwin

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLaunchdRestartUnderThrottleLive restarts a temporary LaunchAgent (never
// the real hevd job) the way `hev update` restarts hevd, with a ThrottleInterval
// of 30s as on the mini, when the restarted process exits once at start. launchd
// holds a job whose process ran for less than its throttle down for the rest of
// the interval before starting it again, longer than the updater's old fixed
// 20s wait. It changes this machine's launchd (a temporary label only), so it
// runs only with HEV_LAUNCHD_LIVE_TEST=1.
func TestLaunchdRestartUnderThrottleLive(t *testing.T) {
	if os.Getenv("HEV_LAUNCHD_LIVE_TEST") != "1" {
		t.Skip("set HEV_LAUNCHD_LIVE_TEST=1 to restart a temporary LaunchAgent on this machine")
	}
	label := fmt.Sprintf("com.hev.kit-test-throttle-%d", os.Getpid())
	t.Setenv("HEV_LAUNCHD_LABEL", label)
	home := t.TempDir()
	// The job exits once at start while this marker exists, as a new daemon
	// does when it cannot start on its first try.
	marker := filepath.Join(home, "fail-next-start")
	job := daemonJob("", home, "")
	plist := job.plistPath(home)
	if err := os.MkdirAll(filepath.Dir(plist), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>` + label + `</string>
    <key>ProgramArguments</key>
    <array>
        <string>/bin/sh</string>
        <string>-c</string>
        <string>if [ -e "$0" ]; then rm -f "$0"; exit 1; fi; trap 'exit 0' TERM; while :; do sleep 1; done</string>
        <string>` + marker + `</string>
    </array>
    <key>KeepAlive</key>
    <true/>
    <key>RunAtLoad</key>
    <true/>
    <key>ThrottleInterval</key>
    <integer>30</integer>
</dict>
</plist>
`
	if err := os.WriteFile(plist, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("launchctl", job.bootstrapArgs(plist)...).CombinedOutput(); err != nil {
		t.Fatalf("bootstrap: %v: %s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("launchctl", job.bootoutArgs()...).Run() })
	if !strings.HasPrefix(job.target(), "gui/") || !strings.HasSuffix(job.target(), label) {
		t.Fatalf("target %q is not the temporary label", job.target())
	}

	s := launchdService{home: home}
	if s.Name() != "hevd (launchd "+label+")" {
		t.Fatalf("service %q is not the temporary label", s.Name())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	waitRunning(t, job)
	// The next start fails once: launchd then holds the job down for its
	// throttle before trying again.
	if err := os.WriteFile(marker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := s.Restart(ctx); err != nil {
		t.Fatalf("restart through one failed start: %v (after %s)", err, time.Since(start).Round(time.Second))
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("the restarted job never made its failing start")
	}
	t.Logf("restarted a job with a 30s throttle through one failed start in %s", time.Since(start).Round(100*time.Millisecond))
}

func waitRunning(t *testing.T, job launchdJob) {
	t.Helper()
	for i := 0; i < 50; i++ {
		out, _ := exec.Command("launchctl", "print", job.target()).Output()
		if strings.Contains(string(out), "state = running") {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("the temporary job did not start")
}
