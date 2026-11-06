//go:build windows

package intake

import (
	"fmt"
	"golang.org/x/sys/windows"
	"runtime"
	"unsafe"
)

func ownerSID() (string, error) {
	token := windows.GetCurrentProcessToken()
	user, err := token.GetTokenUser()
	if err != nil {
		return "", err
	}
	return user.User.Sid.String(), nil
}
func privateDirectory(path string) error {
	sid, err := ownerSID()
	if err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + sid + ")")
	if err != nil {
		return err
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
}
func privateOrder(path string) error {
	sid, err := ownerSID()
	if err != nil {
		return err
	}
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}

	acl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	if acl == nil || acl.AceCount != 1 {
		return fmt.Errorf("order requires exactly one owner ACE")
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err = windows.GetAce(acl, 0, &ace); err != nil {
		return err
	}
	if ace == nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Mask != 0x1f01ff || ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 {
		return fmt.Errorf("order requires effective owner full access")
	}
	actual := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
	expected, err := windows.StringToSid(sid)
	if err != nil {
		return err
	}
	equal := windows.EqualSid(actual, expected)
	runtime.KeepAlive(sd)
	if !equal {
		return fmt.Errorf("order ACE grants another principal")
	}
	return nil
}
