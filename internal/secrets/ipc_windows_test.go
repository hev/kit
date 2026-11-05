package secrets

import (
	"golang.org/x/sys/windows"
	"testing"
)

func TestWindowsPipeUserACL(t *testing.T) {
	s, e := currentSID()
	if e != nil {
		t.Fatal(e)
	}
	sd, e := windows.SecurityDescriptorFromString(pipeSDDL(s))
	if e != nil {
		t.Fatal(e)
	}
	dacl, _, e := sd.DACL()
	if e != nil || dacl.AceCount != 1 {
		t.Fatal("pipe ACL must grant only one user")
	}
	if endpoint() != `\\.\pipe\hev-secrets-`+s {
		t.Fatal("pipe not scoped to user")
	}
}
