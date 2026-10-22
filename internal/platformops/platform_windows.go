//go:build windows

package platformops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func RequireTraceWrites() error { return nil }
func LockFile(f *os.File, nonblocking bool) error {
	flags := uint32(windows.LOCKFILE_EXCLUSIVE_LOCK)
	if nonblocking {
		flags |= windows.LOCKFILE_FAIL_IMMEDIATELY
	}
	return windows.LockFileEx(windows.Handle(f.Fd()), flags, 0, 1, 0, &windows.Overlapped{})
}
func UnlockFile(f *os.File) {
	_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &windows.Overlapped{})
}
func LockPath(path string, nonblocking bool) (func(), error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	sa, err := privateAttributes()
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE|windows.WRITE_DAC, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, sa, windows.OPEN_ALWAYS, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(h), path)
	var info windows.ByHandleFileInformation
	if err = windows.GetFileInformationByHandle(h, &info); err == nil && info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		err = fmt.Errorf("lock path is a reparse point: %s", path)
	}
	if err == nil {
		err = secureHandle(h)
	}
	if err == nil {
		err = LockFile(f, nonblocking)
	}
	if err != nil {
		f.Close()
		return nil, err
	}
	return func() { UnlockFile(f); f.Close() }, nil
}
func IsLockBusy(err error) bool {
	return errors.Is(err, windows.ERROR_LOCK_VIOLATION) || errors.Is(err, windows.ERROR_IO_PENDING)
}
func eventName(pid int) *uint16 {
	p, _ := windows.UTF16PtrFromString(fmt.Sprintf(`Global\hev-hevd-stop-%d`, pid))
	return p
}
func SignalPID(pid int, force bool) error {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return err
	}
	windows.CloseHandle(h)
	if !force {
		e, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, eventName(pid))
		if err != nil {
			return err
		}
		defer windows.CloseHandle(e)
		return windows.SetEvent(e)
	}
	h, err = windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	return windows.TerminateProcess(h, 1)
}
func ProcessGone(err error) bool { return errors.Is(err, windows.ERROR_INVALID_PARAMETER) }

// DaemonContext exposes a private named event for graceful CLI stops. Cancellation
// is observed by scanLoop after the current indexing cycle has completed.
func DaemonContext(parent context.Context) (context.Context, func(), error) {
	sa, err := privateAttributes()
	if err != nil {
		return nil, nil, err
	}
	h, err := windows.CreateEvent(sa, 1, 0, eventName(os.Getpid()))
	if err != nil {
		return nil, nil, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		windows.CloseHandle(h)
		return nil, nil, err
	}
	pidPath := filepath.Join(home, ".hev", "daemon.pid")
	if err = WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
		windows.CloseHandle(h)
		return nil, nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for ctx.Err() == nil {
			result, err := windows.WaitForSingleObject(h, 200)
			if err != nil || result == windows.WAIT_OBJECT_0 {
				cancel()
				return
			}
		}
	}()
	return ctx, func() { cancel(); <-done; windows.CloseHandle(h); os.Remove(pidPath) }, nil
}

func privateDescriptor() (*windows.SECURITY_DESCRIPTOR, error) {
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return nil, err
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return nil, err
	}
	// Protected DACL: no inherited ACEs, full control only to the user, SYSTEM,
	// and built-in administrators. Child objects inherit these restrictions.
	return windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + user.User.Sid.String() + ")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)")
}
func privateAttributes() (*windows.SecurityAttributes, error) {
	sd, err := privateDescriptor()
	if err != nil {
		return nil, err
	}
	return &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}, nil
}
func secureHandle(h windows.Handle) error {
	sd, err := privateDescriptor()
	if err != nil {
		return err
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
}
func securePath(path string, mode os.FileMode) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	h, err := windows.CreateFile(p, windows.READ_CONTROL|windows.WRITE_DAC, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	var info windows.ByHandleFileInformation
	if err = windows.GetFileInformationByHandle(h, &info); err != nil {
		return err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fmt.Errorf("refuse to secure reparse point: %s", path)
	}
	return secureHandle(h)
}

// Windows has no supported directory fsync through os.File. File contents are
// flushed before replacement; MoveFileEx provides replace-existing semantics.
func SyncDir(*os.File) error { return nil }
func Rename(old, new string) error {
	// os.Rename uses MoveFileEx(REPLACE_EXISTING). A short-lived reader lacking
	// FILE_SHARE_DELETE can delay replacement. Retry, never delete the old file.
	var err error
	for i := 0; i < 20; i++ {
		err = os.Rename(old, new)
		if !errors.Is(err, windows.ERROR_SHARING_VIOLATION) && !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			return err
		}
		time.Sleep(25 * time.Millisecond)
	}
	return err
}
