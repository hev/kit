//go:build windows

package windowsfs

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func leaseProofFile(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "private")
	if err := CreatePrivateDir(root, false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "use.lease")
	f, err := CreatePrivateFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = errors.Join(f.Sync(), f.Close(), SyncDirectory(root)); err != nil {
		t.Fatal(err)
	}
	return path
}
func TestWindowsFileLeaseChild(t *testing.T) {
	value := os.Getenv("HEV_LEASE_CHILD_HANDLE")
	if value == "" {
		return
	}
	raw, err := strconv.ParseUint(value, 10, 64)
	if err != nil || raw == 0 {
		t.Fatal("invalid own inherited handle", err)
	}
	f := os.NewFile(uintptr(raw), "own-inherited-lease")
	defer f.Close()
	h := windows.Handle(raw)
	if err = windows.SetHandleInformation(h, windows.HANDLE_FLAG_INHERIT, 0); err != nil {
		t.Fatal(err)
	}
	if err = VerifyPrivateHandle(h, true, false); err != nil {
		t.Fatal(err)
	}
	id, err := leaseIdentity(h)
	if err != nil {
		t.Fatal(err)
	}
	if strconv.FormatUint(id.Device, 10) != os.Getenv("HEV_LEASE_DEVICE") || strconv.FormatUint(id.Inode, 10) != os.Getenv("HEV_LEASE_INODE") {
		t.Fatal("inherited object differs")
	}
	if _, err = f.Write([]byte("forbidden")); err == nil {
		t.Fatal("read-only lease duplicate granted write")
	}
	root := os.Getenv("HEV_LEASE_CHILD_ROOT")
	if err = os.WriteFile(filepath.Join(root, "child-ready"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if _, err = os.Stat(filepath.Join(root, "child-release")); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("owned child release timed out")
}
func TestWindowsFileLeaseInheritedChildRetainsExclusion(t *testing.T) {
	path := leaseProofFile(t)
	root := filepath.Dir(path)
	first, err := OpenFileLease(path, true)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	id, err := first.Identity()
	if err != nil {
		t.Fatal(err)
	}
	fromPath, err := PrivateIdentity(path, false)
	if err != nil || fromPath != id {
		t.Fatal("checked pathname identity differs", fromPath, id, err)
	}
	dirID, err := PrivateIdentity(root, true)
	if err != nil || dirID.Device != id.Device || dirID.Inode == id.Inode {
		t.Fatal("private directory identity invalid", dirID, id, err)
	}
	if _, err = PrivateIdentity(root, false); err == nil {
		t.Fatal("directory accepted as file identity")
	}

	second, err := OpenFileLease(path, true)
	if err != nil {
		t.Fatal("second shared holder", err)
	}
	id2, err := second.Identity()
	if err != nil || id2 != id {
		t.Fatal("shared inode differs", id2, id, err)
	}
	if exclusive, err := OpenFileLease(path, false); !errors.Is(err, ErrLeaseBusy) {
		if exclusive != nil {
			exclusive.Close()
		}
		t.Fatal("shared holders did not exclude publisher", err)
	}
	if err = second.Close(); err != nil {
		t.Fatal(err)
	}
	duplicate, err := first.DuplicateForChild()
	if err != nil {
		t.Fatal(err)
	}
	defer duplicate.Close()
	h := windows.Handle(duplicate.Fd())
	var flags uint32
	getInfo := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetHandleInformation")
	ok, _, getErr := getInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&flags)))
	if ok == 0 || flags&windows.HANDLE_FLAG_INHERIT != 0 {
		t.Fatal("parent duplicate unexpectedly inheritable", flags, getErr)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-test.run=^TestWindowsFileLeaseChild$")
	cmd.Dir = root
	cmd.Env = []string{"SYSTEMROOT=" + os.Getenv("SYSTEMROOT"), "HEV_LEASE_CHILD_HANDLE=" + strconv.FormatUint(uint64(h), 10), "HEV_LEASE_CHILD_ROOT=" + root, "HEV_LEASE_DEVICE=" + strconv.FormatUint(id.Device, 10), "HEV_LEASE_INODE=" + strconv.FormatUint(id.Inode, 10)}
	cmd.SysProcAttr = &syscall.SysProcAttr{AdditionalInheritedHandles: []syscall.Handle{syscall.Handle(h)}}
	var logs bytes.Buffer
	cmd.Stdout, cmd.Stderr = &logs, &logs
	// This is the only inheritance window, around this fixed owned spawn.
	if err = windows.SetHandleInformation(h, windows.HANDLE_FLAG_INHERIT, windows.HANDLE_FLAG_INHERIT); err != nil {
		t.Fatal(err)
	}
	startErr := cmd.Start()
	clearErr := windows.SetHandleInformation(h, windows.HANDLE_FLAG_INHERIT, 0)
	if startErr != nil {
		t.Fatal(errors.Join(startErr, clearErr))
	}
	waited := false
	defer func() {
		if !waited {
			cancel()
			_ = cmd.Wait()
		}
	}()
	if clearErr != nil {
		t.Fatal(clearErr)
	}
	if err = errors.Join(duplicate.Close(), first.Close()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err = os.Stat(filepath.Join(root, "child-ready")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("inherited child not ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if exclusive, err := OpenFileLease(path, false); !errors.Is(err, ErrLeaseBusy) {
		if exclusive != nil {
			exclusive.Close()
		}
		t.Fatal("parent close lost child's publisher exclusion", err)
	}
	if err = os.Remove(path); err == nil {
		t.Fatal("live inherited lease allowed delete")
	}
	if err = os.Rename(path, path+"-replaced"); err == nil {
		t.Fatal("live inherited lease allowed rename")
	}
	if err = os.WriteFile(filepath.Join(root, "child-release"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	err = cmd.Wait()
	waited = true
	if err != nil {
		t.Fatal("inherited child", err, logs.String())
	}
	exclusive, err := OpenFileLease(path, false)
	if err != nil {
		t.Fatal("last child close did not release exclusion", err)
	}
	defer exclusive.Close()
	current, err := exclusive.Identity()
	if err != nil || current != id {
		t.Fatal("permanent identity changed", current, id, err)
	}
	if err = exclusive.Recheck(); err != nil {
		t.Fatal("exclusive metadata recheck", err)
	}
	if child, err := exclusive.DuplicateForChild(); err == nil {
		child.Close()
		t.Fatal("exclusive publisher lease inherited as consumer")
	}
	if shared, err := OpenFileLease(path, true); !errors.Is(err, ErrLeaseBusy) {
		if shared != nil {
			shared.Close()
		}
		t.Fatal("exclusive publisher did not exclude consumer", err)
	}
}
func TestWindowsFileLeaseMissingAndUnsafeFailUnknown(t *testing.T) {
	path := leaseProofFile(t)
	missing := filepath.Join(filepath.Dir(path), "missing")
	if l, err := OpenFileLease(missing, true); err == nil || errors.Is(err, ErrLeaseBusy) {
		if l != nil {
			l.Close()
		}
		t.Fatal("missing lease not unknown", err)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("missing lease was initialized", err)
	}
	sd, err := PrivateDescriptor()
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err = windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.UNPROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		t.Fatal(err)
	}
	before, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	if l, err := OpenFileLease(path, true); err == nil || errors.Is(err, ErrLeaseBusy) {
		if l != nil {
			l.Close()
		}
		t.Fatal("unsafe DACL accepted or called contention", err)
	}
	after, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil || after.String() != before.String() {
		t.Fatal("unsafe existing lease repaired", err)
	}
}

func TestWindowsFileLeaseRejectsAliasedObjects(t *testing.T) {
	t.Run("hardlink", func(t *testing.T) {
		path := leaseProofFile(t)
		if err := os.Link(path, path+"-alias"); err != nil {
			t.Fatal(err)
		}
		if l, err := OpenFileLease(path, true); err == nil || errors.Is(err, ErrLeaseBusy) {
			if l != nil {
				l.Close()
			}
			t.Fatal("multiply linked lease not unknown", err)
		}
	})
	t.Run("reparse", func(t *testing.T) {
		path := leaseProofFile(t)
		alias := path + "-reparse"
		if err := os.Symlink(path, alias); err != nil {
			t.Skipf("actor cannot create symlink fixture with existing rights: %v", err)
		}
		if l, err := OpenFileLease(alias, true); err == nil || errors.Is(err, ErrLeaseBusy) {
			if l != nil {
				l.Close()
			}
			t.Fatal("reparse lease not unknown", err)
		}
	})
}
