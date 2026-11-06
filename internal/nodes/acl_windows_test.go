package nodes

import (
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsProtectedACL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keys with spaces")
	if e := os.WriteFile(path, []byte("owner key\n"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := secure(path, false); e != nil {
		t.Fatal(e)
	}
	u, e := user.Current()
	if e != nil {
		t.Fatal(e)
	}
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
	script := "$ErrorActionPreference='Stop'; $acl=Get-Acl -LiteralPath " + quote(path) + "; if (!$acl.AreAccessRulesProtected) {throw 'inheritance enabled'}; $ids=@($acl.Access | ForEach-Object {$_.IdentityReference.Translate([System.Security.Principal.SecurityIdentifier]).Value}); if ($ids.Count -ne 2 -or $ids -notcontains 'S-1-5-18' -or $ids -notcontains " + quote(u.Uid) + ") {throw ('unexpected ACL: '+$ids)}"
	if out, e := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script).CombinedOutput(); e != nil {
		t.Fatalf("%s: %v", out, e)
	}
	// A failed ACL operation must leave the previously secured live file alone.
	if e := secure(filepath.Join(path, "missing"), false); e == nil {
		t.Fatal("expected ACL failure")
	}
	if out, e := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script).CombinedOutput(); e != nil {
		t.Fatalf("after failure: %s: %v", out, e)
	}
}
