//go:build windows

package windowsfs

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Trusted binaries/PATH entries may be OS-owned and readable by other users.
// Unlike owner-private state, they must have no effective untrusted writer.
// SYSTEM, local Administrators and TrustedInstaller are existing OS authorities,
// not accounts or permissions provisioned by this package.
func trustedSIDs() (map[string]bool, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	allowed := map[string]bool{user.User.Sid.String(): true}
	for _, kind := range []windows.WELL_KNOWN_SID_TYPE{windows.WinLocalSystemSid, windows.WinBuiltinAdministratorsSid} {
		sid, err := windows.CreateWellKnownSid(kind)
		if err != nil {
			return nil, err
		}
		allowed[sid.String()] = true
	}
	if sid, _, _, err := windows.LookupSID("", "NT SERVICE\\TrustedInstaller"); err == nil {
		allowed[sid.String()] = true
	}
	return allowed, nil
}
func verifyTrustedHandle(h windows.Handle, ancestor bool) error {
	allowed, err := trustedSIDs()
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
	if owner == nil || !allowed[owner.String()] {
		return errors.New("untrusted executable/path owner")
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	if acl == nil || acl.AceCount == 0 {
		return errors.New("missing trusted-file DACL")
	}
	// Adding a NEW subdirectory to an ancestor cannot replace the already checked
	// private/trusted child. Final PATH directories themselves must forbid it.
	mutations := windows.ACCESS_MASK(windows.FILE_WRITE_DATA | windows.FILE_APPEND_DATA | windows.FILE_WRITE_EA | windows.FILE_WRITE_ATTRIBUTES | 0x00000040 /* FILE_DELETE_CHILD */ | windows.DELETE | windows.WRITE_DAC | windows.WRITE_OWNER | windows.GENERIC_WRITE | windows.GENERIC_ALL)
	if ancestor {
		mutations &^= windows.FILE_APPEND_DATA
	}
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err = windows.GetAce(acl, i, &ace); err != nil {
			return err
		}
		if ace == nil {
			return errors.New("invalid trusted-file ACE")
		}
		if ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 || ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE {
			continue
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return errors.New("unsupported trusted-file ACE type")
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if ace.Mask&mutations != 0 && !allowed[sid.String()] {
			return errors.New("untrusted executable/path writer")
		}
	}
	runtime.KeepAlive(sd)
	return nil
}

// OpenTrusted returns the SAME no-reparse checked handle to be hashed/read. A
// separate preflight followed by os.Open would discard this ownership proof.
func OpenTrusted(path string, directory bool) (*os.File, error) {
	for parent := filepath.Dir(path); ; parent = filepath.Dir(parent) {
		h, err := checkedHandle(parent, 0, true, false)
		if err != nil {
			return nil, err
		}
		if err = errors.Join(verifyTrustedHandle(h, true), windows.CloseHandle(h)); err != nil {
			return nil, err
		}
		if filepath.Dir(parent) == parent {
			break
		}
	}
	access := uint32(windows.GENERIC_READ)
	if directory {
		access = 0
	}
	h, err := checkedHandle(path, access, directory, false)
	if err != nil {
		return nil, err
	}
	if err = verifyTrustedHandle(h, false); err != nil {
		return nil, errors.Join(err, windows.CloseHandle(h))
	}
	return os.NewFile(uintptr(h), path), nil
}
