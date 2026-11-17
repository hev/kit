package dispatch

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hev/kit/internal/update"
)

// Admission registers a run and then waits out an automatic update's switch
// window, so the update's last check either sees the run or the run starts
// on the new build.
func TestAdmissionWaitsOutAnUpdateSwitch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	marker := update.SwitchingPath(home)
	if err := os.MkdirAll(filepath.Dir(marker), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, []byte("1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	go func() { time.Sleep(150 * time.Millisecond); os.Remove(marker) }()
	start := time.Now()
	awaitUpdateSwitch()
	if time.Since(start) < 150*time.Millisecond {
		t.Fatal("admission did not wait for the switch to end")
	}
	start = time.Now()
	awaitUpdateSwitch()
	if time.Since(start) > time.Second {
		t.Fatal("admission waited with no switch under way")
	}
}
