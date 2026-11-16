//go:build windows

package secrets

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"unsafe"

	"github.com/hev/kit/internal/platformops"
	"golang.org/x/sys/windows"
)

func (s ProtectedTokenStore) flags() (uint32, error) {
	switch s.Scope {
	case "CurrentUser":
		return windows.CRYPTPROTECT_UI_FORBIDDEN, nil
	case "LocalMachine":
		return windows.CRYPTPROTECT_UI_FORBIDDEN | windows.CRYPTPROTECT_LOCAL_MACHINE, nil
	default:
		return 0, fmt.Errorf("DPAPI scope must be CurrentUser or LocalMachine")
	}
}

func blob(b []byte) *windows.DataBlob {
	if len(b) == 0 {
		return nil
	}
	return &windows.DataBlob{Size: uint32(len(b)), Data: &b[0]}
}

// Check both objects at every use. Refuse links, inherited ACLs, NULL ACLs,
// unknown ACE types, and any granting principal outside the allowlist.
func checkTokenACL(path string) ([]string, error) {
	st, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("DPAPI store %s: %w", path, err)
	}
	if st.Mode()&os.ModeSymlink != 0 || (!st.IsDir() && !st.Mode().IsRegular()) {
		return nil, fmt.Errorf("DPAPI store %s: requires an ordinary file or directory", path)
	}
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return nil, fmt.Errorf("DPAPI store %s: cannot inspect ACL: %w", path, err)
	}
	control, _, err := sd.Control()
	if err != nil {
		return nil, err
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		return nil, fmt.Errorf("DPAPI store %s: ACL inherits permissions", path)
	}
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	allowed := map[string]bool{u.User.Sid.String(): true, "S-1-5-18": true, "S-1-5-32-544": true}
	owner, _, err := sd.Owner()
	if err != nil {
		return nil, err
	}
	if owner == nil || !allowed[owner.String()] {
		return nil, fmt.Errorf("DPAPI store %s: unexpected owner principal %v", path, owner)
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil || acl.AceCount == 0 {
		return nil, fmt.Errorf("DPAPI store %s: requires a nonempty private DACL", path)
	}
	readers := []string{}
	for i := uint16(0); i < acl.AceCount; i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, uint32(i), &ace); err != nil {
			return nil, err
		}
		if ace.Header.AceFlags&windows.INHERITED_ACE != 0 {
			return nil, fmt.Errorf("DPAPI store %s: inherited ACE refused", path)
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return nil, fmt.Errorf("DPAPI store %s: unsupported ACL entry type %d", path, ace.Header.AceType)
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart)).String()
		if !allowed[sid] {
			return nil, fmt.Errorf("DPAPI store %s: unexpected principal %s", path, sid)
		}
		readers = append(readers, sid)
	}
	runtime.KeepAlive(sd)
	return readers, nil
}

// Inspect reports only the location's ACL principals, never decrypting a token.
func (s ProtectedTokenStore) Inspect() ([]string, error) {
	if s.Path == "" {
		return nil, fmt.Errorf("set HEV_OP_DPAPI_FILE")
	}
	if _, err := s.flags(); err != nil {
		return nil, err
	}
	if _, err := checkTokenACL(filepath.Dir(s.Path)); err != nil {
		return nil, err
	}
	st, err := os.Lstat(s.Path)
	if err != nil {
		return nil, fmt.Errorf("DPAPI store: %w", err)
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("DPAPI store requires a regular file")
	}
	return checkTokenACL(s.Path)
}

func (s ProtectedTokenStore) Read() (string, error) {
	if _, err := s.Inspect(); err != nil {
		return "", err
	}
	data, err := os.ReadFile(s.Path)
	if err != nil {
		return "", fmt.Errorf("DPAPI store: %w", err)
	}
	flags, _ := s.flags()
	// DPAPI's serialized BLOB header records the protection scope at byte 40.
	// CryptUnprotectData itself ignores the LOCAL_MACHINE flag, so verify it
	// explicitly. Keep the store as a raw .NET ProtectedData-compatible blob.
	if len(data) < 44 || binary.LittleEndian.Uint32(data[:4]) != 1 {
		return "", fmt.Errorf("DPAPI store: invalid protected blob")
	}
	machine := binary.LittleEndian.Uint32(data[40:44]) & windows.CRYPTPROTECT_LOCAL_MACHINE
	if machine != flags&windows.CRYPTPROTECT_LOCAL_MACHINE {
		return "", fmt.Errorf("DPAPI store: protection scope does not match %s", s.Scope)
	}
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(blob(data), nil, blob(s.Entropy), 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return "", fmt.Errorf("DPAPI store: decryption failed (check user, scope and entropy)")
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	plain := unsafe.Slice(out.Data, int(out.Size))
	defer clear(plain)
	return validToken(plain)
}

func (s ProtectedTokenStore) Write(data []byte) error {
	flags, err := s.flags()
	if err != nil {
		return err
	}
	if s.Path == "" {
		return fmt.Errorf("set HEV_OP_DPAPI_FILE")
	}
	t, err := validToken(data)
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.Path)
	// Never silently tighten an existing directory or adopt an existing file.
	// Provision a new dedicated directory, or verify the operator's existing ACL.
	if _, err = os.Stat(dir); os.IsNotExist(err) {
		err = platformops.MkdirAll(dir, 0700)
	}
	if err != nil {
		return err
	}
	if _, err = checkTokenACL(dir); err != nil {
		return err
	}
	if _, err = os.Lstat(s.Path); err == nil {
		return fmt.Errorf("DPAPI store already exists; refusing to overwrite")
	} else if !os.IsNotExist(err) {
		return err
	}
	var out windows.DataBlob
	if err = windows.CryptProtectData(blob([]byte(t)), nil, blob(s.Entropy), 0, nil, flags, &out); err != nil {
		return fmt.Errorf("DPAPI token protection failed")
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	f, err := platformops.OpenFile(s.Path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(unsafe.Slice(out.Data, int(out.Size)))
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		os.Remove(s.Path)
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	_, err = s.Inspect()
	return err
}
