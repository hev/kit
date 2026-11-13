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

func TestWindowsPipeEndpointValidation(t *testing.T) {
	for p, ok := range map[string]bool{
		`\\.\pipe\hev-secrets-test`: true,
		`\\.\PIPE\x`:                true,
		`\\.\pipe\`:                 false,
		`\\.\pipe\a\b`:              false,
		`\\server\pipe\x`:           false,
		`C:\hev\agent.sock`:         false,
	} {
		if (validEndpoint(p) == nil) != ok {
			t.Errorf("validEndpoint(%q) = %v, want ok=%v", p, validEndpoint(p), ok)
		}
	}
}
