package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLicenseCommandsMaskConfiguredTokenAndKeepOSS(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HEV_LICENSE", "")
	t.Setenv("HEV_LICENSE_REF", "")
	cfg := filepath.Join(home, "config.toml")
	t.Setenv("HEV_CONFIG", cfg)
	const token = "v4.public.private-value.signature"
	if err := os.WriteFile(cfg, []byte("[license]\nkey = \""+token+"\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	configShowCmd.SetOut(&out)
	licenseCmd.SetOut(&out)
	licenseCmd.SetContext(t.Context())
	t.Cleanup(func() {
		configShowCmd.SetOut(nil)
		licenseCmd.SetOut(nil)
		licenseCmd.SetContext(context.Background())
	})
	for _, tc := range []struct {
		run  func() error
		want string
	}{
		{func() error { return configShowCmd.RunE(configShowCmd, nil) }, `"key": "[REDACTED]"`},
		{func() error { return licenseCmd.RunE(licenseCmd, nil) }, "floor (invalid Kit license token)"},
	} {
		out.Reset()
		if err := tc.run(); err != nil || strings.Contains(out.String(), token) || !strings.Contains(out.String(), tc.want) {
			t.Fatal("command failed or did not safely describe the Kit license")
		}
	}
}
