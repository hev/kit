//go:build darwin

package nativehost

import (
	n "github.com/hev/kit/pkg/nativejob"
	"os/exec"
	"syscall"
	"testing"
)

// This establishes only the existing kernel birth/group primitive. It does not
// establish launchd ownership, escaped-tree absence or permission to adopt a PID.
func TestDarwinOwnedIndependentGroupBirth(t *testing.T) {
	child := exec.Command("/bin/sleep", "30")
	child.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
	before, err := n.ProcessBirth(child.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	group, err := syscall.Getpgid(child.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	after, err := n.ProcessBirth(child.Process.Pid)
	if err != nil || after != before || group != before.PID || before.BootID == "" || before.Start == "" {
		t.Fatalf("unverified owned group/birth: %v", err)
	}
}
