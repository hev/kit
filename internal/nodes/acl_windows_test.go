package nodes

import (
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestWindowsProtectedACL(t *testing.T) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		admin     bool
		principal string
	}{
		{"user", false, user.User.Sid.String()},
		{"administrators", true, "S-1-5-32-544"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "keys with spaces")
			if err := os.WriteFile(path, []byte("owner key\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := secure(path, tc.admin); err != nil {
				t.Fatal(err)
			}
			assertWindowsACL(t, path, tc.principal)

			if err := secure(filepath.Join(path, "missing"), tc.admin); err == nil {
				t.Fatal("expected ACL failure")
			}
			assertWindowsACL(t, path, tc.principal)
		})
	}
}

func assertWindowsACL(t *testing.T, path, principal string) {
	const fileAllAccess = 0x1f01ff
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	flags, _, err := sd.Control()
	if err != nil || flags&windows.SE_DACL_PROTECTED == 0 {
		t.Fatalf("protected DACL required, flags=%#x, err=%v", flags, err)
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil || acl.AceCount != 2 {
		t.Fatalf("expected exactly two access rules, err=%v", err)
	}
	want := [2]string{"S-1-5-18", principal}
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, i, &ace); err != nil {
			t.Fatal(err)
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Header.AceFlags != 0 ||
			ace.Mask != fileAllAccess || sid.String() != want[i] {
			t.Fatalf("unexpected access rule %d", i)
		}
	}
}
