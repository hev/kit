//go:build windows

package daemon

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hev/kit/internal/platformops"
	"golang.org/x/sys/windows"
)

// Keep this operator repair in sync with Kit Cloud's Windows upgrade guide.
// Run as the daemon account on a dedicated local directory it already owns.
const repairBootstrapACL = `
$ErrorActionPreference = 'Stop'
$sid = [System.Security.Principal.WindowsIdentity]::GetCurrent().User.Value
$sddl = "D:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;FA;;;${sid})"
foreach ($path in ($env:HEV_TEST_REPAIR_PATHS -split '\|')) {
    $item = Get-Item -LiteralPath $path
    if ($item.PSIsContainer) {
        $acl = [System.Security.AccessControl.DirectorySecurity]::new()
    } else {
        $acl = [System.Security.AccessControl.FileSecurity]::new()
    }
    $acl.SetSecurityDescriptorSddlForm($sddl, [System.Security.AccessControl.AccessControlSections]::Access)
    [System.IO.FileSystemAclExtensions]::SetAccessControl($item, $acl)
}
`

func TestHEVDBootstrapOperatorACLRepair(t *testing.T) {
	isolate(t, "")
	const key = "fixture-windows-bootstrap-key"
	var accepted atomic.Int32
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+key {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		accepted.Add(1)
		_, _ = w.Write([]byte("{}"))
	}))
	defer gateway.Close()
	dir := t.TempDir()
	config := filepath.Join(dir, "config.toml")
	keyFile := filepath.Join(dir, "layer.key")
	t.Setenv("HEV_CONFIG", config)

	for path, data := range map[string]string{
		config:  fmt.Sprintf("[layer]\nendpoint = %q\napi_key = %q\n", gateway.URL, key),
		keyFile: key + "\n",
	} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal("could not write synthetic bootstrap fixture")
		}
	}
	if err := platformops.SecureDir(dir); err != nil {
		t.Fatal("could not protect fixture directory")
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal("could not identify synthetic daemon account")
	}
	widen := func(path string) {
		t.Helper()
		sd, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;FA;;;" + user.User.Sid.String() + ")(A;;FR;;;WD)")
		if err != nil {
			t.Fatal("could not build unsafe fixture ACL")
		}
		acl, _, err := sd.DACL()
		if err != nil || windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
			windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil) != nil {
			t.Fatal("could not widen fixture ACL")
		}
	}
	refuses := func(diagnostic string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := Run(ctx)
		if err == nil || !strings.Contains(err.Error(), diagnostic) ||
			strings.Contains(err.Error(), key) || strings.Contains(strings.ToLower(err.Error()), strings.ToLower(dir)) || accepted.Load() != 0 {
			t.Fatal("expected redacted protection refusal before gateway authentication")
		}
		t.Log(diagnostic)
	}
	repair := func(paths ...string) {
		t.Helper()
		t.Setenv("HEV_TEST_REPAIR_PATHS", strings.Join(paths, "|"))
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := exec.CommandContext(ctx, "pwsh.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", repairBootstrapACL).Run(); err != nil {
			t.Fatalf("operator PowerShell ACL repair failed: %T", err)
		}
	}
	widen(config)
	refuses("hevd: protect the Layer configuration file before starting capture")
	repair(dir, config)
	if _, err := loadConfig(true); err != nil {
		t.Fatal("repaired inline configuration was refused")
	}

	data := fmt.Sprintf("[layer]\nendpoint = %q\napi_key_file = 'layer.key'\n", gateway.URL)
	if err := os.WriteFile(config, []byte(data), 0600); err != nil {
		t.Fatal("could not write migrated configuration")
	}
	widen(keyFile)
	refuses("hevd: [layer].api_key_file is unreadable or unprotected")
	repair(dir, keyFile)

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- Run(ctx) }()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-result:
			t.Fatal("daemon stopped before reporting an authenticated scan")
		case <-ctx.Done():
			t.Fatal("daemon did not report an authenticated scan before the deadline")
		case <-ticker.C:
			status, err := ReadStatus()
			if err == nil && !status.LastRun.IsZero() && status.LastError == "" && accepted.Load() >= 2 {
				cancel()
				select {
				case err := <-result:
					if err != nil {
						t.Fatal("daemon returned an error after cancellation")
					}
				case <-time.After(5 * time.Second):
					t.Fatal("daemon did not stop after cancellation")
				}
				t.Log("protected key file accepted; daemon authenticated and completed a scan")
				return
			}
		}
	}
}
