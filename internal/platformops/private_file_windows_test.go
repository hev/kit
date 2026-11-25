package platformops

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsReadPrivateFileRefusesWidenedACL(t *testing.T) {
	dir := t.TempDir()
	if err := SecureDir(dir); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "credential")
	if err := WriteFileAtomic(path, []byte("fixture-private"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadPrivateFile(path, 64); err != nil || string(got) != "fixture-private" {
		t.Fatal("private file refused")
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	for _, object := range []string{path, dir} {
		if err := windows.SetNamedSecurityInfo(object, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadPrivateFile(path, 64); err == nil {
			t.Fatal("widened ACL accepted")
		}
		if object == path {
			if err := SecureFile(path); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := SecureDir(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}
