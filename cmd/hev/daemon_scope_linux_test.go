package main

import (
	"io"
	"os"
	"os/exec"
	"testing"
)

func TestDaemonScopeChecksProcessEnvironmentAndOwner(t *testing.T) {
	if os.Getenv("HEV_SCOPE_TEST_CHILD") == "1" {
		// Start returning is not a readiness signal for /proc's new environment.
		// Publish readiness from the executed process, then wait for parent EOF.
		os.Stdout.Write([]byte("ready"))
		io.Copy(io.Discard, os.Stdin)
		os.Exit(0)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("HEV_CONFIG", "")
	for _, tc := range []struct {
		home string
		want bool
	}{{home, true}, {home + "-other", false}} {
		child := exec.Command(os.Args[0], "-test.run=^TestDaemonScopeChecksProcessEnvironmentAndOwner$")
		child.Env = []string{"HOME=" + tc.home, "HEV_SCOPE_TEST_CHILD=1"}
		input, err := child.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		output, err := child.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := child.Start(); err != nil {
			t.Fatal(err)
		}
		ready := make([]byte, 5)
		if _, err = io.ReadFull(output, ready); err != nil {
			child.Process.Kill()
			child.Wait()
			t.Fatal(err)
		}
		got := daemonInScope(child.Process.Pid)
		input.Close()
		if err = child.Wait(); err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Fatalf("process scope=%v want %v", got, tc.want)
		}
	}
}
