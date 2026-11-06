//go:build !windows

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

func TestDispatchAmendInputsAndShow(t *testing.T) {
	for _, input := range []string{"file", "stdin", "implicit-stdin"} {
		t.Run(input, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			bin := t.TempDir()
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			if err := os.WriteFile(filepath.Join(bin, "tmux"), []byte("#!/bin/sh\nif [ \"$1\" = list-sessions ]; then echo d-example; fi\nexit 0\n"), 0700); err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(home, "dispatch", "example")
			if err := os.MkdirAll(filepath.Join(dir, "wt"), 0700); err != nil {
				t.Fatal(err)
			}
			original := "---\nname: example\nhost: worker\nrepo: project\n---\n## Acceptance\n- original\n"
			revised := strings.Replace(original, "- original", "- revised", 1)
			order, err := dispatch.Parse(original)
			if err != nil {
				t.Fatal(err)
			}
			if err = dispatch.WriteJSON(filepath.Join(dir, "meta.json"), dispatch.Meta{Order: order, Branch: "d/example"}); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(dir, "order.md"), []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			config := filepath.Join(bin, "config.toml")
			if err = os.WriteFile(config, []byte("[hosts.worker]\nlocal=true\ncap=2\n"), 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{"--config", config, "amend", "example"}
			if input == "file" {
				path := filepath.Join(bin, "revised.md")
				if err = os.WriteFile(path, []byte(revised), 0600); err != nil {
					t.Fatal(err)
				}
				args = append(args, path)
			} else if input == "stdin" {
				args = append(args, "-")
			}
			cmd := newDispatchCommand()
			cmd.SetArgs(args)
			cmd.SetIn(strings.NewReader(revised))
			cmd.SetOut(&bytes.Buffer{})
			if err = cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			b, _ := os.ReadFile(filepath.Join(dir, "order.md"))
			if string(b) != revised {
				t.Fatal(string(b))
			}
			for _, asJSON := range []bool{false, true} {
				cmd = newDispatchCommand()
				args = []string{"--config", config, "show", "example"}
				if asJSON {
					args = append(args, "--json")
				}
				var out bytes.Buffer
				cmd.SetArgs(args)
				cmd.SetOut(&out)
				if err = cmd.Execute(); err != nil {
					t.Fatal(err)
				}
				if asJSON {
					var response dispatch.Response
					if err = json.Unmarshal(out.Bytes(), &response); err != nil {
						t.Fatal(err)
					}
					if response.History == nil || len(response.History.Revisions) != 2 || len(response.History.Attempts) != 1 {
						t.Fatal(out.String())
					}
				} else if !strings.Contains(out.String(), "Revision 2") || !strings.Contains(out.String(), "Attempt 1") {
					t.Fatal(out.String())
				}
			}
		})
	}
}

func TestEffectiveOrderHost(t *testing.T) {
	for _, original := range []string{
		"---\nname: example\nhost: original\nrepo: project\n---\n## Acceptance\n- works\n",
		"---\nname: example\nrepo: project\n---\n## Acceptance\n- works\n",
	} {
		saved := effectiveOrderHost(original, "override")
		parsed, err := dispatch.Parse(saved)
		if err != nil || parsed.Host != "override" {
			t.Fatalf("%s: %v", saved, err)
		}
		revised := strings.Replace(saved, "- works", "- revised", 1)
		parsed, err = dispatch.Parse(revised)
		if err != nil || parsed.Host != "override" {
			t.Fatalf("body edit changed effective host: %s %v", revised, err)
		}
	}
}
