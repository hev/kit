package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsProjectGlobs(t *testing.T) {
	for _, tc := range []struct {
		pattern, project string
		want             bool
	}{
		{`C:\Users\hev\workspace\**`, `c:\USERS\HEV\workspace\kit`, true},
		{`c:/users/hev/workspace/**`, `C:\Users\hev\workspace`, true},
		{`C:\Users\hev\workspace\*`, `c:/users/hev/workspace/a/b`, false},
		{`C:\Users\hev\workspace\**\secret*`, `c:/users/hev/workspace/a/Secrets`, true},
		{`D:\workspace\**`, `c:/workspace/a`, false},
		{`\\server\share\**`, `\\SERVER\SHARE\a`, true},
		{`C:/repo/**/**/x`, `c:/repo/a/b/x`, true},
	} {
		got, err := matchProjectGlob(tc.pattern, tc.project)
		if err != nil || got != tc.want {
			t.Errorf("%q against %q: %v %v", tc.pattern, tc.project, got, err)
		}
	}
	if _, err := matchProjectGlob(`C:\repo\[`, `C:\repo\x`); err == nil {
		t.Fatal("invalid glob accepted")
	}
}
func TestWindowsHomeExpansion(t *testing.T) {
	home := `C:\Users\hev`
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for _, pattern := range []string{`~/workspace/**`, `~\workspace\**`} {
		got, err := matchProjectGlob(pattern, `c:\users\HEV\workspace\kit`)
		if err != nil || !got {
			t.Fatalf("home expansion %q: %v %v", pattern, got, err)
		}
	}
}

func TestNativeHomeDefaults(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HEV_CONFIG", filepath.Join(home, "absent.toml"))
	t.Setenv("HEVD_GH_USER", "unit-test")
	t.Setenv("HEVD_HOST", "")
	t.Setenv("HEVD_SCAN_ROOTS", "")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.ScanRoots) != 1 || cfg.ScanRoots[0] != filepath.Join(home, "workspace") {
		t.Fatalf("scan roots: %v", cfg.ScanRoots)
	}
	hostname, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host != hostname {
		t.Fatalf("host %q, want %q", cfg.Host, hostname)
	}
}
