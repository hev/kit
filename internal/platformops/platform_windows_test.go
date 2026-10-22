//go:build windows

package platformops

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestWindowsLockExclusionAndRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	release, err := LockPath(path, false)
	if err != nil {
		t.Fatal(err)
	}
	release = onceRelease(release)
	defer func() { release() }()
	if again, err := LockPath(path, true); !IsLockBusy(err) {
		if again != nil {
			again()
		}
		t.Fatalf("second writer: %v", err)
	}
	waiting := make(chan error, 1)
	go func() {
		unlock, err := LockPath(path, false)
		if err == nil {
			unlock()
		}
		waiting <- err
	}()
	select {
	case err := <-waiting:
		t.Fatalf("blocking lock returned early: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	release()
	select {
	case err := <-waiting:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("lock did not release")
	}
	unlock, err := LockPath(path, true)
	if err != nil {
		t.Fatal(err)
	}
	unlock()
}
func onceRelease(f func()) func() {
	done := false
	return func() {
		if !done {
			done = true
			f()
		}
	}
}

func TestWindowsLockRefusesReparsePoint(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "link")
	if err := os.WriteFile(target, []byte("sentinel"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink privilege unavailable: %v", err)
	}
	if release, err := LockPath(link, true); err == nil {
		release()
		t.Fatal("followed reparse point")
	}
	b, err := os.ReadFile(target)
	if err != nil || string(b) != "sentinel" {
		t.Fatalf("target modified: %q %v", b, err)
	}
}
func TestWindowsPrivateACL(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	if err := MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.toml")
	if err := WriteFile(path, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		t.Fatal(err)
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{dir, path} {
		sd, err := windows.GetNamedSecurityInfo(p, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			t.Fatal(err)
		}
		s := sd.String()
		if !strings.Contains(s, "D:P") || !strings.Contains(s, user.User.Sid.String()) || !strings.Contains(s, ";;;SY)") || !strings.Contains(s, ";;;BA)") || strings.Count(s, "(A;") != 3 {
			t.Fatalf("unexpected DACL %s: %s", p, s)
		}
	}
}
func TestWindowsGracefulStop(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	if err := MkdirAll(filepath.Join(home, ".hev"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, closeControl, err := DaemonContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer closeControl()
	if err = SignalPID(os.Getpid(), false); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("stop event not observed")
	}
	if !ProcessGone(windows.ERROR_INVALID_PARAMETER) || ProcessGone(windows.ERROR_ACCESS_DENIED) {
		t.Fatal("incorrect ProcessGone mapping")
	}
}
func TestWindowsRenameWithOpenReader(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "state")
	tmp := filepath.Join(dir, "tmp")
	if err := WriteFile(target, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := OpenFile(tmp, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.WriteString("new"); err != nil {
		t.Fatal(err)
	}
	if err = f.Sync(); err != nil {
		t.Fatal(err)
	}
	f.Close()
	reader, err := os.Open(target)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- Rename(tmp, target) }()
	time.Sleep(75 * time.Millisecond)
	reader.Close()
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(target)
	if err != nil || string(b) != "new" {
		t.Fatalf("replace failed: %q %v", b, err)
	}
}

func TestWindowsForceStop(t *testing.T) {
	if os.Getenv("HEV_TEST_FORCE_CHILD") == "1" {
		time.Sleep(10 * time.Minute)
		return
	}
	child := exec.Command(os.Args[0], "-test.run=^TestWindowsForceStop$")
	child.Env = append(os.Environ(), "HEV_TEST_FORCE_CHILD=1")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer child.Process.Kill()
	if err := SignalPID(child.Process.Pid, true); err != nil {
		t.Fatal(err)
	}
	if err := child.Wait(); err == nil {
		t.Fatal("terminated child exited successfully")
	}
	if err := SignalPID(0, false); !ProcessGone(err) {
		t.Fatalf("nonexistent PID: %v", err)
	}
}
