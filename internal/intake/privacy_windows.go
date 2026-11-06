//go:build windows

package intake

import (
	"fmt"
	"golang.org/x/sys/windows"
	"strings"
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
	text := sd.String()
	if strings.Count(text, "(A;") != 1 || !strings.Contains(text, sid) {
		return fmt.Errorf("order lacks owner-only ACL")
	}
	return nil
}
