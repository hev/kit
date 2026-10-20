//go:build windows

package nativejob

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"unsafe"

	fs "github.com/hev/kit/pkg/nativefs/windowsfs"
	"golang.org/x/sys/windows"
)

// OwnedHarness is OS lifecycle ownership for an already admitted, pinned model
// turn. It creates no journal authority, admission token, IPC or PID service.
// Wait returns a known exit only after the original owned Job is empty.
type OwnedHarness interface {
	Birth() (Birth, error)
	Wait() (int, error)
	Kill() error
}

// StartOwnedHarness reuses the journal workload's original-handle kernel backend.
// The caller pins Path/Args/Dir/Env and supplies file-based stdio (pipes allowed;
// nil means NUL). It owns pipe copying/draining and closes its child-side ends
// after Start returns. Cmd.Process/ProcessState are not populated or adopted.
//
// ExtraFiles must be read-only FileLease.DuplicateForChild references. They remain
// caller-owned and must not be closed concurrently with Start. Their numeric
// handle values are preserved, inheritable only around CreateProcess and included
// explicitly in its handle allowlist. A caller retains whole-tree/reference
// authority even if a child voluntarily closes a lease early.
//
// Use exec.Command, not CommandContext/SysProcAttr. Cancellation calls the owned
// Kill then the same Wait; a cancelled client does not justify PID termination.
// A nonnil result with an error still owns an uncertain effect and must be waited.
func StartOwnedHarness(cmd *exec.Cmd) (OwnedHarness, error) {
	if cmd == nil || !filepath.IsAbs(cmd.Path) || !filepath.IsAbs(cmd.Dir) {
		return nil, ErrOwnedCommandUnknown
	}
	child, err := startWindowsCommand(cmd, true)
	if child == nil {
		return nil, err
	}
	return child, err
}

func harnessFile(value any, allowNil bool) (*os.File, error) {
	if value == nil && allowNil {
		return nil, nil
	}
	file, ok := value.(*os.File)
	if !ok || file == nil {
		return nil, ErrOwnedCommandUnknown
	}
	return file, nil
}

func readonlyHarnessFiles(files []*os.File) ([]windows.Handle, error) {
	if len(files) > 64 {
		return nil, ErrOwnedCommandUnknown
	}
	handles := make([]windows.Handle, 0, len(files))
	seen := map[windows.Handle]bool{}
	for _, file := range files {
		if file == nil {
			return nil, ErrOwnedCommandUnknown
		}
		h := windows.Handle(file.Fd())
		if seen[h] {
			return nil, ErrOwnedCommandUnknown
		}
		seen[h] = true
		typ, err := windows.GetFileType(h)
		if err != nil || typ != windows.FILE_TYPE_DISK {
			return nil, errors.Join(ErrOwnedCommandUnknown, err)
		}
		var info windows.ByHandleFileInformation
		if err = windows.GetFileInformationByHandle(h, &info); err != nil {
			return nil, errors.Join(ErrOwnedCommandUnknown, err)
		}
		if info.FileAttributes&(windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_REPARSE_POINT) != 0 || info.NumberOfLinks != 1 {
			return nil, ErrOwnedCommandUnknown
		}
		if err = fs.VerifyPrivateHandle(h, true, false); err != nil {
			return nil, errors.Join(ErrOwnedCommandUnknown, err)
		}
		var fsName [32]uint16
		var fsFlags uint32
		if err = windows.GetVolumeInformationByHandle(h, nil, 0, nil, nil, &fsFlags, &fsName[0], uint32(len(fsName))); err != nil || windows.UTF16ToString(fsName[:]) != "NTFS" || fsFlags&windows.FILE_PERSISTENT_ACLS == 0 {
			return nil, errors.Join(ErrOwnedCommandUnknown, err)
		}
		// FileAccessInformation reports rights granted to THIS open file object,
		// not just a readonly attribute or a guessed mode bit.
		var access uint32
		var status windows.IO_STATUS_BLOCK
		if err = windows.NtQueryInformationFile(h, &status, (*byte)(unsafe.Pointer(&access)), 4, 8 /* FileAccessInformation */); err != nil || status.Information != 4 || access&windows.FILE_READ_DATA == 0 || access & ^uint32(windows.FILE_GENERIC_READ) != 0 {
			return nil, errors.Join(ErrOwnedCommandUnknown, err)
		}
		var flags uint32
		ok, _, callErr := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetHandleInformation").Call(uintptr(h), uintptr(unsafe.Pointer(&flags)))
		if ok == 0 || flags&windows.HANDLE_FLAG_INHERIT != 0 {
			return nil, errors.Join(ErrOwnedCommandUnknown, callErr)
		}
		handles = append(handles, h)
	}
	return handles, nil
}
