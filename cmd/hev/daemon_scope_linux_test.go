package main

import (
	"os"
	"os/exec"
	"testing"
)

func TestDaemonScopeChecksProcessEnvironmentAndOwner(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("HEV_CONFIG", "")
	for _, tc := range []struct {
		home string
		want bool
	}{{home, true}, {home + "-other", false}} {
		child := exec.Command("sleep", "60")
		child.Env = []string{"HOME=" + tc.home, "PATH=" + os.Getenv("PATH")}
		if err := child.Start(); err != nil {
			t.Fatal(err)
		}
		got := daemonInScope(child.Process.Pid)
		child.Process.Kill()
		child.Wait()
		if got != tc.want {
			t.Fatalf("process scope=%v want %v", got, tc.want)
		}
	}
}
