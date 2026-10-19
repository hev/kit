//go:build windows

package windowsfs

import (
	"errors"
	"golang.org/x/sys/windows"
	"runtime"
	"testing"
	"unsafe"
)

func TestWindowsPrivateJobSecurity(t *testing.T) {
	user, e := windows.GetCurrentProcessToken().GetTokenUser()
	if e != nil {
		t.Fatal(e)
	}
	sid := user.User.Sid.String()
	for _, tc := range []struct {
		name, sddl string
		want       bool
	}{
		{"private", "O:" + sid + "D:P(A;;GA;;;" + sid + ")", true},
		{"unprotected", "O:" + sid + "D:(A;;GA;;;" + sid + ")", false},
		{"foreign-extra", "O:" + sid + "D:P(A;;GA;;;" + sid + ")(A;;GA;;;WD)", false},
		{"insufficient", "O:" + sid + "D:P(A;;GR;;;" + sid + ")", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sd, e := windows.SecurityDescriptorFromString(tc.sddl)
			if e != nil {
				t.Fatal(e)
			}
			sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
			job, e := windows.CreateJobObject(&sa, nil)
			runtime.KeepAlive(sd)
			if e != nil {
				t.Fatal(e)
			}
			e = VerifyPrivateJobHandle(job)
			closeErr := windows.CloseHandle(job)
			if closeErr != nil {
				t.Fatal(closeErr)
			}
			if (e == nil) != tc.want {
				t.Fatal(tc.name, e)
			}
			if errors.Is(e, windows.ERROR_INVALID_HANDLE) {
				t.Fatal("wrong kernel object security API", e)
			}
		})
	}
}
