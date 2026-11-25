package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/dispatch"
)

func TestAdmissionListOwners(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfg := filepath.Join(home, "dispatch.toml")
	t.Setenv("HEV_DISPATCH_CONFIG", cfg)
	const owner = "github.com/hev"
	for _, tc := range []struct {
		name, config string
		owners       int
		admitAll     bool
	}{
		{"configured", `[admission]
owners = ["github.com/hev"]
`, 1, false},
		{"empty", "[admission]\nowners = []\nrepos = []\n", 0, false},
		{"absent", "", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(cfg, []byte(tc.config), 0600); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			cmd := newAdmissionCommand()
			cmd.SetOut(&out)
			cmd.SetArgs([]string{"ls"})
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(out.String(), "owner "+owner+"\n") != (tc.owners == 1) ||
				strings.Contains(out.String(), "every repository") != tc.admitAll {
				t.Fatalf("unexpected human output: %s", &out)
			}

			out.Reset()
			cmd = newAdmissionCommand()
			cmd.SetOut(&out)
			cmd.SetArgs([]string{"ls", "--json"})
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			var policy dispatch.RepoPolicy
			if err := json.Unmarshal(out.Bytes(), &policy); err != nil {
				t.Fatal(err)
			}
			if len(policy.Owners) != tc.owners || (tc.owners == 1 && policy.Owners[0] != owner) || policy.AdmitAll != tc.admitAll {
				t.Fatalf("unexpected JSON policy: %s", &out)
			}
		})
	}
}
