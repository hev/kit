package nodes

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func secure(path string, admin bool) error {
	principal := "BA"
	if !admin {
		user, err := windows.GetCurrentProcessToken().GetTokenUser()
		if err != nil {
			return fmt.Errorf("identify authorized keys owner: %w", err)
		}
		principal = user.User.Sid.String()
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;SY)(A;;FA;;;" + principal + ")")
	if err != nil {
		return fmt.Errorf("build authorized keys permissions: %w", err)
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("read authorized keys permissions: %w", err)
	}
	// Apply the replacement ACL and disable inheritance in the same operation.
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, acl, nil); err != nil {
		return fmt.Errorf("secure authorized keys: %w", err)
	}
	return nil
}
