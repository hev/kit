//go:build windows

package platformops

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

type DaemonIdentity struct {
	PID     int
	Created uint64
	Event   string
}

func creationTime(h windows.Handle) (uint64, error) {
	var created, exit, kernel, user windows.Filetime
	err := windows.GetProcessTimes(h, &created, &exit, &kernel, &user)
	return uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime), err
}

type DaemonProcess struct {
	process, event windows.Handle
	pid            int
}

func (p *DaemonProcess) Close() { windows.CloseHandle(p.process); windows.CloseHandle(p.event) }
func (p *DaemonProcess) Exited() (bool, error) {
	r, err := windows.WaitForSingleObject(p.process, 0)
	return r == windows.WAIT_OBJECT_0, err
}
func (p *DaemonProcess) Signal(force bool) error {
	if gone, err := p.Exited(); err != nil || gone {
		return err
	}
	if force {
		return windows.TerminateProcess(p.process, 1)
	}
	return windows.SetEvent(p.event)
}

// OpenDaemon binds both control operations to one verified process incarnation.
// The handle remains open through graceful waiting and force escalation.
func OpenDaemon(path string) (*DaemonProcess, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var id DaemonIdentity
	if err = json.Unmarshal(b, &id); err != nil {
		return nil, err
	}
	if id.PID <= 0 || id.Created == 0 || id.Event == "" {
		return nil, fmt.Errorf("invalid daemon identity")
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, uint32(id.PID))
	if err != nil {
		return nil, err
	}
	created, err := creationTime(h)
	if err != nil {
		windows.CloseHandle(h)
		return nil, err
	}
	if created != id.Created {
		windows.CloseHandle(h)
		return nil, fmt.Errorf("stale daemon incarnation: %w", windows.ERROR_INVALID_PARAMETER)
	}
	name, err := windows.UTF16PtrFromString(id.Event)
	if err != nil {
		windows.CloseHandle(h)
		return nil, err
	}
	e, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, name)
	if err != nil {
		windows.CloseHandle(h)
		return nil, err
	}
	return &DaemonProcess{h, e, id.PID}, nil
}
func SignalPID(pid int, force bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	p, err := OpenDaemon(filepath.Join(home, ".hev", "daemon.pid"))
	if err != nil {
		return err
	}
	defer p.Close()
	if p.pid != pid {
		return windows.ERROR_INVALID_PARAMETER
	}
	return p.Signal(force)
}
func ProcessGone(err error) bool { return errors.Is(err, windows.ERROR_INVALID_PARAMETER) }

// Never adopt an event created by another process, even with a matching DACL.
func createDaemonEvent(sa *windows.SecurityAttributes, name *uint16) (windows.Handle, error) {
	h, err := windows.CreateEvent(sa, 1, 0, name)
	if err != nil && h != 0 {
		windows.CloseHandle(h)
		h = 0
	}
	return h, err
}

// DaemonContext exposes a private named event for graceful CLI stops. Cancellation
// is observed by scanLoop after the current indexing cycle has completed.
func DaemonContext(parent context.Context) (context.Context, func(), error) {
	sa, err := privateAttributes()
	if err != nil {
		return nil, nil, err
	}
	var nonce [32]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return nil, nil, err
	}
	name := `Global\hev-hevd-stop-` + hex.EncodeToString(nonce[:])
	event, _ := windows.UTF16PtrFromString(name)
	h, err := createDaemonEvent(sa, event)
	if err != nil {
		if h != 0 {
			windows.CloseHandle(h)
		}
		return nil, nil, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		windows.CloseHandle(h)
		return nil, nil, err
	}
	created, err := creationTime(windows.CurrentProcess())
	if err != nil {
		windows.CloseHandle(h)
		return nil, nil, err
	}
	pidPath := filepath.Join(home, ".hev", "daemon.pid")
	data, err := json.Marshal(DaemonIdentity{os.Getpid(), created, name})
	if err == nil {
		err = WriteFile(pidPath, data, 0600)
	}
	if err != nil {
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
