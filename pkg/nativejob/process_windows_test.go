//go:build windows

package nativejob

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWindowsBirthChild(t *testing.T) {
	if os.Getenv("NATIVE_WINDOWS_BIRTH_CHILD") == "1" {
		time.Sleep(time.Minute)
		os.Exit(0)
	}
}

// Run this exact cross-compiled test on the intended Windows guest as a native
// readiness prerequisite. Cross-compilation alone does not execute this proof.
func TestWindowsBirthAndOwnChild(t *testing.T) {
	self, err := processBirth(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if !alive(&self) {
		t.Fatal("unstable own birth")
	}
	wrong := self
	wrong.Start += "0"
	if alive(&wrong) {
		t.Fatal("creation mismatch accepted")
	}
	wrong = self
	wrong.BootID = "other-boot"
	if alive(&wrong) {
		t.Fatal("boot mismatch accepted")
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestWindowsBirthChild$")
	cmd.Env = append(os.Environ(), "NATIVE_WINDOWS_BIRTH_CHILD=1")
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	child, err := processBirth(cmd.Process.Pid)
	if err != nil || !alive(&child) {
		t.Fatal(child, err)
	}
	if err = cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	if alive(&child) {
		t.Fatal("dead child accepted")
	}
}
func TestWindowsJournalFailsClosedBeforeMutation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "not-created")
	s, err := Open(root, Enrollment{HostID: "windows-proof", ExecutorID: "windows-proof"})
	if err == nil {
		s.Close()
		t.Fatal("unaccepted Windows journal enabled")
	}
	if !strings.Contains(err.Error(), "Windows native journal not enabled") {
		t.Fatal(err)
	}
	if _, err = os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("unsupported platform mutated filesystem", err)
	}
}
