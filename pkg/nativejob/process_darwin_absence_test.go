//go:build darwin

package nativejob

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"os/exec"
	"testing"
)

func TestDarwinReapedOwnedProcessIsAbsent(t *testing.T) {
	child := exec.Command("/bin/sleep", "30")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		if !waited {
			_ = child.Process.Kill()
			_ = child.Wait()
		}
	}()
	before, err := ProcessBirth(child.Process.Pid)
	if err != nil || before.PID != child.Process.Pid || before.BootID == "" || before.Start == "" {
		t.Fatalf("missing live owned birth: %v", err)
	}
	if err = child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = child.Wait()
	waited = true
	if _, err = ProcessBirth(before.PID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("reaped owned child must report definite absence, got %v", err)
	}
}

func TestDarwinAbsenceRefusesAmbiguousOrPermissionFailure(t *testing.T) {
	for _, err := range []error{nil, unix.EPERM, unix.EACCES, unix.EIO} {
		if errors.Is(darwinAbsenceError(err), os.ErrNotExist) {
			t.Fatalf("ambiguous/permission observation reported absent: %v", err)
		}
	}
	if !errors.Is(darwinAbsenceError(unix.ESRCH), os.ErrNotExist) {
		t.Fatal("definite ESRCH absence lost")
	}
}
