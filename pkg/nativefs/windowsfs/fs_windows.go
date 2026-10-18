//go:build windows

// Package windowsfs implements the selected-account private Windows filesystem
// contract used by native execution. It grants no admission or execution authority.
package windowsfs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// These primitives never bypass the native journal or host role guards. Never
// turn a failed barrier into success or repair an unsafe existing object.
const windowsFileAllAccess = windows.ACCESS_MASK(0x001f01ff)

func PrivateDescriptor() (*windows.SECURITY_DESCRIPTOR, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	return windows.SecurityDescriptorFromString("O:" + user.User.Sid.String() + "D:P(A;OICI;FA;;;" + user.User.Sid.String() + ")")
}

// LocalPath rejects UNC/device paths, alternate streams and every
// observed reparse component. This is an enrollment-owner filesystem, not a
// sandbox against another process holding the same account's full authority.
func PathSyntax(path string) error {
	volume := filepath.VolumeName(path)
	if !filepath.IsAbs(path) || len(volume) != 2 || volume[1] != ':' || strings.Contains(path[len(volume):], ":") {
		return errors.New("journal requires an absolute local drive path without alternate streams")
	}
	relative := strings.TrimLeft(path[len(volume):], `\/`)
	if relative != "" && !filepath.IsLocal(relative) {
		return errors.New("journal rejects reserved or escaping path")
	}
	for _, part := range strings.FieldsFunc(relative, func(r rune) bool { return r == '\\' || r == '/' }) {
		if strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return errors.New("journal rejects aliased path component")
		}
	}
	return nil
}

func LocalPath(path string) error {
	if err := PathSyntax(path); err != nil {
		return err
	}
	volume := filepath.VolumeName(path)
	drive, err := windows.UTF16PtrFromString(volume + `\`)
	if err != nil {
		return err
	}
	if windows.GetDriveType(drive) != windows.DRIVE_FIXED {
		return errors.New("journal requires a fixed local drive")
	}
	for p := filepath.Clean(path); ; p = filepath.Dir(p) {
		ptr, err := windows.UTF16PtrFromString(p)
		if err != nil {
			return err
		}
		attrs, err := windows.GetFileAttributes(ptr)
		if err != nil {
			return err
		}
		if attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return fmt.Errorf("journal rejects reparse path %q", p)
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	return nil
}

func JournalHandle(path string, access uint32, directory bool) (windows.Handle, error) {
	return checkedHandle(path, access, directory, true)
}

func checkedHandle(path string, access uint32, directory, singleLink bool) (windows.Handle, error) {
	if err := LocalPath(path); err != nil {
		return 0, err
	}
	ptr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	h, err := windows.CreateFile(ptr, access|windows.READ_CONTROL, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return 0, err
	}
	fail := func(err error) (windows.Handle, error) { return 0, errors.Join(err, windows.CloseHandle(h)) }
	var info windows.ByHandleFileInformation
	if err = windows.GetFileInformationByHandle(h, &info); err != nil {
		return fail(err)
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || (info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != directory {
		return fail(errors.New("journal object type/reparse mismatch"))
	}
	if singleLink && !directory && info.NumberOfLinks != 1 {
		return fail(errors.New("journal rejects multiply linked file"))
	}
	var fs [32]uint16
	var flags uint32
	if err = windows.GetVolumeInformationByHandle(h, nil, 0, nil, nil, &flags, &fs[0], uint32(len(fs))); err != nil {
		return fail(err)
	}
	if windows.UTF16ToString(fs[:]) != "NTFS" || flags&windows.FILE_PERSISTENT_ACLS == 0 {
		return fail(errors.New("journal requires NTFS with persistent ACLs"))
	}
	return h, nil
}

func VerifyPrivateHandle(h windows.Handle, protected, directory bool) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	sd, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	if owner == nil || !windows.EqualSid(owner, user.User.Sid) {
		return errors.New("journal owner differs from current token user")
	}
	control, _, err := sd.Control()
	if err != nil {
		return err
	}
	if control&windows.SE_DACL_PRESENT == 0 || (protected && control&windows.SE_DACL_PROTECTED == 0) {
		return errors.New("journal requires a present protected DACL")
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	// Accept a deliberately narrow canonical ACL, rather than attempting a
	// partial AccessCheck emulator that misses groups, callback or object ACEs.
	if acl == nil || acl.AceCount != 1 {
		return errors.New("journal requires exactly one owner-only allow ACE")
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err = windows.GetAce(acl, 0, &ace); err != nil {
		return err
	}
	if ace == nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Mask != windowsFileAllAccess || ace.Header.AceFlags&(windows.INHERIT_ONLY_ACE|windows.NO_PROPAGATE_INHERIT_ACE) != 0 {
		return errors.New("journal requires effective owner full access")
	}
	if directory && ace.Header.AceFlags&(windows.OBJECT_INHERIT_ACE|windows.CONTAINER_INHERIT_ACE) != windows.OBJECT_INHERIT_ACE|windows.CONTAINER_INHERIT_ACE {
		return errors.New("journal directory must inherit privacy to files and subdirectories")
	}
	sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
	equal := windows.EqualSid(sid, user.User.Sid)
	runtime.KeepAlive(sd)
	if !equal {
		return errors.New("journal ACE grants a different principal")
	}
	return nil
}

func VerifyPrivate(path string, protected, directory bool) error {
	h, err := JournalHandle(path, 0, directory)
	if err != nil {
		return err
	}
	return errors.Join(VerifyPrivateHandle(h, protected, directory), windows.CloseHandle(h))
}

func CreatePrivateDir(path string, existing bool) error {
	if err := PathSyntax(path); err != nil {
		return err
	}
	if err := LocalPath(filepath.Dir(path)); err != nil {
		return err
	}
	sd, err := PrivateDescriptor()
	if err != nil {
		return err
	}
	ptr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	err = windows.CreateDirectory(ptr, &sa)
	runtime.KeepAlive(sd)
	if err != nil && !(existing && errors.Is(err, windows.ERROR_ALREADY_EXISTS)) {
		return err
	}
	return VerifyPrivate(path, true, true)
}

func CreatePrivateFile(path string) (*os.File, error) {
	if err := PathSyntax(path); err != nil {
		return nil, err
	}
	if err := VerifyPrivate(filepath.Dir(path), true, true); err != nil {
		return nil, err
	}
	if err := LocalPath(filepath.Dir(path)); err != nil {
		return nil, err
	}
	sd, err := PrivateDescriptor()
	if err != nil {
		return nil, err
	}
	ptr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	if strings.Contains(filepath.Base(path), ":") {
		return nil, errors.New("journal rejects alternate stream")
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	h, err := windows.CreateFile(ptr, windows.GENERIC_READ|windows.GENERIC_WRITE|windows.READ_CONTROL, windows.FILE_SHARE_READ, &sa, windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_WRITE_THROUGH, 0)
	runtime.KeepAlive(sd)
	if err != nil {
		return nil, err
	}
	if err = VerifyPrivateHandle(h, true, false); err != nil {
		return nil, errors.Join(err, windows.CloseHandle(h))
	}
	return os.NewFile(uintptr(h), path), nil
}

// SyncDirectory is a required metadata barrier, not best effort. An
// unsupported or denied directory flush is a readiness failure. Never substitute
// MoveFileEx WRITE_THROUGH (documented copy/delete behavior) for this barrier.
func SyncDirectory(path string) error {
	h, err := JournalHandle(path, windows.GENERIC_READ|windows.GENERIC_WRITE, true)
	if err != nil {
		return fmt.Errorf("open directory for durable flush: %w", err)
	}
	return errors.Join(windows.FlushFileBuffers(h), windows.CloseHandle(h))
}

// ReplacePrivateFile replaces only a same-directory, already flushed
// private temporary file. No cross-volume copy or silent non-durable fallback.
// A failure after rename may have taken effect: callers must reconcile, not retry.
func ReplacePrivateFile(temp, target string) error { return replacePrivateFile(temp, target, nil) }

func replacePrivateFile(temp, target string, finalCheck func() error) error {
	if filepath.Clean(filepath.Dir(temp)) != filepath.Clean(filepath.Dir(target)) || filepath.Clean(temp) == filepath.Clean(target) {
		return errors.New("journal replacement requires distinct same-directory paths")
	}
	if err := VerifyPrivate(temp, true, false); err != nil {
		return err
	}
	if err := VerifyPrivate(filepath.Dir(target), true, true); err != nil {
		return err
	}
	if _, err := os.Lstat(target); err == nil {
		if err = VerifyPrivate(target, true, false); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	f, err := os.OpenFile(temp, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	if err = errors.Join(f.Sync(), f.Close()); err != nil {
		return err
	}
	from, err := windows.UTF16PtrFromString(temp)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	if finalCheck != nil {
		if err = finalCheck(); err != nil {
			return err
		}
	}
	if err = windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH); err != nil {
		return err
	}
	if err = VerifyPrivate(target, true, false); err != nil {
		return err
	}
	f, err = os.OpenFile(target, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	if err = errors.Join(f.Sync(), f.Close()); err != nil {
		return err
	}
	return SyncDirectory(filepath.Dir(target))
}
