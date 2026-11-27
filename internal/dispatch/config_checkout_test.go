package dispatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckoutConfigAliases(t *testing.T) {
	const fields = "mode = 'pro'\nroot = '~/repos'\ndriver = 'cow'\nauto_compact_done = true\n"
	policy := WorkspaceConfig{Mode: "pro", Root: "~/repos", Driver: "cow", AutoCompactDone: true}
	for _, tc := range []struct {
		name, config, root, refusal string
		policy                      WorkspaceConfig
	}{
		{name: "checkout table", config: "[checkout]\n" + fields, root: "~/repos", policy: policy},
		{name: "legacy table", config: "[workspace]\n" + fields, root: "~/repos", policy: policy},
		{name: "legacy root", config: "workspace = '~/repos'\n", root: "~/repos"},
		{name: "empty legacy root", config: "workspace = ''\n", root: "~/workspace"},
		{name: "checkout root overrides legacy root", config: "workspace = '~/legacy'\n[checkout]\n" + fields, root: "~/repos", policy: policy},
		{name: "legacy root fills omitted checkout root", config: "workspace = '~/legacy'\n[checkout]\nmode = 'oss'\n", root: "~/legacy", policy: WorkspaceConfig{Mode: "oss"}},
		{name: "both tables", config: "[checkout]\n[workspace]\n", refusal: "cannot specify both [checkout] and [workspace]"},
		{name: "both tables reversed", config: "[workspace]\n[checkout]\n", refusal: "cannot specify both [checkout] and [workspace]"},
		{name: "invalid mode", config: "[checkout]\nmode = 'invalid'\n", refusal: "checkout mode must be auto, oss or pro"},
		{name: "invalid driver", config: "[workspace]\ndriver = 'invalid'\n", refusal: "checkout driver must be cow or git"},
		{name: "unknown key", config: "[checkout]\nunknown = true\n", refusal: "unknown config key: checkout.unknown"},
		{name: "invalid type", config: "checkout = 'private-value'\n", refusal: "invalid checkout configuration"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "dispatch.toml")
			if err := os.WriteFile(path, []byte("cap = 2\n"+tc.config), 0600); err != nil {
				t.Fatal(err)
			}
			for _, target := range []bool{false, true} {
				cfg, err := loadConfig(path, "local", target)
				if tc.refusal != "" {
					if err == nil || !strings.Contains(err.Error(), tc.refusal) || strings.Contains(err.Error(), "private-value") || strings.Contains(err.Error(), path) {
						t.Fatalf("target=%t: expected %q without config values or path, got %v", target, tc.refusal, err)
					}
				} else if err != nil || cfg.Workspace != tc.root || cfg.WorkspacePolicy != tc.policy {
					t.Fatalf("target=%t: root %q, policy %+v, error %v", target, cfg.Workspace, cfg.WorkspacePolicy, err)
				}
			}
		})
	}
}
