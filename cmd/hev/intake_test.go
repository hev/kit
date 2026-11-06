package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestWatchAndIntakeCLICompatibility(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "intake.toml")
	template := filepath.Join(dir, "order.md")
	if err := os.WriteFile(template, []byte("---\nname: {{name}}\nrepo: example\n---\nReview {{identifier}}\n## Acceptance\n- Verify behavior.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	text := fmt.Sprintf("state_dir = %q\n[linear]\napi_key_env = \"TEST_KEY\"\nteam = \"TEST\"\nassignee = \"bot\"\nallowed_actor_team = \"TEST\"\n[[status]]\nname = \"Ready\"\naction = \"fix\"\nconcurrency = \"code\"\ntemplate = %q\nhost = \"worker\"\n", filepath.ToSlash(dir), filepath.ToSlash(template))
	if err := os.WriteFile(config, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	defer func() { rootCmd.SetArgs(nil); rootCmd.SetOut(nil); rootCmd.SetErr(nil) }()
	for _, action := range []string{"check", "render"} {
		var expected string
		for _, entry := range []string{"watch", "intake"} {
			for _, prefix := range [][]string{nil, {"linear"}} {
				args := append([]string{entry}, prefix...)
				args = append(args, action, "--config", config)
				if action == "render" {
					args = append(args, "--status", "Ready", "--identifier", "TEST-1", "--url", "https://example.invalid/TEST-1", "--title", "Example change")
				}
				var out bytes.Buffer
				rootCmd.SetOut(&out)
				rootCmd.SetErr(&out)
				rootCmd.SetArgs(args)
				if err := rootCmd.Execute(); err != nil {
					t.Fatalf("%v: %v %s", args, err, out.String())
				}
				if expected == "" {
					expected = out.String()
				}
				if out.String() != expected {
					t.Fatalf("%v: %q != %q", args, out.String(), expected)
				}
			}
		}
	}
	for _, entry := range []string{"watch", "intake"} {
		var out bytes.Buffer
		rootCmd.SetOut(&out)
		rootCmd.SetErr(&out)
		rootCmd.SetArgs([]string{entry, "watch", "linear", "check", "--config", config})
		if err := rootCmd.Execute(); err == nil {
			t.Fatalf("%s accepted doubled command", entry)
		}
	}
}
