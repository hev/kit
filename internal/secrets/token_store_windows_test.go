//go:build windows

package secrets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/platformops"
	"golang.org/x/sys/windows"
)

func testStore(t *testing.T, scope string) ProtectedTokenStore {
	t.Helper()
	return ProtectedTokenStore{Path: filepath.Join(t.TempDir(), "private", "token.dpapi"), Scope: scope, Entropy: []byte("example-fixed-entropy")}
}

func setStoreACL(t *testing.T, path, descriptor string, flags uint32) {
	t.Helper()
	sd, err := windows.SecurityDescriptorFromString(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err = windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.SECURITY_INFORMATION(flags), nil, nil, acl, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = platformops.SecureDir(filepath.Dir(path)); _ = platformops.SecureFile(path) })
}

func TestDPAPIStoreRoundTrip(t *testing.T) {
	for _, scope := range []string{"CurrentUser", "LocalMachine"} {
		t.Run(scope, func(t *testing.T) {
			s := testStore(t, scope)
			if err := s.Write([]byte("fake-service-token\n")); err != nil {
				t.Fatal(err)
			}
			got, err := s.Read()
			if err != nil || got != "fake-service-token" {
				t.Fatalf("roundtrip failed: %v", err)
			}
			readers, err := s.Inspect()
			if err != nil || len(readers) != 3 {
				t.Fatalf("inspect: %v %v", readers, err)
			}
			raw, err := os.ReadFile(s.Path)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), "fake-service-token") {
				t.Fatal("plaintext on disk")
			}
			if err := s.Write([]byte("replacement")); err == nil {
				t.Fatal("overwrote existing store")
			}
			t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "")
			t.Setenv("HEV_OP_DPAPI_FILE", s.Path)
			t.Setenv("HEV_OP_DPAPI_SCOPE", s.Scope)
			t.Setenv("HEV_OP_DPAPI_ENTROPY", string(s.Entropy))
			token, source, err := tokenCredential()
			if err != nil || token != got || source != "DPAPI protected file" {
				t.Fatalf("credential discovery: %s %v", source, err)
			}
			cred, err := (onePassword{}).Credential()
			if err != nil || !strings.Contains(cred, "DPAPI") {
				t.Fatalf("status credential: %s %v", cred, err)
			}
			wrong := s
			wrong.Entropy = []byte("wrong")
			if _, err := wrong.Read(); err == nil {
				t.Fatal("accepted wrong entropy")
			}
			wrong = s
			if scope == "CurrentUser" {
				wrong.Scope = "LocalMachine"
			} else {
				wrong.Scope = "CurrentUser"
			}
			if _, err := wrong.Read(); err == nil {
				t.Fatal("accepted wrong scope")
			}
		})
	}
}

func TestDPAPIStoreRefusesWidenedAndInheritedACL(t *testing.T) {
	for _, object := range []string{"file", "directory"} {
		for _, kind := range []string{"unexpected", "inherited"} {
			t.Run(object+"/"+kind, func(t *testing.T) {
				s := testStore(t, "CurrentUser")
				if err := s.Write([]byte("fake-service-token")); err != nil {
					t.Fatal(err)
				}
				target := s.Path
				if object == "directory" {
					target = filepath.Dir(target)
				}
				u, err := windows.GetCurrentProcessToken().GetTokenUser()
				if err != nil {
					t.Fatal(err)
				}
				acl := "D:P(A;OICI;FA;;;" + u.User.Sid.String() + ")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)"
				flags := uint32(windows.PROTECTED_DACL_SECURITY_INFORMATION)
				if kind == "unexpected" {
					acl += "(A;;FR;;;WD)"
				} else {
					flags = windows.UNPROTECTED_DACL_SECURITY_INFORMATION
				}
				setStoreACL(t, target, acl, flags)
				_, err = s.Read()
				if err == nil {
					t.Fatal("accepted unsafe ACL")
				}
				if kind == "unexpected" && !strings.Contains(err.Error(), "S-1-1-0") {
					t.Fatalf("principal missing: %v", err)
				}
				if kind == "inherited" && !strings.Contains(err.Error(), "inherit") {
					t.Fatalf("inheritance missing: %v", err)
				}
				t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "")
				t.Setenv("HEV_OP_DPAPI_FILE", s.Path)
				t.Setenv("HEV_OP_TOKEN_FILE", filepath.Join(t.TempDir(), "plaintext"))
				if err := os.WriteFile(TokenFile(), []byte("ambient-fallback"), 0600); err != nil {
					t.Fatal(err)
				}
				if serviceToken() != "" {
					t.Fatal("unsafe store fell back to plaintext")
				}
			})
		}
	}
}

func TestDPAPIStoreMissing(t *testing.T) {
	s := testStore(t, "CurrentUser")
	if err := platformops.MkdirAll(filepath.Dir(s.Path), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Read(); err == nil {
		t.Fatal("accepted missing file")
	}
}
